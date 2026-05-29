// Package version exposes the FutrixData build version that both the daemon
// and CLI compare during IPC handshake. Mismatch is a hard error: the design
// (TASK-20260425-140054) requires CLI and daemon to ship in the same package
// and share the same version.
//
// The constant defaults to "dev" for source builds; release builds inject the
// real semver via -ldflags "-X futrixdata/platform/internal/version.Version=...".
package version

// Version is the build-time-injected version string. Stable comparison key
// for the IPC handshake.
var Version = "dev"
