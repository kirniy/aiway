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
	configDir string
	store     *Store
	runner    SSHRunner
	router    RouterController
	updater   Updater
	mu        sync.Mutex
	web       http.Handler
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
		configDir: configDir,
		store:     store,
		runner:    SSHRunner{},
		router:    NewRouterController(configDir),
		updater:   NewUpdater(),
		web:       http.FileServer(http.FS(distFS)),
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
	mux.HandleFunc("/api/actions/update/check", a.handleUpdateCheck)
	mux.HandleFunc("/api/actions/update/apply", a.handleUpdateApply)
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
	snapshot := a.store.Snapshot()
	if address, sni, err := a.router.CurrentTLSState(); err == nil {
		snapshot.RouterDNS = RouterDNSState{Active: address != "" || sni != "", Address: address, SNI: sni}
	}
	writeJSON(w, http.StatusOK, snapshot)
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
	active, ok := a.store.ActiveProfile()
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("active profile is not configured"))
		return
	}
	var payload DNSActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	config := a.store.Config()
	address, sni := dnsTarget(config, active)
	message, err := a.router.EnsureDNSState(address, sni, payload.Enabled)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := a.store.SetDNSDesired(payload.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if payload.Enabled {
		_ = a.store.SetFailsafe(false)
	}
	if message != "" {
		_ = a.store.AppendLog("info", message)
	}
	_ = a.store.AppendLog("info", fmt.Sprintf("Режим aiway DNS переключен: %v", payload.Enabled))
	status, _ := a.runActiveCheck("toggle-dns")
	writeJSON(w, http.StatusOK, map[string]any{"desiredDnsOn": payload.Enabled, "message": message, "status": status})
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
	if !profileCanManage(active) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("кастомные домены доступны только для SSH-управляемого VPS профиля"))
		return
	}

	var output string
	var err error
	if add {
		output, err = a.runner.RemoteAddDomain(active, payload.Domain)
		if err == nil {
			err = a.store.AddDomain(payload.Domain)
		}
	} else {
		output, err = a.runner.RemoteRemoveDomain(active, payload.Domain)
		if err == nil {
			err = a.store.RemoveDomain(payload.Domain)
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
	if !profileCanManage(profile) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("этот профиль работает в DNS-only режиме; операции install/sync/reset/uninstall доступны только для SSH-управляемого VPS"))
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

func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	info, err := a.updater.Check()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *App) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	info, err := a.updater.Apply()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	_ = a.store.AppendLog("info", fmt.Sprintf("Запущено обновление панели до %s", info.Latest))
	writeJSON(w, http.StatusOK, info)
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
	status.CustomDomains = cloneStrings(a.store.Config().Routing.CustomDomains)
	status.ServiceCount = enabledDomainCount(a.store.Config())
	config := a.store.Config()
	address, sni := dnsTarget(config, profile)

	if profile.Host == "" && address != "" {
		status.Reachable = true
		status.Installed = false
		status.Angie = "external"
		status.Blocky = "external"
		status.InstallState = "external-dns"
		status.LastError = ""
		status.DesiredDNSOn = config.Routing.DesiredDNSOn
		status.EffectiveDNSOn = status.DesiredDNSOn && !config.Routing.FailsafeActive
		if err := quickTCPCheck(address, 853); err != nil {
			status.Reachable = false
			status.LastError = err.Error()
			status.ConsecutiveFailures++
			_ = a.store.AppendLog("warn", fmt.Sprintf("Проверка %s: внешний DNS %s недоступен: %s", reason, sni, err.Error()))
			_ = a.store.UpdateProfileStatus(*status)
			a.applyFailsafe(status)
			return status, err
		}
		status.LastSuccessAt = nowRFC3339()
		status.ConsecutiveFailures = 0
		if status.DesiredDNSOn && !config.Routing.FailsafeActive {
			if message, err := a.router.EnsureDNSState(address, sni, true); err == nil && message != "" {
				_ = a.store.AppendLog("info", message)
			}
		}
		_ = a.store.UpdateProfileStatus(*status)
		return status, nil
	}

	if profile.Host == "" {
		status.Reachable = false
		status.LastError = "Заполни либо адрес VPS, либо DNS endpoint для отдельной установки aiway"
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
	remoteStatus.CustomDomains = mergeStrings(remoteStatus.CustomDomains, a.store.Config().Routing.CustomDomains)
	if remoteStatus.ServiceCount == 0 {
		remoteStatus.ServiceCount = enabledDomainCount(a.store.Config())
	}

	if a.store.Config().Routing.FailsafeActive && a.store.Config().Safety.AutoRecover {
		if _, err := a.router.EnsureDNSState(address, sni, true); err == nil {
			_ = a.store.AppendLog("info", "Маршрут aiway DNS снова привязан к основному WAN")
		}
		_ = a.store.SetFailsafe(false)
		remoteStatus.EffectiveDNSOn = remoteStatus.DesiredDNSOn
		_ = a.store.AppendLog("info", "Фейлсейф автоматически снят после успешной проверки")
	}

	if remoteStatus.DesiredDNSOn && !a.store.Config().Routing.FailsafeActive {
		if message, err := a.router.EnsureDNSState(address, sni, true); err == nil {
			if message != "" {
				_ = a.store.AppendLog("info", message)
			}
		} else {
			_ = a.store.AppendLog("warn", fmt.Sprintf("Не удалось применить DNS-маршрут на роутере: %v", err))
		}
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
		if message, err := a.router.EnsureDNSState("", "", false); err == nil {
			if message != "" {
				_ = a.store.AppendLog("warn", message)
			}
		} else {
			_ = a.store.AppendLog("error", fmt.Sprintf("Не удалось отключить aiway DNS при фейлсейфе: %v", err))
		}
	}
	status.EffectiveDNSOn = false
	_ = a.store.UpdateProfileStatus(*status)
}

func dnsTarget(config Config, profile Profile) (string, string) {
	address := strings.TrimSpace(config.Routing.UpstreamAddress)
	sni := strings.TrimSpace(config.Routing.UpstreamSNI)
	if address == "" {
		address = strings.TrimSpace(profile.Host)
	}
	if sni == "" {
		sni = strings.TrimSpace(profile.Domain)
	}
	return address, sni
}

func profileCanManage(profile Profile) bool {
	return strings.TrimSpace(profile.Host) != ""
}

func enabledDomainCount(config Config) int {
	seen := map[string]struct{}{}
	for _, service := range config.Routing.Services {
		if !service.Enabled {
			continue
		}
		for _, domain := range service.Domains {
			domain = strings.TrimSpace(domain)
			if domain == "" {
				continue
			}
			seen[domain] = struct{}{}
		}
	}
	for _, domain := range config.Routing.CustomDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		seen[domain] = struct{}{}
	}
	return len(seen)
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
