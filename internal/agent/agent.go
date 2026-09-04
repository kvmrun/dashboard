// Package agent talks to the guest's phoenix-guest-agent over AF_VSOCK:
// it fetches the SSH user key from the agent's gRPC endpoint (vsock port
// 8383, mutual TLS) and dials the agent's built-in SSH server (vsock port
// 4949) for the shell console.
package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	grpc "google.golang.org/grpc"
	grpc_credentials "google.golang.org/grpc/credentials"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/mdlayher/vsock"

	"github.com/0xef53/kvmrun-dashboard/cert"
	secure_shell "github.com/0xef53/kvmrun-dashboard/internal/agent/secure_shell"
)

// Vsock ports of the guest agent endpoints (phoenix-guest-agent core.GRPCPort
// and core.RCPPort).
const (
	GRPCPort = 8383
	SSHPort  = 4949
)

// The guest agent uses its own PKI (separate from the kvmrund daemon's):
// the client cert and CA are embedded in the binary (see package cert). The
// private key files are intentionally not committed to the repository — the
// build host must keep them next to embed.go so go:embed can pick them up.
var (
	agentTLSOnce  sync.Once
	agentTLSValue *tls.Config
	agentTLSErr   error
)

// agentTLSConfig builds the mutual-TLS client configuration for talking to
// the guest agent, from the embedded agent PKI.
//
// InsecureSkipVerify mirrors the pga client (client/client.go): the agent's
// server certificate is issued for names like "guest-agent" or "localhost"
// that never match the vsock dial target, so the certificate name check
// would always fail. Skipping server verification is acceptable here:
// AF_VSOCK is not exposed to the network, and the agent still requires our
// client certificate (mTLS via RequireAndVerifyClientCert).
func agentTLSConfig() (*tls.Config, error) {
	agentTLSOnce.Do(func() {
		cfg, err := cert.NewClientConfig(cert.EmbedStore)
		if err != nil {
			agentTLSErr = fmt.Errorf("load embedded agent certificates: %w", err)
			return
		}
		cfg.InsecureSkipVerify = true
		agentTLSValue = cfg
	})
	return agentTLSValue, agentTLSErr
}

// GetUserKey asks the guest agent for the SSH user key (PEM-encoded private
// key). The agent generates a fresh key on every start, so it must be
// fetched before each SSH connection. The client certificate of the embedded
// agent PKI is presented via mutual TLS.
func GetUserKey(ctx context.Context, cid uint32) ([]byte, error) {
	tlsConfig, err := agentTLSConfig()
	if err != nil {
		return nil, err
	}

	// The "passthrough" scheme makes gRPC hand the target to the dialer
	// as-is instead of resolving it via DNS (the default for schemeless
	// targets in grpc.NewClient).
	conn, err := grpc.NewClient(
		fmt.Sprintf("passthrough:cid:%d", cid),
		grpc.WithTransportCredentials(grpc_credentials.NewTLS(tlsConfig)),
		grpc.WithDialer(func(_ string, _ time.Duration) (net.Conn, error) {
			return vsock.Dial(cid, GRPCPort, nil)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("guest agent gRPC dial: %w", err)
	}
	defer conn.Close()

	client := secure_shell.NewAgentSecureShellServiceClient(conn)
	resp, err := client.GetUserKey(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("guest agent GetUserKey: %w", err)
	}
	if len(resp.Key) == 0 {
		return nil, fmt.Errorf("guest agent returned an empty SSH key")
	}
	return resp.Key, nil
}
