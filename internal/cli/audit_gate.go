// audit_gate.go gates the CLI's *direct* subcommands (datasource, console, d1,
// dynamodb-sso) on --agent-access-key. These bypass the daemon's tool.call IPC
// op and would otherwise leak two ways: a revoked key still works, and the
// audit log never sees the call. This file's sole job is to make those calls
// behave like `tool call` does for agent invocation, while leaving human-ops
// usage (no flag) untouched.
//
// Why not route through the daemon? `console execute` already produces fine-
// grained audit rows via this gate; daemon-routing every datasource subcommand
// would force a daemon spawn-on-miss for each invocation and double-write the
// same row, with no security benefit because the access-key check is the
// authority — not the channel it travels over.
//
// Why gate-when-present (instead of always-required)? The CLI is also a human
// tool: revoking the flag would break `futrixdata-cli datasource list` from a
// developer terminal. The contract is "agents pass --agent-access-key";
// enforcement is "if you pass it, we validate it and audit you". Skill
// templates always thread the flag (TASK-20260426-133841 hardened this), so an
// agent that drops the flag is misconfigured at the template layer, not
// silently abusing this gate.
package cli

import (
	"context"
	"errors"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

// auditedCall wraps a CLI subcommand's underlying service operation with
// access-key validation and audit-row writing. When --agent-access-key is
// empty, op runs unchanged (human-ops path, no audit row). When non-empty:
//
//  1. Pre-flight CheckAccess. Unknown/revoked keys short-circuit before op
//     runs; revoked keys also append a (rate-limited) audit row so the
//     forensic trail captures the rejected attempt — same behavior as
//     toolexec.Dispatch.
//  2. op runs.
//  3. Post-flight recheck on success. A mid-flight revoke flips a clean
//     return into a revocation error and an audit row, mirroring Dispatch's
//     post-execution recheck so the audit log can't be bypassed by revoking
//     a key the moment after a long-running call started.
//  4. Errors carry RiskAttribution when one is encoded in the error chain
//     (riskengine block path), so policy-blocked direct-CLI calls land in
//     the audit log with the matched-rule detail.
//
// `service` may be nil; agentaudit.AppendToolCall tolerates that and just
// skips datasource-name enrichment. Audit-write failures are intentionally
// swallowed: a flaky audit log must not cause a gated call that already ran
// to surface as an error to the agent (the row will be missing, but logs
// elsewhere will catch it).
func auditedCall[T any](
	ctx context.Context,
	opts Options,
	service toolreg.Service,
	toolName string,
	params map[string]any,
	op func(context.Context) (T, error),
) (T, error) {
	var zero T
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return op(ctx)
	}
	source := string(toolexec.SourceCLI)
	identity, err := agentaudit.CheckAccess(opts.DataPath, accessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, err.Error())
		}
		return zero, err
	}
	if err := validateAgentDatasourceScope(opts, source, accessKey, identity, toolName, params); err != nil {
		return zero, err
	}
	// Schema-egress preflight. The gate is a no-op for tools outside
	// schemaEgressTriggers; for the four schema-emitting tools it enforces
	// per-datasource consent before the underlying op runs. Same policy as
	// toolexec.Dispatch — direct CLI agent commands cannot bypass it by
	// switching surfaces.
	gateInput := toolexec.SchemaEgressGateInput{
		DataPath:  opts.DataPath,
		Service:   service,
		Source:    source,
		AccessKey: accessKey,
		ToolName:  toolName,
		Params:    params,
		Store:     schemaprivacy.NewAuditStore(bootstrap.SchemaPrivacyAuditPath(opts.DataPath)),
	}
	dsID, trigger, recheck, gateErr := toolexec.SchemaEgressPreflight(ctx, gateInput)
	if gateErr != nil {
		return zero, gateErr
	}
	result, err := op(ctx)
	if err != nil {
		attribution := agentaudit.AttributionFromError(err)
		_ = agentaudit.AppendToolCallWithAttribution(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusError, err.Error(), attribution)
		return zero, err
	}
	if _, recheckErr := agentaudit.CheckAccess(opts.DataPath, accessKey); recheckErr != nil {
		if errors.Is(recheckErr, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, "access revoked during execution")
			return result, recheckErr
		}
		if errors.Is(recheckErr, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, "access expired during execution")
			return result, recheckErr
		}
	}
	if recheck {
		if pErr := toolexec.SchemaEgressPostflight(ctx, gateInput, dsID, trigger); pErr != nil {
			return zero, pErr
		}
	}
	_ = agentaudit.AppendToolCall(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusSuccess, "")
	return result, nil
}

func rejectAgentApproveFlag(opts Options, service toolreg.Service, toolName string, params map[string]any) error {
	msg := "--approve is rejected when --agent-access-key is present; third-party agents cannot approve FutrixData operations"
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return nil
	}
	source := string(toolexec.SourceCLI)
	identity, err := agentaudit.CheckAccess(opts.DataPath, accessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, err.Error())
		}
		return err
	}
	if err := validateAgentDatasourceScope(opts, source, accessKey, identity, toolName, params); err != nil {
		return err
	}
	_ = agentaudit.AppendToolCall(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusError, msg)
	return errors.New(msg)
}

// auditedCallApprovalRedirect is auditedCall with one twist: an err carrying
// console.ExecuteRiskInfo is returned WITHOUT a StatusError row. The caller
// MUST then route the err into an approval prompt
// (validateAgentAccessForApproval + r.approvalRequired) so the canonical
// StatusApprovalRequired row is the only audit-trail entry for that call.
//
// Use only at call sites that route RiskInfo errs into an approval flow —
// in this codebase, that is the `console execute` optimistic-run path, where
// a static pre-check said "allow" but the adapter Guard's EXPLAIN probe
// upgrades the statement to require approval. Everywhere else, including
// `console execute --approve` where a riskengine.BlockedError is a hard
// block (and also satisfies ExecuteRiskInfoProvider), use the regular
// auditedCall so the forensic trail records the blocked execution.
func auditedCallApprovalRedirect[T any](
	ctx context.Context,
	opts Options,
	service toolreg.Service,
	toolName string,
	params map[string]any,
	op func(context.Context) (T, error),
) (T, error) {
	var zero T
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return op(ctx)
	}
	source := string(toolexec.SourceCLI)
	identity, err := agentaudit.CheckAccess(opts.DataPath, accessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, err.Error())
		}
		return zero, err
	}
	if err := validateAgentDatasourceScope(opts, source, accessKey, identity, toolName, params); err != nil {
		return zero, err
	}
	result, err := op(ctx)
	if err != nil {
		// Block is the only RiskInfo action that's a hard rejection: the
		// audit trail records it as StatusError and the agent sees the
		// block. `require_approval` and `warn` are both approval-eligible
		// (the legacy CLI redirected any RiskInfo to the approval prompt;
		// the daemon path also doesn't treat warn as a hard error — it
		// just suppresses the adapter Guard via WithUserApproved). For
		// those, the row is skipped here so the caller can route the err
		// into validateAgentAccessForApproval, which writes the canonical
		// StatusApprovalRequired row. Anything else — non-RiskInfo errs,
		// or block-action RiskInfo — gets a StatusError row preserving
		// matched-rule attribution.
		if info, ok := console.RiskInfoFromError(err); ok && info.Action != string(riskengine.ActionBlock) {
			return zero, err
		}
		attribution := agentaudit.AttributionFromError(err)
		_ = agentaudit.AppendToolCallWithAttribution(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusError, err.Error(), attribution)
		return zero, err
	}
	if _, recheckErr := agentaudit.CheckAccess(opts.DataPath, accessKey); recheckErr != nil {
		if errors.Is(recheckErr, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, "access revoked during execution")
			return result, recheckErr
		}
		if errors.Is(recheckErr, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, "access expired during execution")
			return result, recheckErr
		}
	}
	_ = agentaudit.AppendToolCall(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusSuccess, "")
	return result, nil
}

// validateAgentAccessForApproval gates a CLI subcommand's approval-required
// branch on agent-key validation BEFORE any human approval envelope is
// rendered. Agent-key calls cannot approve operations, so a valid key writes a
// rejected audit row and returns an error instead of continuing to the prompt.
// Mirrors toolexec.Dispatch's behavior:
//
//   - empty key (human ops) → return nil; the caller renders the prompt
//     locally without auditing.
//   - valid key → write a StatusError audit row with attribution and return a
//     rejection error.
//   - revoked key → log a (rate-limited) revocation row and return the
//     revocation error; the prompt is never rendered.
//   - unknown key → return the error without auditing (AppendToolCall rejects
//     unknown keys by design — there is no identity to attribute the row to).
//
// `attribution` carries the matched-rule detail when the approval was driven
// by a riskengine assessment (e.g. `console execute` against a Warn rule).
// When nil — the categorical case (datasource delete/create/update, d1
// mutations) — the row is written with PolicyAttribution(require_approval) so
// the audit log records *why* approval was required, mirroring
// toolexec.Dispatch's gateAttribution fallback.
//
// Audit-row params come in pre-sanitized; callers must strip secrets
// (passwords, API tokens) from the map before passing it here, since the
// prompt envelope and the audit row will both reflect the same parameters.
func validateAgentAccessForApproval(
	opts Options,
	service toolreg.Service,
	toolName string,
	params map[string]any,
	attribution *agentaudit.RiskAttribution,
) error {
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return nil
	}
	source := string(toolexec.SourceCLI)
	identity, err := agentaudit.CheckAccess(opts.DataPath, accessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, source, accessKey, toolName, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, err.Error())
		}
		return err
	}
	if err := validateAgentDatasourceScope(opts, source, accessKey, identity, toolName, params); err != nil {
		return err
	}
	if attribution == nil {
		attribution = agentaudit.PolicyAttribution(string(riskengine.ActionRequireApproval))
	}
	message := toolexec.AgentApprovalRejectedMessage(toolName, params)
	_ = agentaudit.AppendToolCallWithAttribution(opts.DataPath, service, source, accessKey, toolName, params, agentaudit.StatusError, message, attribution)
	return &errorWithRiskAttribution{message: message, riskAttribution: attribution}
}

// preflightAgentAccess validates an agent access key without writing any
// success or approval row. It is the cheapest gate the CLI can apply when a
// subcommand needs to read service state (e.g. `console execute` calling
// AssessStatementApproval reads the datasource and runs the riskengine) BEFORE
// the auditedCall / validateAgentAccessForApproval path is reached. Without
// this pre-flight, an invalid key would still trigger that service access —
// breaking the "validate first, then act" contract that auditedCall and
// toolexec.Dispatch both follow.
//
// Empty key returns nil so the human-ops contract holds. Revoked keys still
// write the (rate-limited) revocation row so the security trail captures the
// rejected attempt; unknown keys return their error without a row, matching
// AppendToolCall's design (no identity → nowhere to attribute the row).
func preflightAgentAccess(
	opts Options,
	service toolreg.Service,
	toolName string,
	params map[string]any,
) error {
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return nil
	}
	identity, err := agentaudit.CheckAccess(opts.DataPath, accessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, service, string(toolexec.SourceCLI), accessKey, toolName, params, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, string(toolexec.SourceCLI), accessKey, toolName, params, agentaudit.StatusError, err.Error())
		}
		return err
	}
	if err := validateAgentDatasourceScope(opts, string(toolexec.SourceCLI), accessKey, identity, toolName, params); err != nil {
		return err
	}
	return nil
}

func validateAgentDatasourceScope(
	opts Options,
	source string,
	accessKey string,
	identity agentaudit.AgentIdentity,
	toolName string,
	params map[string]any,
) error {
	def, ok := toolreg.ByName(toolName)
	if !ok {
		return nil
	}
	if def.Name == "list_datasources" {
		if err := agentaudit.CheckDatasourceInventoryScope(identity); err != nil {
			message := "agent " + agentaudit.MaskAccessKey(accessKey) + " cannot list all datasources: " + err.Error()
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, message)
			return errors.New(message)
		}
	}
	dsID := toolreg.DatasourceIDFromToolDef(def, params)
	if dsID == "" {
		return nil
	}
	if err := agentaudit.CheckDatasourceScope(identity, dsID); err != nil {
		message := "agent " + agentaudit.MaskAccessKey(accessKey) + " cannot access datasource " + dsID + ": " + err.Error()
		_ = agentaudit.AppendToolCall(opts.DataPath, nil, source, accessKey, toolName, params, agentaudit.StatusError, message)
		return errors.New(message)
	}
	return nil
}
