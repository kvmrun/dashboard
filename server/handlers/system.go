package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// SystemIndex renders the main dashboard page with daemon configuration.
func (h *Handlers) SystemIndex(c *gin.Context) {
	h.render(c, "system.html", http.StatusOK,
		gin.H{"Title": "System", "Info": h.systemInfo(c)})
}

// SystemJSON returns daemon/host information — the system service equivalent.
func (h *Handlers) SystemJSON(c *gin.Context) {
	c.JSON(http.StatusOK, h.systemInfo(c))
}

// systemInfo reads the daemon's application configuration. A daemon outage
// degrades to the minimal (Go-version-only) info instead of failing.
func (h *Handlers) systemInfo(c *gin.Context) model.SystemInfo {
	info := model.SystemInfo{GoVersion: runtime.Version()}
	resp, err := h.Daemon.System.GetAppConf(c.Request.Context(), &emptypb.Empty{})
	if err == nil && resp.AppConf != nil && resp.AppConf.Kvmrun != nil {
		info.QemuRootdir = resp.AppConf.Kvmrun.QemuRootdir
		info.CertDir = resp.AppConf.Kvmrun.CertDir
	}
	return info
}
