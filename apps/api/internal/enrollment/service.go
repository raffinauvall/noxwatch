package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden          = errors.New("workspace access denied")
	ErrTokenInvalid       = errors.New("enrollment token invalid")
	ErrAgentUnauthorized  = errors.New("agent unauthorized")
	ErrEnrollmentNotFound = errors.New("enrollment not found")
)

const enrollmentLifetime = 15 * time.Minute

type Token struct {
	ID        string    `json:"id"`
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
	ServerID  string    `json:"server_id,omitempty"`
}

type EnrollInput struct {
	Token         string `json:"token"`
	Hostname      string `json:"hostname"`
	MachineID     string `json:"machine_id"`
	OS            string `json:"os"`
	OSVersion     string `json:"os_version"`
	KernelVersion string `json:"kernel_version"`
	Architecture  string `json:"architecture"`
	AgentVersion  string `json:"agent_version"`
}

type EnrollResult struct {
	ServerID         string `json:"server_id"`
	AgentID          string `json:"agent_id"`
	Credential       string `json:"credential"`
	HeartbeatSeconds int    `json:"heartbeat_interval_seconds"`
	MetricsSeconds   int    `json:"metrics_interval_seconds"`
}

type AgentIdentity struct {
	AgentID     string
	ServerID    string
	WorkspaceID string
}

type Service struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) CreateToken(ctx context.Context, userID, workspaceID, name, environment, description string, tags []string, ip string) (Token, error) {
	if tags == nil {
		tags = []string{}
	}
	var allowed bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2 AND role IN ('owner', 'admin'))`, workspaceID, userID).Scan(&allowed); err != nil {
		return Token{}, err
	}
	if !allowed {
		return Token{}, ErrForbidden
	}
	plain, err := randomToken("nox_enroll_")
	if err != nil {
		return Token{}, err
	}
	expires := s.now().UTC().Add(enrollmentLifetime)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Token{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var result Token
	err = tx.QueryRow(ctx, `INSERT INTO enrollment_tokens (workspace_id, token_hash, server_name, environment, description, tags, expires_at, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		workspaceID, hash(plain), name, environment, description, tags, expires, userID).Scan(&result.ID)
	if err != nil {
		return Token{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id, actor_user_id, action, target_type, target_id, ip_address) VALUES ($1,$2,'enrollment.create','enrollment_token',$3,NULLIF($4,'')::inet)`, workspaceID, userID, result.ID, ip); err != nil {
		return Token{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Token{}, err
	}
	result.Token, result.ExpiresAt, result.Status = plain, expires, "pending"
	return result, nil
}

func (s *Service) TokenStatus(ctx context.Context, userID, tokenID string) (Token, error) {
	var result Token
	var usedAt, revokedAt *time.Time
	err := s.db.QueryRow(ctx, `SELECT et.id, et.expires_at, et.used_at, et.revoked_at, COALESCE(et.server_id::text,''),
	  CASE WHEN et.used_at IS NOT NULL THEN 'connected' WHEN et.revoked_at IS NOT NULL THEN 'revoked' WHEN et.expires_at <= $3 THEN 'expired' ELSE 'pending' END
	  FROM enrollment_tokens et JOIN workspace_members wm ON wm.workspace_id = et.workspace_id WHERE et.id = $1 AND wm.user_id = $2`, tokenID, userID, s.now().UTC()).
		Scan(&result.ID, &result.ExpiresAt, &usedAt, &revokedAt, &result.ServerID, &result.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrEnrollmentNotFound
	}
	return result, err
}

func (s *Service) RevokeToken(ctx context.Context, userID, tokenID string) error {
	command, err := s.db.Exec(ctx, `UPDATE enrollment_tokens et SET revoked_at = now() FROM workspace_members wm WHERE et.id = $1 AND wm.workspace_id = et.workspace_id AND wm.user_id = $2 AND wm.role IN ('owner','admin') AND et.used_at IS NULL AND et.revoked_at IS NULL`, tokenID, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrEnrollmentNotFound
	}
	return nil
}

func (s *Service) Enroll(ctx context.Context, input EnrollInput) (EnrollResult, error) {
	if len(input.Token) < len("nox_enroll_")+20 {
		return EnrollResult{}, ErrTokenInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EnrollResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var tokenID, workspaceID, name, environment, description string
	var tags []string
	err = tx.QueryRow(ctx, `SELECT id, workspace_id, server_name, environment, description, tags FROM enrollment_tokens WHERE token_hash=$1 AND expires_at>$2 AND used_at IS NULL AND revoked_at IS NULL FOR UPDATE`, hash(input.Token), s.now().UTC()).
		Scan(&tokenID, &workspaceID, &name, &environment, &description, &tags)
	if errors.Is(err, pgx.ErrNoRows) {
		return EnrollResult{}, ErrTokenInvalid
	}
	if err != nil {
		return EnrollResult{}, err
	}
	credential, err := randomToken("nox_agent_")
	if err != nil {
		return EnrollResult{}, err
	}
	var result EnrollResult
	err = tx.QueryRow(ctx, `INSERT INTO servers (workspace_id,name,hostname,description,environment,operating_system,os_version,kernel_version,architecture,agent_version,enrolled_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		workspaceID, name, input.Hostname, description, environment, input.OS, input.OSVersion, input.KernelVersion, input.Architecture, input.AgentVersion, s.now().UTC()).Scan(&result.ServerID)
	if err != nil {
		return EnrollResult{}, err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO server_tags (server_id, tag) VALUES ($1,$2)`, result.ServerID, tag); err != nil {
			return EnrollResult{}, err
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO agents (server_id, credential_hash, machine_id_hash) VALUES ($1,$2,$3) RETURNING id`, result.ServerID, hash(credential), hash(input.MachineID)).Scan(&result.AgentID)
	if err != nil {
		return EnrollResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE enrollment_tokens SET used_at=$1, server_id=$2 WHERE id=$3`, s.now().UTC(), result.ServerID, tokenID); err != nil {
		return EnrollResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id, actor_agent_id, action, target_type, target_id) VALUES ($1,$2,'server.enroll','server',$3)`, workspaceID, result.AgentID, result.ServerID); err != nil {
		return EnrollResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EnrollResult{}, err
	}
	result.Credential, result.HeartbeatSeconds, result.MetricsSeconds = credential, 20, 45
	return result, nil
}

func (s *Service) Heartbeat(ctx context.Context, credential, serverID string) error {
	identity, err := s.AuthenticateAgent(ctx, credential, serverID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE servers s SET last_seen_at=$1,status=CASE
	 WHEN status='maintenance' THEN status
	 WHEN EXISTS (SELECT 1 FROM alert_events WHERE server_id=s.id AND state IN ('firing','acknowledged') AND severity='critical') THEN 'degraded'
	 WHEN EXISTS (SELECT 1 FROM alert_events WHERE server_id=s.id AND state IN ('firing','acknowledged') AND severity='warning') THEN 'warning'
	 ELSE 'online' END,updated_at=$1 WHERE id=$2`, s.now().UTC(), identity.ServerID)
	return err
}

func (s *Service) UnregisterAgent(ctx context.Context, credential, serverID string) error {
	identity, err := s.AuthenticateAgent(ctx, credential, serverID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE agents SET revoked_at=now() WHERE id=$1`, identity.AgentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET status='offline',updated_at=now() WHERE id=$1`, identity.ServerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_agent_id,action,target_type,target_id) VALUES ($1,$2,'agent.unregister','server',$3)`, identity.WorkspaceID, identity.AgentID, identity.ServerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AuthenticateAgent(ctx context.Context, credential, serverID string) (AgentIdentity, error) {
	var identity AgentIdentity
	err := s.db.QueryRow(ctx, `SELECT a.id, a.server_id, s.workspace_id FROM agents a JOIN servers s ON s.id=a.server_id WHERE a.credential_hash=$1 AND a.server_id=$2 AND a.revoked_at IS NULL`, hash(credential), serverID).
		Scan(&identity.AgentID, &identity.ServerID, &identity.WorkspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentIdentity{}, ErrAgentUnauthorized
	}
	return identity, err
}

func (s *Service) RevokeAgent(ctx context.Context, userID, serverID, ip string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	command, err := tx.Exec(ctx, `UPDATE agents a SET revoked_at=now() FROM servers s, workspace_members wm WHERE a.server_id=$1 AND s.id=a.server_id AND wm.workspace_id=s.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') AND a.revoked_at IS NULL`, serverID, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrForbidden
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id,ip_address) SELECT workspace_id,$2,'agent.revoke','server',id,NULLIF($3,'')::inet FROM servers WHERE id=$1`, serverID, userID, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) MarkOffline(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `UPDATE servers SET status='offline', updated_at=now() WHERE status NOT IN ('maintenance','unknown','offline') AND last_seen_at < now() - interval '2 minutes'`)
	return err
}

func randomToken(prefix string) (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
