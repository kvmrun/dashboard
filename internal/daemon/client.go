// Package daemon provides a long-lived gRPC client to the kvmrund daemon.
//
// The connection is built on top of the go-grpc client helpers, which
// attach the standard request-identifier and request-logging interceptors.
// The dashboard keeps a single connection for its whole lifetime; kvmrun
// operations are executed through service clients created on top of it.
package daemon

import (
	"crypto/tls"
	"fmt"

	grpcclient "github.com/0xef53/go-grpc/client"
	log "github.com/sirupsen/logrus"

	pb_cloudinit "github.com/0xef53/kvmrun/api/services/cloudinit/v2"
	pb_hardware "github.com/0xef53/kvmrun/api/services/hardware/v2"
	pb_machines "github.com/0xef53/kvmrun/api/services/machines/v2"
	pb_misc "github.com/0xef53/kvmrun/api/services/misc/v2"
	pb_network "github.com/0xef53/kvmrun/api/services/network/v2"
	pb_system "github.com/0xef53/kvmrun/api/services/system/v2"
	pb_tasks "github.com/0xef53/kvmrun/api/services/tasks/v2"

	"google.golang.org/grpc"
)

var logger = log.StandardLogger().WithField("subsystem", "grpc-client")

func init() {
	// go-grpc routes its request logging through the logger set here.
	grpcclient.SetLogger(logger)
}

// Client is a long-lived connection to a single kvmrund daemon instance
// together with the generated service clients for it. Safe for concurrent
// use.
type Client struct {
	conn *grpc.ClientConn

	CloudInit pb_cloudinit.CloudInitServiceClient
	Hardware  pb_hardware.HardwareServiceClient
	Machines  pb_machines.MachineServiceClient
	Misc      pb_misc.MiscServiceClient
	Network   pb_network.NetworkServiceClient
	System    pb_system.SystemServiceClient
	Tasks     pb_tasks.TaskServiceClient
}

// New dials the kvmrund daemon at addr. addr is a host:port pair or an
// abstract/unix socket path (e.g. "unix:@/run/kvmrund.sock"). When
// tlsConfig is non-nil, mutual TLS is used — the same transport the vmm
// CLI uses.
func New(addr string, tlsConfig *tls.Config) (*Client, error) {
	var (
		conn *grpc.ClientConn
		err  error
	)

	if tlsConfig == nil {
		conn, err = grpcclient.NewInsecureConnection(addr)
	} else {
		conn, err = grpcclient.NewSecureConnection(addr, tlsConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to kvmrund at %q: %w", addr, err)
	}

	return &Client{
		conn:      conn,
		CloudInit: pb_cloudinit.NewCloudInitServiceClient(conn),
		Hardware:  pb_hardware.NewHardwareServiceClient(conn),
		Machines:  pb_machines.NewMachineServiceClient(conn),
		Misc:      pb_misc.NewMiscServiceClient(conn),
		Network:   pb_network.NewNetworkServiceClient(conn),
		System:    pb_system.NewSystemServiceClient(conn),
		Tasks:     pb_tasks.NewTaskServiceClient(conn),
	}, nil
}

// Conn returns the underlying gRPC connection so that service clients
// (generated from the kvmrun .proto files) can be created on top of it.
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
