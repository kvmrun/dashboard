// Package version holds the version strings shown in the dashboard's
// header: the dashboard build and the kvmrun daemon it talks to.
package version

const (
	// Dashboard is the version of the dashboard build.
	Dashboard = "0.1.0"

	// Kvmrun is the version of the kvmrun daemon the dashboard is built
	// against. Hardcoded for now — the daemon does not expose its version
	// over gRPC yet.
	Kvmrun = "1.0.1"
)
