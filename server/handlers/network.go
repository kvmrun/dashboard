package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NetworkListJSON lists the host network configuration (bridges, VLANs,
// VXLAN tunnels) — the network service / vnetctl equivalent.
//
// TODO: implement via Daemon.Network (ListEndpoints) — next iteration.
func (h *Handlers) NetworkListJSON(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet"})
}
