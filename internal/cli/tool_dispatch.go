// CLI-side dispatcher for `tool call`. Every invocation goes through the main
// app's IPC daemon — there is no in-process fallback. If the socket isn't up,
// we try one spawn-and-retry; failing that, the call surfaces an error so the
// user can fix the install. The CLI process must never load datasources.json
// itself: sandboxed agents (codex, claude-code) don't have keychain access,
// and a local Service would clobber the main app's writes anyway.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

// daemonSpawnTimeout caps how long we wait for a freshly spawned daemon to
// publish its handshake. 3s is plenty on warm caches.
const daemonSpawnTimeout = 3 * time.Second

// No per-tool-call wrapper timeout: tool.call rides the caller's ctx so
// long-running ops (D1OAuthLogin up to 3min, DynamoDB SSO authorization,
// slow analytics queries) don't get cut off client-side while the daemon
// keeps executing them — that mismatch was producing false failures and
// duplicate-execution risk on retry. The IPC client's DialTimeout (3s)
// still bounds the connect step; the response read inherits the caller's
// ctx deadline via Client.Roundtrip's SetDeadline call.

// dispatchToolCallViaDaemon performs the agent → daemon → tool flow. Returns
// the decoded result, an approval-gated result, or an error. At most one is
// non-nil. Infra failures (no daemon, version skew) become errors after the
// spawn-and-retry attempt — there is intentionally no in-process fallback.
func (r *Runner) dispatchToolCallViaDaemon(ctx context.Context, opts Options, def toolreg.ToolDef, payload map[string]any) (any, *daemon.ToolCallApprovalResult, error) {
	// opts.DataPath is the file path to datasources.json; sibling files
	// (socket, handshake) live in its parent directory.
	dataDir := filepath.Dir(opts.DataPath)
	client := ipc.NewClient(ipc.ClientConfig{DataDir: dataDir})
	defer client.Close()

	resp, err := r.daemonRoundtrip(ctx, client, def, payload, opts.AgentAccessKey)
	if err == nil {
		return decodeDaemonResponse(resp, def.Name)
	}

	if !isInfraError(err) {
		return nil, nil, asLocalError(err)
	}

	// Cold start / crash recovery: trigger the spawn-and-retry path for
	// "no daemon listening" AND for raw transport drops (EOF/broken pipe/
	// connection reset). The conn-drop case fires when a previously-open
	// connection survives a daemon restart: ipc.Client.Roundtrip returns
	// the underlying network error verbatim with empty wire code, and
	// without recognising it here the very first call after a daemon
	// crash/restart would fail user-visibly instead of recovering. Version
	// skew and install corruption still need user action and skip the
	// spawn.
	code := ipc.ErrorCode(err)
	spawnEligible := code == ipc.CodeDaemonNotRunning || code == ipc.CodeDaemonUnreachable || ipc.IsConnDropError(err)
	if !spawnEligible {
		return nil, nil, asLocalError(err)
	}
	spawnCtx, spawnCancel := context.WithTimeout(ctx, daemonSpawnTimeout+1*time.Second)
	defer spawnCancel()
	// Tell the spawned daemon which store to serve. Without this it would
	// fall back to its own default DataPath, publish the handshake in a
	// different directory, and the WaitForHandshake below would time out
	// while a daemon happily ran against the wrong datasources.json.
	//
	// Forward AuthBaseURL too: installs that point at a non-default auth
	// service via config or FUTRIX_AUTH_BASE_URL must not have a cold-
	// spawned daemon silently fall back to auth.DefaultBaseURL — the next
	// EnsureAuthenticated call would fail against the wrong environment.
	extraArgs := []string{"--data-path", opts.DataPath}
	if v := strings.TrimSpace(opts.AuthBaseURL); v != "" {
		extraArgs = append(extraArgs, "--auth-base-url", v)
	}
	spawnCfg := ipc.SpawnConfig{ExtraArgs: extraArgs}
	if spawnErr := ipc.SpawnDaemon(spawnCtx, spawnCfg); spawnErr != nil {
		return nil, nil, fmt.Errorf("spawn main app: %w", spawnErr)
	}
	if _, hsErr := ipc.WaitForHandshake(spawnCtx, dataDir, daemonSpawnTimeout); hsErr != nil {
		return nil, nil, fmt.Errorf("main app spawned but never published handshake: %w", hsErr)
	}
	client.MarkDisconnected()
	resp2, retryErr := r.daemonRoundtrip(ctx, client, def, payload, opts.AgentAccessKey)
	if retryErr != nil {
		return nil, nil, asLocalError(retryErr)
	}
	return decodeDaemonResponse(resp2, def.Name)
}

func (r *Runner) daemonRoundtrip(ctx context.Context, client *ipc.Client, def toolreg.ToolDef, payload map[string]any, accessKey string) (ipc.Response, error) {
	args := daemon.ToolCallArgs{
		Tool:   def.Name,
		Params: payload,
		Source: string(toolexec.SourceSkill),
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

// decodeDaemonResponse converts the daemon's wire response into:
//
//   - a success payload (response != nil): caller renders ok envelope
//   - an approval-gated result (gated != nil): caller renders an unsupported-approval error
//   - a business error (err != nil): caller renders toolCallFailure
//
// At most one of the three is non-nil.
func decodeDaemonResponse(resp ipc.Response, toolName string) (any, *daemon.ToolCallApprovalResult, error) {
	if !resp.OK {
		if resp.Error == nil {
			return nil, nil, fmt.Errorf("daemon response not OK and no error body")
		}
		return nil, nil, daemonResponseError(resp.Error)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &probe); err != nil {
		return nil, nil, fmt.Errorf("decode daemon result: %w", err)
	}
	if _, ok := probe["approvalRequired"]; ok {
		var gated daemon.ToolCallApprovalResult
		if err := json.Unmarshal(resp.Result, &gated); err != nil {
			return nil, nil, fmt.Errorf("decode approval body: %w", err)
		}
		return nil, &gated, nil
	}
	var success daemon.ToolCallResult
	if err := json.Unmarshal(resp.Result, &success); err != nil {
		return nil, nil, fmt.Errorf("decode tool result: %w", err)
	}
	if strings.TrimSpace(success.Tool) == "" {
		success.Tool = toolName
	}
	return success, nil, nil
}

type errorWithRiskAttribution struct {
	message         string
	riskAttribution *agentaudit.RiskAttribution
}

func (e *errorWithRiskAttribution) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func daemonResponseError(e *ipc.Error) error {
	if e == nil {
		return errors.New("")
	}
	if attr := riskAttributionFromDetails(e.Details); attr != nil {
		return &errorWithRiskAttribution{message: e.Message, riskAttribution: attr}
	}
	return errors.New(e.Message)
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

func riskAttributionFromError(err error) *agentaudit.RiskAttribution {
	var attributed *errorWithRiskAttribution
	if errors.As(err, &attributed) && attributed.riskAttribution != nil {
		return attributed.riskAttribution
	}
	return agentaudit.AttributionFromError(err)
}

// isInfraError returns true when err signals "the daemon isn't reachable", so
// the CLI should attempt a spawn-and-retry. Auth/business errors returned by
// the daemon authoritatively are NOT infra errors — they bubble straight up.
//
// Raw transport drops (EOF / broken pipe / connection reset / "use of closed
// network connection") count as infra: they fire when the daemon crashes or
// is restarted while the client still holds an open connection, and the
// recovery path is the same as DAEMON_NOT_RUNNING — drop the conn and let
// the spawn-and-retry probe the current state. Without this branch,
// ipc.Client.Roundtrip's bare network errors carry no wire code and would
// short-circuit the recovery, making the first call after any daemon
// restart fail unnecessarily.
func isInfraError(err error) bool {
	switch ipc.ErrorCode(err) {
	case ipc.CodeDaemonNotRunning, ipc.CodeDaemonUnreachable, ipc.CodeVersionMismatch, ipc.CodeInstallCorrupted, ipc.CodeLocateMainApp:
		return true
	}
	return ipc.IsConnDropError(err)
}

// asLocalError unwraps an errorWithCode-wrapped error so toolCallFailure
// renders only the user-facing message, not the wire code prefix.
func asLocalError(err error) error {
	if err == nil {
		return nil
	}
	if wire := ipc.AsWireError(err); wire != nil && wire.Message != "" {
		return daemonResponseError(wire)
	}
	return err
}

// renderDaemonSuccess maps a daemon-decoded result into the CLI's standard
// envelope. response is the raw decoded payload (daemon.ToolCallResult); if
// gated is non-nil it takes priority.
func (r *Runner) renderDaemonSuccess(opts Options, response any, gated *daemon.ToolCallApprovalResult) error {
	if gated != nil {
		return r.toolCallApprovalUnsupported(opts, gated)
	}
	tcr, ok := response.(daemon.ToolCallResult)
	if !ok {
		return r.printJSON(map[string]any{
			"ok":     true,
			"result": response,
		})
	}
	return r.printJSON(map[string]any{
		"ok":     true,
		"tool":   tcr.Tool,
		"result": datasourceops.RedactValue(tcr.Result),
	})
}

func (r *Runner) toolCallApprovalUnsupported(opts Options, gated *daemon.ToolCallApprovalResult) error {
	kind := strings.TrimSpace(gated.ApprovalRequired.Kind)
	summary := strings.TrimSpace(gated.ApprovalRequired.Summary)
	if kind == "" {
		kind = "tool call"
	}
	detail := gated.ApprovalRequired
	detail.Kind = kind
	detail.Summary = summary
	detail.RiskAttribution = approvalResponseAttribution(detail.RiskAttribution)
	msg := fmt.Sprintf("%s requires approval and was rejected because third-party agents cannot approve FutrixData operations through `tool call`", kind)
	if summary != "" {
		msg = fmt.Sprintf("%s: %s", msg, summary)
	}
	err := errors.New(msg)
	return r.commandFailure(opts, map[string]any{
		"ok":               false,
		"tool":             kind,
		"error":            map[string]any{"message": msg},
		"approvalRequired": detail,
	}, err)
}
