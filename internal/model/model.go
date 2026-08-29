// Package model contains the data-transfer types shared between the HTTP
// handlers and the frontend. They intentionally mirror the data the vmm
// CLI prints, so the dashboard visualizes the same information without
// leaking gRPC/protobuf types through the API.
package model

// MachineSummary is one row of the "vmm list" table.
type MachineSummary struct {
	Name       string `json:"name"`
	PID        uint32 `json:"pid,omitempty"`
	MemActual  uint32 `json:"mem_actual"`
	MemTotal   uint32 `json:"mem_total"`
	CPUsActual uint32 `json:"cpus_actual"`
	CPUsTotal  uint32 `json:"cpus_total"`
	CpuPercent uint32 `json:"cpu_percent,omitempty"`
	State      string `json:"state"`
	Lifetime   int64  `json:"lifetime,omitempty"`
}

// MachineDetail is the full description of a VM ("vmm inspect" output).
type MachineDetail struct {
	Name          string `json:"name"`
	State         string `json:"state"`
	PID           uint32 `json:"pid,omitempty"`
	Lifetime      int64  `json:"lifetime,omitempty"`
	MachineType   string `json:"machine_type,omitempty"`
	FirmwareImage string `json:"firmware_image,omitempty"`
	VGAType       string `json:"vga_type,omitempty"`
	MemActual     uint32 `json:"mem_actual"`
	MemTotal      uint32 `json:"mem_total"`
	CPUsActual    uint32 `json:"cpus_actual"`
	CPUsTotal     uint32 `json:"cpus_total"`
	CPUQuota      uint32 `json:"cpu_quota,omitempty"`
	DiskCount     int    `json:"disks"`
	NetIfaceCount int    `json:"net_ifaces"`
}

// TaskInfo describes a long-running operation (migration, backup) reported
// by the tasks service.
type TaskInfo struct {
	TaskID    string `json:"task_id"`
	State     string `json:"state"`
	StateDesc string `json:"state_desc,omitempty"`
	Progress  uint32 `json:"progress"`
}

// SystemInfo describes the daemon and the host as reported by the system
// service.
type SystemInfo struct {
	GoVersion   string `json:"go_version"`
	QemuRootdir string `json:"qemu_rootdir,omitempty"`
	CertDir     string `json:"cert_dir,omitempty"`
}
