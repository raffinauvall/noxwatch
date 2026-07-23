package servers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("server not found")

type UpdateInput struct {
	Name        *string
	Description *string
	Environment *string
	Tags        *[]string
	Maintenance *bool
}

type Status struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type ListOptions struct {
	Limit, Offset                          int
	Search, Status, Environment, Tag, Sort string
}

type Server struct {
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspace_id"`
	Name               string     `json:"name"`
	Hostname           string     `json:"hostname"`
	Description        string     `json:"description"`
	Environment        string     `json:"environment"`
	OperatingSystem    string     `json:"operating_system"`
	OSVersion          string     `json:"os_version"`
	KernelVersion      string     `json:"kernel_version"`
	Architecture       string     `json:"architecture"`
	AgentVersion       string     `json:"agent_version"`
	Status             string     `json:"status"`
	LastSeenAt         *time.Time `json:"last_seen_at"`
	EnrolledAt         *time.Time `json:"enrolled_at"`
	Tags               []string   `json:"tags"`
	AgentRevoked       bool       `json:"agent_revoked"`
	CPUUsagePercent    *float64   `json:"cpu_usage_percent"`
	MemoryUsagePercent *float64   `json:"memory_usage_percent"`
	DiskUsagePercent   *float64   `json:"disk_usage_percent"`
	UptimeSeconds      *int64     `json:"uptime_seconds"`
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context, userID, workspaceID string, options ListOptions) ([]Server, error) {
	rows, err := s.db.Query(ctx, `SELECT s.id,s.workspace_id,s.name,s.hostname,s.description,s.environment,s.operating_system,s.os_version,s.kernel_version,s.architecture,s.agent_version,
	  CASE WHEN s.status IN ('maintenance','unknown') THEN s.status WHEN s.last_seen_at IS NULL OR s.last_seen_at < now()-interval '2 minutes' THEN 'offline' ELSE s.status END,s.last_seen_at,s.enrolled_at,
	  COALESCE((SELECT array_agg(tag ORDER BY tag) FROM server_tags WHERE server_id=s.id),'{}'),a.revoked_at IS NOT NULL,latest.cpu_usage_percent,latest.memory_usage_percent,latest.disk_usage_percent,latest.uptime_seconds
	  FROM servers s JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 JOIN agents a ON a.server_id=s.id
	  LEFT JOIN LATERAL (SELECT ms.cpu_usage_percent,ms.memory_usage_percent,ms.uptime_seconds,(SELECT max(d.usage_percent) FROM disk_metric_samples d WHERE d.metric_sample_id=ms.id) disk_usage_percent FROM metric_samples ms WHERE ms.server_id=s.id ORDER BY ms.collected_at DESC LIMIT 1) latest ON true
	  WHERE s.workspace_id=$2
	    AND ($5='' OR s.name ILIKE '%'||$5||'%' OR s.hostname ILIKE '%'||$5||'%')
	    AND ($6='' OR CASE WHEN s.status IN ('maintenance','unknown') THEN s.status WHEN s.last_seen_at IS NULL OR s.last_seen_at < now()-interval '2 minutes' THEN 'offline' ELSE s.status END=$6) AND ($7='' OR s.environment=$7)
	    AND ($8='' OR EXISTS (SELECT 1 FROM server_tags st WHERE st.server_id=s.id AND st.tag=$8))
	  ORDER BY CASE WHEN $9='name' THEN s.name END,CASE WHEN $9='status' THEN CASE WHEN s.status IN ('maintenance','unknown') THEN s.status WHEN s.last_seen_at IS NULL OR s.last_seen_at < now()-interval '2 minutes' THEN 'offline' ELSE s.status END END,s.created_at DESC LIMIT $3 OFFSET $4`, userID, workspaceID, options.Limit, options.Offset, options.Search, options.Status, options.Environment, options.Tag, options.Sort)
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
	row := s.db.QueryRow(ctx, `SELECT s.id,s.workspace_id,s.name,s.hostname,s.description,s.environment,s.operating_system,s.os_version,s.kernel_version,s.architecture,s.agent_version,
	  CASE WHEN s.status IN ('maintenance','unknown') THEN s.status WHEN s.last_seen_at IS NULL OR s.last_seen_at < now()-interval '2 minutes' THEN 'offline' ELSE s.status END,s.last_seen_at,s.enrolled_at,
	  COALESCE((SELECT array_agg(tag ORDER BY tag) FROM server_tags WHERE server_id=s.id),'{}'),a.revoked_at IS NOT NULL,latest.cpu_usage_percent,latest.memory_usage_percent,latest.disk_usage_percent,latest.uptime_seconds
	  FROM servers s JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 JOIN agents a ON a.server_id=s.id
	  LEFT JOIN LATERAL (SELECT ms.cpu_usage_percent,ms.memory_usage_percent,ms.uptime_seconds,(SELECT max(d.usage_percent) FROM disk_metric_samples d WHERE d.metric_sample_id=ms.id) disk_usage_percent FROM metric_samples ms WHERE ms.server_id=s.id ORDER BY ms.collected_at DESC LIMIT 1) latest ON true
	  WHERE s.id=$2`, userID, serverID)
	server, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return server, err
}

func (s *Service) Update(ctx context.Context, userID, serverID string, input UpdateInput, ip string) (Server, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Server{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var workspaceID string
	err = tx.QueryRow(ctx, `UPDATE servers s SET name=COALESCE($3,name),description=COALESCE($4,description),environment=COALESCE($5,environment),status=CASE WHEN $6::boolean IS NULL THEN status WHEN $6 THEN 'maintenance' WHEN last_seen_at > now()-interval '2 minutes' THEN 'online' ELSE 'offline' END,updated_at=now()
	 FROM workspace_members wm WHERE s.id=$1 AND wm.workspace_id=s.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') RETURNING s.workspace_id`, serverID, userID, input.Name, input.Description, input.Environment, input.Maintenance).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	if err != nil {
		return Server{}, err
	}
	if input.Tags != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM server_tags WHERE server_id=$1`, serverID); err != nil {
			return Server{}, err
		}
		for _, tag := range *input.Tags {
			if _, err := tx.Exec(ctx, `INSERT INTO server_tags (server_id,tag) VALUES ($1,$2)`, serverID, tag); err != nil {
				return Server{}, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id,ip_address) VALUES ($1,$2,'server.update','server',$3,NULLIF($4,'')::inet)`, workspaceID, userID, serverID, ip); err != nil {
		return Server{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Server{}, err
	}
	return s.Get(ctx, userID, serverID)
}

func (s *Service) Delete(ctx context.Context, userID, serverID, ip string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var workspaceID string
	err = tx.QueryRow(ctx, `DELETE FROM servers s USING workspace_members wm WHERE s.id=$1 AND wm.workspace_id=s.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') RETURNING s.workspace_id`, serverID, userID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id,ip_address) VALUES ($1,$2,'server.delete','server',$3,NULLIF($4,'')::inet)`, workspaceID, userID, serverID, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Statuses(ctx context.Context, userID, workspaceID string) ([]Status, error) {
	rows, err := s.db.Query(ctx, `SELECT s.id,CASE WHEN s.status IN ('maintenance','unknown') THEN s.status WHEN s.last_seen_at IS NULL OR s.last_seen_at < now()-interval '2 minutes' THEN 'offline' ELSE s.status END,s.last_seen_at FROM servers s JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 WHERE s.workspace_id=$2 ORDER BY s.id`, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Status{}
	for rows.Next() {
		var status Status
		if err := rows.Scan(&status.ID, &status.Status, &status.LastSeenAt); err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Server, error) {
	var server Server
	err := row.Scan(&server.ID, &server.WorkspaceID, &server.Name, &server.Hostname, &server.Description, &server.Environment,
		&server.OperatingSystem, &server.OSVersion, &server.KernelVersion, &server.Architecture, &server.AgentVersion, &server.Status,
		&server.LastSeenAt, &server.EnrolledAt, &server.Tags, &server.AgentRevoked, &server.CPUUsagePercent, &server.MemoryUsagePercent, &server.DiskUsagePercent, &server.UptimeSeconds)
	return server, err
}
