package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Store struct {
	mu         sync.RWMutex
	config     Config
	statuses   map[string]*ProfileStatus
	logs       []LogEntry
	configPath string
	statePath  string
}

type persistedState struct {
	Statuses map[string]*ProfileStatus `json:"statuses"`
	Logs     []LogEntry                `json:"logs"`
}

func NewStore(configDir string) (*Store, error) {
	if configDir == "" {
		return nil, errors.New("config dir is required")
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}

	store := &Store{
		configPath: filepath.Join(configDir, "config.json"),
		statePath:  filepath.Join(configDir, "state.json"),
		statuses:   map[string]*ProfileStatus{},
		logs:       []LogEntry{},
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	if _, err := os.Stat(s.configPath); errors.Is(err, os.ErrNotExist) {
		s.config = DefaultConfig()
		if err := s.saveLocked(); err != nil {
			return err
		}
	} else if err == nil {
		payload, readErr := os.ReadFile(s.configPath)
		if readErr != nil {
			return readErr
		}
		if err := json.Unmarshal(payload, &s.config); err != nil {
			return err
		}
	} else {
		return err
	}

	if payload, err := os.ReadFile(s.statePath); err == nil {
		var state persistedState
		if json.Unmarshal(payload, &state) == nil {
			s.statuses = state.Statuses
			s.logs = state.Logs
		}
	}

	for _, profile := range s.config.Profiles {
		if _, ok := s.statuses[profile.ID]; !ok {
			s.statuses[profile.ID] = &ProfileStatus{ProfileID: profile.ID, InstallState: "unknown", CustomDomains: cloneStrings(profile.CustomDomains)}
		}
	}
	s.normalizeLocked()
	s.pruneStatuses()
	return nil
}

func (s *Store) normalizeLocked() {
	if s.config.Routing.CustomDomains == nil {
		s.config.Routing.CustomDomains = []string{}
	}
	if s.config.Routing.Services == nil {
		s.config.Routing.Services = []ServiceToggle{}
	}
	if s.logs == nil {
		s.logs = []LogEntry{}
	}
	for i := range s.config.Profiles {
		if s.config.Profiles[i].CustomDomains == nil {
			s.config.Profiles[i].CustomDomains = []string{}
		}
	}
	for id, status := range s.statuses {
		if status == nil {
			continue
		}
		if status.CustomDomains == nil {
			status.CustomDomains = []string{}
		}
		s.statuses[id] = status
	}
}

func (s *Store) pruneStatuses() {
	valid := map[string]struct{}{}
	for _, profile := range s.config.Profiles {
		valid[profile.ID] = struct{}{}
	}
	for id := range s.statuses {
		if _, ok := valid[id]; !ok {
			delete(s.statuses, id)
		}
	}
	if s.config.ActiveID == "" && len(s.config.Profiles) > 0 {
		s.config.ActiveID = s.config.Profiles[0].ID
	}
}

func (s *Store) saveLocked() error {
	configPayload, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.configPath, configPayload, 0o600); err != nil {
		return err
	}

	statePayload, err := json.MarshalIndent(persistedState{Statuses: s.statuses, Logs: s.logs}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, statePayload, 0o600)
}

func (s *Store) Snapshot() OverviewResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make(map[string]*ProfileStatus, len(s.statuses))
	for id, status := range s.statuses {
		clone := *status
		clone.CustomDomains = cloneStrings(status.CustomDomains)
		statuses[id] = &clone
	}

	profiles := sanitizeProfiles(s.config.Profiles)
	logs := append([]LogEntry(nil), s.logs...)
	configCopy := s.config
	configCopy.Profiles = profiles
	configCopy.Routing.CustomDomains = cloneStrings(s.config.Routing.CustomDomains)
	configCopy.Routing.Services = append([]ServiceToggle(nil), s.config.Routing.Services...)
	if configCopy.Routing.CustomDomains == nil {
		configCopy.Routing.CustomDomains = []string{}
	}
	if logs == nil {
		logs = []LogEntry{}
	}

	response := OverviewResponse{
		Config:      configCopy,
		Profiles:    profiles,
		Statuses:    statuses,
		Logs:        logs,
		Version:     Version,
		GeneratedAt: nowRFC3339(),
	}
	for _, profile := range profiles {
		if profile.ID == s.config.ActiveID {
			profileCopy := profile
			response.ActiveProfile = &profileCopy
			break
		}
	}
	return response
}

func (s *Store) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	configCopy := s.config
	configCopy.Profiles = sanitizeProfiles(s.config.Profiles)
	configCopy.Routing.CustomDomains = cloneStrings(s.config.Routing.CustomDomains)
	configCopy.Routing.Services = append([]ServiceToggle(nil), s.config.Routing.Services...)
	if configCopy.Routing.CustomDomains == nil {
		configCopy.Routing.CustomDomains = []string{}
	}
	return configCopy
}

func (s *Store) UpdateConfig(next Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if next.Dashboard.Port == 0 {
		next.Dashboard.Port = 2233
	}
	if next.Dashboard.ListenAddress == "" {
		next.Dashboard.ListenAddress = ":2233"
	}
	if next.Safety.IntervalSeconds < 15 {
		next.Safety.IntervalSeconds = 15
	}
	if next.Safety.FailThreshold < 1 {
		next.Safety.FailThreshold = 1
	}
	if len(next.Profiles) == 0 {
		next.Profiles = DefaultConfig().Profiles
	}
	if next.ActiveID == "" {
		next.ActiveID = next.Profiles[0].ID
	}
	mergeProfileSecrets(s.config.Profiles, next.Profiles)

	s.config = next
	s.normalizeLocked()
	s.pruneStatuses()
	return s.saveLocked()
}

func sanitizeProfiles(profiles []Profile) []Profile {
	copyProfiles := append([]Profile(nil), profiles...)
	for i := range copyProfiles {
		copyProfiles[i].Password = ""
		copyProfiles[i].PrivateKey = ""
		copyProfiles[i].SudoPassword = ""
	}
	return copyProfiles
}

func mergeProfileSecrets(existing []Profile, next []Profile) {
	existingByID := map[string]Profile{}
	for _, profile := range existing {
		existingByID[profile.ID] = profile
	}
	for i := range next {
		current, ok := existingByID[next[i].ID]
		if !ok {
			continue
		}
		if next[i].Password == "" {
			next[i].Password = current.Password
		}
		if next[i].PrivateKey == "" {
			next[i].PrivateKey = current.PrivateKey
		}
		if next[i].SudoPassword == "" {
			next[i].SudoPassword = current.SudoPassword
		}
	}
}

func (s *Store) SetDNSDesired(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Routing.DesiredDNSOn = enabled
	return s.saveLocked()
}

func (s *Store) SetFailsafe(active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Routing.FailsafeActive = active
	if active {
		s.config.Routing.LastAppliedAt = nowRFC3339()
	}
	return s.saveLocked()
}

func (s *Store) AddDomain(domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.config.Routing.CustomDomains {
		if existing == domain {
			return nil
		}
	}
	s.config.Routing.CustomDomains = append(s.config.Routing.CustomDomains, domain)
	sort.Strings(s.config.Routing.CustomDomains)
	return s.saveLocked()
}

func (s *Store) RemoveDomain(domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.config.Routing.CustomDomains[:0]
	for _, existing := range s.config.Routing.CustomDomains {
		if existing != domain {
			filtered = append(filtered, existing)
		}
	}
	s.config.Routing.CustomDomains = cloneStrings(filtered)
	return s.saveLocked()
}

func (s *Store) FindProfile(id string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, profile := range s.config.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}

func (s *Store) ActiveProfile() (Profile, bool) {
	return s.FindProfile(s.Config().ActiveID)
}

func (s *Store) UpdateProfileStatus(status ProfileStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := status
	if clone.CustomDomains == nil {
		clone.CustomDomains = []string{}
	}
	s.statuses[status.ProfileID] = &clone
	return s.saveLocked()
}

func (s *Store) ProfileStatus(id string) *ProfileStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.statuses[id]
	if !ok {
		return nil
	}
	clone := *current
	clone.CustomDomains = cloneStrings(current.CustomDomains)
	return &clone
}

func (s *Store) AppendLog(level string, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, LogEntry{Timestamp: nowRFC3339(), Level: level, Message: message})
	if len(s.logs) > 300 {
		s.logs = append([]LogEntry(nil), s.logs[len(s.logs)-300:]...)
	}
	return s.saveLocked()
}

func (s *Store) ClearLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = nil
	s.normalizeLocked()
	return s.saveLocked()
}
