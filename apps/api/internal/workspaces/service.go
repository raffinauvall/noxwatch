package workspaces

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("workspace not found")

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) List(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.db.Query(ctx, `SELECT w.id, w.name, w.slug, wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id = w.id WHERE wm.user_id = $1 ORDER BY w.created_at LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Workspace{}
	for rows.Next() {
		var workspace Workspace
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.Role); err != nil {
			return nil, err
		}
		result = append(result, workspace)
	}
	return result, rows.Err()
}

func (s *Service) Create(ctx context.Context, userID, name, ip string) (Workspace, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	workspace := Workspace{Name: strings.TrimSpace(name), Slug: slug(strings.TrimSpace(name)) + "-" + randomSuffix()}
	err = tx.QueryRow(ctx, `INSERT INTO workspaces (name, slug, created_by) VALUES ($1, $2, $3) RETURNING id`, workspace.Name, workspace.Slug, userID).Scan(&workspace.ID)
	if err != nil {
		return Workspace{}, err
	}
	workspace.Role = "owner"
	if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, workspace.ID, userID); err != nil {
		return Workspace{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id, actor_user_id, action, target_type, target_id, ip_address) VALUES ($1, $2, 'workspace.create', 'workspace', $1, NULLIF($3, '')::inet)`, workspace.ID, userID, ip); err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (s *Service) Get(ctx context.Context, userID, workspaceID string) (Workspace, error) {
	var workspace Workspace
	err := s.db.QueryRow(ctx, `SELECT w.id, w.name, w.slug, wm.role FROM workspaces w JOIN workspace_members wm ON wm.workspace_id = w.id WHERE w.id = $1 AND wm.user_id = $2`, workspaceID, userID).
		Scan(&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	return workspace, err
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
		} else if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "workspace"
	}
	return result
}

func randomSuffix() string {
	var value [3]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "local"
	}
	return hex.EncodeToString(value[:])
}
