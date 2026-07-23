package main

import (
	"os"
	"testing"
)

func TestTunnelStorePersistsProfileUpdates(t *testing.T) {
	path := t.TempDir() + "/tunnels.json"
	store, err := newTunnelStore(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := tunnelProfile{ID: "12345678-abcd", Name: "api", Target: "deploy@192.0.2.10", Port: "22", LocalPort: "8082", RemotePort: "18082"}
	if err := store.save(profile); err != nil {
		t.Fatal(err)
	}
	profile.Name = "renamed"
	if err := store.save(profile); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newTunnelStore(path)
	if err != nil {
		t.Fatal(err)
	}
	profiles := reloaded.all()
	if len(profiles) != 1 || profiles[0].Name != "renamed" {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestTunnelStoreRejectsUnsafeProfile(t *testing.T) {
	path := t.TempDir() + "/tunnels.json"
	if err := os.WriteFile(path, []byte(`[{"id":"../bad","name":"api","target":"-oProxyCommand=bad@host","port":"22","local_port":"8082","remote_port":"18082"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newTunnelStore(path); err == nil {
		t.Fatal("unsafe tunnel profile was accepted")
	}
}
