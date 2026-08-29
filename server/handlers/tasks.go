package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pb_tasks "github.com/0xef53/kvmrun/api/services/tasks/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// TasksListJSON lists long-running operations (migrations, backups)
// reported by the tasks service.
func (h *Handlers) TasksListJSON(c *gin.Context) {
	resp, err := h.Daemon.Tasks.List(c.Request.Context(), &pb_tasks.ListRequest{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	tasks := make([]model.TaskInfo, 0, len(resp.Tasks))
	for _, t := range resp.Tasks {
		tasks = append(tasks, model.TaskInfo{
			TaskID:    t.TaskID,
			State:     t.State.String(),
			StateDesc: t.StateDesc,
			Progress:  t.Progress,
		})
	}
	c.JSON(http.StatusOK, tasks)
}
