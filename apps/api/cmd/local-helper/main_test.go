package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

const validPayload = `{"profile_id":"12345678-abcd","target":"deploy@192.0.2.10","port":"2326","endpoint":"http://127.0.0.1:18082","token":"nox_enroll_12345678901234567890","server_name":"API server","environment":"production"}`

func TestBootstrapLaunch(t *testing.T) {
	var launched bootstrapRequest
	store, err := newTunnelStore(t.TempDir() + "/tunnels.json")
	if err != nil {
		t.Fatal(err)
	}
	handler := helper{
		origin:       "http://localhost:3002",
		localAPIPort: "8082",
		store:        store,
		launch: func(input bootstrapRequest, controlPath string) error {
			launched = input
			if controlPath == "" {
				t.Fatal("missing managed control path")
			}
			return nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/bootstrap", bytes.NewBufferString(validPayload))
	request.Header.Set("Origin", "http://localhost:3002")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if launched.Target != "deploy@192.0.2.10" || launched.Endpoint != "http://127.0.0.1:18082" {
		t.Fatalf("unexpected launch: %+v", launched)
	}
	profiles := store.all()
	if len(profiles) != 1 || profiles[0].RemotePort != "18082" || profiles[0].LocalPort != "8082" {
		t.Fatalf("unexpected profiles: %+v", profiles)
	}
}

func TestBootstrapRejectsOtherOriginsAndUnsafeInput(t *testing.T) {
	store, err := newTunnelStore(t.TempDir() + "/tunnels.json")
	if err != nil {
		t.Fatal(err)
	}
	handler := helper{origin: "http://localhost:3002", store: store, launch: func(bootstrapRequest, string) error { t.Fatal("launched unsafe request"); return nil }}
	tests := []struct {
		name    string
		origin  string
		payload string
		status  int
	}{
		{name: "origin", origin: "https://attacker.example", payload: validPayload, status: http.StatusForbidden},
		{name: "target", origin: "http://localhost:3002", payload: `{"profile_id":"12345678-abcd","target":"deploy@host;bad","port":"22","endpoint":"http://127.0.0.1:18082","token":"nox_enroll_12345678901234567890","server_name":"server","environment":"production"}`, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/bootstrap", bytes.NewBufferString(test.payload))
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestReverseTunnelNormalizesLoopback(t *testing.T) {
	endpoint, port, ok := reverseTunnel("http://localhost:18082")
	if !ok || endpoint != "http://127.0.0.1:18082" || port != "18082" {
		t.Fatalf("endpoint = %q, port = %q, reverse = %v", endpoint, port, ok)
	}
	if endpoint, _, ok := reverseTunnel("https://api.example.com"); ok || endpoint != "https://api.example.com" {
		t.Fatalf("public endpoint unexpectedly enabled reverse tunnel: %q", endpoint)
	}
}

func TestStartAllLaunchesConfiguredProfiles(t *testing.T) {
	store, err := newTunnelStore(t.TempDir() + "/tunnels.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.save(tunnelProfile{ID: "12345678-abcd", Name: "api", Target: "deploy@192.0.2.10", Port: "22", LocalPort: "8082", RemotePort: "18082"}); err != nil {
		t.Fatal(err)
	}
	launched := false
	var ids []string
	handler := helper{origin: "http://localhost:3002", store: store, launchTunnels: func(selected []string) error { launched = true; ids = selected; return nil }}
	request := httptest.NewRequest(http.MethodPost, "/tunnels/start-all", bytes.NewBufferString(`{"ids":["12345678-abcd"]}`))
	request.Header.Set("Origin", "http://localhost:3002")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !launched || len(ids) != 1 || ids[0] != "12345678-abcd" {
		t.Fatalf("status = %d, launched = %v, ids = %v", response.Code, launched, ids)
	}
}

func TestRegisterAndStartExistingServerTunnel(t *testing.T) {
	store, err := newTunnelStore(t.TempDir() + "/tunnels.json")
	if err != nil {
		t.Fatal(err)
	}
	var launched string
	handler := helper{
		origin:       "http://localhost:3002",
		localAPIPort: "8082",
		store:        store,
		launchTunnel: func(id string) error { launched = id; return nil },
	}
	register := httptest.NewRequest(http.MethodPost, "/tunnels/register", bytes.NewBufferString(`{"id":"profile-1234","server_id":"server-12345","name":"api","target":"deploy@192.0.2.10","port":"22","remote_port":"18082"}`))
	register.Header.Set("Origin", "http://localhost:3002")
	registerResponse := httptest.NewRecorder()
	handler.ServeHTTP(registerResponse, register)
	if registerResponse.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", registerResponse.Code, registerResponse.Body.String())
	}
	start := httptest.NewRequest(http.MethodPost, "/tunnels/start", bytes.NewBufferString(`{"id":"server-12345"}`))
	start.Header.Set("Origin", "http://localhost:3002")
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, start)
	if startResponse.Code != http.StatusAccepted || launched != "profile-1234" {
		t.Fatalf("start status = %d, launched = %q", startResponse.Code, launched)
	}
}

func TestOpenTerminalUsesRegisteredProfile(t *testing.T) {
	store, err := newTunnelStore(t.TempDir() + "/tunnels.json")
	if err != nil {
		t.Fatal(err)
	}
	profile := tunnelProfile{ID: "profile-1234", ServerID: "server-12345", Name: "api", Target: "deploy@192.0.2.10", Port: "22", LocalPort: "8082", RemotePort: "18082"}
	if err := store.save(profile); err != nil {
		t.Fatal(err)
	}
	var launched tunnelProfile
	handler := helper{origin: "http://localhost:3002", store: store, launchShell: func(profile tunnelProfile, _ string) error { launched = profile; return nil }}
	request := httptest.NewRequest(http.MethodPost, "/terminal", bytes.NewBufferString(`{"id":"server-12345"}`))
	request.Header.Set("Origin", "http://localhost:3002")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || launched.Target != profile.Target {
		t.Fatalf("status = %d, launched = %+v", response.Code, launched)
	}
}
