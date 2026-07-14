package servers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("server not found")

type Server struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspace_id"`
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	Description     string     `json:"description"`
	Environment     string     `json:"environment"`
	OperatingSystem string     `json:"operating_system"`
	OSVersion       string     `json:"os_version"`
	KernelVersion   string     `json:"kernel_version"`
	Architecture    string     `json:"architecture"`
	AgentVersion    string     `json:"agent_version"`
	Status          string     `json:"status"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	EnrolledAt      *time.Time `json:"enrolled_at"`
	Tags            []string   `json:"tags"`
	AgentRevoked    bool       `json:"agent_revoked"`
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context, userID, workspaceID string, limit, offset int) ([]Server, error) {
	rows, err := s.db.Query(ctx, `SELECT s.id,s.workspace_id,s.name,s.hostname,s.description,s.environment,s.operating_system,s.os_version,s.kernel_version,s.architecture,s.agent_version,s.status,s.last_seen_at,s.enrolled_at,
	  COALESCE(array_agg(st.tag) FILTER (WHERE st.tag IS NOT NULL),'{}'), a.revoked_at IS NOT NULL
	  FROM servers s JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 JOIN agents a ON a.server_id=s.id LEFT JOIN server_tags st ON st.server_id=s.id
	  WHERE s.workspace_id=$2 GROUP BY s.id,a.revoked_at ORDER BY s.created_at DESC LIMIT $3 OFFSET $4`, userID, workspaceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Server{}
	for rows.Next() {
		server, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, server)
	}
	return result, rows.Err()
}

func (s *Service) Get(ctx context.Context, userID, serverID string) (Server, error) {
	row := s.db.QueryRow(ctx, `SELECT s.id,s.workspace_id,s.name,s.hostname,s.description,s.environment,s.operating_system,s.os_version,s.kernel_version,s.architecture,s.agent_version,s.status,s.last_seen_at,s.enrolled_at,
	  COALESCE(array_agg(st.tag) FILTER (WHERE st.tag IS NOT NULL),'{}'), a.revoked_at IS NOT NULL
	  FROM servers s JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 JOIN agents a ON a.server_id=s.id LEFT JOIN server_tags st ON st.server_id=s.id
	  WHERE s.id=$2 GROUP BY s.id,a.revoked_at`, userID, serverID)
	server, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return server, err
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Server, error) {
	var server Server
	err := row.Scan(&server.ID, &server.WorkspaceID, &server.Name, &server.Hostname, &server.Description, &server.Environment,
		&server.OperatingSystem, &server.OSVersion, &server.KernelVersion, &server.Architecture, &server.AgentVersion, &server.Status,
		&server.LastSeenAt, &server.EnrolledAt, &server.Tags, &server.AgentRevoked)
	return server, err
}
