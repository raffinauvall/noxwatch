package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type bootstrapRequest struct {
	Target      string `json:"target"`
	Port        string `json:"port"`
	Endpoint    string `json:"endpoint"`
	Token       string `json:"token"`
	ServerName  string `json:"server_name"`
	Environment string `json:"environment"`
}

type helper struct {
	origin string
	launch func(bootstrapRequest) error
}

var (
	safeTarget = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+$`)
	safeToken  = regexp.MustCompile(`^nox_enroll_[A-Za-z0-9_-]{20,}$`)
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9734", "local helper address")
	origin := flag.String("origin", "http://localhost:3000", "allowed dashboard origin")
	repoRoot := flag.String("repo-root", ".", "NoxWatch repository root")
	localAPIPort := flag.String("local-api-port", "8080", "host port of the local NoxWatch API")
	flag.Parse()
	if err := validatePort(*localAPIPort); err != nil {
		log.Fatal("invalid local API port")
	}

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		log.Fatal(err)
	}
	launch, terminal, err := terminalLauncher(root, *localAPIPort)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           helper{origin: strings.TrimRight(*origin, "/"), launch: launch},
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("NoxWatch local helper listening on http://%s for %s using %s", *addr, *origin, terminal)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (h helper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/bootstrap" {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Origin") != h.origin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dashboard origin is not allowed"})
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", h.origin)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var input bootstrapRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bootstrap request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bootstrap request"})
		return
	}
	if err := validate(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.launch(input); err != nil {
		log.Printf("bootstrap terminal launch failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not open a local terminal"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "terminal opened"})
}

func validate(input bootstrapRequest) error {
	if !safeTarget.MatchString(input.Target) {
		return errors.New("invalid SSH target")
	}
	port, err := strconv.Atoi(input.Port)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid SSH port")
	}
	endpoint, err := url.ParseRequestURI(input.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return errors.New("invalid API endpoint")
	}
	if !safeToken.MatchString(input.Token) {
		return errors.New("invalid enrollment token")
	}
	if input.ServerName == "" || strings.ContainsAny(input.ServerName, "'\r\n") {
		return errors.New("invalid server name")
	}
	switch input.Environment {
	case "production", "staging", "development", "testing", "other":
	default:
		return errors.New("invalid environment")
	}
	return nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid port")
	}
	return nil
}

func reverseTunnel(endpoint string) (string, string, bool) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") {
		return endpoint, "", false
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	parsed.Host = net.JoinHostPort("127.0.0.1", port)
	return parsed.String(), port, true
}

func terminalLauncher(repoRoot, localAPIPort string) (func(bootstrapRequest) error, string, error) {
	script := filepath.Join(repoRoot, "deployments", "scripts", "bootstrap-ssh.sh")
	binary := filepath.Join(repoRoot, "dist", "noxwatch-agent")
	service := filepath.Join(repoRoot, "deployments", "systemd", "noxwatch-agent.service")
	for _, path := range []string{script, binary, service} {
		if _, err := os.Stat(path); err != nil {
			return nil, "", fmt.Errorf("required file is missing: %s", path)
		}
	}

	type candidate struct {
		name   string
		prefix []string
	}
	candidates := []candidate{
		{name: "kitty", prefix: []string{"--detach", "--hold"}},
		{name: "xdg-terminal-exec"},
		{name: "alacritty", prefix: []string{"-e"}},
		{name: "foot", prefix: []string{"--hold"}},
		{name: "xterm", prefix: []string{"-hold", "-e"}},
	}
	for _, item := range candidates {
		terminal, err := exec.LookPath(item.name)
		if err != nil {
			continue
		}
		return func(input bootstrapRequest) error {
			endpoint, remotePort, reverse := reverseTunnel(input.Endpoint)
			args := append([]string{}, item.prefix...)
			args = append(args, script,
				"--target", input.Target,
				"--port", input.Port,
				"--endpoint", endpoint,
				"--token", input.Token,
				"--server-name", input.ServerName,
				"--environment", input.Environment,
				"--binary", binary,
				"--service", service,
			)
			if reverse {
				args = append(args, "--reverse-local-port", localAPIPort, "--reverse-remote-port", remotePort)
			}
			cmd := exec.Command(terminal, args...)
			cmd.Dir = repoRoot
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		}, item.name, nil
	}
	return nil, "", errors.New("no supported terminal found (kitty, xdg-terminal-exec, alacritty, foot, or xterm)")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
