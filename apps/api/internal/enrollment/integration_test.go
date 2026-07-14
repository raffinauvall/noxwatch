package enrollment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEnrollmentLifecycleIntegration(t *testing.T) {
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

	var userID, workspaceID string
	email := fmt.Sprintf("enroll-%d@example.test", time.Now().UnixNano())
	if err := db.QueryRow(ctx, `INSERT INTO users (email,password_hash) VALUES ($1,'test-only') RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID) //nolint:errcheck
	if err := db.QueryRow(ctx, `INSERT INTO workspaces (name,slug,created_by) VALUES ('Enrollment Test',$1,$2) RETURNING id`, fmt.Sprintf("enroll-%d", time.Now().UnixNano()), userID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workspaces WHERE id=$1`, workspaceID) //nolint:errcheck
	if _, err := db.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner')`, workspaceID, userID); err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	token, err := service.CreateToken(ctx, userID, workspaceID, "api-01", "production", "", []string{"api"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	input := EnrollInput{Token: token.Token, Hostname: "api-01", MachineID: "machine-01", OS: "linux", OSVersion: "test", Architecture: "amd64", AgentVersion: "0.1.0"}
	enrolled, err := service.Enroll(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Enroll(ctx, input); !errors.Is(err, ErrTokenInvalid) {
		t.Fatal("one-time token was reusable")
	}
	if err := service.Heartbeat(ctx, enrolled.Credential, enrolled.ServerID); err != nil {
		t.Fatal(err)
	}
	if err := service.Heartbeat(ctx, enrolled.Credential, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrAgentUnauthorized) {
		t.Fatal("credential accepted for another server")
	}
	if err := service.RevokeAgent(ctx, userID, enrolled.ServerID, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Heartbeat(ctx, enrolled.Credential, enrolled.ServerID); !errors.Is(err, ErrAgentUnauthorized) {
		t.Fatal("revoked credential remained valid")
	}

	expired, err := service.CreateToken(ctx, userID, workspaceID, "old-01", "testing", "", nil, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE enrollment_tokens SET expires_at=now()-interval '1 second' WHERE id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	input.Token = expired.Token
	if _, err := service.Enroll(ctx, input); !errors.Is(err, ErrTokenInvalid) {
		t.Fatal("expired token was accepted")
	}
}
