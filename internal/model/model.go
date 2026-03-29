package model

import "time"

// Host represents a managed server/VPS.
type Host struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	IP           string     `json:"ip"`
	Provider     string     `json:"provider"`
	Plan         string     `json:"plan"`
	Cost         float64    `json:"cost"`
	Currency     string     `json:"currency"`
	BillingCycle string     `json:"billing_cycle"`
	SubscribeAt  *time.Time `json:"subscribe_at"`
	ExpireAt     *time.Time `json:"expire_at"`
	Note         string     `json:"note"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Metric represents a point-in-time resource snapshot from an agent.
type Metric struct {
	ID          int64     `json:"id"`
	HostID      int64     `json:"host_id"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemTotal    uint64    `json:"mem_total"`
	MemUsed     uint64    `json:"mem_used"`
	MemBuffered uint64    `json:"mem_buffered"`
	MemCached   uint64    `json:"mem_cached"`
	SwapTotal   uint64    `json:"swap_total"`
	SwapUsed    uint64    `json:"swap_used"`
	DiskTotal   uint64    `json:"disk_total"`
	DiskUsed    uint64    `json:"disk_used"`
	NetIn       uint64    `json:"net_in"`
	NetOut      uint64    `json:"net_out"`
	Load1       float64   `json:"load_1"`
	Load5       float64   `json:"load_5"`
	Load15      float64   `json:"load_15"`
	Uptime      int64     `json:"uptime"`
	ProcessCount int      `json:"process_count"`
	CollectedAt time.Time `json:"collected_at"`
}

// DiskPartition represents a single disk mount point.
type DiskPartition struct {
	Device     string `json:"device"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type"`
	Total      uint64 `json:"total"`
	Used       uint64 `json:"used"`
	Free       uint64 `json:"free"`
}

// NetInterface represents a single network interface.
type NetInterface struct {
	Name   string `json:"name"`
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
	RxPkts  uint64 `json:"rx_pkts"`
	TxPkts  uint64 `json:"tx_pkts"`
	IPv4    string `json:"ipv4"`
}

// DetailedData holds extra per-report data stored as JSON.
type DetailedData struct {
	Disks      []DiskPartition `json:"disks,omitempty"`
	Interfaces []NetInterface  `json:"interfaces,omitempty"`
	TopCPU     []ProcessInfo   `json:"top_cpu,omitempty"`
	TopMem     []ProcessInfo   `json:"top_mem,omitempty"`
}

// ProcessInfo represents a top process.
type ProcessInfo struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	MemRSS  uint64  `json:"mem_rss"`
}

// Service represents a running service on a host.
type Service struct {
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
}

// Alert represents a triggered alert.
type Alert struct {
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"created_at"`
}

// HostInfo contains static system information.
type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kernel   string `json:"kernel"`
	Distro   string `json:"distro"`
	CPUModel string `json:"cpu_model"`
	CPUCores int    `json:"cpu_cores"`
}

// AgentReport is the payload an agent sends to the server.
type AgentReport struct {
	HostKey  string        `json:"host_key"`
	Metric   Metric        `json:"metric"`
	Services []Service     `json:"services"`
	HostInfo *HostInfo     `json:"host_info,omitempty"`
	Detail   *DetailedData `json:"detail,omitempty"`
}

// HostWithMetric combines host info with its latest metric.
type HostWithMetric struct {
	Host
	LatestMetric *Metric `json:"latest_metric,omitempty"`
}
