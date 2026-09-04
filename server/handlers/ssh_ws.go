package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mdlayher/vsock"
	"golang.org/x/crypto/ssh"

	log "github.com/sirupsen/logrus"

	"github.com/0xef53/kvmrun-dashboard/internal/agent"
)

// SSH console: a WebSocket proxy to the guest agent's built-in SSH server
// (AF_VSOCK). The browser runs an xterm.js terminal; terminal I/O travels
// as binary WebSocket frames, terminal resizes travel as JSON text frames
// ({"type":"resize","cols":N,"rows":N}). The SSH user key is fetched from
// the guest agent's gRPC endpoint (vsock port 8383, mutual TLS) — the
// agent regenerates the key on every start.
var (
	// vsock dialing is retried for a short while in case the guest agent
	// is still coming up when the console is opened.
	sshDialAttempts = 20
	sshDialDelay    = 250 * time.Millisecond

	// sshChunkSize bounds the WebSocket message size used when pumping
	// bytes from the SSH session to the browser.
	sshChunkSize = 32 * 1024

	// sshSetupTimeout bounds the whole setup phase (key fetch, vsock dial,
	// SSH handshake) before the WebSocket upgrade.
	sshSetupTimeout = 15 * time.Second
)

// vsockDialFunc dials a VM's vsock endpoint. Package-level variable so
// tests can substitute a plain TCP dialer.
var vsockDialFunc = func(cid uint32, port uint32, cfg *vsock.Config) (net.Conn, error) {
	return vsock.Dial(cid, port, cfg)
}

// agentKeyFunc fetches the SSH user key from the guest agent. Package-level
// variable so tests can stub it.
var agentKeyFunc = agent.GetUserKey

var sshUpgrader = websocket.Upgrader{
	ReadBufferSize:  sshChunkSize,
	WriteBufferSize: sshChunkSize,
}

// SSHProxyWS is the WebSocket endpoint of the agent built-in SSH console:
// it fetches the SSH user key from the guest agent, opens an SSH shell
// session to the VM over AF_VSOCK and proxies terminal I/O between it and
// the xterm.js client in the browser.
func (h *Handlers) SSHProxyWS(c *gin.Context) {
	name := c.Param("name")

	detail, err := h.getMachine(c.Request.Context(), name)
	if err != nil {
		log.WithError(err).WithField("machine", name).
			Warn("ssh console: machine not found")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if !strings.EqualFold(detail.State, "RUNNING") {
		log.WithField("machine", name).WithField("state", detail.State).
			Warn("ssh console: VM is not running")
		c.JSON(http.StatusBadRequest,
			gin.H{"error": "SSH console is available only for running VMs"})
		return
	}
	cid := detail.VsockCid
	if cid == 0 {
		log.WithField("machine", name).
			Warn("ssh console: VM has no vsock device")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the VM has no vsock device, the agent built-in SSH is not available",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), sshSetupTimeout)
	defer cancel()

	// 1. Fetch the SSH user key from the agent (gRPC over vsock, mTLS).
	key, err := agentKeyFunc(ctx, cid)
	if err != nil {
		log.WithError(err).WithField("machine", name).
			Error("ssh console: cannot fetch the user key from the guest agent")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// 2. Dial the agent's built-in SSH server (vsock port 4949).
	conn, err := dialVsock(ctx, cid)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"machine": name, "cid": cid}).
			Error("ssh console: cannot reach the agent SSH server")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer conn.Close()

	// 3. Open the SSH connection and a shell session.
	session, client, pipes, err := openSSHSession(ctx, conn, key)
	if err != nil {
		log.WithError(err).WithField("machine", name).
			Error("ssh console: SSH handshake failed")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer client.Close()
	defer session.Close()
	defer pipes.close()

	// 4. Upgrade the request to a WebSocket and proxy terminal I/O.
	ws, err := sshUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // the upgrader has already written an HTTP error response
	}
	defer ws.Close()

	proxySSH(session, ws, pipes)
}

// dialVsock connects to the agent's built-in SSH server, retrying for a
// short while in case the agent is still starting up.
func dialVsock(ctx context.Context, cid uint32) (net.Conn, error) {
	var lastErr error
	for i := 0; i < sshDialAttempts; i++ {
		conn, err := vsockDialFunc(cid, agent.SSHPort, nil)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sshDialDelay):
		}
	}
	return nil, fmt.Errorf(
		"cannot reach the agent SSH server on vsock %d:%d: %w",
		cid, agent.SSHPort, lastErr)
}

// sshPipes wires an SSH session's stdio to the WebSocket pump. The pipes
// must be attached to the session before Shell() is called: x/crypto/ssh
// starts its stdio copy goroutines when the session starts and silently
// skips a stream whose reader/writer is still nil.
type sshPipes struct {
	// out carries terminal output from the SSH session (pty) to the pump.
	outR *io.PipeReader
	outW *io.PipeWriter
	// in carries terminal input from the pump to the SSH session.
	inR *io.PipeReader
	inW *io.PipeWriter
}

func (p *sshPipes) close() {
	_ = p.outR.Close()
	_ = p.outW.Close()
	_ = p.inR.Close()
	_ = p.inW.Close()
}

// openSSHSession runs the SSH handshake on conn and opens an interactive
// shell (pty) session. The agent regenerates the user and host keys on
// every start, so host key checking is not possible; the connection is
// machine-local (AF_VSOCK) and authenticated by the freshly fetched user
// key.
func openSSHSession(ctx context.Context, conn net.Conn, key []byte) (*ssh.Session, *ssh.Client, *sshPipes, error) {
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse agent SSH key: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	ncc, chans, reqs, err := ssh.NewClientConn(conn, "agent-ssh", cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ssh handshake: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})

	client := ssh.NewClient(ncc, chans, reqs)

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, nil, nil, fmt.Errorf("ssh session: %w", err)
	}

	// A pty merges stdout and stderr, so one output stream suffices.
	pipes := &sshPipes{}
	pipes.outR, pipes.outW = io.Pipe()
	pipes.inR, pipes.inW = io.Pipe()
	session.Stdout = pipes.outW
	session.Stdin = pipes.inR

	// Start an interactive pty shell with a default size; the browser
	// sends resize updates as JSON text frames (see proxySSH).
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.ICANON: 1}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("request pty: %w", err)
	}
	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return nil, nil, nil, fmt.Errorf("start shell: %w", err)
	}
	return session, client, pipes, nil
}

// proxySSH pumps bytes between the SSH session (pty) and the xterm.js
// client (WebSocket) until either side closes or errors out, then tears
// both ends down. Binary WebSocket frames carry terminal input; text
// frames are control messages ({"type":"resize","cols":N,"rows":N}).
func proxySSH(session *ssh.Session, ws *websocket.Conn, pipes *sshPipes) {
	done := make(chan struct{}, 2)

	// SSH (pty) -> browser (WS).
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, sshChunkSize)
		for {
			n, err := pipes.outR.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Browser (WS) -> SSH (pty).
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mtype, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			switch mtype {
			case websocket.BinaryMessage:
				if _, werr := pipes.inW.Write(data); werr != nil {
					return
				}
			case websocket.TextMessage:
				// A malformed control frame must not kill the console:
				// log and drop it.
				if werr := handleSSHControl(session, data); werr != nil {
					log.WithError(werr).Warn("ssh console: dropping control frame")
				}
			}
		}
	}()

	<-done

	// Tell the client the session ended (safe to call concurrently), then
	// close both ends; the other goroutine observes the closed connection
	// and exits.
	_ = ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	_ = session.Close()
	_ = ws.Close()
}

// handleSSHControl processes a JSON control frame from the browser.
func handleSSHControl(session *ssh.Session, data []byte) error {
	var msg struct {
		Type string `json:"type"`
		Cols uint   `json:"cols"`
		Rows uint   `json:"rows"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	if msg.Type != "resize" {
		return fmt.Errorf("unknown message type %q", msg.Type)
	}
	if msg.Cols == 0 || msg.Rows == 0 || msg.Cols > 500 || msg.Rows > 300 {
		return fmt.Errorf("invalid terminal size %dx%d", msg.Rows, msg.Cols)
	}
	return session.WindowChange(int(msg.Rows), int(msg.Cols))
}
