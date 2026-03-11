package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/kirniy/aiway/router/manager/webui"
)

type App struct {
	store  *Store
	runner SSHRunner
	mu     sync.Mutex
	web    http.Handler
}

func New(configDir string) (*App, error) {
	store, err := NewStore(configDir)
	if err != nil {
		return nil, err
	}

	distFS, err := fs.Sub(webui.Files, "dist")
	if err != nil {
		return nil, err
	}

	return &App{
		store:  store,
		runner: SSHRunner{},
		web:    http.FileServer(http.FS(distFS)),
	}, nil
}

func (a *App) Start(ctx context.Context) {
	go a.monitorLoop(ctx)
}

func (a *App) monitorLoop(ctx context.Context) {
	a.runActiveCheck("initial")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			config := a.store.Config()
			interval := time.Duration(config.Safety.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 90 * time.Second
			}
			ticker.Reset(interval)
			a.runActiveCheck("scheduled")
		}
	}
}

func (a *App) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, a.routes())
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/api/overview", a.handleOverview)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/logs", a.handleLogs)
	mux.HandleFunc("/api/actions/check-now", a.handleCheckNow)
	mux.HandleFunc("/api/actions/toggle-dns", a.handleToggleDNS)
	mux.HandleFunc("/api/actions/domain/add", a.handleAddDomain)
	mux.HandleFunc("/api/actions/domain/remove", a.handleRemoveDomain)
	mux.HandleFunc("/api/actions/profile/install", a.handleProfileInstall)
	mux.HandleFunc("/api/actions/profile/uninstall", a.handleProfileUninstall)
	mux.HandleFunc("/api/actions/profile/reset", a.handleProfileReset)
	mux.HandleFunc("/api/actions/profile/sync", a.handleProfileSync)
	mux.HandleFunc("/", a.handleSPA)
	return loggingMiddleware(mux)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": nowRFC3339()})
}

func (a *App) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, a.store.Snapshot())
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.store.Config())
	case http.MethodPut:
		var next Config
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.store.UpdateConfig(next); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		_ = a.store.AppendLog("info", "Конфигурация обновлена через веб-интерфейс")
		writeJSON(w, http.StatusOK, next)
	default:
		methodNotAllowed(w)
	}
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"logs": a.store.Snapshot().Logs})
	case http.MethodDelete:
		if err := a.store.ClearLogs(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
	default:
		methodNotAllowed(w)
	}
}

func (a *App) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	status, err := a.runActiveCheck("manual")
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleToggleDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload DNSActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.store.SetDNSDesired(payload.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_ = a.store.AppendLog("info", fmt.Sprintf("Режим aiway DNS переключен: %v", payload.Enabled))
	writeJSON(w, http.StatusOK, map[string]any{"desiredDnsOn": payload.Enabled})
}

func (a *App) handleAddDomain(w http.ResponseWriter, r *http.Request) {
	a.handleDomainAction(w, r, true)
}

func (a *App) handleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	a.handleDomainAction(w, r, false)
}

func (a *App) handleDomainAction(w http.ResponseWriter, r *http.Request, add bool) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload DomainActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload.Domain = strings.TrimSpace(payload.Domain)
	if payload.Domain == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("domain is required"))
		return
	}
	active, ok := a.store.ActiveProfile()
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("active profile is not configured"))
		return
	}

	var output string
	var err error
	if add {
		if err = a.store.AddDomain(payload.Domain); err == nil {
			output, err = a.runner.RemoteAddDomain(active, payload.Domain)
		}
	} else {
		if err = a.store.RemoveDomain(payload.Domain); err == nil {
			output, err = a.runner.RemoteRemoveDomain(active, payload.Domain)
		}
	}
	if err != nil {
		_ = a.store.AppendLog("error", err.Error())
		writeError(w, http.StatusBadGateway, err)
		return
	}
	_ = a.store.AppendLog("info", output)
	status, _ := a.runActiveCheck("domain-update")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output, "status": status})
}

func (a *App) handleProfileInstall(w http.ResponseWriter, r *http.Request) {
	a.handleProfileAction(w, r, "install")
}

func (a *App) handleProfileUninstall(w http.ResponseWriter, r *http.Request) {
	a.handleProfileAction(w, r, "uninstall")
}

func (a *App) handleProfileReset(w http.ResponseWriter, r *http.Request) {
	a.handleProfileAction(w, r, "reset")
}

func (a *App) handleProfileSync(w http.ResponseWriter, r *http.Request) {
	a.handleProfileAction(w, r, "sync")
}

func (a *App) handleProfileAction(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload ProfileActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile, ok := a.store.FindProfile(payload.ProfileID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("profile not found"))
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	var (
		output string
		err    error
	)
	switch action {
	case "install":
		output, err = a.runner.Install(profile)
	case "uninstall":
		output, err = a.runner.RemoteUninstall(profile)
	case "reset":
		output, err = a.runner.RemoteReset(profile)
	case "sync":
		output, err = a.runner.RemoteReapply(profile)
	}
	if err != nil {
		_ = a.store.AppendLog("error", fmt.Sprintf("%s: %v", action, err))
		writeError(w, http.StatusBadGateway, err)
		return
	}
	_ = a.store.AppendLog("info", output)
	status, _ := a.checkProfile(profile, action)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output, "status": status})
}

func (a *App) runActiveCheck(reason string) (*ProfileStatus, error) {
	profile, ok := a.store.ActiveProfile()
	if !ok {
		return nil, fmt.Errorf("active profile is not configured")
	}
	return a.checkProfile(profile, reason)
}

func (a *App) checkProfile(profile Profile, reason string) (*ProfileStatus, error) {
	status := a.store.ProfileStatus(profile.ID)
	if status == nil {
		status = &ProfileStatus{ProfileID: profile.ID, InstallState: "unknown"}
	}
	status.DesiredDNSOn = a.store.Config().Routing.DesiredDNSOn
	status.EffectiveDNSOn = status.DesiredDNSOn && !a.store.Config().Routing.FailsafeActive
	status.LastCheckAt = nowRFC3339()
	status.CustomDomains = append([]string(nil), a.store.Config().Routing.CustomDomains...)
	status.ServiceCount = len(a.store.Config().Routing.Services) + len(status.CustomDomains)

	if profile.Host == "" {
		status.Reachable = false
		status.LastError = "У активного профиля не заполнен адрес VPS"
		status.ConsecutiveFailures++
		_ = a.store.UpdateProfileStatus(*status)
		return status, fmt.Errorf(status.LastError)
	}

	if err := quickTCPCheck(profile.Host, profile.Port); err != nil {
		status.Reachable = false
		status.LastError = err.Error()
		status.ConsecutiveFailures++
		status.InstallState = "offline"
		_ = a.store.AppendLog("warn", fmt.Sprintf("Проверка %s: %s", reason, err.Error()))
		_ = a.store.UpdateProfileStatus(*status)
		a.applyFailsafe(status)
		return status, err
	}

	remoteStatus, err := a.runner.RemoteStatus(profile)
	if err != nil {
		status.Reachable = true
		status.LastError = err.Error()
		status.ConsecutiveFailures++
		status.InstallState = "not-installed"
		_ = a.store.AppendLog("warn", fmt.Sprintf("Проверка %s: %s", reason, err.Error()))
		_ = a.store.UpdateProfileStatus(*status)
		a.applyFailsafe(status)
		return status, err
	}

	remoteStatus.DesiredDNSOn = status.DesiredDNSOn
	remoteStatus.EffectiveDNSOn = remoteStatus.DesiredDNSOn && !a.store.Config().Routing.FailsafeActive
	remoteStatus.LastCheckAt = nowRFC3339()
	remoteStatus.LastSuccessAt = nowRFC3339()
	remoteStatus.ConsecutiveFailures = 0
	remoteStatus.CustomDomains = append([]string(nil), a.store.Config().Routing.CustomDomains...)
	remoteStatus.ServiceCount = len(a.store.Config().Routing.Services) + len(remoteStatus.CustomDomains)

	if a.store.Config().Routing.FailsafeActive && a.store.Config().Safety.AutoRecover {
		_ = a.store.SetFailsafe(false)
		remoteStatus.EffectiveDNSOn = remoteStatus.DesiredDNSOn
		_ = a.store.AppendLog("info", "Фейлсейф автоматически снят после успешной проверки")
	}

	_ = a.store.UpdateProfileStatus(remoteStatus)
	return &remoteStatus, nil
}

func (a *App) applyFailsafe(status *ProfileStatus) {
	config := a.store.Config()
	if !config.Safety.Enabled || !config.Safety.DisableDNSOnFailure {
		return
	}
	if status.ConsecutiveFailures < config.Safety.FailThreshold {
		return
	}
	if !config.Routing.FailsafeActive {
		_ = a.store.SetFailsafe(true)
		_ = a.store.AppendLog("warn", "Фейлсейф активирован: aiway DNS временно отключен на роутере")
	}
	status.EffectiveDNSOn = false
	_ = a.store.UpdateProfileStatus(*status)
}

func (a *App) handleSPA(w http.ResponseWriter, r *http.Request) {
	clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if clean == "." || clean == "" {
		a.serveIndex(w, r)
		return
	}
	if strings.HasPrefix(clean, "api/") {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(clean, ".") {
		if file, err := webui.Files.Open("dist/" + clean); err == nil {
			_ = file.Close()
			a.web.ServeHTTP(w, r)
			return
		}
	}
	a.serveIndex(w, r)
}

func (a *App) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(webui.Files, "dist/index.html")
	if err != nil {
		http.Error(w, "frontend build is missing", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}
