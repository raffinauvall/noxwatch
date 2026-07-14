package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/enrollment"
)

var ErrNotFound = errors.New("metrics not found")

type Payload struct {
	ServerID    string           `json:"server_id"`
	CollectedAt time.Time        `json:"collected_at"`
	Sequence    int64            `json:"sequence"`
	System      SystemMetrics    `json:"system"`
	CPU         CPUMetrics       `json:"cpu"`
	Memory      MemoryMetrics    `json:"memory"`
	Swap        SwapMetrics      `json:"swap"`
	Disks       []DiskMetrics    `json:"disks"`
	Networks    []NetworkMetrics `json:"networks"`
}

type SystemMetrics struct {
	UptimeSeconds      int64 `json:"uptime_seconds"`
	ProcessCount       int   `json:"process_count"`
	ZombieProcessCount int   `json:"zombie_process_count"`
}
type CPUMetrics struct {
	UsagePercent     float64 `json:"usage_percent"`
	Load1            float64 `json:"load_1"`
	Load5            float64 `json:"load_5"`
	Load15           float64 `json:"load_15"`
	LogicalCPUCount  int     `json:"logical_cpu_count"`
	PhysicalCPUCount int     `json:"physical_cpu_count"`
}
type MemoryMetrics struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	AvailableBytes int64   `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}
type SwapMetrics struct {
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}
type DiskMetrics struct {
	MountPoint        string  `json:"mount_point"`
	Filesystem        string  `json:"filesystem"`
	TotalBytes        int64   `json:"total_bytes"`
	UsedBytes         int64   `json:"used_bytes"`
	AvailableBytes    int64   `json:"available_bytes"`
	UsagePercent      float64 `json:"usage_percent"`
	InodeUsagePercent float64 `json:"inode_usage_percent"`
}
type NetworkMetrics struct {
	Interface        string  `json:"interface"`
	RXBytesTotal     int64   `json:"rx_bytes_total"`
	TXBytesTotal     int64   `json:"tx_bytes_total"`
	RXPacketsTotal   int64   `json:"rx_packets_total"`
	TXPacketsTotal   int64   `json:"tx_packets_total"`
	RXErrorsTotal    int64   `json:"rx_errors_total"`
	TXErrorsTotal    int64   `json:"tx_errors_total"`
	RXBytesPerSecond float64 `json:"rx_bytes_per_second"`
	TXBytesPerSecond float64 `json:"tx_bytes_per_second"`
}

type Sample struct {
	CollectedAt        time.Time `json:"collected_at"`
	UptimeSeconds      int64     `json:"uptime_seconds"`
	ProcessCount       int       `json:"process_count"`
	CPUUsagePercent    float64   `json:"cpu_usage_percent"`
	Load1              float64   `json:"load_1"`
	Load5              float64   `json:"load_5"`
	Load15             float64   `json:"load_15"`
	MemoryTotalBytes   int64     `json:"memory_total_bytes"`
	MemoryUsedBytes    int64     `json:"memory_used_bytes"`
	MemoryUsagePercent float64   `json:"memory_usage_percent"`
	SwapTotalBytes     int64     `json:"swap_total_bytes"`
	SwapUsedBytes      int64     `json:"swap_used_bytes"`
	SwapUsagePercent   float64   `json:"swap_usage_percent"`
}

type Snapshot struct {
	Sample
	Disks    []DiskMetrics    `json:"disks"`
	Networks []NetworkMetrics `json:"networks"`
}

type Service struct {
	db         *pgxpool.Pool
	enrollment *enrollment.Service
}

func NewService(db *pgxpool.Pool, enrollmentService *enrollment.Service) *Service {
	return &Service{db: db, enrollment: enrollmentService}
}

func (s *Service) Ingest(ctx context.Context, credential string, payload Payload) (bool, error) {
	identity, err := s.enrollment.AuthenticateAgent(ctx, credential, payload.ServerID)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sampleID string
	err = tx.QueryRow(ctx, `INSERT INTO metric_samples (workspace_id,server_id,agent_id,sequence,collected_at,uptime_seconds,process_count,zombie_process_count,cpu_usage_percent,load_1,load_5,load_15,logical_cpu_count,physical_cpu_count,memory_total_bytes,memory_used_bytes,memory_available_bytes,memory_usage_percent,swap_total_bytes,swap_used_bytes,swap_usage_percent)
	 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT (agent_id,sequence) DO NOTHING RETURNING id`,
		identity.WorkspaceID, identity.ServerID, identity.AgentID, payload.Sequence, payload.CollectedAt, payload.System.UptimeSeconds, payload.System.ProcessCount, payload.System.ZombieProcessCount,
		payload.CPU.UsagePercent, payload.CPU.Load1, payload.CPU.Load5, payload.CPU.Load15, payload.CPU.LogicalCPUCount, payload.CPU.PhysicalCPUCount,
		payload.Memory.TotalBytes, payload.Memory.UsedBytes, payload.Memory.AvailableBytes, payload.Memory.UsagePercent, payload.Swap.TotalBytes, payload.Swap.UsedBytes, payload.Swap.UsagePercent).Scan(&sampleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, disk := range payload.Disks {
		if _, err := tx.Exec(ctx, `INSERT INTO disk_metric_samples (metric_sample_id,mount_point,filesystem,total_bytes,used_bytes,available_bytes,usage_percent,inode_usage_percent) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, sampleID, disk.MountPoint, disk.Filesystem, disk.TotalBytes, disk.UsedBytes, disk.AvailableBytes, disk.UsagePercent, disk.InodeUsagePercent); err != nil {
			return false, err
		}
	}
	for _, network := range payload.Networks {
		if _, err := tx.Exec(ctx, `INSERT INTO network_metric_samples (metric_sample_id,interface_name,rx_bytes_total,tx_bytes_total,rx_packets_total,tx_packets_total,rx_errors_total,tx_errors_total,rx_bytes_per_second,tx_bytes_per_second) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, sampleID, network.Interface, network.RXBytesTotal, network.TXBytesTotal, network.RXPacketsTotal, network.TXPacketsTotal, network.RXErrorsTotal, network.TXErrorsTotal, network.RXBytesPerSecond, network.TXBytesPerSecond); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET last_seen_at=now(),status=CASE WHEN status='maintenance' THEN status ELSE 'online' END,updated_at=now() WHERE id=$1`, identity.ServerID); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func (s *Service) History(ctx context.Context, userID, serverID string, from, to time.Time, limit int) ([]Sample, error) {
	rows, err := s.db.Query(ctx, `SELECT ms.collected_at,ms.uptime_seconds,ms.process_count,COALESCE(ms.cpu_usage_percent,0),COALESCE(ms.load_1,0),COALESCE(ms.load_5,0),COALESCE(ms.load_15,0),COALESCE(ms.memory_total_bytes,0),COALESCE(ms.memory_used_bytes,0),COALESCE(ms.memory_usage_percent,0),COALESCE(ms.swap_total_bytes,0),COALESCE(ms.swap_used_bytes,0),COALESCE(ms.swap_usage_percent,0)
	 FROM metric_samples ms JOIN servers s ON s.id=ms.server_id JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 WHERE ms.server_id=$2 AND ms.collected_at BETWEEN $3 AND $4 ORDER BY ms.collected_at LIMIT $5`, userID, serverID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Sample{}
	for rows.Next() {
		var sample Sample
		if err := scanSample(rows, &sample); err != nil {
			return nil, err
		}
		result = append(result, sample)
	}
	return result, rows.Err()
}

func (s *Service) Latest(ctx context.Context, userID, serverID string) (Snapshot, error) {
	row := s.db.QueryRow(ctx, `SELECT ms.id,ms.collected_at,ms.uptime_seconds,ms.process_count,COALESCE(ms.cpu_usage_percent,0),COALESCE(ms.load_1,0),COALESCE(ms.load_5,0),COALESCE(ms.load_15,0),COALESCE(ms.memory_total_bytes,0),COALESCE(ms.memory_used_bytes,0),COALESCE(ms.memory_usage_percent,0),COALESCE(ms.swap_total_bytes,0),COALESCE(ms.swap_used_bytes,0),COALESCE(ms.swap_usage_percent,0)
	 FROM metric_samples ms JOIN servers s ON s.id=ms.server_id JOIN workspace_members wm ON wm.workspace_id=s.workspace_id AND wm.user_id=$1 WHERE ms.server_id=$2 ORDER BY ms.collected_at DESC LIMIT 1`, userID, serverID)
	var result Snapshot
	var sampleID string
	err := row.Scan(&sampleID, &result.CollectedAt, &result.UptimeSeconds, &result.ProcessCount, &result.CPUUsagePercent, &result.Load1, &result.Load5, &result.Load15,
		&result.MemoryTotalBytes, &result.MemoryUsedBytes, &result.MemoryUsagePercent, &result.SwapTotalBytes, &result.SwapUsedBytes, &result.SwapUsagePercent)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}
	result.Disks = []DiskMetrics{}
	diskRows, err := s.db.Query(ctx, `SELECT mount_point,filesystem,total_bytes,used_bytes,COALESCE(available_bytes,0),COALESCE(usage_percent,0),COALESCE(inode_usage_percent,0) FROM disk_metric_samples WHERE metric_sample_id=$1 ORDER BY mount_point`, sampleID)
	if err != nil {
		return Snapshot{}, err
	}
	for diskRows.Next() {
		var disk DiskMetrics
		if err := diskRows.Scan(&disk.MountPoint, &disk.Filesystem, &disk.TotalBytes, &disk.UsedBytes, &disk.AvailableBytes, &disk.UsagePercent, &disk.InodeUsagePercent); err != nil {
			diskRows.Close()
			return Snapshot{}, err
		}
		result.Disks = append(result.Disks, disk)
	}
	diskRows.Close()
	if err := diskRows.Err(); err != nil {
		return Snapshot{}, err
	}
	result.Networks = []NetworkMetrics{}
	networkRows, err := s.db.Query(ctx, `SELECT interface_name,rx_bytes_total,tx_bytes_total,COALESCE(rx_packets_total,0),COALESCE(tx_packets_total,0),COALESCE(rx_errors_total,0),COALESCE(tx_errors_total,0),COALESCE(rx_bytes_per_second,0),COALESCE(tx_bytes_per_second,0) FROM network_metric_samples WHERE metric_sample_id=$1 ORDER BY interface_name`, sampleID)
	if err != nil {
		return Snapshot{}, err
	}
	defer networkRows.Close()
	for networkRows.Next() {
		var network NetworkMetrics
		if err := networkRows.Scan(&network.Interface, &network.RXBytesTotal, &network.TXBytesTotal, &network.RXPacketsTotal, &network.TXPacketsTotal, &network.RXErrorsTotal, &network.TXErrorsTotal, &network.RXBytesPerSecond, &network.TXBytesPerSecond); err != nil {
			return Snapshot{}, err
		}
		result.Networks = append(result.Networks, network)
	}
	return result, networkRows.Err()
}

type scanner interface{ Scan(...any) error }

func scanSample(row scanner, sample *Sample) error {
	return row.Scan(&sample.CollectedAt, &sample.UptimeSeconds, &sample.ProcessCount, &sample.CPUUsagePercent, &sample.Load1, &sample.Load5, &sample.Load15,
		&sample.MemoryTotalBytes, &sample.MemoryUsedBytes, &sample.MemoryUsagePercent, &sample.SwapTotalBytes, &sample.SwapUsedBytes, &sample.SwapUsagePercent)
}
