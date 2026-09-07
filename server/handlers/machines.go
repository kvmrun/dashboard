package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_network "github.com/0xef53/kvmrun/api/services/network/v2"
	pb_tasks "github.com/0xef53/kvmrun/api/services/tasks/v2"
	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// MachinesList renders the master-detail VM page. The VM list and the
// selected machine's detail are loaded client-side from the JSON API
// (/api/v1/machines), so the page itself needs no daemon call.
func (h *Handlers) MachinesList(c *gin.Context) {
	h.render(c, "machines.html", http.StatusOK, gin.H{"Title": "Machines", "Page": "machines"})
}

// MachinesListJSON is the JSON variant of the VM list for the frontend.
func (h *Handlers) MachinesListJSON(c *gin.Context) {
	machines, err := h.listMachines(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, machines)
}

// MachineDetail renders the master-detail VM page with a preselected VM
// (shareable /machines/{name} URLs and the redirect target of power-control
// actions). Data still comes from the JSON API client-side.
func (h *Handlers) MachineDetail(c *gin.Context) {
	data := gin.H{"Title": "Machines", "Page": "machines", "Selected": c.Param("name")}
	if err := c.Query("error"); err != "" {
		data["Error"] = err
	}
	h.render(c, "machines.html", http.StatusOK, data)
}

// MachineDetailJSON returns the full description of one VM.
func (h *Handlers) MachineDetailJSON(c *gin.Context) {
	name := c.Param("name")
	detail, err := h.getMachine(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// MachineNetworksJSON returns the VM's network interface schemes
// (NetworkService.GetConf) — the same data the "vmm nets" console output
// prints for the machine.
func (h *Handlers) MachineNetworksJSON(c *gin.Context) {
	name := c.Param("name")
	resp, err := h.Daemon.Network.GetConf(c.Request.Context(), &pb_network.GetConfRequest{Name: name})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	out := make([]model.NetworkScheme, 0, len(resp.Schemes))
	for _, s := range resp.Schemes {
		out = append(out, networkScheme(s))
	}
	c.JSON(http.StatusOK, out)
}

// StartMachine starts a VM ("vmm start").
func (h *Handlers) StartMachine(c *gin.Context) {
	name := c.Param("name")
	_, err := h.Daemon.Machines.Start(c.Request.Context(), &pb_machines.StartRequest{Name: name})
	h.finishAction(c, name, err)
}

// StopMachine stops a VM ("vmm stop").
func (h *Handlers) StopMachine(c *gin.Context) {
	name := c.Param("name")
	_, err := h.Daemon.Machines.Stop(c.Request.Context(), &pb_machines.StopRequest{Name: name, Wait: true})
	h.finishAction(c, name, err)
}

// RestartMachine restarts a VM ("vmm restart").
func (h *Handlers) RestartMachine(c *gin.Context) {
	name := c.Param("name")
	_, err := h.Daemon.Machines.Restart(c.Request.Context(), &pb_machines.RestartRequest{Name: name, Wait: true})
	h.finishAction(c, name, err)
}

// ResetMachine resets a VM to its saved state ("vmm reset").
func (h *Handlers) ResetMachine(c *gin.Context) {
	name := c.Param("name")
	_, err := h.Daemon.Machines.Reset(c.Request.Context(), &pb_machines.ResetRequest{Name: name, Wait: true})
	h.finishAction(c, name, err)
}

// MigrateRequest is the JSON body for starting a VM migration.
type MigrateRequest struct {
	DstServer   string `json:"dst_server" binding:"required"`
	RemoveAfter bool   `json:"remove_after"`
}

// MigrateMachine starts migrating a VM to the destination server
// (equivalent of `vmm migration start <name> <dst> --with-local-disks --create-disks`).
// All local disks are copied and the disks are created on the destination
// (create_disks = true). remove_after is taken from the request, so the
// source machine may optionally be deleted once the migration completes.
func (h *Handlers) MigrateMachine(c *gin.Context) {
	name := c.Param("name")
	var req MigrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dst_server is required"})
		return
	}
	ctx := c.Request.Context()

	resp, err := h.Daemon.Machines.Get(ctx, &pb_machines.GetRequest{Name: name})
	if err != nil || resp.Machine == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("machine %q not found", name)})
		return
	}
	switch resp.Machine.State {
	case pb_types.MachineState_RUNNING, pb_types.MachineState_PAUSED:
	default:
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("machine is not running or paused: %s", resp.Machine.State.String())})
		return
	}

	var disks []string
	if runtime := resp.Machine.Runtime; runtime != nil {
		for _, d := range runtime.Storage {
			if d != nil && d.Path != "" {
				disks = append(disks, d.Path)
			}
		}
	}
	if len(disks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "machine has no local disks to migrate"})
		return
	}

	migResp, err := h.Daemon.Machines.StartMigrationProcess(ctx, &pb_machines.StartMigrationRequest{
		Name:        name,
		DstServer:   req.DstServer,
		Disks:       disks,
		CreateDisks: true,
		RemoveAfter: req.RemoveAfter,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_key": migResp.TaskKey})
}

// MigrationStatusJSON returns the state of the VM's migration task
// (task key: "<name>/migration") for polling from the frontend.
func (h *Handlers) MigrationStatusJSON(c *gin.Context) {
	name := c.Param("name")
	resp, err := h.Daemon.Tasks.Get(c.Request.Context(), &pb_tasks.GetRequest{Key: name + "/migration"})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "no migration task found"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	t := resp.Task
	if t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no migration task found"})
		return
	}

	out := model.MigrationStatus{
		TaskID:    t.TaskID,
		State:     t.State.String(),
		StateDesc: t.StateDesc,
		Progress:  t.Progress,
	}
	if mig := t.GetMigration(); mig != nil {
		out.DstServer = mig.DstServer
		for diskName, stat := range mig.Disks {
			if stat == nil {
				continue
			}
			out.Disks = append(out.Disks, model.DiskMigrationProgress{
				Name:      diskName,
				Progress:  stat.Progress,
				Total:     stat.Total,
				Transferred: stat.Transferred,
				Remaining: stat.Remaining,
				Speed:     stat.Speed,
			})
		}
		sort.Slice(out.Disks, func(i, j int) bool { return out.Disks[i].Name < out.Disks[j].Name })
	}
	c.JSON(http.StatusOK, out)
}

// finishAction reports the result of a power-control action: JSON for
// API clients, a redirect back to the machine page for browsers.
func (h *Handlers) finishAction(c *gin.Context, name string, err error) {
	if err != nil {
		if isJSONRequest(c) {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		} else {
			c.Redirect(http.StatusSeeOther, "/machines/"+url.PathEscape(name)+"?error="+url.QueryEscape(err.Error()))
		}
		return
	}
	if isJSONRequest(c) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	} else {
		c.Redirect(http.StatusSeeOther, "/machines/"+url.PathEscape(name))
	}
}

func isJSONRequest(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}

func (h *Handlers) listMachines(ctx context.Context) ([]model.MachineSummary, error) {
	resp, err := h.Daemon.Machines.List(ctx, &pb_machines.ListRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]model.MachineSummary, 0, len(resp.Machines))
	for _, m := range resp.Machines {
		out = append(out, machineSummary(m))
	}
	return out, nil
}

func (h *Handlers) getMachine(ctx context.Context, name string) (*model.MachineDetail, error) {
	resp, err := h.Daemon.Machines.Get(ctx, &pb_machines.GetRequest{Name: name})
	if err != nil {
		return nil, err
	}
	if resp.Machine == nil {
		return nil, fmt.Errorf("machine %q not found", name)
	}
	return machineDetail(resp.Machine), nil
}

// machineSummary maps a proto Machine to the list-row model, preferring
// runtime values over configured ones (same precedence as "vmm list").
func machineSummary(m *pb_types.Machine) model.MachineSummary {
	s := model.MachineSummary{
		Name:     m.Name,
		PID:      m.PID,
		State:    m.State.String(),
		Lifetime: int64(m.LifeTime),
	}
	opts := m.Runtime
	if opts == nil {
		opts = m.Config
	}
	if opts != nil {
		if mem := opts.Memory; mem != nil {
			s.MemActual = mem.Actual
			s.MemTotal = mem.Total
		}
		if cpu := opts.CPU; cpu != nil {
			s.CPUsActual = cpu.Actual
			s.CPUsTotal = cpu.Total
			s.CpuPercent = cpu.Quota
		}
	}
	return s
}

// networkScheme maps a proto NetworkSchemeOpts to the frontend model,
// following the same rules as the "vmm nets" console output: the scheme
// type is derived from the attrs oneof and an unset MTU means 1500.
func networkScheme(o *pb_types.NetworkSchemeOpts) model.NetworkScheme {
	s := model.NetworkScheme{
		Ifname:   o.Ifname,
		MTU:      1500,
		Addrs:    o.Addrs,
		Gateway4: o.Gateway4,
		Gateway6: o.Gateway6,
	}
	if o.MTU > 0 {
		s.MTU = o.MTU
	}
	if s.Addrs == nil {
		s.Addrs = []string{}
	}
	switch a := o.Attrs.(type) {
	case *pb_types.NetworkSchemeOpts_Vlan:
		s.Type = "vlan"
		s.VlanID = a.Vlan.VlanID
		s.ParentInterface = a.Vlan.ParentInterface
	case *pb_types.NetworkSchemeOpts_Vxlan:
		s.Type = "vxlan"
		s.VNI = a.Vxlan.VNI
		s.BindInterface = a.Vxlan.BindInterface
	case *pb_types.NetworkSchemeOpts_Router:
		s.Type = "routed"
		s.BindInterface = a.Router.BindInterface
		s.InLimit = a.Router.InLimit
		s.OutLimit = a.Router.OutLimit
	case *pb_types.NetworkSchemeOpts_Bridge:
		s.Type = "bridge"
		s.BridgeName = a.Bridge.BridgeName
	default:
		s.Type = "manual"
	}
	return s
}

// machineDetail maps a proto Machine to the detail model.
func machineDetail(m *pb_types.Machine) *model.MachineDetail {
	s := machineSummary(m)
	d := &model.MachineDetail{
		Name:       s.Name,
		State:      s.State,
		PID:        s.PID,
		Lifetime:   s.Lifetime,
		MemActual:  s.MemActual,
		MemTotal:   s.MemTotal,
		CPUsActual: s.CPUsActual,
		CPUsTotal:  s.CPUsTotal,
		CPUQuota:   s.CpuPercent,
	}
	if opts := m.Config; opts != nil {
		d.MachineType = opts.MachineType
		if cpu := opts.CPU; cpu != nil {
			d.CpuModel = cpu.Model
		}
		if fw := opts.Firmware; fw != nil {
			d.FirmwareImage = fw.Image
			d.FirmwareFlash = fw.Flash
		}
		if vga := opts.VGA; vga != nil {
			d.VGAType = vga.Type
		}
		if vs := opts.VsockDevice; vs != nil {
			d.VsockCid = vs.ContextID
		}
		d.DiskCount = len(opts.Storage)
		d.NetIfaceCount = len(opts.Network)
		d.Disks = diskInfoList(opts.Storage)
	}
	return d
}

// diskInfoList maps proto storage drives to the frontend model. The drive
// name is the base name of the path — the same identifier the "vmm inspect"
// console prints as the drive label.
func diskInfoList(storage []*pb_types.MachineOpts_Disk) []model.DiskInfo {
	out := make([]model.DiskInfo, 0, len(storage))
	for _, s := range storage {
		if s == nil {
			continue
		}
		out = append(out, model.DiskInfo{
			Name:      path.Base(s.Path),
			Path:      s.Path,
			Driver:    s.Driver,
			IopsRd:    s.IopsRd,
			IopsWr:    s.IopsWr,
			Bootindex: s.Bootindex,
			Addr:      s.Addr,
		})
	}
	return out
}
