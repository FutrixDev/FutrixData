package mcp

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/ipc"

	"github.com/mark3labs/mcp-go/server"
)

// ServerConfig configures the MCP server.
type ServerConfig struct {
	DataPath       string
	AuthBaseURL    string
	AgentAccessKey string
}

// Serve starts the MCP server on stdio. It blocks until stdin is closed.
//
// The MCP process is a thin client: it does NOT load datasources.json,
// the auth store, or any other data file. Every tool call routes through
// the main app's IPC daemon, where the privileged GUI (or its --headless
// twin) owns the Service, the keychain, and the audit log. This is the
// only design that works for sandboxed agents (codex, claude-code) which
// can't reach the keychain to decrypt datasources.json themselves, and
// it also avoids the doubled-write hazard of two processes editing the
// same files.
//
// Authentication is enforced per-call by the daemon (via the
// agent-access-key on every tool.call op), not at MCP startup — adding a
// startup auth check would force this process to load auth state, which
// is also encrypted.
func Serve(ctx context.Context, cfg ServerConfig) error {
	mcpServer := server.NewMCPServer(
		"futrixdata",
		"1.0.27",
		server.WithToolCapabilities(true),
	)

	dataPath := cfg.DataPath
	if dataPath == "" {
		dataPath = bootstrap.ResolveDataPath("")
	}
	// dataPath is the file path to datasources.json; the daemon publishes
	// its socket + handshake in the parent directory.
	dataDir := filepath.Dir(dataPath)

	// Long-lived IPC client. Lazy-connects on first Roundtrip; reconnects
	// after a daemon restart by closing on read errors and redialing on
	// the next call. Closed on `mcp serve` shutdown.
	client := ipc.NewClient(ipc.ClientConfig{DataDir: dataDir})
	defer client.Close()

	for _, reg := range BuildToolsWithClient(client, dataPath, cfg.AgentAccessKey, cfg.AuthBaseURL) {
		mcpServer.AddTool(reg.Tool, reg.Handler)
	}

	// Redirect any log output to stderr (stdout is the MCP transport).
	logger := log.New(os.Stderr, "[futrixdata-mcp] ", log.LstdFlags)

	return server.ServeStdio(mcpServer, server.WithErrorLogger(logger))
}
