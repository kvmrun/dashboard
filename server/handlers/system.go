package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/emptypb"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// SystemIndex renders the main dashboard page with daemon configuration.
func (h *Handlers) SystemIndex(c *gin.Context) {
	h.render(c, "system.html", http.StatusOK,
		gin.H{"Title": "Overview", "Page": "home", "Info": h.systemInfo(c)})
}

// SystemJSON returns daemon/host information — the system service equivalent.
func (h *Handlers) SystemJSON(c *gin.Context) {
	c.JSON(http.StatusOK, h.systemInfo(c))
}

// systemInfo reads the daemon's application configuration and VM counters. A
// daemon outage degrades to the minimal (Go-version-only) info instead of
// failing.
func (h *Handlers) systemInfo(c *gin.Context) model.SystemInfo {
	ctx := c.Request.Context()
	info := model.SystemInfo{GoVersion: runtime.Version()}
	resp, err := h.Daemon.System.GetAppConf(ctx, &emptypb.Empty{})
	if err == nil && resp.AppConf != nil && resp.AppConf.Kvmrun != nil {
		info.QemuRootdir = resp.AppConf.Kvmrun.QemuRootdir
		info.CertDir = resp.AppConf.Kvmrun.CertDir
	}

	// VM counters from the machine service. RUNNING and PAUSED machines count
	// as "running" (the machine page treats them the same — both offer
	// Stop/Restart); everything else counts as not running, so the stat cards
	// always sum to the total.
	if machines, err := h.Daemon.Machines.List(ctx, &pb_machines.ListRequest{}); err == nil {
		for _, m := range machines.Machines {
			if m == nil {
				continue
			}
			info.TotalVMs++
			if m.State == pb_types.MachineState_RUNNING || m.State == pb_types.MachineState_PAUSED {
				info.RunningVMs++
			}
		}
		info.StoppedVMs = info.TotalVMs - info.RunningVMs
	}
	return info
}
