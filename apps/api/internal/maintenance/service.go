package maintenance

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Result struct {
	MetricsDeleted     int64
	SessionsDeleted    int64
	EnrollmentsDeleted int64
}

type Service struct {
	db            *pgxpool.Pool
	retentionDays int
}

func NewService(db *pgxpool.Pool, retentionDays int) *Service {
	return &Service{db: db, retentionDays: retentionDays}
}

func (s *Service) Run(ctx context.Context) (Result, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var result Result
	command, err := tx.Exec(ctx, `DELETE FROM metric_samples WHERE collected_at < now() - make_interval(days => $1)`, s.retentionDays)
	if err != nil {
		return Result{}, err
	}
	result.MetricsDeleted = command.RowsAffected()
	command, err = tx.Exec(ctx, `DELETE FROM sessions WHERE (expires_at < now() OR revoked_at IS NOT NULL) AND created_at < now() - interval '7 days'`)
	if err != nil {
		return Result{}, err
	}
	result.SessionsDeleted = command.RowsAffected()
	command, err = tx.Exec(ctx, `DELETE FROM enrollment_tokens WHERE (expires_at < now() OR used_at IS NOT NULL OR revoked_at IS NOT NULL) AND created_at < now() - interval '7 days'`)
	if err != nil {
		return Result{}, err
	}
	result.EnrollmentsDeleted = command.RowsAffected()
	return result, tx.Commit(ctx)
}
