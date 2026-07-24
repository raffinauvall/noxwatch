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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type bootstrapRequest struct {
	ProfileID   string `json:"profile_id"`
	Target      string `json:"target"`
	Port        string `json:"port"`
	Endpoint    string `json:"endpoint"`
	Token       string `json:"token"`
	ServerName  string `json:"server_name"`
	Environment string `json:"environment"`
}

type helper struct {
	origin        string
	localAPIPort  string
	store         *tunnelStore
	launch        func(bootstrapRequest, string) error
	launchTunnels func([]string) error
	launchTunnel  func(string) error
	launchShell   func(tunnelProfile, string) error
}

var (
	safeTarget = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+$`)
	safeToken  = regexp.MustCompile(`^nox_enroll_[A-Za-z0-9_-]{20,}$`)
	safeID     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	addr := flag.String("addr", "127.0.0.1:9734", "local helper address")
	origin := flag.String("origin", "http://localhost:3000", "allowed dashboard origin")
	repoRoot := flag.String("repo-root", ".", "NoxWatch repository root")
	localAPIPort := flag.String("local-api-port", "8080", "host port of the local NoxWatch API")
	profilesPath := flag.String("tunnel-profiles", filepath.Join(configDir, "noxwatch", "tunnels.json"), "local tunnel profile file")
	runTunnelProfiles := flag.String("run-tunnels", "", "start profiles from a local tunnel profile file")
	runTunnelID := flag.String("tunnel-id", "", "start only one tunnel profile")
	runTunnelIDs := flag.String("tunnel-ids", "", "start selected comma-separated tunnel profiles")
	flag.Parse()
	if err := validatePort(*localAPIPort); err != nil {
		log.Fatal("invalid local API port")
	}

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		log.Fatal(err)
	}
	store, err := newTunnelStore(*profilesPath)
	if err != nil {
		log.Fatal(err)
	}
	if *runTunnelProfiles != "" {
		runStore, err := newTunnelStore(*runTunnelProfiles)
		if err != nil {
			log.Fatal(err)
		}
		selected := map[string]bool{}
		for _, id := range append([]string{*runTunnelID}, strings.Split(*runTunnelIDs, ",")...) {
			if id == "" {
				continue
			}
			if !safeID.MatchString(id) {
				log.Fatal("invalid tunnel profile ID")
			}
			selected[id] = true
		}
		if err := runTunnels(root, runStore, selected); err != nil {
			log.Fatal(err)
		}
		return
	}
	launch, launchTunnels, launchTunnel, launchShell, terminal, err := terminalLauncher(root, *localAPIPort, *profilesPath)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              *addr,
		Handler:           helper{origin: strings.TrimRight(*origin, "/"), localAPIPort: *localAPIPort, store: store, launch: launch, launchTunnels: launchTunnels, launchTunnel: launchTunnel, launchShell: launchShell},
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
	if !slices.Contains([]string{"/bootstrap", "/terminal", "/tunnels", "/tunnels/register", "/tunnels/start", "/tunnels/stop", "/tunnels/start-all", "/tunnels/stop-all"}, r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Origin") != h.origin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "dashboard origin is not allowed"})
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", h.origin)
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.URL.Path {
	case "/bootstrap":
		h.bootstrap(w, r)
	case "/terminal":
		h.openTerminal(w, r)
	case "/tunnels":
		h.tunnels(w, r)
	case "/tunnels/register":
		h.registerTunnel(w, r)
	case "/tunnels/start":
		h.startTunnel(w, r)
	case "/tunnels/stop":
		h.stopTunnel(w, r)
	case "/tunnels/start-all":
		h.startAll(w, r)
	case "/tunnels/stop-all":
		h.stopAll(w, r)
	}
}

func (h helper) openTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if !decodeLocalRequest(w, r, &input) {
		return
	}
	if !safeID.MatchString(input.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tunnel profile ID"})
		return
	}
	profile, ok := h.store.find(input.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel profile not found"})
		return
	}
	if err := h.launchShell(profile, h.store.controlPath(profile.ID)); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not open a local SSH terminal"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "terminal opened"})
}

func (h helper) registerTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var profile tunnelProfile
	if !decodeLocalRequest(w, r, &profile) {
		return
	}
	profile.LocalPort = h.localAPIPort
	if err := h.store.save(profile); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tunnel profile"})
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h helper) startTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if !decodeLocalRequest(w, r, &input) {
		return
	}
	if !safeID.MatchString(input.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tunnel profile ID"})
		return
	}
	profile, ok := h.store.find(input.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel profile not found"})
		return
	}
	if err := h.launchTunnel(profile.ID); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not open a local terminal"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "terminal opened"})
}

func (h helper) stopTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if !decodeLocalRequest(w, r, &input) {
		return
	}
	if !safeID.MatchString(input.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tunnel profile ID"})
		return
	}
	profile, ok := h.store.find(input.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel profile not found"})
		return
	}
	if err := stopTunnel(profile, h.store.controlPath(profile.ID)); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tunnel could not be stopped"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h helper) bootstrap(w http.ResponseWriter, r *http.Request) {
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
	controlPath := ""
	if _, remotePort, reverse := reverseTunnel(input.Endpoint); reverse {
		profile := tunnelProfile{ID: input.ProfileID, Name: input.ServerName, Target: input.Target, Port: input.Port, LocalPort: h.localAPIPort, RemotePort: remotePort}
		if err := h.store.save(profile); err != nil {
			log.Printf("save tunnel profile failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the local tunnel profile"})
			return
		}
		controlPath = h.store.controlPath(profile.ID)
		if err := os.MkdirAll(h.store.stateDir, 0o700); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not prepare local tunnel state"})
			return
		}
	}
	if err := h.launch(input, controlPath); err != nil {
		log.Printf("bootstrap terminal launch failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not open a local terminal"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "terminal opened"})
}

func (h helper) tunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	profiles := h.store.all()
	statuses := make([]tunnelStatus, 0, len(profiles))
	for _, profile := range profiles {
		statuses = append(statuses, tunnelStatus{tunnelProfile: profile, Running: tunnelRunning(profile, h.store.controlPath(profile.ID))})
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (h helper) startAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	profiles, ok := h.selectedProfiles(w, r)
	if !ok {
		return
	}
	if len(profiles) == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no local tunnel profiles are configured"})
		return
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
	}
	if err := h.launchTunnels(ids); err != nil {
		log.Printf("tunnel terminal launch failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not open a local terminal"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "terminal opened"})
}

func (h helper) stopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	profiles, ok := h.selectedProfiles(w, r)
	if !ok {
		return
	}
	for _, profile := range profiles {
		if err := stopTunnel(profile, h.store.controlPath(profile.ID)); err != nil {
			log.Printf("stop tunnel failed for %s: %v", profile.ID, err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "one or more tunnels could not be stopped"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h helper) selectedProfiles(w http.ResponseWriter, r *http.Request) ([]tunnelProfile, bool) {
	if r.ContentLength == 0 {
		return h.store.all(), true
	}
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeLocalRequest(w, r, &input) {
		return nil, false
	}
	profiles := make([]tunnelProfile, 0, len(input.IDs))
	seen := map[string]bool{}
	for _, id := range input.IDs {
		if !safeID.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tunnel profile ID"})
			return nil, false
		}
		profile, ok := h.store.find(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tunnel profile not found"})
			return nil, false
		}
		if !seen[profile.ID] {
			profiles = append(profiles, profile)
			seen[profile.ID] = true
		}
	}
	return profiles, true
}

func validate(input bootstrapRequest) error {
	if !safeID.MatchString(input.ProfileID) {
		return errors.New("invalid tunnel profile ID")
	}
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

func terminalLauncher(repoRoot, localAPIPort, profilesPath string) (func(bootstrapRequest, string) error, func([]string) error, func(string) error, func(tunnelProfile, string) error, string, error) {
	script := filepath.Join(repoRoot, "deployments", "scripts", "bootstrap-ssh.sh")
	binary := filepath.Join(repoRoot, "dist", "noxwatch-agent")
	service := filepath.Join(repoRoot, "deployments", "systemd", "noxwatch-agent.service")
	wrapper := filepath.Join(repoRoot, "deployments", "scripts", "terminal-command.sh")
	for _, path := range []string{script, binary, service, wrapper} {
		if _, err := os.Stat(path); err != nil {
			return nil, nil, nil, nil, "", fmt.Errorf("required file is missing: %s", path)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return nil, nil, nil, nil, "", errors.New("ssh is required")
	}

	type candidate struct {
		name   string
		prefix []string
	}
	candidates := []candidate{
		{name: "kitty", prefix: []string{"--detach"}},
		{name: "xdg-terminal-exec"},
		{name: "alacritty", prefix: []string{"-e"}},
		{name: "foot"},
		{name: "xterm", prefix: []string{"-e"}},
	}
	for _, item := range candidates {
		terminal, err := exec.LookPath(item.name)
		if err != nil {
			continue
		}
		open := func(command ...string) error {
			args := append([]string{}, item.prefix...)
			args = append(args, wrapper)
			args = append(args, command...)
			cmd := exec.Command(terminal, args...)
			cmd.Dir = repoRoot
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		}
		launchBootstrap := func(input bootstrapRequest, controlPath string) error {
			endpoint, remotePort, reverse := reverseTunnel(input.Endpoint)
			args := []string{script,
				"--target", input.Target,
				"--port", input.Port,
				"--endpoint", endpoint,
				"--token", input.Token,
				"--server-name", input.ServerName,
				"--environment", input.Environment,
				"--binary", binary,
				"--service", service,
			}
			if reverse {
				args = append(args, "--reverse-local-port", localAPIPort, "--reverse-remote-port", remotePort)
			}
			if controlPath != "" {
				args = append(args, "--control-path", controlPath)
			}
			return open(args...)
		}
		launchAll := func(ids []string) error {
			return open(executable, "-run-tunnels", profilesPath, "-tunnel-ids", strings.Join(ids, ","), "-repo-root", repoRoot)
		}
		launchOne := func(id string) error {
			return open(executable, "-run-tunnels", profilesPath, "-tunnel-id", id, "-repo-root", repoRoot)
		}
		launchShell := func(profile tunnelProfile, controlPath string) error {
			args := []string{ssh, "-p", profile.Port}
			if tunnelRunning(profile, controlPath) {
				args = append(args, "-S", controlPath)
			}
			return open(append(args, profile.Target)...)
		}
		return launchBootstrap, launchAll, launchOne, launchShell, item.name, nil
	}
	return nil, nil, nil, nil, "", errors.New("no supported terminal found (kitty, xdg-terminal-exec, alacritty, foot, or xterm)")
}

func decodeLocalRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
