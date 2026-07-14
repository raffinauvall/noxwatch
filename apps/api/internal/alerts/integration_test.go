package alerts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/notifications"
)

func TestAlertLifecycleWebhookAndIsolationIntegration(t *testing.T) {
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

	var mu sync.Mutex
	var bodies [][]byte
	var signatures []string
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		signatures = append(signatures, r.Header.Get("X-NoxWatch-Signature"))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	suffix := time.Now().UnixNano()
	var ownerID, outsiderID, workspaceID, serverID string
	for email, target := range map[string]*string{fmt.Sprintf("alerts-owner-%d@example.test", suffix): &ownerID, fmt.Sprintf("alerts-outsider-%d@example.test", suffix): &outsiderID} {
		if err := db.QueryRow(ctx, `INSERT INTO users (email,password_hash) VALUES ($1,'test-only') RETURNING id`, email).Scan(target); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, *target) //nolint:errcheck
	}
	if err := db.QueryRow(ctx, `INSERT INTO workspaces (name,slug,created_by) VALUES ('Alert Test',$1,$2) RETURNING id`, fmt.Sprintf("alerts-%d", suffix), ownerID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceID) //nolint:errcheck
	if _, err := db.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceID, ownerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO servers (workspace_id,name,hostname,environment) VALUES ($1,'api-01','api-01','testing') RETURNING id`, workspaceID).Scan(&serverID); err != nil {
		t.Fatal(err)
	}

	notifier := notifications.NewService(db, "01234567890123456789012345678901", "https://app.example.test", true)
	channel, err := notifier.Create(ctx, ownerID, workspaceID, "Operations", webhook.URL, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, notifier)
	warning, critical := 80.0, 90.0
	rule, err := service.Create(ctx, ownerID, workspaceID, serverID, "High CPU", "cpu_usage", &warning, &critical, 60, 300, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, outsiderID, workspaceID, serverID, "Forbidden", "cpu_usage", &warning, &critical, 0, 0, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider create returned %v", err)
	}

	t0 := time.Now().UTC().Add(-10 * time.Minute)
	for _, at := range []time.Time{t0, t0.Add(30 * time.Second), t0.Add(61 * time.Second)} {
		if err := service.EvaluateMetrics(ctx, serverID, at, Values{CPU: 95}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := service.Events(ctx, ownerID, serverID)
	if err != nil || len(events) != 1 || events[0].State != "firing" || events[0].Severity != "critical" {
		t.Fatalf("firing events=%+v err=%v", events, err)
	}
	if err := service.EvaluateMetrics(ctx, serverID, t0.Add(70*time.Second), Values{CPU: 10}); err != nil {
		t.Fatal(err)
	}
	if err := service.EvaluateMetrics(ctx, serverID, t0.Add(100*time.Second), Values{CPU: 95}); err != nil {
		t.Fatal(err)
	}
	if err := service.EvaluateMetrics(ctx, serverID, t0.Add(161*time.Second), Values{CPU: 95}); err != nil {
		t.Fatal(err)
	}

	events, err = service.Events(ctx, ownerID, serverID)
	if err != nil || len(events) != 2 || events[0].State != "firing" || events[1].State != "resolved" {
		t.Fatalf("lifecycle events=%+v err=%v", events, err)
	}
	outsiderEvents, err := service.Events(ctx, outsiderID, serverID)
	if err != nil || len(outsiderEvents) != 0 {
		t.Fatalf("outsider events=%+v err=%v", outsiderEvents, err)
	}
	workspaceEvents, err := service.WorkspaceEvents(ctx, ownerID, workspaceID)
	if err != nil || len(workspaceEvents) != 2 {
		t.Fatalf("workspace events=%+v err=%v", workspaceEvents, err)
	}
	if outsiderEvents, err := service.WorkspaceEvents(ctx, outsiderID, workspaceID); err != nil || len(outsiderEvents) != 0 {
		t.Fatalf("outsider workspace events=%+v err=%v", outsiderEvents, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("webhook deliveries=%d, want firing and resolved only", len(bodies))
	}
	for index, body := range bodies {
		mac := hmac.New(sha256.New, []byte(channel.Secret))
		_, _ = mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(signatures[index]), []byte(want)) {
			t.Fatalf("invalid webhook signature %q", signatures[index])
		}
	}
	if rules, err := service.List(ctx, outsiderID, workspaceID); err != nil || len(rules) != 0 {
		t.Fatalf("outsider rules=%+v err=%v", rules, err)
	}
	_ = rule
}
