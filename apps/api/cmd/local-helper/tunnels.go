package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type tunnelProfile struct {
	ID         string `json:"id"`
	ServerID   string `json:"server_id,omitempty"`
	Name       string `json:"name"`
	Target     string `json:"target"`
	Port       string `json:"port"`
	LocalPort  string `json:"local_port"`
	RemotePort string `json:"remote_port"`
}

type tunnelStatus struct {
	tunnelProfile
	Running bool `json:"running"`
}

type tunnelStore struct {
	mu       sync.Mutex
	path     string
	stateDir string
	profiles []tunnelProfile
}

func newTunnelStore(path string) (*tunnelStore, error) {
	store := &tunnelStore{path: path, stateDir: filepath.Join(filepath.Dir(path), "run")}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &store.profiles); err != nil {
		return nil, fmt.Errorf("read tunnel profiles: %w", err)
	}
	for _, profile := range store.profiles {
		if err := validateTunnelProfile(profile); err != nil {
			return nil, fmt.Errorf("read tunnel profiles: %w", err)
		}
	}
	return store, nil
}

func (s *tunnelStore) save(profile tunnelProfile) error {
	if err := validateTunnelProfile(profile); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.profiles {
		if s.profiles[index].ID == profile.ID || (profile.ServerID != "" && s.profiles[index].ServerID == profile.ServerID) {
			profile.ID = s.profiles[index].ID
			s.profiles[index] = profile
			return s.write()
		}
	}
	s.profiles = append(s.profiles, profile)
	return s.write()
}

func (s *tunnelStore) all() []tunnelProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tunnelProfile(nil), s.profiles...)
}

func (s *tunnelStore) find(id string) (tunnelProfile, bool) {
	for _, profile := range s.all() {
		if profile.ID == id || profile.ServerID == id {
			return profile, true
		}
	}
	return tunnelProfile{}, false
}

func (s *tunnelStore) write() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}

func (s *tunnelStore) controlPath(id string) string {
	return filepath.Join(s.stateDir, id+".sock")
}

func validateTunnelProfile(profile tunnelProfile) error {
	if !safeID.MatchString(profile.ID) || (profile.ServerID != "" && !safeID.MatchString(profile.ServerID)) || profile.Name == "" || strings.ContainsAny(profile.Name, "\r\n") || !safeTarget.MatchString(profile.Target) {
		return errors.New("invalid tunnel profile")
	}
	for _, port := range []string{profile.Port, profile.LocalPort, profile.RemotePort} {
		if err := validatePort(port); err != nil {
			return errors.New("invalid tunnel profile")
		}
	}
	return nil
}

func tunnelRunning(profile tunnelProfile, controlPath string) bool {
	if _, err := os.Stat(controlPath); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ssh", "-S", controlPath, "-O", "check", "-p", profile.Port, profile.Target).Run() == nil
}

func stopTunnel(profile tunnelProfile, controlPath string) error {
	if !tunnelRunning(profile, controlPath) {
		_ = os.Remove(controlPath)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "ssh", "-S", controlPath, "-O", "exit", "-p", profile.Port, profile.Target).Run(); err != nil {
		return err
	}
	if err := os.Remove(controlPath); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func runTunnels(repoRoot string, store *tunnelStore, onlyIDs map[string]bool) error {
	script := filepath.Join(repoRoot, "deployments", "scripts", "reverse-tunnel-ssh.sh")
	if _, err := os.Stat(script); err != nil {
		return err
	}
	if err := os.MkdirAll(store.stateDir, 0o700); err != nil {
		return err
	}
	var failed int
	for _, profile := range store.all() {
		if len(onlyIDs) > 0 && !onlyIDs[profile.ID] && !onlyIDs[profile.ServerID] {
			continue
		}
		controlPath := store.controlPath(profile.ID)
		if tunnelRunning(profile, controlPath) {
			fmt.Printf("%s is already connected.\n", profile.Name)
			continue
		}
		fmt.Printf("Connecting %s (%s). Enter its SSH password when prompted.\n", profile.Name, profile.Target)
		cmd := exec.Command(script,
			"--target", profile.Target,
			"--port", profile.Port,
			"--local-port", profile.LocalPort,
			"--remote-port", profile.RemotePort,
			"--background",
			"--control-path", controlPath,
		)
		cmd.Dir = repoRoot
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "%s failed: %v\n", profile.Name, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d tunnel(s) failed", failed)
	}
	fmt.Println("All tunnels are running in background.")
	return nil
}
