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
	Name        string `json:"name"`
	State       string `json:"state"`
	PID         uint32 `json:"pid,omitempty"`
	Lifetime    int64  `json:"lifetime,omitempty"`
	MachineType string `json:"machine_type,omitempty"`
	// CpuModel is the CPU model string (MachineOpts.CPU.Model), e.g.
	// "Broadwell-IBRS,+pcid".
	CpuModel      string `json:"cpu_model,omitempty"`
	FirmwareImage string `json:"firmware_image,omitempty"`
	// FirmwareFlash is the firmware flash (EFI vars) path
	// (MachineOpts.Firmware.Flash).
	FirmwareFlash string `json:"firmware_flash,omitempty"`
	VGAType       string `json:"vga_type,omitempty"`
	// VsockCid is the VM's AF_VSOCK context ID (MachineOpts.VsockDevice
	// .ContextID) used to reach the guest agent's built-in services (SSH
	// console over vsock). Zero when the VM has no vsock device.
	VsockCid      uint32 `json:"vsock_cid,omitempty"`
	MemActual     uint32 `json:"mem_actual"`
	MemTotal      uint32 `json:"mem_total"`
	CPUsActual    uint32 `json:"cpus_actual"`
	CPUsTotal     uint32 `json:"cpus_total"`
	CPUQuota      uint32 `json:"cpu_quota,omitempty"`
	DiskCount     int    `json:"disks"`
	NetIfaceCount int    `json:"net_ifaces"`
	// Disks is the list of storage drives (MachineOpts.Storage), mirroring
	// the "Storage" block of the console output.
	Disks []DiskInfo `json:"disk_list,omitempty"`
}

// DiskInfo is one storage drive of a VM (MachineOpts.Disk), mirroring one
// line of the "Storage" block in the "vmm inspect" console output. The drive
// size the console prints is not available in the daemon API, so it is not
// exposed here.
type DiskInfo struct {
	// Name is the base name of the path — the identifier the console
	// prints as the drive label.
	Name      string `json:"name"`
	Path      string `json:"path"`
	Driver    string `json:"driver,omitempty"`
	IopsRd    uint32 `json:"iops_rd,omitempty"`
	IopsWr    uint32 `json:"iops_wr,omitempty"`
	Bootindex uint32 `json:"bootindex,omitempty"`
	Addr      string `json:"addr,omitempty"`
}

// NetworkScheme is one interface scheme of a VM as reported by the network
// service GetConf (NetworkSchemeOpts), mirroring the per-interface block
// printed by the "vmm nets" console output.
type NetworkScheme struct {
	Ifname string `json:"ifname"`
	// Type is the scheme type derived from the attrs oneof: routed, bridge,
	// vxlan or vlan ("manual" when the scheme carries no attrs).
	Type string `json:"type"`
	// MTU is the interface MTU; the daemon leaves it 0 when unset and the
	// console applies a 1500 default, which we mirror here.
	MTU   uint32   `json:"mtu"`
	Addrs []string `json:"addrs"`

	Gateway4 string `json:"gateway4,omitempty"`
	Gateway6 string `json:"gateway6,omitempty"`

	// VLAN scheme attributes.
	VlanID          uint32 `json:"vlan_id,omitempty"`
	ParentInterface string `json:"parent_interface,omitempty"`

	// VxLAN scheme attributes.
	VNI           uint32 `json:"vni,omitempty"`
	BindInterface string `json:"bind_interface,omitempty"`

	// Router scheme attributes (limits in mbit/s).
	InLimit  uint32 `json:"in_limit,omitempty"`
	OutLimit uint32 `json:"out_limit,omitempty"`

	// Bridge scheme attributes.
	BridgeName string `json:"bridge_name,omitempty"`
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
