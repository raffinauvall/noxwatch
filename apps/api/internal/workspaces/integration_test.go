package workspaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkspaceIsolationIntegration(t *testing.T) {
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

	var ownerID, outsiderID string
	suffix := time.Now().UnixNano()
	for email, target := range map[string]*string{
		fmt.Sprintf("owner-%d@example.test", suffix):    &ownerID,
		fmt.Sprintf("outsider-%d@example.test", suffix): &outsiderID,
	} {
		if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'test-only') RETURNING id`, email).Scan(target); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(ctx, `DELETE FROM users WHERE id = $1`, *target) //nolint:errcheck
	}

	service := NewService(db)
	workspace, err := service.Create(ctx, ownerID, "Private Operations", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, ownerID, workspace.ID); err != nil {
		t.Fatalf("owner cannot read workspace: %v", err)
	}
	if _, err := service.Get(ctx, outsiderID, workspace.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace read returned %v", err)
	}
}
