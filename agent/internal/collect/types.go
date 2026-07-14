package collect

import "time"

type Identity struct {
	Hostname      string `json:"hostname"`
	MachineID     string `json:"machine_id"`
	OS            string `json:"os"`
	OSVersion     string `json:"os_version"`
	KernelVersion string `json:"kernel_version"`
	Architecture  string `json:"architecture"`
	AgentVersion  string `json:"agent_version"`
}

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
