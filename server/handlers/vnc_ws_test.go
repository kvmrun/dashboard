package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
)

// fakeMachineService is a MachineServiceClient stub: only Get is
// implemented, which is all the VNC proxy needs.
type fakeMachineService struct {
	pb_machines.MachineServiceClient
}

func (fakeMachineService) Get(ctx context.Context, in *pb_machines.GetRequest, _ ...grpc.CallOption) (*pb_machines.GetResponse, error) {
	return &pb_machines.GetResponse{
		Machine: &pb_types.Machine{Name: in.Name, State: pb_types.MachineState_RUNNING},
	}, nil
}

func newVNCProxyEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &Handlers{Daemon: &daemon.Client{Machines: fakeMachineService{}}}
	e := gin.New()
	e.GET("/machines/:name/vnc-ws", h.VNCProxyWS)
	return e
}

// startVNCStub runs a TCP server that greets like a VNC server
// ("RFB 003.008\n") and then echoes the client back, listening on
// host:0. It returns the port.
func startVNCStub(t *testing.T, host string) int {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = conn.Write([]byte("RFB 003.008\n"))
				buf := make([]byte, 1024)
				for {
					n, rerr := conn.Read(buf)
					if n > 0 {
						if _, werr := conn.Write(buf[:n]); werr != nil {
							return
						}
					}
					if rerr != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// freePort returns a port that is not currently listening on 127.0.0.1.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// TestVNCProxyWS_ProxiesData checks the full round trip: the WS client
// receives the stub's RFB greeting and the bytes it sends are echoed back
// through the proxy.
func TestVNCProxyWS_ProxiesData(t *testing.T) {
	// Stub listens on 127.0.0.1 (the fallback host): speed up the round-robin
	// delay so the test does not wait for the 127.0.0.2 attempts.
	oldDelay := vncDialDelay
	vncDialDelay = 10 * time.Millisecond
	defer func() { vncDialDelay = oldDelay }()

	port := startVNCStub(t, "127.0.0.1")
	srv := httptest.NewServer(newVNCProxyEngine())
	defer srv.Close()

	proxyRoundTrip(t, srv, port)
}

// proxyRoundTrip checks the full round trip through the proxy: the WS client
// receives the stub's RFB greeting and the bytes it sends are echoed back.
func proxyRoundTrip(t *testing.T, srv *httptest.Server, port int) {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/machines/web-01/vnc-ws?port=" + strconv.Itoa(port)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mtype, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if mtype != websocket.BinaryMessage || string(data) != "RFB 003.008\n" {
		t.Fatalf("greeting = %q (type %d), want %q", data, mtype, "RFB 003.008\n")
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("01")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mtype, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if mtype != websocket.BinaryMessage || string(data) != "01" {
		t.Fatalf("echo = %q (type %d), want %q", data, mtype, "01")
	}
}

// TestVNCProxyWS_VNCOn127002: the kvmrun daemon binds QEMU's VNC server to
// 127.0.0.2 by default (the default of CommandLineFeatures.VNCHost), so the
// proxy must reach a VNC server listening there.
func TestVNCProxyWS_VNCOn127002(t *testing.T) {
	if _, err := net.Listen("tcp", "127.0.0.2:0"); err != nil {
		t.Skipf("127.0.0.2 is not available in this environment: %v", err)
	}
	port := startVNCStub(t, "127.0.0.2")
	srv := httptest.NewServer(newVNCProxyEngine())
	defer srv.Close()

	proxyRoundTrip(t, srv, port)
}

// TestVNCProxyWS_RejectsBadPort: an invalid port query parameter must not
// open a connection.
func TestVNCProxyWS_RejectsBadPort(t *testing.T) {
	srv := httptest.NewServer(newVNCProxyEngine())
	defer srv.Close()

	for _, port := range []string{"", "0", "-1", "99999", "abc"} {
		req, err := http.NewRequest(http.MethodGet,
			srv.URL+"/machines/web-01/vnc-ws?port="+port, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET ?port=%q: %v", port, err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET ?port=%q: status = %d, want %d", port, res.StatusCode, http.StatusBadRequest)
		}
		res.Body.Close()
	}
}

// TestVNCProxyWS_VNCServerUnreachable: a valid port with no listener must
// yield a 502 after the dial retries are exhausted.
func TestVNCProxyWS_VNCServerUnreachable(t *testing.T) {
	port := freePort(t)

	oldAttempts, oldDelay := vncDialAttempts, vncDialDelay
	vncDialAttempts, vncDialDelay = 2, 10*time.Millisecond
	defer func() { vncDialAttempts, vncDialDelay = oldAttempts, oldDelay }()

	srv := httptest.NewServer(newVNCProxyEngine())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet,
		srv.URL+"/machines/web-01/vnc-ws?port="+strconv.Itoa(port), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadGateway)
	}
}
