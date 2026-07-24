package servers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServerLifecycleAndIsolationIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	suffix := time.Now().UnixNano()
	var ownerID, outsiderID, workspaceID, serverID string
	for email, target := range map[string]*string{fmt.Sprintf("servers-owner-%d@example.test", suffix): &ownerID, fmt.Sprintf("servers-outsider-%d@example.test", suffix): &outsiderID} {
		if err := db.QueryRow(ctx, `INSERT INTO users (email,password_hash) VALUES ($1,'test-only') RETURNING id`, email).Scan(target); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, *target) //nolint:errcheck
	}
	if err := db.QueryRow(ctx, `INSERT INTO workspaces (name,slug,created_by) VALUES ('Servers Test',$1,$2) RETURNING id`, fmt.Sprintf("servers-%d", suffix), ownerID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceID) //nolint:errcheck
	if _, err := db.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO servers (workspace_id,name,hostname,environment,status,last_seen_at) VALUES ($1,'api-01','api-01','testing','online',now()) RETURNING id`, workspaceID).Scan(&serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO agents (server_id,credential_hash) VALUES ($1,'test-only')`, serverID); err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	tunneled, err := service.SaveTunnel(ctx, ownerID, serverID, "deploy", "192.0.2.10", 2326, 18082, "127.0.0.1")
	if err != nil || tunneled.SSHUser != "deploy" || tunneled.SSHHost != "192.0.2.10" || tunneled.SSHPort == nil || *tunneled.SSHPort != 2326 {
		t.Fatalf("tunneled=%+v err=%v", tunneled, err)
	}
	if _, err := service.SaveTunnel(ctx, outsiderID, serverID, "deploy", "192.0.2.10", 22, 18082, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider tunnel update returned %v", err)
	}
	disconnected, err := service.Disconnect(ctx, ownerID, serverID, "127.0.0.1")
	if err != nil || disconnected.Status != "offline" {
		t.Fatalf("disconnected=%+v err=%v", disconnected, err)
	}
	if _, err := service.Disconnect(ctx, outsiderID, serverID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider disconnect returned %v", err)
	}
	name, environment, maintenance := "renamed-api", "production", true
	tags := []string{"role:api", "region:sg"}
	updated, err := service.Update(ctx, ownerID, serverID, UpdateInput{Name: &name, Environment: &environment, Maintenance: &maintenance, Tags: &tags}, "127.0.0.1")
	if err != nil || updated.Name != name || updated.Status != "maintenance" || len(updated.Tags) != 2 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err := service.Update(ctx, outsiderID, serverID, UpdateInput{Name: &name}, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider update returned %v", err)
	}
	listed, err := service.List(ctx, ownerID, workspaceID, ListOptions{Limit: 10, Search: "renamed", Status: "maintenance", Environment: "production", Tag: "role:api", Sort: "name"})
	if err != nil || len(listed) != 1 || listed[0].ID != serverID {
		t.Fatalf("filtered servers=%+v err=%v", listed, err)
	}
	statuses, err := service.Statuses(ctx, ownerID, workspaceID)
	if err != nil || len(statuses) != 1 || statuses[0].Status != "maintenance" {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	if statuses, err := service.Statuses(ctx, outsiderID, workspaceID); err != nil || len(statuses) != 0 {
		t.Fatalf("outsider statuses=%+v err=%v", statuses, err)
	}
	if _, err := db.Exec(ctx, `UPDATE servers SET status='online',last_seen_at=now()-interval '3 minutes' WHERE id=$1`, serverID); err != nil {
		t.Fatal(err)
	}
	stale, err := service.Get(ctx, ownerID, serverID)
	if err != nil || stale.Status != "offline" {
		t.Fatalf("stale server=%+v err=%v", stale, err)
	}
	listed, err = service.List(ctx, ownerID, workspaceID, ListOptions{Limit: 10, Status: "offline"})
	if err != nil || len(listed) != 1 || listed[0].ID != serverID {
		t.Fatalf("stale filtered servers=%+v err=%v", listed, err)
	}
	if err := service.Delete(ctx, outsiderID, serverID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider delete returned %v", err)
	}
	if err := service.Delete(ctx, ownerID, serverID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, ownerID, serverID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted server returned %v", err)
	}
}
