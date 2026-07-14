package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/enrollment"
	"github.com/raffinauvall/noxwatch/apps/api/internal/servers"
)

func TestIngestionIdempotencyAndIsolationIntegration(t *testing.T) {
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

	var ownerID, outsiderID, workspaceID string
	suffix := time.Now().UnixNano()
	for email, target := range map[string]*string{fmt.Sprintf("metrics-owner-%d@example.test", suffix): &ownerID, fmt.Sprintf("metrics-outsider-%d@example.test", suffix): &outsiderID} {
		if err := db.QueryRow(ctx, `INSERT INTO users (email,password_hash) VALUES ($1,'test-only') RETURNING id`, email).Scan(target); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, *target) //nolint:errcheck
	}
	if err := db.QueryRow(ctx, `INSERT INTO workspaces (name,slug,created_by) VALUES ('Metrics Test',$1,$2) RETURNING id`, fmt.Sprintf("metrics-%d", suffix), ownerID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceID) //nolint:errcheck
	if _, err := db.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceID, ownerID); err != nil {
		t.Fatal(err)
	}

	enrollmentService := enrollment.NewService(db)
	token, err := enrollmentService.CreateToken(ctx, ownerID, workspaceID, "metrics-01", "testing", "", nil, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := enrollmentService.Enroll(ctx, enrollment.EnrollInput{Token: token.Token, Hostname: "metrics-01", MachineID: "machine", OS: "linux", Architecture: "amd64", AgentVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, enrollmentService)
	payload := Payload{ServerID: agent.ServerID, Sequence: 1, CollectedAt: time.Now().UTC(), CPU: CPUMetrics{UsagePercent: 22}, Memory: MemoryMetrics{TotalBytes: 100, UsedBytes: 50, AvailableBytes: 50, UsagePercent: 50}, Disks: []DiskMetrics{{MountPoint: "/", Filesystem: "ext4", TotalBytes: 100, UsedBytes: 25, AvailableBytes: 75, UsagePercent: 25}}}
	duplicate, err := service.Ingest(ctx, agent.Credential, payload)
	if err != nil || duplicate {
		t.Fatalf("first ingest duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = service.Ingest(ctx, agent.Credential, payload)
	if err != nil || !duplicate {
		t.Fatalf("replay duplicate=%v err=%v", duplicate, err)
	}
	latest, err := service.Latest(ctx, ownerID, agent.ServerID)
	if err != nil || latest.CPUUsagePercent != 22 || len(latest.Disks) != 1 {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	server, err := servers.NewService(db).Get(ctx, ownerID, agent.ServerID)
	if err != nil || server.CPUUsagePercent == nil || *server.CPUUsagePercent != 22 || server.DiskUsagePercent == nil || *server.DiskUsagePercent != 25 {
		t.Fatalf("server snapshot=%+v err=%v", server, err)
	}
	if _, err := service.Latest(ctx, outsiderID, agent.ServerID); !errors.Is(err, ErrNotFound) {
		t.Fatal("outsider could read server metrics")
	}
}
