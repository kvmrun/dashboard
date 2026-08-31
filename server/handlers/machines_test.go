package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/0xef53/kvmrun-dashboard/server/templates"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pages := map[string]*template.Template{
		"machines.html": template.Must(
			template.New("machines.html").ParseFS(templates.FS, "layout.html", "machines.html")),
	}
	return &Handlers{Pages: pages}
}

// TestMachinesPagesRender ensures both /machines and /machines/{name}
// render the master-detail page, and that the name URL preselects the VM
// via the data-selected attribute read by the frontend.
func TestMachinesPagesRender(t *testing.T) {
	h := newTestHandlers(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/machines", nil)
	h.MachinesList(c)

	if w.Code != http.StatusOK {
		t.Fatalf("MachinesList status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `class="vm-master"`) {
		t.Error("MachinesList: master-detail markup not found")
	}
	if !strings.Contains(body, `data-selected=""`) {
		t.Error("MachinesList: expected empty data-selected")
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/machines/web-01", nil)
	c2.Params = gin.Params{{Key: "name", Value: "web-01"}}
	h.MachineDetail(c2)

	if w2.Code != http.StatusOK {
		t.Fatalf("MachineDetail status = %d, want %d", w2.Code, http.StatusOK)
	}
	if !strings.Contains(w2.Body.String(), `data-selected="web-01"`) {
		t.Error(`MachineDetail: expected data-selected="web-01"`)
	}
}
