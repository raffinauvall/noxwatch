package config

import "testing"

func TestValidateRejectsPlainHTTP(t *testing.T) {
	cfg := Config{Endpoint: "http://example.com", ServerName: "api-01", Environment: "production"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("plain HTTP accepted")
	}
	cfg.AllowInsecureHTTP = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("explicit local HTTP rejected: %v", err)
	}
}
