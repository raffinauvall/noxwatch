package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
	"github.com/raffinauvall/noxwatch/apps/api/internal/database"
	"github.com/raffinauvall/noxwatch/apps/api/internal/observability"
)

func main() {
	logger := observability.NewLogger(os.Stdout)
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fail(logger, err)
	}
	if cfg.AppEnv != "development" {
		fail(logger, errors.New("seed is restricted to APP_ENV=development"))
	}
	password := os.Getenv("SEED_DEMO_PASSWORD")
	if len(password) < 12 {
		fail(logger, errors.New("SEED_DEMO_PASSWORD must contain at least 12 characters"))
	}
	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(logger, err)
	}
	defer db.Close()
	if err := seed(ctx, db, cfg.AuthSecret, password); err != nil {
		fail(logger, err)
	}
	logger.Info("development seed complete", "email", "demo@noxwatch.local", "workspace", "NoxWatch Demo", "servers", 3)
}

func seed(ctx context.Context, db *pgxpool.Pool, authSecret, password string) error {
	userID, err := ensureUser(ctx, db, authSecret, password)
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var workspaceID string
	err = tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE slug='noxwatch-demo'`).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO workspaces (name,slug,created_by) VALUES ('NoxWatch Demo','noxwatch-demo',$1) RETURNING id`, userID).Scan(&workspaceID)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,'owner') ON CONFLICT DO NOTHING`, workspaceID, userID); err != nil {
		return err
	}
	servers := []struct {
		name, hostname, environment, status string
		lastSeen                            time.Time
	}{{"prod-api-01", "prod-api-01", "production", "online", time.Now().UTC()}, {"staging-worker-02", "staging-worker-02", "staging", "warning", time.Now().UTC()}, {"dev-cache-01", "dev-cache-01", "development", "offline", time.Now().UTC().Add(-10 * time.Minute)}}
	serverIDs := make([]string, 0, len(servers))
	for index, item := range servers {
		serverID, agentID, err := ensureServer(ctx, tx, workspaceID, item.name, item.hostname, item.environment, item.status, item.lastSeen)
		if err != nil {
			return err
		}
		serverIDs = append(serverIDs, serverID)
		if _, err := tx.Exec(ctx, `DELETE FROM metric_samples WHERE server_id=$1`, serverID); err != nil {
			return err
		}
		if err := seedMetrics(ctx, tx, workspaceID, serverID, agentID, index); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM alert_rules WHERE workspace_id=$1 AND name LIKE '[Demo] %'`, workspaceID); err != nil {
		return err
	}
	var ruleID string
	if err := tx.QueryRow(ctx, `INSERT INTO alert_rules (workspace_id,server_id,name,metric,warning_threshold,critical_threshold,evaluation_seconds,cooldown_seconds,created_by) VALUES ($1,$2,'[Demo] Worker CPU pressure','cpu_usage',75,90,300,900,$3) RETURNING id`, workspaceID, serverIDs[1], userID).Scan(&ruleID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO alert_events (workspace_id,alert_rule_id,server_id,severity,state,current_value,threshold,triggered_at,notified_at) VALUES ($1,$2,$3,'warning','firing',82.4,75,now()-interval '8 minutes',now()-interval '3 minutes')`, workspaceID, ruleID, serverIDs[1]); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensureUser(ctx context.Context, db *pgxpool.Pool, secret, password string) (string, error) {
	var userID string
	err := db.QueryRow(ctx, `SELECT id FROM users WHERE email_normalized='demo@noxwatch.local'`).Scan(&userID)
	if err == nil {
		result, loginErr := auth.NewService(db, secret).Login(ctx, "demo@noxwatch.local", password, "noxwatch-seed", "127.0.0.1")
		if loginErr != nil {
			return "", errors.New("existing demo user password does not match SEED_DEMO_PASSWORD")
		}
		return result.User.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	result, err := auth.NewService(db, secret).Register(ctx, "demo@noxwatch.local", password, "Demo Operator", "noxwatch-seed", "127.0.0.1")
	return result.User.ID, err
}

func ensureServer(ctx context.Context, tx pgx.Tx, workspaceID, name, hostname, environment, status string, lastSeen time.Time) (string, string, error) {
	var serverID string
	err := tx.QueryRow(ctx, `SELECT id FROM servers WHERE workspace_id=$1 AND name=$2`, workspaceID, name).Scan(&serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO servers (workspace_id,name,hostname,description,environment,operating_system,os_version,kernel_version,architecture,agent_version,status,last_seen_at,enrolled_at) VALUES ($1,$2,$3,'Simulated development seed data',$4,'linux','Ubuntu 24.04 LTS','6.8.0','x86_64','0.1.0',$5,$6,now()-interval '14 days') RETURNING id`, workspaceID, name, hostname, environment, status, lastSeen).Scan(&serverID)
	} else if err == nil {
		_, err = tx.Exec(ctx, `UPDATE servers SET hostname=$2,environment=$3,status=$4,last_seen_at=$5,updated_at=now() WHERE id=$1`, serverID, hostname, environment, status, lastSeen)
	}
	if err != nil {
		return "", "", err
	}
	var agentID string
	err = tx.QueryRow(ctx, `SELECT id FROM agents WHERE server_id=$1`, serverID).Scan(&agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		unusableCredentialHash := make([]byte, 32)
		if _, randomErr := rand.Read(unusableCredentialHash); randomErr != nil {
			return "", "", randomErr
		}
		err = tx.QueryRow(ctx, `INSERT INTO agents (server_id,credential_hash) VALUES ($1,$2) RETURNING id`, serverID, hex.EncodeToString(unusableCredentialHash)).Scan(&agentID)
	}
	return serverID, agentID, err
}

func seedMetrics(ctx context.Context, tx pgx.Tx, workspaceID, serverID, agentID string, serverIndex int) error {
	for sample := 1; sample <= 48; sample++ {
		collectedAt := time.Now().UTC().Add(-time.Duration(48-sample) * 30 * time.Minute)
		cpu := float64(18 + serverIndex*14 + (sample%8)*3)
		memory := float64(42 + serverIndex*9 + sample%5)
		disk := float64(51 + serverIndex*11)
		var sampleID string
		err := tx.QueryRow(ctx, `INSERT INTO metric_samples (workspace_id,server_id,agent_id,sequence,collected_at,uptime_seconds,process_count,cpu_usage_percent,load_1,load_5,load_15,logical_cpu_count,physical_cpu_count,memory_total_bytes,memory_used_bytes,memory_available_bytes,memory_usage_percent,swap_total_bytes,swap_used_bytes,swap_usage_percent) VALUES ($1,$2,$3,$4,$5,$6,148,$7,$8,$9,$10,8,4,17179869184,$11,$12,$13,4294967296,268435456,6.25) RETURNING id`, workspaceID, serverID, agentID, sample, collectedAt, int64(604800+sample*1800), cpu, cpu/20, cpu/24, cpu/30, int64(memory/100*17179869184), int64((100-memory)/100*17179869184), memory).Scan(&sampleID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO disk_metric_samples (metric_sample_id,mount_point,filesystem,total_bytes,used_bytes,available_bytes,usage_percent,inode_usage_percent) VALUES ($1,'/','ext4',536870912000,$2,$3,$4,24.5)`, sampleID, int64(disk/100*536870912000), int64((100-disk)/100*536870912000), disk); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO network_metric_samples (metric_sample_id,interface_name,rx_bytes_total,tx_bytes_total,rx_packets_total,tx_packets_total,rx_errors_total,tx_errors_total,rx_bytes_per_second,tx_bytes_per_second) VALUES ($1,'eth0',$2,$3,100000,80000,0,0,$4,$5)`, sampleID, int64(sample)*10000000, int64(sample)*6000000, 32000+serverIndex*4000, 18000+serverIndex*2500); err != nil {
			return err
		}
	}
	return nil
}

func fail(logger *slog.Logger, err error) {
	logger.Error("development seed failed", "error", err)
	os.Exit(1)
}
