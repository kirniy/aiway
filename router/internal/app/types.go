package app

import "time"

type Config struct {
	Dashboard DashboardConfig `json:"dashboard"`
	Safety    SafetyConfig    `json:"safety"`
	Routing   RoutingConfig   `json:"routing"`
	Profiles  []Profile       `json:"profiles"`
	ActiveID  string          `json:"activeId"`
}

type DashboardConfig struct {
	Port            int    `json:"port"`
	ListenAddress   string `json:"listenAddress"`
	AuthEnabled     bool   `json:"authEnabled"`
	ThemePreference string `json:"themePreference"`
}

type SafetyConfig struct {
	Enabled             bool     `json:"enabled"`
	IntervalSeconds     int      `json:"intervalSeconds"`
	FailThreshold       int      `json:"failThreshold"`
	AutoRecover         bool     `json:"autoRecover"`
	DisableDNSOnFailure bool     `json:"disableDnsOnFailure"`
	CanaryDomains       []string `json:"canaryDomains"`
}

type RoutingConfig struct {
	DesiredDNSOn    bool            `json:"desiredDnsOn"`
	UpstreamAddress string          `json:"upstreamAddress"`
	UpstreamSNI     string          `json:"upstreamSni"`
	CustomDomains   []string        `json:"customDomains"`
	Services        []ServiceToggle `json:"services"`
	LastAppliedAt   string          `json:"lastAppliedAt"`
	FailsafeActive  bool            `json:"failsafeActive"`
}

type ServiceToggle struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domains     []string `json:"domains"`
	Enabled     bool     `json:"enabled"`
}

type Profile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Username         string   `json:"username"`
	AuthMethod       string   `json:"authMethod"`
	Password         string   `json:"password,omitempty"`
	PrivateKey       string   `json:"privateKey,omitempty"`
	UseSudo          bool     `json:"useSudo"`
	SudoPassword     string   `json:"sudoPassword,omitempty"`
	Domain           string   `json:"domain"`
	Email            string   `json:"email"`
	RepoRef          string   `json:"repoRef"`
	CustomDomains    []string `json:"customDomains"`
	InstallOnConnect bool     `json:"installOnConnect"`
}

type ProfileStatus struct {
	ProfileID            string       `json:"profileId"`
	Reachable            bool         `json:"reachable"`
	Installed            bool         `json:"installed"`
	Angie                string       `json:"angie"`
	Blocky               string       `json:"blocky"`
	LastError            string       `json:"lastError,omitempty"`
	LastCheckAt          string       `json:"lastCheckAt,omitempty"`
	LastSuccessAt        string       `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures  int          `json:"consecutiveFailures"`
	DesiredDNSOn         bool         `json:"desiredDnsOn"`
	EffectiveDNSOn       bool         `json:"effectiveDnsOn"`
	LastDoctor           DoctorResult `json:"lastDoctor"`
	InstallState         string       `json:"installState"`
	InstallOutputPreview string       `json:"installOutputPreview"`
	CustomDomains        []string     `json:"customDomains"`
	ServiceCount         int          `json:"serviceCount"`
}

type DoctorResult struct {
	Angie     bool   `json:"angie"`
	Blocky    bool   `json:"blocky"`
	DNS       bool   `json:"dns"`
	DNSResult string `json:"dnsResult"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

type OverviewResponse struct {
	Config        Config                    `json:"config"`
	Profiles      []Profile                 `json:"profiles"`
	Statuses      map[string]*ProfileStatus `json:"statuses"`
	Logs          []LogEntry                `json:"logs"`
	ActiveProfile *Profile                  `json:"activeProfile,omitempty"`
	RouterDNS     RouterDNSState            `json:"routerDns"`
	Version       string                    `json:"version"`
	GeneratedAt   string                    `json:"generatedAt"`
}

type RouterDNSState struct {
	Active  bool   `json:"active"`
	Address string `json:"address"`
	SNI     string `json:"sni"`
}

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Package   string `json:"package,omitempty"`
	URL       string `json:"url,omitempty"`
}

type ProfileActionRequest struct {
	ProfileID string `json:"profileId"`
}

type DomainActionRequest struct {
	Domain string `json:"domain"`
}

type DNSActionRequest struct {
	Enabled bool `json:"enabled"`
}

func DefaultConfig() Config {
	return Config{
		Dashboard: DashboardConfig{
			Port:            2233,
			ListenAddress:   ":2233",
			AuthEnabled:     false,
			ThemePreference: "awg-dark",
		},
		Safety: SafetyConfig{
			Enabled:             true,
			IntervalSeconds:     90,
			FailThreshold:       3,
			AutoRecover:         false,
			DisableDNSOnFailure: true,
			CanaryDomains:       []string{"openai.com", "claude.ai", "chatgpt.com"},
		},
		Routing: RoutingConfig{
			DesiredDNSOn: true,
			Services: []ServiceToggle{
				{ID: "openai", Name: "OpenAI / ChatGPT", Description: "ChatGPT, OpenAI API, oaiusercontent", Domains: []string{"openai.com", "chatgpt.com", "oaiusercontent.com"}, Enabled: true},
				{ID: "anthropic", Name: "Claude / Anthropic", Description: "claude.ai и anthropic.com", Domains: []string{"claude.ai", "anthropic.com"}, Enabled: true},
				{ID: "gemini", Name: "Gemini / Google AI", Description: "Gemini, AI Studio, Generative Language API", Domains: []string{"gemini.google.com", "aistudio.google.com", "generativelanguage.googleapis.com"}, Enabled: true},
				{ID: "copilot", Name: "GitHub Copilot", Description: "GitHub, Copilot, Microsoft Copilot", Domains: []string{"github.com", "githubcopilot.com", "copilot.microsoft.com"}, Enabled: true},
				{ID: "other", Name: "Прочие AI-сервисы", Description: "Perplexity, Mistral, xAI, Meta AI, Replicate, Stability", Domains: []string{"perplexity.ai", "mistral.ai", "x.ai", "grok.com", "meta.ai", "replicate.com", "stability.ai", "udio.com", "cohere.ai", "huggingface.co", "midjourney.com", "poe.com", "character.ai", "you.com", "pi.ai"}, Enabled: true},
			},
		},
		Profiles: []Profile{
			{
				ID:         "primary-vps",
				Name:       "Основной VPS",
				Host:       "",
				Port:       22,
				Username:   "root",
				AuthMethod: "key",
				PrivateKey: "/opt/etc/aiway-manager/id_ed25519",
				UseSudo:    false,
				RepoRef:    "main",
			},
		},
		ActiveID: "primary-vps",
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func mergeStrings(values ...[]string) []string {
	seen := map[string]struct{}{}
	merged := []string{}
	for _, group := range values {
		for _, value := range group {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
}
