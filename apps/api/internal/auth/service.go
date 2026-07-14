package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailExists        = errors.New("unable to create account")
	ErrInvalidSession     = errors.New("invalid session")
)

const (
	accessLifetime  = 15 * time.Minute
	refreshLifetime = 30 * 24 * time.Hour
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Result struct {
	User         User   `json:"user"`
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"-"`
}

type Service struct {
	db     *pgxpool.Pool
	secret []byte
	now    func() time.Time
}

func NewService(db *pgxpool.Pool, secret string) *Service {
	return &Service{db: db, secret: []byte(secret), now: time.Now}
}

func (s *Service) Register(ctx context.Context, email, password, name, userAgent, ip string) (Result, error) {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return Result{}, fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var user User
	err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id, email, name`,
		strings.ToLower(strings.TrimSpace(email)), passwordHash, strings.TrimSpace(name)).Scan(&user.ID, &user.Email, &user.Name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Result{}, ErrEmailExists
		}
		return Result{}, err
	}
	result, err := s.createSession(ctx, tx, user, userAgent, ip)
	if err != nil {
		return Result{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, ip_address) VALUES ($1, 'auth.register', 'user', $1, NULLIF($2, '')::inet)`, user.ID, ip)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (Result, error) {
	var user User
	var passwordHash string
	err := s.db.QueryRow(ctx, `SELECT id, email, name, password_hash FROM users WHERE email_normalized = lower($1)`, strings.TrimSpace(email)).Scan(&user.ID, &user.Email, &user.Name, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		verifyPassword(dummyPasswordHash, password)
		_, _ = s.db.Exec(ctx, `INSERT INTO audit_logs (action, target_type, ip_address) VALUES ('auth.login.failed', 'user', NULLIF($1, '')::inet)`, ip)
		return Result{}, ErrInvalidCredentials
	}
	if err == nil && !verifyPassword(passwordHash, password) {
		_, _ = s.db.Exec(ctx, `INSERT INTO audit_logs (action, target_type, ip_address) VALUES ('auth.login.failed', 'user', NULLIF($1, '')::inet)`, ip)
		return Result{}, ErrInvalidCredentials
	}
	if err != nil {
		return Result{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result, err := s.createSession(ctx, tx, user, userAgent, ip)
	if err != nil {
		return Result{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, ip_address) VALUES ($1, 'auth.login', 'user', $1, NULLIF($2, '')::inet)`, user.ID, ip)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, user User, userAgent, ip string) (Result, error) {
	refresh, err := newOpaqueToken("nox_refresh_")
	if err != nil {
		return Result{}, err
	}
	now := s.now().UTC()
	var sessionID string
	err = tx.QueryRow(ctx, `INSERT INTO sessions (user_id, refresh_token_hash, user_agent, ip_address, expires_at) VALUES ($1, $2, $3, NULLIF($4, '')::inet, $5) RETURNING id`,
		user.ID, tokenHash(refresh), userAgent, ip, now.Add(refreshLifetime)).Scan(&sessionID)
	if err != nil {
		return Result{}, err
	}
	return s.result(user, sessionID, refresh, now), nil
}

func (s *Service) Refresh(ctx context.Context, refresh string) (Result, error) {
	if !strings.HasPrefix(refresh, "nox_refresh_") {
		return Result{}, ErrInvalidSession
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var user User
	var sessionID string
	err = tx.QueryRow(ctx, `SELECT u.id, u.email, u.name, s.id FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.refresh_token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2 FOR UPDATE`,
		tokenHash(refresh), s.now().UTC()).Scan(&user.ID, &user.Email, &user.Name, &sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrInvalidSession
	}
	if err != nil {
		return Result{}, err
	}
	rotated, err := newOpaqueToken("nox_refresh_")
	if err != nil {
		return Result{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET refresh_token_hash = $1 WHERE id = $2`, tokenHash(rotated), sessionID); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return s.result(user, sessionID, rotated, s.now().UTC()), nil
}

func (s *Service) Revoke(ctx context.Context, sessionID, userID string) error {
	command, err := s.db.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrInvalidSession
	}
	return nil
}

func (s *Service) ValidateAccess(ctx context.Context, token string) (AccessClaims, error) {
	claims, err := parseAccess(s.secret, token, s.now().UTC())
	if err != nil {
		return AccessClaims{}, ErrInvalidSession
	}
	var active bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > now())`, claims.SessionID, claims.UserID).Scan(&active)
	if err != nil || !active {
		return AccessClaims{}, ErrInvalidSession
	}
	return claims, nil
}

func (s *Service) result(user User, sessionID, refresh string, now time.Time) Result {
	expires := now.Add(accessLifetime)
	return Result{
		User: user, AccessToken: signAccess(s.secret, AccessClaims{UserID: user.ID, SessionID: sessionID, ExpiresAt: expires}),
		ExpiresIn: int64(accessLifetime.Seconds()), RefreshToken: refresh,
	}
}
