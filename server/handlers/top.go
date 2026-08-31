package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TopIndex renders the top (live resource usage) page. The content is a
// placeholder until the page is built out.
func (h *Handlers) TopIndex(c *gin.Context) {
	h.render(c, "top.html", http.StatusOK, gin.H{"Title": "Top", "Page": "top"})
}
