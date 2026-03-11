package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHRunner struct{}

type SSHResult struct {
	Stdout string
	Stderr string
	Code   int
}

func (r SSHRunner) Run(profile Profile, script string) (SSHResult, error) {
	config, err := sshConfig(profile)
	if err != nil {
		return SSHResult{}, err
	}

	addr := fmt.Sprintf("%s:%d", profile.Host, profile.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return SSHResult{}, err
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return SSHResult{}, err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	wrapped := wrapRemoteScript(profile, script)
	err = session.Run(wrapped)
	result := SSHResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*ssh.ExitError); ok {
		result.Code = exitErr.ExitStatus()
		return result, fmt.Errorf("remote exit status %d: %s", result.Code, strings.TrimSpace(stderr.String()))
	}
	return result, err
}

func (r SSHRunner) Install(profile Profile) (string, error) {
	repoRef := profile.RepoRef
	if repoRef == "" {
		repoRef = "main"
	}

	installScript := fmt.Sprintf(`set -euo pipefail
tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT
mkdir -p "$tmp/lib" "$tmp/server"
base="https://raw.githubusercontent.com/kirniy/aiway/%s"
curl -fsSL "$base/install.sh" -o "$tmp/install.sh"
curl -fsSL "$base/uninstall.sh" -o "$tmp/uninstall.sh"
curl -fsSL "$base/lib/utils.sh" -o "$tmp/lib/utils.sh"
curl -fsSL "$base/lib/domains.sh" -o "$tmp/lib/domains.sh"
curl -fsSL "$base/server/aiwayctl.sh" -o "$tmp/server/aiwayctl.sh"
chmod +x "$tmp/install.sh" "$tmp/uninstall.sh" "$tmp/server/aiwayctl.sh"
AIWAY_NONINTERACTIVE=1 AIWAY_YES=1 AIWAY_NO_CLEAR=1 AIWAY_VPS_IP=%q AIWAY_DOT_DOMAIN=%q AIWAY_ACME_EMAIL=%q bash "$tmp/install.sh"
`, repoRef, shellEscape(profile.Host), shellEscape(profile.Domain), shellEscape(profile.Email))

	result, err := r.Run(profile, installScript)
	combined := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	return combined, err
}

func (r SSHRunner) RemoteStatus(profile Profile) (ProfileStatus, error) {
	result, err := r.Run(profile, `aiwayctl status --json`)
	if err != nil {
		return ProfileStatus{}, err
	}
	var payload struct {
		Angie     string `json:"angie"`
		Blocky    string `json:"blocky"`
		VPSIP     string `json:"vpsIp"`
		DotDomain string `json:"dotDomain"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &payload); err != nil {
		return ProfileStatus{}, err
	}

	doctor, doctorErr := r.RemoteDoctor(profile)
	status := ProfileStatus{
		ProfileID:     profile.ID,
		Reachable:     true,
		Installed:     payload.Angie != "" || payload.Blocky != "",
		Angie:         payload.Angie,
		Blocky:        payload.Blocky,
		LastCheckAt:   nowRFC3339(),
		LastSuccessAt: nowRFC3339(),
		InstallState:  "installed",
		CustomDomains: append([]string(nil), profile.CustomDomains...),
		ServiceCount:  len(profile.CustomDomains),
	}
	if doctorErr == nil {
		status.LastDoctor = doctor
		status.Installed = doctor.Angie || doctor.Blocky || status.Installed
	}
	return status, nil
}

func (r SSHRunner) RemoteDoctor(profile Profile) (DoctorResult, error) {
	result, err := r.Run(profile, `aiwayctl doctor --json`)
	if err != nil {
		return DoctorResult{}, err
	}
	var payload DoctorResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &payload); err != nil {
		return DoctorResult{}, err
	}
	return payload, nil
}

func (r SSHRunner) RemoteAddDomain(profile Profile, domain string) (string, error) {
	result, err := r.Run(profile, fmt.Sprintf("aiwayctl add-domain %s", shellEscape(domain)))
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr), err
}

func (r SSHRunner) RemoteRemoveDomain(profile Profile, domain string) (string, error) {
	result, err := r.Run(profile, fmt.Sprintf("aiwayctl remove-domain %s", shellEscape(domain)))
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr), err
}

func (r SSHRunner) RemoteReapply(profile Profile) (string, error) {
	result, err := r.Run(profile, `aiwayctl reapply`)
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr), err
}

func (r SSHRunner) RemoteUninstall(profile Profile) (string, error) {
	result, err := r.Run(profile, `aiwayctl uninstall`)
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr), err
}

func (r SSHRunner) RemoteReset(profile Profile) (string, error) {
	result, err := r.Run(profile, `rm -f /etc/aiway/custom-domains.txt && aiwayctl reapply`)
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr), err
}

func sshConfig(profile Profile) (*ssh.ClientConfig, error) {
	authMethod, err := buildAuthMethod(profile)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            profile.Username,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         12 * time.Second,
	}, nil
}

func buildAuthMethod(profile Profile) (ssh.AuthMethod, error) {
	switch profile.AuthMethod {
	case "password":
		return ssh.Password(profile.Password), nil
	default:
		key, err := os.ReadFile(profile.PrivateKey)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	}
}

func wrapRemoteScript(profile Profile, script string) string {
	prefix := ""
	if profile.UseSudo && profile.Username != "root" {
		if profile.SudoPassword != "" {
			prefix = fmt.Sprintf("printf %%s\\n %s | sudo -S bash -lc ", shellEscape(profile.SudoPassword))
		} else {
			prefix = "sudo bash -lc "
		}
	} else {
		prefix = "bash -lc "
	}
	return prefix + shellEscape(script)
}

func shellEscape(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func quickTCPCheck(host string, port int) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, _ = io.Copy(io.Discard, strings.NewReader(""))
	return nil
}
