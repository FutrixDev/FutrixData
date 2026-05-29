package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolRegistration pairs an MCP tool definition with its handler.
type ToolRegistration struct {
	Tool    gomcp.Tool
	Handler server.ToolHandlerFunc
}

// BuildToolsWithClient is the production constructor: every tool handler
// routes through `client` to the main app's IPC daemon. There is no
// in-process fallback. Sandboxed agents (codex, claude-code) running this
// MCP binary cannot reach the keychain to decrypt datasources.json, and a
// divergent local Service would clobber the GUI's writes — so the only
// supported path is daemon round-trip plus a single spawn-and-retry on
// cold-start. dataPath is the file path to datasources.json (sibling files
// like the handshake live next to it); accessKey is the agent identity
// minted at install time.
func BuildToolsWithClient(client *ipc.Client, dataPath, accessKey, authBaseURL string) []ToolRegistration {
	return buildToolsWithClient(nil, client, dataPath, accessKey, authBaseURL)
}

func buildToolsWithClient(svc toolreg.Service, client *ipc.Client, dataPath, accessKey, authBaseURL string) []ToolRegistration {
	defs := toolreg.AllTools()
	regs := make([]ToolRegistration, 0, len(defs))
	for _, def := range defs {
		def := def
		regs = append(regs, ToolRegistration{
			Tool:    buildMCPSchema(def),
			Handler: makeHandlerWithClient(def, svc, client, dataPath, accessKey, authBaseURL),
		})
	}
	return regs
}

// makeHandler is the legacy 2-arg constructor used by tools_trust_test.go
// to validate in-process gate behavior. It pins the no-IPC path so existing
// trust/approval/danger-mode tests keep exercising the dispatch logic
// directly without standing up a daemon. Production code does not use this
// constructor.
func makeHandler(def toolreg.ToolDef, svc toolreg.Service, extras ...string) server.ToolHandlerFunc {
	dataPath := ""
	accessKey := ""
	if len(extras) > 0 {
		dataPath = extras[0]
	}
	if len(extras) > 1 {
		accessKey = extras[1]
	}
	return makeHandlerWithClient(def, svc, nil, dataPath, accessKey, "")
}

func makeHandlerWithClient(def toolreg.ToolDef, svc toolreg.Service, client *ipc.Client, dataPath, accessKey, authBaseURL string) server.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		params := req.GetArguments()
		if params == nil {
			params = map[string]any{}
		}
		if _, ok := params["approve"]; ok {
			return rejectApproveArgument(dataPath, accessKey, svc, def, params), nil
		}

		// Production: client is non-nil. Every call routes through the
		// daemon — same gate logic, same audit row, no divergent local
		// Service. dispatchViaDaemon is total: it always returns a
		// renderable result (success, approval-gated, or error).
		if client != nil {
			return dispatchViaDaemon(ctx, client, dataPath, accessKey, authBaseURL, def, params), nil
		}

		// Tests only: tools_trust_test wires a stub Service with no client
		// and no access key to validate the approval/trust gate logic
		// directly. Production never reaches this branch.
		return dispatchInProcessUnauthenticated(ctx, svc, def, params), nil
	}
}

func rejectApproveArgument(dataPath, accessKey string, svc toolreg.Service, def toolreg.ToolDef, params map[string]any) *gomcp.CallToolResult {
	msg := `The "approve" parameter is rejected for MCP tool calls. Third-party agents cannot approve FutrixData operations.`
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return errorResult(msg)
	}
	source := string(toolexec.SourceMCP)
	identity, err := agentaudit.CheckAccess(dataPath, trimmedKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(dataPath, svc, source, trimmedKey, def.Name, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(dataPath, nil, source, trimmedKey, def.Name, params, agentaudit.StatusError, err.Error())
		}
		return errorResult(err.Error())
	}
	if def.Name == "list_datasources" {
		if err := agentaudit.CheckDatasourceInventoryScope(identity); err != nil {
			message := "agent " + agentaudit.MaskAccessKey(trimmedKey) + " cannot list all datasources: " + err.Error()
			_ = agentaudit.AppendToolCall(dataPath, nil, source, trimmedKey, def.Name, params, agentaudit.StatusError, message)
			return errorResult(message)
		}
	}
	if dsID := toolreg.DatasourceIDFromToolDef(def, params); dsID != "" {
		if err := agentaudit.CheckDatasourceScope(identity, dsID); err != nil {
			message := "agent " + agentaudit.MaskAccessKey(trimmedKey) + " cannot access datasource " + dsID + ": " + err.Error()
			_ = agentaudit.AppendToolCall(dataPath, nil, source, trimmedKey, def.Name, params, agentaudit.StatusError, message)
			return errorResult(message)
		}
	}
	_ = agentaudit.AppendToolCall(dataPath, svc, source, trimmedKey, def.Name, params, agentaudit.StatusError, msg)
	return errorResult(msg)
}

func datasourceIDFromParams(p map[string]any) string {
	return toolreg.DatasourceIDFromParams(p)
}

func successResult(data any) *gomcp.CallToolResult {
	text, _ := json.MarshalIndent(datasourceops.RedactValue(data), "", "  ")
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{
			gomcp.TextContent{
				Type: "text",
				Text: string(text),
			},
		},
	}
}

func errorResult(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{
			gomcp.TextContent{
				Type: "text",
				Text: msg,
			},
		},
	}
}

func approvalRequiredResult(toolName string, params map[string]any) *gomcp.CallToolResult {
	return errorResult(toolexec.AgentApprovalRejectedMessage(toolName, params))
}
