package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mdlayher/vsock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"

	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_types "github.com/0xef53/kvmrun/api/types/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
)

// fakeMachineServiceSSH is a MachineServiceClient stub returning a
// fixed machine (or NotFound when machine is nil).
type fakeMachineServiceSSH struct {
	pb_machines.MachineServiceClient
	machine *pb_types.Machine
}

func (f fakeMachineServiceSSH) Get(ctx context.Context, in *pb_machines.GetRequest, _ ...grpc.CallOption) (*pb_machines.GetResponse, error) {
	_ = ctx
	if f.machine == nil {
		return nil, errors.New(`machine "` + in.Name + `" not found`)
	}
	return &pb_machines.GetResponse{Machine: f.machine}, nil
}

// sshStub is a real x/crypto/ssh server on 127.0.0.1 that authenticates the
// client with a fixed ECDSA user key, accepts a pty+shell session, writes
// a banner and echoes stdin back. It also records window-change requests so
// the resize path can be asserted.
type sshStub struct {
	ln      net.Listener
	userPEM []byte

	mu     sync.Mutex
	resize [2]int // rows, cols of the last window-change
}

func startSSHStub(t *testing.T) *sshStub {
	t.Helper()

	userPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate user key: %v", err)
	}
	userSigner, err := ssh.NewSignerFromKey(userPriv)
	if err != nil {
		t.Fatalf("ssh signer: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(userPriv, "")
	if err != nil {
		t.Fatalf("marshal user key: %v", err)
	}

	hostPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), userSigner.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unauthorized public key")
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	s := &sshStub{ln: ln, userPEM: pem.EncodeToMemory(block)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn, cfg)
		}
	}()
	return s
}

func (s *sshStub) serve(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newCh.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			// Echo stdin back so the test can observe the full round trip.
			go func() {
				buf := make([]byte, 4096)
				for {
					n, rerr := channel.Read(buf)
					if n > 0 {
						_, _ = channel.Write(buf[:n])
					}
					if rerr != nil {
						return
					}
				}
			}()
			for req := range requests {
				switch req.Type {
				case "pty-req":
					_ = req.Reply(true, nil)
				case "window-change":
					var wc struct {
						Cols   uint32
						Rows   uint32
						Width  uint32
						Height uint32
					}
					if err := ssh.Unmarshal(req.Payload, &wc); err == nil {
						s.mu.Lock()
						s.resize = [2]int{int(wc.Rows), int(wc.Cols)}
						s.mu.Unlock()
					}
					_ = req.Reply(true, nil)
				case "shell":
					_ = req.Reply(true, nil)
					_, _ = channel.Write([]byte("SSH-STUB banner\n$ "))
				default:
					_ = req.Reply(false, nil)
				}
			}
		}()
	}
}

// waitResize polls the stub for the expected window-change.
func (s *sshStub) waitResize(t *testing.T, rows, cols int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := s.resize
		s.mu.Unlock()
		if got == [2]int{rows, cols} {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("window-change %dx%d not observed by the SSH stub", rows, cols)
}

// newSSHProxyEngine wires the SSH proxy route with the given machine.
func newSSHProxyEngine(m *pb_types.Machine) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &Handlers{
		Daemon: &daemon.Client{Machines: fakeMachineServiceSSH{machine: m}},
	}
	e := gin.New()
	e.GET("/machines/:name/ssh-ws", h.SSHProxyWS)
	return e
}

func vsockMachine(name string, state pb_types.MachineState, cid uint32) *pb_types.Machine {
	m := &pb_types.Machine{Name: name, State: state}
	if cid != 0 {
		m.Config = &pb_types.MachineOpts{
			VsockDevice: &pb_types.MachineOpts_ChannelVSock{ContextID: cid},
		}
	}
	return m
}

// stubVsockDial redirects the handler's vsock dial and key fetch to the
// stub's TCP listener and key.
func stubVsockDial(t *testing.T, s *sshStub) {
	t.Helper()
	oldDial, oldKey := vsockDialFunc, agentKeyFunc
	vsockDialFunc = func(_ uint32, _ uint32, _ *vsock.Config) (net.Conn, error) {
		return net.DialTimeout("tcp", s.ln.Addr().String(), 2*time.Second)
	}
	agentKeyFunc = func(_ context.Context, _ uint32) ([]byte, error) {
		return s.userPEM, nil
	}
	t.Cleanup(func() { vsockDialFunc, agentKeyFunc = oldDial, oldKey })
}

// TestSSHProxyWS_HappyPath: full round trip through a real (stub) SSH
// server — banner in, echoed input out, resize propagated as window-change.
func TestSSHProxyWS_HappyPath(t *testing.T) {
	s := startSSHStub(t)
	stubVsockDial(t, s)

	srv := httptest.NewServer(newSSHProxyEngine(
		vsockMachine("web-01", pb_types.MachineState_RUNNING, 3)))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/machines/web-01/ssh-ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mtype, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read banner: %v", err)
	}
	// The banner and the echo may be split across frames; read until the
	// banner is seen.
	found := strings.Contains(string(data), "SSH-STUB banner")
	deadline := time.Now().Add(3 * time.Second)
	for !found && time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		_, data, err = conn.ReadMessage()
		if err != nil {
			t.Fatalf("read banner: %v", err)
		}
		found = strings.Contains(string(data), "SSH-STUB banner")
	}
	if mtype != websocket.BinaryMessage || !found {
		t.Fatalf("banner = %q (type %d), want to contain %q", data, mtype, "SSH-STUB banner")
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("echo hello\r")); err != nil {
		t.Fatalf("write: %v", err)
	}
	found = false
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		_, data, err = conn.ReadMessage()
		if err != nil {
			t.Fatalf("read echo: %v", err)
		}
		if found = strings.Contains(string(data), "echo hello"); found {
			break
		}
	}
	if !found {
		t.Fatalf("echo %q never seen, last frame %q", "echo hello", data)
	}

	if err := conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"resize","cols":120,"rows":40}`)); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	s.waitResize(t, 40, 120)
}

// TestSSHProxyWS_NotRunning: a stopped VM must be rejected before any
// vsock dial attempt.
func TestSSHProxyWS_NotRunning(t *testing.T) {
	srv := httptest.NewServer(newSSHProxyEngine(
		vsockMachine("web-01", pb_types.MachineState_SHUTDOWN, 3)))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/machines/web-01/ssh-ws")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}

// TestSSHProxyWS_NoVsock: a running VM without a vsock device must be
// rejected (the agent SSH is only reachable over vsock).
func TestSSHProxyWS_NoVsock(t *testing.T) {
	srv := httptest.NewServer(newSSHProxyEngine(
		vsockMachine("web-01", pb_types.MachineState_RUNNING, 0)))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/machines/web-01/ssh-ws")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}

// TestSSHProxyWS_AuthError: a valid connection setup but a wrong user key
// must surface as a 502 (the WS is never upgraded).
func TestSSHProxyWS_AuthError(t *testing.T) {
	s := startSSHStub(t)
	oldDial := vsockDialFunc
	vsockDialFunc = func(_ uint32, _ uint32, _ *vsock.Config) (net.Conn, error) {
		return net.DialTimeout("tcp", s.ln.Addr().String(), 2*time.Second)
	}
	t.Cleanup(func() { vsockDialFunc = oldDial })

	otherPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(otherPriv, "")
	if err != nil {
		t.Fatal(err)
	}
	oldKey := agentKeyFunc
	agentKeyFunc = func(_ context.Context, _ uint32) ([]byte, error) {
		return pem.EncodeToMemory(block), nil
	}
	t.Cleanup(func() { agentKeyFunc = oldKey })

	srv := httptest.NewServer(newSSHProxyEngine(
		vsockMachine("web-01", pb_types.MachineState_RUNNING, 3)))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/machines/web-01/ssh-ws")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadGateway)
	}
}
