package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

const validPayload = `{"target":"deploy@192.0.2.10","port":"2326","endpoint":"http://127.0.0.1:18082","token":"nox_enroll_12345678901234567890","server_name":"API server","environment":"production"}`

func TestBootstrapLaunch(t *testing.T) {
	var launched bootstrapRequest
	handler := helper{
		origin: "http://localhost:3002",
		launch: func(input bootstrapRequest) error {
			launched = input
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
}

func TestBootstrapRejectsOtherOriginsAndUnsafeInput(t *testing.T) {
	handler := helper{origin: "http://localhost:3002", launch: func(bootstrapRequest) error { t.Fatal("launched unsafe request"); return nil }}
	tests := []struct {
		name    string
		origin  string
		payload string
		status  int
	}{
		{name: "origin", origin: "https://attacker.example", payload: validPayload, status: http.StatusForbidden},
		{name: "target", origin: "http://localhost:3002", payload: `{"target":"deploy@host;bad","port":"22","endpoint":"http://127.0.0.1:18082","token":"nox_enroll_12345678901234567890","server_name":"server","environment":"production"}`, status: http.StatusBadRequest},
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
