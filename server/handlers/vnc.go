package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
)

// VNCActivateJSON activates the VNC server for a VM — the "vmm vnc activate"
// equivalent. JSON-only: the response contains the generated VNC password,
// which must not end up in URLs or HTML pages.
func (h *Handlers) VNCActivateJSON(c *gin.Context) {
	name := c.Param("name")
	req := &pb_machines.VNCActivateRequest{Name: name, Password: c.PostForm("password")}
	resp, err := h.Daemon.Machines.VNCActivate(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	out := gin.H{}
	if reqs := resp.Requisites; reqs != nil {
		out = gin.H{
			"password": reqs.Password,
			"display":  reqs.Display,
			"port":     reqs.Port,
			"wsPort":   reqs.WSPort,
		}
	}
	c.JSON(http.StatusOK, out)
}
