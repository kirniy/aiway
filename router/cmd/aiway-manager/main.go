package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kirniy/aiway/router/manager/internal/app"
)

func main() {
	if len(os.Args) < 2 {
		serveCmd(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "serve":
		serveCmd(os.Args[2:])
	case "status":
		apiCmd(os.Args[2:], http.MethodGet, "/api/overview", nil)
	case "check":
		apiCmd(os.Args[2:], http.MethodPost, "/api/actions/check-now", map[string]any{})
	case "logs":
		apiCmd(os.Args[2:], http.MethodGet, "/api/logs", nil)
	case "domains":
		domainsCmd(os.Args[2:])
	case "dns":
		dnsCmd(os.Args[2:])
	case "profiles":
		profilesCmd(os.Args[2:])
	default:
		serveCmd(os.Args[1:])
	}
}

func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configDir := fs.String("config-dir", "/opt/etc/aiway-manager", "directory for aiway manager state")
	addr := fs.String("addr", ":2222", "listen address")
	_ = fs.Parse(args)

	application, err := app.New(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init failed: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	application.Start(ctx)

	if err := application.ListenAndServe(*addr); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
		os.Exit(1)
	}
}

func endpointFlagSet(name string, args []string) (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	endpoint := fs.String("endpoint", "http://192.168.1.1:2222", "aiway manager endpoint")
	asJSON := fs.Bool("json", false, "print raw JSON")
	_ = fs.Parse(args)
	return fs, endpoint, asJSON
}

func apiCmd(args []string, method string, path string, body any) {
	_, endpoint, asJSON := endpointFlagSet("api", args)
	status, payload, err := request(*endpoint, method, path, body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		fmt.Println(string(payload))
		return
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, payload, "", "  "); err != nil {
		fmt.Printf("HTTP %d\n%s\n", status, string(payload))
		return
	}
	fmt.Printf("HTTP %d\n%s\n", status, pretty.String())
}

func request(endpoint string, method string, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(payload)
	}

	endpoint = strings.TrimRight(endpoint, "/")
	req, err := http.NewRequest(method, endpoint+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	if resp.StatusCode >= 400 {
		return resp.StatusCode, payload, fmt.Errorf("http %d", resp.StatusCode)
	}
	return resp.StatusCode, payload, nil
}

func domainsCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("usage: aiway-manager domains <list|add|remove> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		apiCmd(args[1:], http.MethodGet, "/api/config", nil)
	case "add":
		if len(args) < 2 {
			fmt.Println("usage: aiway-manager domains add <domain>")
			os.Exit(1)
		}
		_, endpoint, asJSON := endpointFlagSet("domains-add", args[2:])
		status, payload, err := request(*endpoint, http.MethodPost, "/api/actions/domain/add", map[string]any{"domain": args[1]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "add failed: %v\n", err)
			os.Exit(1)
		}
		if *asJSON {
			fmt.Println(string(payload))
			return
		}
		fmt.Printf("HTTP %d\n%s\n", status, string(payload))
	case "remove":
		if len(args) < 2 {
			fmt.Println("usage: aiway-manager domains remove <domain>")
			os.Exit(1)
		}
		_, endpoint, asJSON := endpointFlagSet("domains-remove", args[2:])
		status, payload, err := request(*endpoint, http.MethodPost, "/api/actions/domain/remove", map[string]any{"domain": args[1]})
		if err != nil {
			fmt.Fprintf(os.Stderr, "remove failed: %v\n", err)
			os.Exit(1)
		}
		if *asJSON {
			fmt.Println(string(payload))
			return
		}
		fmt.Printf("HTTP %d\n%s\n", status, string(payload))
	default:
		fmt.Println("usage: aiway-manager domains <list|add|remove> [args]")
		os.Exit(1)
	}
}

func dnsCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("usage: aiway-manager dns <on|off>")
		os.Exit(1)
	}

	enabled := args[0] == "on"
	if args[0] != "on" && args[0] != "off" {
		fmt.Println("usage: aiway-manager dns <on|off>")
		os.Exit(1)
	}
	_, endpoint, asJSON := endpointFlagSet("dns", args[1:])
	status, payload, err := request(*endpoint, http.MethodPost, "/api/actions/toggle-dns", map[string]any{"enabled": enabled})
	if err != nil {
		fmt.Fprintf(os.Stderr, "toggle failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		fmt.Println(string(payload))
		return
	}
	fmt.Printf("HTTP %d\n%s\n", status, string(payload))
}

func profilesCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("usage: aiway-manager profiles <install|uninstall|reset|sync> --profile <id>")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("profiles", flag.ExitOnError)
	endpoint := fs.String("endpoint", "http://192.168.1.1:2222", "aiway manager endpoint")
	profile := fs.String("profile", "", "profile id")
	asJSON := fs.Bool("json", false, "print raw JSON")
	_ = fs.Parse(args[1:])

	if *profile == "" {
		fmt.Println("--profile is required")
		os.Exit(1)
	}

	path := map[string]string{
		"install":   "/api/actions/profile/install",
		"uninstall": "/api/actions/profile/uninstall",
		"reset":     "/api/actions/profile/reset",
		"sync":      "/api/actions/profile/sync",
	}[args[0]]
	if path == "" {
		fmt.Println("usage: aiway-manager profiles <install|uninstall|reset|sync> --profile <id>")
		os.Exit(1)
	}

	status, payload, err := request(*endpoint, http.MethodPost, path, map[string]any{"profileId": *profile})
	if err != nil {
		fmt.Fprintf(os.Stderr, "profile action failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		fmt.Println(string(payload))
		return
	}
	fmt.Printf("HTTP %d\n%s\n", status, string(payload))
}
