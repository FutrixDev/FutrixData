// MCP-side dispatcher for tools/call. Every invocation routes through the
// main app's IPC daemon — there is no in-process fallback in production. If
// the socket isn't up, dispatchViaDaemon does one spawn-and-retry; failing
// that, the call surfaces an error so the agent can report a fixable
// install. The MCP process must never load datasources.json itself —
// sandboxed agents (codex, claude-code) can't reach the keychain anyway,
// and a divergent local Service would write to the same files the main app
// is editing.
//
// The legacy in-process gate (dispatchInProcessUnauthenticated) is retained
// only for tools_trust_test, which exercises the approval/danger logic
// directly without standing up a real daemon. Production handlers always
// have a non-nil *ipc.Client and never reach that path.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"

	gomcp "github.com/mark3labs/mcp-go/mcp"
)

const daemonSpawnTimeout = 3 * time.Second

// No per-tool-call wrapper timeout: tool.call rides the caller's ctx so
// long-running ops (D1OAuthLogin up to 3min, DynamoDB SSO authorization,
// slow analytics queries) don't get cut off client-side while the daemon
// keeps executing them — the mismatch produced false failures and
// duplicate-execution risk on retry. The IPC client's DialTimeout (3s)
// still bounds the connect step; the response read inherits the caller's
// ctx deadline via Client.Roundtrip's SetDeadline call.

// dispatchViaDaemon performs the agent → daemon → tool flow and always
// returns a renderable MCP CallToolResult — success, approval-gated,
// authoritative business error, or infra failure. It never falls back to
// in-process dispatch: if the daemon path can't be made to work after one
// spawn-and-retry, the failure surfaces verbatim.
func dispatchViaDaemon(ctx context.Context, client *ipc.Client, dataPath, accessKey, authBaseURL string, def toolreg.ToolDef, params map[string]any) *gomcp.CallToolResult {
	if client == nil {
		// Production must wire a client. Hitting nil here is a programming
		// bug, not a runtime fallback condition.
		return errorResult("mcp: ipc client not configured")
	}
	resp, err := mcpDaemonRoundtrip(ctx, client, def, params, accessKey)
	if err == nil {
		return decodeMCPDaemonResponse(resp, def.Name)
	}
	if !isInfraError(err) {
		return errorResult(err.Error())
	}
	// Cold start / crash recovery: trigger the spawn-and-retry path for
	// "no daemon listening" AND for raw transport drops (EOF/broken pipe/
	// connection reset). The conn-drop case fires when a previously-open
	// connection survives a daemon restart: ipc.Client.Roundtrip returns
	// the underlying network error verbatim with empty wire code, and
	// without recognising it here the very first MCP tool call after a
	// daemon crash/restart would surface as a hard failure instead of
	// recovering. Version skew and install corruption still need user
	// action and skip the spawn.
	code := ipc.ErrorCode(err)
	spawnEligible := code == ipc.CodeDaemonNotRunning || code == ipc.CodeDaemonUnreachable || ipc.IsConnDropError(err)
	if !spawnEligible {
		return errorResult(err.Error())
	}
	spawnCtx, cancel := context.WithTimeout(ctx, daemonSpawnTimeout+1*time.Second)
	defer cancel()
	// Forward the resolved dataPath so the spawned daemon serves the same
	// store the MCP process is targeting; otherwise it would default to
	// its own config, publish its handshake elsewhere, and the wait below
	// would time out against an empty directory.
	//
	// Forward AuthBaseURL too: if MCP was launched with a custom auth
	// service (config or FUTRIX_AUTH_BASE_URL), the cold-spawned daemon
	// must use the same one — otherwise EnsureAuthenticated on the very
	// next tool call would talk to the wrong environment.
	extraArgs := []string{"--data-path", dataPath}
	if v := strings.TrimSpace(authBaseURL); v != "" {
		extraArgs = append(extraArgs, "--auth-base-url", v)
	}
	spawnCfg := ipc.SpawnConfig{ExtraArgs: extraArgs}
	if spawnErr := ipc.SpawnDaemon(spawnCtx, spawnCfg); spawnErr != nil {
		return errorResult(fmt.Sprintf("spawn main app: %v", spawnErr))
	}
	// dataPath is the file path to datasources.json; handshake lives in
	// its parent directory.
	dataDir := filepath.Dir(dataPath)
	if _, hsErr := ipc.WaitForHandshake(spawnCtx, dataDir, daemonSpawnTimeout); hsErr != nil {
		return errorResult(fmt.Sprintf("main app spawned but never published handshake: %v", hsErr))
	}
	client.MarkDisconnected()
	resp2, retryErr := mcpDaemonRoundtrip(ctx, client, def, params, accessKey)
	if retryErr != nil {
		return errorResult(retryErr.Error())
	}
	return decodeMCPDaemonResponse(resp2, def.Name)
}

func mcpDaemonRoundtrip(ctx context.Context, client *ipc.Client, def toolreg.ToolDef, params map[string]any, accessKey string) (ipc.Response, error) {
	args := daemon.ToolCallArgs{
		Tool:   def.Name,
		Params: params,
		Source: string(toolexec.SourceMCP),
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return ipc.Response{}, fmt.Errorf("encode tool.call args: %w", err)
	}
	return client.Roundtrip(ctx, ipc.Request{
		Op:   "tool.call",
		ID:   def.Name,
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: accessKey},
	})
}

// decodeMCPDaemonResponse converts a daemon wire response into an MCP
// CallToolResult. The three expected shapes — success, approval-gated,
// authoritative business error — all map to MCP content blocks.
func decodeMCPDaemonResponse(resp ipc.Response, toolName string) *gomcp.CallToolResult {
	if !resp.OK {
		if resp.Error == nil {
			return errorResult("daemon returned non-OK with no error body")
		}
		if recovery, ok := resp.Error.Details["startupRecovery"]; ok {
			text, _ := json.MarshalIndent(map[string]any{
				"ok":              false,
				"error":           resp.Error.Message,
				"startupRecovery": recovery,
			}, "", "  ")
			return errorResult(string(text))
		}
		if attr := riskAttributionFromDetails(resp.Error.Details); attr != nil {
			text, err := json.MarshalIndent(map[string]any{
				"ok": false,
				"error": map[string]any{
					"message":         resp.Error.Message,
					"riskAttribution": attr,
				},
			}, "", "  ")
			if err == nil {
				return errorResult(string(text))
			}
		}
		return errorResult(resp.Error.Message)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &probe); err != nil {
		return errorResult(fmt.Sprintf("decode daemon result: %v", err))
	}
	if _, ok := probe["approvalRequired"]; ok {
		var gated daemon.ToolCallApprovalResult
		if err := json.Unmarshal(resp.Result, &gated); err != nil {
			return errorResult(fmt.Sprintf("decode approval body: %v", err))
		}
		summary := gated.ApprovalRequired.Summary
		if strings.TrimSpace(summary) == "" {
			summary = toolreg.ApprovalSummary(toolName, paramsFromArgs(gated.ApprovalRequired.Arguments))
		}
		msg := toolexec.AgentApprovalRejectedMessage(toolName, paramsFromArgs(gated.ApprovalRequired.Arguments))
		detail := gated.ApprovalRequired
		detail.Summary = summary
		if strings.TrimSpace(detail.Kind) == "" {
			detail.Kind = toolName
		}
		if detail.RiskAttribution == nil {
			detail.RiskAttribution = agentaudit.PolicyAttribution(string(riskengine.ActionRequireApproval))
		}
		text, err := json.MarshalIndent(map[string]any{
			"ok":               false,
			"message":          msg,
			"approvalRequired": detail,
		}, "", "  ")
		if err != nil {
			return errorResult(msg)
		}
		return errorResult(string(text))
	}
	var success daemon.ToolCallResult
	if err := json.Unmarshal(resp.Result, &success); err != nil {
		return errorResult(fmt.Sprintf("decode tool result: %v", err))
	}
	return successResult(datasourceops.RedactValue(success.Result))
}

func riskAttributionFromDetails(details map[string]any) *agentaudit.RiskAttribution {
	if details == nil {
		return nil
	}
	raw, ok := details["riskAttribution"]
	if !ok || raw == nil {
		return nil
	}
	var attr agentaudit.RiskAttribution
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(body, &attr); err != nil {
		return nil
	}
	if strings.TrimSpace(attr.Source) == "" && strings.TrimSpace(attr.RuleCode) == "" && strings.TrimSpace(attr.RuleID) == "" {
		return nil
	}
	return &attr
}

// paramsFromArgs unwraps an arguments any (which the daemon already
// redacted) into a map[string]any if possible. Used only for the approval
// summary fallback path; otherwise the daemon-supplied summary is used.
func paramsFromArgs(args any) map[string]any {
	if args == nil {
		return nil
	}
	if m, ok := args.(map[string]any); ok {
		return m
	}
	return nil
}

// isInfraError returns true when err signals "the daemon isn't reachable",
// so dispatchViaDaemon should attempt a spawn-and-retry. Auth/business
// errors returned by the daemon authoritatively are NOT infra errors.
//
// Raw transport drops (EOF / broken pipe / connection reset / "use of closed
// network connection") count as infra: they fire when the daemon crashes or
// is restarted while the client still holds an open connection, and the
// recovery path is the same as DAEMON_NOT_RUNNING — drop the conn and let
// the spawn-and-retry probe the current state. Without this branch,
// ipc.Client.Roundtrip's bare network errors carry no wire code and would
// short-circuit the recovery, surfacing a hard failure to the agent on the
// first tool call after any daemon restart.
func isInfraError(err error) bool {
	switch ipc.ErrorCode(err) {
	case ipc.CodeDaemonNotRunning, ipc.CodeDaemonUnreachable, ipc.CodeVersionMismatch, ipc.CodeInstallCorrupted, ipc.CodeLocateMainApp:
		return true
	}
	return ipc.IsConnDropError(err)
}

// dispatchInProcessUnauthenticated is the legacy MCP test path: tools_trust_test
// passes a stub Service with no IPC client and no access key to validate the
// approval/trust gate logic without standing up a real daemon. The handler in
// makeHandlerWithClient routes here only when client == nil. Production code
// always wires a real client and never reaches this path.
func dispatchInProcessUnauthenticated(ctx context.Context, svc toolreg.Service, def toolreg.ToolDef, params map[string]any) *gomcp.CallToolResult {
	needsApproval := def.ApprovalRequired
	var writePreview *console.WritePreview
	writePreviewUnavailable := false
	switch {
	case def.AssessApproval != nil:
		decision, err := def.AssessApproval(ctx, svc, params)
		if err != nil {
			return errorResult(fmt.Sprintf("approval check failed: %v", err))
		}
		needsApproval = decision.NeedsApproval
		writePreview = decision.WritePreview
		writePreviewUnavailable = decision.WritePreviewUnavailable
		if decision.Blocked {
			return errorResult(toolreg.BlockedErrorFromDecision(decision).Error())
		}
	case def.NeedsApproval != nil:
		na, err := def.NeedsApproval(ctx, svc, params)
		if err != nil {
			return errorResult(fmt.Sprintf("approval check failed: %v", err))
		}
		needsApproval = na
	}
	if def.DangerousScopable && needsApproval {
		if dsID := datasourceIDFromParams(params); dsID != "" && toolreg.IsDatasourceDangerous(ctx, svc, dsID) && !writePreviewUnavailable && (writePreview == nil || !writePreview.RequiresElevatedApproval) {
			needsApproval = false
		}
	}
	if needsApproval {
		return approvalRequiredResult(def.Name, params)
	}
	ctx = datasourceops.WithUserApproved(ctx)
	result, err := def.Call(ctx, svc, params)
	if err != nil {
		return errorResult(err.Error())
	}
	return successResult(result)
}
