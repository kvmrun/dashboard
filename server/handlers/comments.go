package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pb_misc "github.com/0xef53/kvmrun/api/services/misc/v2"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MachineCommentJSON returns the comment for one VM
// (MiscService.GetMachineComment). An absent comment is an empty string,
// not an error.
func (h *Handlers) MachineCommentJSON(c *gin.Context) {
	name := c.Param("name")
	resp, err := h.Daemon.Misc.GetMachineComment(c.Request.Context(), &pb_misc.GetMachineCommentRequest{Name: name})
	if err != nil {
		// No comment stored yet — treat as an empty comment.
		if status.Code(err) == codes.NotFound {
			c.JSON(http.StatusOK, gin.H{"comment": ""})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comment": string(resp.Data)})
}

// UpdateMachineCommentJSON saves a comment for one VM
// (MiscService.UpdateMachineComment). The error text returned by kvmrun
// (e.g. "machine is in locked state") is passed through to the client
// as-is in the "error" field.
func (h *Handlers) UpdateMachineCommentJSON(c *gin.Context) {
	name := c.Param("name")

	var in struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	_, err := h.Daemon.Misc.UpdateMachineComment(c.Request.Context(),
		&pb_misc.UpdateMachineCommentRequest{Name: name, Data: []byte(in.Comment)})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
