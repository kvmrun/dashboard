package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// MachinesList renders the VM list page — the "vmm list" equivalent.
func (h *Handlers) MachinesList(c *gin.Context) {
	machines, err := h.listMachines(c.Request.Context())
	if err != nil {
		h.render(c, "machines.html", http.StatusBadGateway, gin.H{"Title": "Machines", "Error": err.Error()})
		return
	}
	h.render(c, "machines.html", http.StatusOK, gin.H{"Title": "Machines", "Machines": machines})
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

// MachineDetail renders the detail page for one VM — the "vmm info" /
// "vmm inspect" equivalent, with power-control actions.
func (h *Handlers) MachineDetail(c *gin.Context) {
	name := c.Param("name")
	detail, err := h.getMachine(c.Request.Context(), name)
	if err != nil {
		h.render(c, "machine_detail.html", http.StatusNotFound,
			gin.H{"Title": name, "Name": name, "Error": err.Error()})
		return
	}
	h.render(c, "machine_detail.html", http.StatusOK,
		gin.H{"Title": name, "Name": name, "Machine": detail})
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
		if fw := opts.Firmware; fw != nil {
			d.FirmwareImage = fw.Image
		}
		if vga := opts.VGA; vga != nil {
			d.VGAType = vga.Type
		}
		d.DiskCount = len(opts.Storage)
		d.NetIfaceCount = len(opts.Network)
	}
	return d
}
