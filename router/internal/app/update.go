package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Updater struct {
	repo string
}

func NewUpdater() Updater {
	return Updater{repo: "kirniy/aiway"}
}

func (u Updater) Check() (UpdateInfo, error) {
	arch, err := detectKeeneticArch()
	if err != nil {
		return UpdateInfo{}, err
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", u.repo), nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "aiway-manager/"+Version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return UpdateInfo{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UpdateInfo{}, err
	}
	if resp.StatusCode >= 400 {
		return UpdateInfo{}, fmt.Errorf("github releases responded with http %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return UpdateInfo{}, err
	}

	for _, release := range releases {
		latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if latest == "" {
			continue
		}
		packageName := fmt.Sprintf("aiway-manager_%s_%s.ipk", latest, arch)
		for _, asset := range release.Assets {
			if asset.Name == packageName {
				return UpdateInfo{
					Current:   Version,
					Latest:    latest,
					Available: latest != Version,
					Package:   asset.Name,
					URL:       asset.URL,
				}, nil
			}
		}
	}

	return UpdateInfo{Current: Version, Latest: Version, Available: false}, nil
}

func (u Updater) Apply() (UpdateInfo, error) {
	info, err := u.Check()
	if err != nil {
		return info, err
	}
	if !info.Available {
		return info, nil
	}

	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf(`set -e
tmp="/opt/tmp/%s"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$tmp" "%s"
else
  wget -qO "$tmp" "%s"
fi
opkg install "$tmp"
`, info.Package, info.URL, info.URL))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("update failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return info, nil
}

func detectKeeneticArch() (string, error) {
	cmd := exec.Command("/bin/sh", "-c", `opkg print-architecture`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cannot detect architecture: %v", err)
	}

	type item struct {
		name     string
		priority int
	}
	var matches []item
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "_kn") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		name := parts[1]
		priority := 0
		fmt.Sscanf(parts[2], "%d", &priority)
		matches = append(matches, item{name: name, priority: priority})
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no Keenetic architecture found in opkg print-architecture")
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].priority > matches[j].priority })
	return matches[0].name, nil
}
