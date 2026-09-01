package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/model"
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

// TestNetworkScheme checks that the proto -> model mapping for
// NetworkSchemeOpts follows the same rules as the "vmm nets" output.
func TestNetworkScheme(t *testing.T) {
	tests := []struct {
		name string
		opts *pb_types.NetworkSchemeOpts
		want model.NetworkScheme
	}{
		{
			name: "routed",
			opts: &pb_types.NetworkSchemeOpts{
				Ifname:   "vm_k8s52bce",
				Addrs:    []string{"2a01:5560:1001:2371::1/64", "45.86.181.154/32"},
				Gateway4: "10.11.11.11",
				Gateway6: "auto",
				Attrs: &pb_types.NetworkSchemeOpts_Router{
					Router: &pb_types.NetworkSchemeOpts_Attrs_Router{
						BindInterface: "bond0.12",
						InLimit:       1024,
						OutLimit:      1024,
					},
				},
			},
			want: model.NetworkScheme{
				Ifname:        "vm_k8s52bce",
				Type:          "routed",
				MTU:           1500,
				Addrs:         []string{"2a01:5560:1001:2371::1/64", "45.86.181.154/32"},
				Gateway4:      "10.11.11.11",
				Gateway6:      "auto",
				BindInterface: "bond0.12",
				InLimit:       1024,
				OutLimit:      1024,
			},
		},
		{
			name: "vxlan with explicit mtu",
			opts: &pb_types.NetworkSchemeOpts{
				Ifname: "t_k8s52bce",
				MTU:    9000,
				Addrs:  []string{"100.106.0.252/24"},
				Attrs: &pb_types.NetworkSchemeOpts_Vxlan{
					Vxlan: &pb_types.NetworkSchemeOpts_Attrs_VxLAN{
						BindInterface: "bond0.14",
						VNI:           1093,
					},
				},
			},
			want: model.NetworkScheme{
				Ifname:        "t_k8s52bce",
				Type:          "vxlan",
				MTU:           9000,
				Addrs:         []string{"100.106.0.252/24"},
				BindInterface: "bond0.14",
				VNI:           1093,
			},
		},
		{
			name: "vlan",
			opts: &pb_types.NetworkSchemeOpts{
				Ifname: "v_k8s52bce",
				Addrs:  []string{"10.0.0.5/24"},
				Attrs: &pb_types.NetworkSchemeOpts_Vlan{
					Vlan: &pb_types.NetworkSchemeOpts_Attrs_VLAN{
						VlanID:         12,
						ParentInterface: "bond0.12",
					},
				},
			},
			want: model.NetworkScheme{
				Ifname:          "v_k8s52bce",
				Type:            "vlan",
				MTU:             1500,
				Addrs:           []string{"10.0.0.5/24"},
				VlanID:          12,
				ParentInterface: "bond0.12",
			},
		},
		{
			name: "bridge",
			opts: &pb_types.NetworkSchemeOpts{
				Ifname: "b_k8s52bce",
				Addrs:  []string{"100.127.80.2/16"},
				Attrs: &pb_types.NetworkSchemeOpts_Bridge{
					Bridge: &pb_types.NetworkSchemeOpts_Attrs_Bridge{
						BridgeName: "br-0",
					},
				},
			},
			want: model.NetworkScheme{
				Ifname:     "b_k8s52bce",
				Type:       "bridge",
				MTU:        1500,
				Addrs:      []string{"100.127.80.2/16"},
				BridgeName: "br-0",
			},
		},
		{
			name: "manual without attrs",
			opts: &pb_types.NetworkSchemeOpts{
				Ifname: "manual0",
				MTU:    1400,
			},
			want: model.NetworkScheme{
				Ifname: "manual0",
				Type:   "manual",
				MTU:    1400,
				Addrs:  []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := networkScheme(tt.opts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("networkScheme() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
