package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	log "github.com/sirupsen/logrus"
)

// Dialing the VM's VNC server: the daemon may need a short moment to bring
// the VNC server up after activation, so the proxy retries the local dial.
// The values are package-level (not const) so tests can shorten them.
var (
	// vncDialHosts are the loopback addresses the VM's VNC server may be
	// bound to, tried in round-robin. The kvmrun daemon binds QEMU's VNC
	// server to 127.0.0.2 by default (the default of
	// kvmrun.CommandLineFeatures.VNCHost); 127.0.0.1 is kept as a fallback
	// for deployments that configure a different VNCHost.
	vncDialHosts = []string{"127.0.0.2", "127.0.0.1"}

	vncDialAttempts = 20
	vncDialDelay    = 250 * time.Millisecond

	// vncChunkSize bounds the WebSocket message size used when pumping
	// bytes from the VNC server to the browser.
	vncChunkSize = 32 * 1024
)

// vncUpgrader upgrades the HTTP request to a WebSocket. CheckOrigin is left
// at the gorilla default: the noVNC client is always served from the same
// origin as the dashboard.
var vncUpgrader = websocket.Upgrader{
	ReadBufferSize:  vncChunkSize,
	WriteBufferSize: vncChunkSize,
}

// VNCProxyWS is a mini websockify: it upgrades the request to a WebSocket
// and transparently proxies bytes between it and the VM's local VNC server
// at 127.0.0.2:port (127.0.0.1 as fallback), where port is the TCP port
// reported by VNCActivate. The embedded noVNC client (served at /novnc)
// connects here instead of an external websockify.
func (h *Handlers) VNCProxyWS(c *gin.Context) {
	name := c.Param("name")
	port, err := strconv.Atoi(c.Query("port"))
	if err != nil || port < 1 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port"})
		return
	}

	// Make sure the VM exists (and the session is allowed to see it) before
	// opening the tunnel.
	if _, err := h.getMachine(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	conn, err := dialVNC(c.Request.Context(), port)
	if err != nil {
		log.WithError(err).
			WithFields(log.Fields{"machine": name, "port": port}).
			Error("vnc proxy: cannot reach VNC server, refusing the WebSocket upgrade")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer conn.Close()

	ws, err := vncUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // the upgrader has already written an HTTP error response
	}
	defer ws.Close()

	proxyVNC(conn, ws)
}

// dialVNC connects to the VM's VNC server, retrying for a short while in
// case the VNC server is still coming up. The candidate loopback hosts are
// tried in round-robin, so the dial does not block on a single address.
func dialVNC(ctx context.Context, port int) (net.Conn, error) {
	tried := make([]string, 0, len(vncDialHosts))
	var lastErr error
	for i := 0; i < vncDialAttempts; i++ {
		addr := net.JoinHostPort(vncDialHosts[i%len(vncDialHosts)], strconv.Itoa(port))
		if len(tried) == 0 || tried[len(tried)-1] != addr {
			tried = append(tried, addr)
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(vncDialDelay):
		}
	}
	return nil, fmt.Errorf("cannot reach VNC server at %s: %w", strings.Join(tried, " / "), lastErr)
}

// proxyVNC pumps bytes between the VNC server (TCP) and the noVNC client
// (WebSocket) until either side closes or errors out, then tears both ends
// down.
func proxyVNC(conn net.Conn, ws *websocket.Conn) {
	done := make(chan struct{}, 2)

	// VNC (TCP) -> noVNC (WS).
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, vncChunkSize)
		for {
			n, err := conn.Read(buf)
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

	// noVNC (WS) -> VNC (TCP). Non-binary messages (e.g. text) are ignored;
	// pings are answered automatically by ReadMessage.
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mtype, data, err := ws.ReadMessage()
			if mtype != websocket.BinaryMessage {
				if err != nil {
					return
				}
				continue
			}
			if _, err := conn.Write(data); err != nil {
				return
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
	_ = conn.Close()
	_ = ws.Close()
}
