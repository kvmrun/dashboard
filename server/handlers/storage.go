package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DisksListJSON lists the block devices attached to a VM — the
// "vmm storage list" equivalent.
//
// TODO: implement via Daemon.Machines (Disk* RPCs) — next iteration.
func (h *Handlers) DisksListJSON(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet"})
}
