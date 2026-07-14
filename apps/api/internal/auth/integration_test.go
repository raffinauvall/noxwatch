package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionLifecycleIntegration(t *testing.T) {
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

	service := NewService(db, "01234567890123456789012345678901")
	email := fmt.Sprintf("auth-%d@example.test", time.Now().UnixNano())
	registered, err := service.Register(ctx, email, "a-secure-password", "Test User", "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id = $1`, registered.User.ID) //nolint:errcheck

	if _, err := service.Login(ctx, email, "wrong-password", "test", "127.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	refreshed, err := service.Refresh(ctx, registered.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, registered.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("rotated refresh token was reusable")
	}
	claims, err := service.ValidateAccess(ctx, refreshed.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(ctx, claims.SessionID, claims.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ValidateAccess(ctx, refreshed.AccessToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("revoked session remained valid")
	}
}
