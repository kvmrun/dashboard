package templates

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
)

// TestPagesExecute ensures every page template parses together with the
// shared layout and renders with the data the handlers pass — the same
// pairing the server builds at startup.
func TestPagesExecute(t *testing.T) {
	base := map[string]any{
		"User":             "admin",
		"KvmrunVersion":    "1.0.1",
		"DashboardVersion": "0.1.0",
	}

	pages := map[string]map[string]any{
		"machines.html": {
			"Title":    "Machines",
			"Page":     "machines",
			"Machines": []model.MachineSummary{{Name: "vm1", State: "running"}},
		},
		"machine_detail.html": {
			"Title":   "vm1",
			"Page":    "machines",
			"Name":    "vm1",
			"Machine": &model.MachineDetail{Name: "vm1", State: "running"},
		},
		"system.html": {
			"Title": "Overview",
			"Page":  "home",
			"Info":  model.SystemInfo{GoVersion: "go1.24"},
		},
		"tasks.html": {"Title": "Tasks", "Page": "tasks"},
		"top.html":   {"Title": "Top", "Page": "top"},
	}

	for name, extra := range pages {
		data := make(map[string]any, len(base)+len(extra))
		for k, v := range base {
			data[k] = v
		}
		for k, v := range extra {
			data[k] = v
		}
		tmpl := template.Must(template.New(name).ParseFS(FS, "layout.html", name))
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
