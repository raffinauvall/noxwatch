package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	want := Credentials{ServerID: "server", AgentID: "agent", Credential: "secret", HeartbeatSeconds: 20, MetricsSeconds: 45}
	if err := SaveCredentials(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	got, err := LoadCredentials(path)
	if err != nil || got != want {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
