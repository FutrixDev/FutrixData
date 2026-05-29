// Package toolexec is the single dispatch path for "agent invokes a registered
// tool". Both the CLI's `tool call` (when running locally / before IPC routing
// was added) and the daemon's `tool.call` IPC op route through this package, so
// the access-key check, approval gate, danger-mode bypass, post-execution
// revocation re-check, and audit row writes are guaranteed to behave identically
// regardless of whether the agent reached the tool via stdin/MCP or via socket.
//
// Keeping this logic in one place is a security contract: any future code path
// that wants to invoke a tool on behalf of an agent must call Dispatch — bypass
// would leave us with un-attributed audit rows and undermine the "agent has no
// path into any tool without an access key" rule from the architecture design.
package toolexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/toolreg"
)

// Source identifies the agent that originated the call. Goes into the audit
// row so we can tell which channel an agent used.
type Source string

const (
	// SourceSkill is the CLI `tool call` path used by skill templates (codex,
	// claude-code, cursor, custom skills).
	SourceSkill Source = "skill"
	// SourceMCP is the MCP server path used by IDE integrations.
	SourceMCP Source = "mcp"
	// SourceCLI tags audit rows produced by the direct CLI subcommands
	// (`datasource list`, `console execute`, `d1 *`, `dynamodb-sso *`) when an
	// agent passes --agent-access-key. These bypass tool.call dispatch and the
	// daemon — keeping a distinct source keeps the audit log honest about the
	// channel an agent actually used.
	SourceCLI Source = "cli"
)

// Input collects everything the dispatcher needs to evaluate one call. The
// caller — typically the daemon's tool.call handler or the legacy in-process
// CLI fallback — fills these from its own request shape.
type Input struct {
	DataPath  string
	Source    Source
	AccessKey string
	ToolName  string
	Params    map[string]any
	// SchemaPrivacy is the audit/consent store used to gate schema-emitting
	// tools (list_entities, describe_entity, get_schema_knowledge,
	// get_er_knowledge). When nil the schema egress gate is skipped — leaving
	// the historical behavior intact for tests and headless setups that do
	// not wire a store. Callers in the daemon pass the same store the Wails
	// app uses so the audit log is unified.
	SchemaPrivacy *schemaprivacy.AuditStore
}

// schemaEgressTriggers maps tool names to the TriggerSource that should be
// recorded when an external Skill/MCP agent invokes a schema-emitting tool.
// Tools not in this map are not gated by schemaprivacy here (their consent, if
// any, is enforced at their own call sites — e.g. AI Chat tools live in
// app_aichat.go).
var schemaEgressTriggers = map[string]schemaprivacy.TriggerSource{
	"list_entities":        schemaprivacy.TriggerMCPListEntities,
	"describe_entity":      schemaprivacy.TriggerMCPDescribeEntity,
	"get_schema_knowledge": schemaprivacy.TriggerMCPGetSchemaKnowledge,
	"get_er_knowledge":     schemaprivacy.TriggerMCPGetERKnowledge,
}

// Result is the dispatcher's structured success response. Result is whatever
// the underlying tool returned, already redacted via datasourceops.RedactValue
// so the caller doesn't need to redact again.
type Result struct {
	ToolName string
	Result   any
}

// ApprovalGated is the legacy approval envelope shape. Agent-routed calls now
// reject approval-required operations directly, but callers keep decoding this
// shape for compatibility with older daemon responses.
type ApprovalGated struct {
	ToolName        string
	Summary         string
	Params          map[string]any
	RiskAttribution *agentaudit.RiskAttribution
	WritePreview    *console.WritePreview
}

func toolErrorWithAttribution(message string, attribution *agentaudit.RiskAttribution) *ipc.Error {
	e := ipc.NewError(ipc.CodeToolError, message)
	details := map[string]any{}
	if attribution != nil {
		details["riskAttribution"] = attribution
	}
	if len(details) > 0 {
		e.Details = details
	}
	return e
}

func toolErrorFromError(err error, attribution *agentaudit.RiskAttribution) *ipc.Error {
	e := toolErrorWithAttribution(err.Error(), attribution)
	if limitErr, ok := console.ExecutionLimitErrorFrom(err); ok {
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		e.Details["executionLimits"] = limitErr.Details()
	}
	return e
}

// AgentApprovalRejectedMessage is the canonical user-facing reason for
// third-party Skill/MCP/agent-key CLI calls that hit an approval-required gate.
func AgentApprovalRejectedMessage(toolName string, params map[string]any) string {
	kind := strings.TrimSpace(toolName)
	if kind == "" {
		kind = "operation"
	}
	msg := fmt.Sprintf("%s requires approval and was rejected because third-party agents cannot approve FutrixData operations", kind)
	if summary := strings.TrimSpace(toolreg.ApprovalSummary(toolName, params)); summary != "" {
		msg = fmt.Sprintf("%s: %s", msg, summary)
	}
	return msg
}

func approvalRejectedError(toolName string, params map[string]any, attribution *agentaudit.RiskAttribution) *ipc.Error {
	e := toolErrorWithAttribution(AgentApprovalRejectedMessage(toolName, params), attribution)
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details["approvalRejected"] = true
	return e
}

// Dispatch performs the full agent → tool path. Returns one of:
//
//   - (*Result, nil, nil): tool ran successfully. Caller renders Result.Result.
//   - (nil, nil, *ipc.Error): approval-required operation was rejected, or
//     something failed (bad input, access denied, tool errored). Caller renders
//     the error code/message; codes follow the wire protocol so daemon callers
//     can pass them straight through.
//
// `service` is the toolreg.Service the dispatcher will pass to the tool. On the
// daemon, this is the daemon-local Service (same one Wails uses); in the legacy
// in-process CLI path, this is the local CLI Service.
//
// Empty AccessKey is rejected with CodeAccessKeyRequired — the design explicitly
// fails closed here ("agent has no path into any tool without an access key").
func Dispatch(ctx context.Context, service toolreg.Service, in Input) (*Result, *ApprovalGated, *ipc.Error) {
	if strings.TrimSpace(in.ToolName) == "" {
		return nil, nil, ipc.NewError(ipc.CodeBadRequest, "tool name required")
	}
	def, ok := toolreg.ByName(in.ToolName)
	if !ok {
		return nil, nil, ipc.NewError(ipc.CodeBadRequest, fmt.Sprintf("unknown tool: %s", in.ToolName))
	}
	if strings.TrimSpace(in.AccessKey) == "" {
		return nil, nil, ipc.NewError(ipc.CodeAccessKeyRequired, "agent access key required for tool.call")
	}
	source := in.Source
	if source == "" {
		source = SourceSkill
	}
	params := in.Params
	if params == nil {
		params = map[string]any{}
	}

	// 1) Access-key check — this is the agent identity gate. Revoked keys
	// also write an audit row before failing so the security trail captures
	// the rejected attempt.
	identity, err := agentaudit.CheckAccess(in.DataPath, in.AccessKey)
	if err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(in.DataPath, service, string(source), in.AccessKey, def.Name, params, err.Error())
			return nil, nil, ipc.NewError(ipc.CodeAccessKeyRevoked, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(in.DataPath, nil, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, err.Error())
			return nil, nil, ipc.NewError(ipc.CodeAccessKeyExpired, err.Error())
		}
		return nil, nil, ipc.NewError(ipc.CodeAccessKeyUnknown, err.Error())
	}

	if def.Name == "list_datasources" {
		if scopeErr := agentaudit.CheckDatasourceInventoryScope(identity); scopeErr != nil {
			message := fmt.Sprintf("agent %s cannot list all datasources: %v", agentaudit.MaskAccessKey(in.AccessKey), scopeErr)
			_ = agentaudit.AppendToolCall(in.DataPath, nil, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message)
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, message)
		}
	}
	if dsID := toolreg.DatasourceIDFromToolDef(def, params); dsID != "" {
		if scopeErr := agentaudit.CheckDatasourceScope(identity, dsID); scopeErr != nil {
			message := fmt.Sprintf("agent %s cannot access datasource %s: %v", agentaudit.MaskAccessKey(in.AccessKey), dsID, scopeErr)
			_ = agentaudit.AppendToolCall(in.DataPath, nil, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message)
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, message)
		}
	}

	// 1.5) Per-tool permission gate. Currently scoped to the sensitivity
	// write tools — they mutate the user's masking policy, so an agent
	// without the explicit grant must be rejected before any state change
	// or approval evaluation runs. Reject is audited as StatusError so the
	// rejected attempt is visible in the audit log alongside other denials.
	if requiresSensitivityGrant(def.Name) && !identity.SensitivityClassificationGrant {
		message := fmt.Sprintf("agent %s lacks sensitivity-classification grant for tool %s", agentaudit.MaskAccessKey(in.AccessKey), def.Name)
		_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message)
		return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, message)
	}
	// 1.6) Risk-rule write gate. set_risk_rule / delete_risk_rule mutate
	// the daemon's live rule cache. Without an explicit grant the call is
	// rejected closed: an agent that holds a normal access key cannot
	// silently insert or remove rules, and the audit row records the
	// denied attempt. Identities with the grant skip the approval prompt
	// below — this is the trusted-automation path used by the regression
	// test harness to seed and tear down user rules in the live daemon.
	riskRuleGrantBypass := false
	if requiresRiskRuleGrant(def.Name) {
		if !identity.RiskRuleManagementGrant {
			message := fmt.Sprintf("agent %s lacks risk-rule-management grant for tool %s", agentaudit.MaskAccessKey(in.AccessKey), def.Name)
			_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message)
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, message)
		}
		riskRuleGrantBypass = true
	}
	datasourceManagementGrantBypass := false
	if requiresDatasourceManagementGrant(def.Name) && identity.DatasourceManagementGrant {
		if agentaudit.UsesDatasourceAllowList(identity) {
			message := fmt.Sprintf("agent %s cannot use datasource-management grant while datasource scope is allowlist", agentaudit.MaskAccessKey(in.AccessKey))
			_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message)
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, message)
		}
		var payload datasourceops.DataSourcePayload
		if err := toolreg.MapToStruct(params, &payload); err != nil {
			_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, err.Error())
			return nil, nil, ipc.NewError(ipc.CodeToolError, err.Error())
		}
		if err := datasourceops.ValidateAgentDatasourceCreatePayload(payload); err != nil {
			_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, err.Error())
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, err.Error())
		}
		datasourceManagementGrantBypass = true
	}

	// 1.7) Schema egress gate (preflight). The four tools listed in
	// schemaEgressTriggers emit datasource schema metadata (entity / field
	// names, types, comments, indexes). Per-datasource consent (managed in the
	// "AI 获取 Schema 设置" panel under Data Sensitivity) must be Allowed before
	// the agent learns anything about the structure. Other tools carry their
	// own gates and are not re-checked here. dsID is required for consent
	// lookup; if the tool declares datasourceId required (all four do) and
	// it's missing, the tool's own param validation will surface a clearer
	// error than this gate.
	//
	// SchemaEgressPreflight + SchemaEgressPostflight back both this surface
	// and the direct-CLI surface (cli.auditedCall). Keeping the policy in one
	// helper means tools added to schemaEgressTriggers are gated everywhere.
	gateInput := SchemaEgressGateInput{
		DataPath:  in.DataPath,
		Service:   service,
		Source:    string(source),
		AccessKey: in.AccessKey,
		ToolName:  def.Name,
		Params:    params,
		Store:     in.SchemaPrivacy,
	}
	schemaEgressDSID, schemaEgressTrigger, schemaEgressRecheck, gErr := SchemaEgressPreflight(ctx, gateInput)
	if gErr != nil {
		return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, gErr.Error())
	}

	// 2) Approval evaluation. Mirrors mcp/tools.go and cli/commands_tool.go —
	// AssessApproval is preferred so the audit row carries the matched rule;
	// NeedsApproval is the fallback for tools without structured assessment.
	needsApproval := def.ApprovalRequired
	var attribution *agentaudit.RiskAttribution
	var writePreview *console.WritePreview
	writePreviewUnavailable := false
	switch {
	case def.AssessApproval != nil:
		decision, err := def.AssessApproval(ctx, service, params)
		if err != nil {
			return nil, nil, ipc.NewError(ipc.CodeToolError, fmt.Sprintf("approval check failed: %v", err))
		}
		needsApproval = decision.NeedsApproval
		writePreview = decision.WritePreview
		writePreviewUnavailable = decision.WritePreviewUnavailable
		if decision.Assessment != nil {
			attribution = agentaudit.AttributionFromAssessment(*decision.Assessment)
		}
		if decision.Blocked {
			blockedErr := toolreg.BlockedErrorFromDecision(decision)
			errAttribution := agentaudit.AttributionFromError(blockedErr)
			_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, blockedErr.Error(), errAttribution)
			return nil, nil, toolErrorFromError(blockedErr, errAttribution)
		}
	case def.NeedsApproval != nil:
		na, err := def.NeedsApproval(ctx, service, params)
		if err != nil {
			return nil, nil, ipc.NewError(ipc.CodeToolError, fmt.Sprintf("approval check failed: %v", err))
		}
		needsApproval = na
	}

	// 3) Danger-mode bypass — only for tools that target a datasource via a
	// trusted parameter (DangerousScopable). Other tools cannot opt in by
	// passing an unrelated `id` field.
	if needsApproval && def.DangerousScopable {
		if dsID := toolreg.DatasourceIDFromParams(params); dsID != "" && toolreg.IsDatasourceDangerous(ctx, service, dsID) && !writePreviewUnavailable && !writePreviewRequiresElevatedApproval(writePreview) {
			needsApproval = false
		}
	}
	// 3.5) Risk-rule grant bypass — risk-rule write tools are
	// ApprovalRequired so production agents face the prompt, but trusted
	// local automation (tests, supervised tooling) holding the explicit
	// RiskRuleManagementGrant skips the prompt and runs through. The grant
	// itself was checked above; this only flips the gate when needed.
	if needsApproval && riskRuleGrantBypass {
		needsApproval = false
	}
	if needsApproval && datasourceManagementGrantBypass {
		needsApproval = false
	}

	if needsApproval {
		gateAttribution := attribution
		if gateAttribution == nil {
			gateAttribution = agentaudit.PolicyAttribution(string(riskengine.ActionRequireApproval))
		}
		message := AgentApprovalRejectedMessage(def.Name, params)
		_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, message, gateAttribution)
		return nil, nil, approvalRejectedError(def.Name, params, gateAttribution)
	}

	// 4) Execute. WithUserApproved tells the datasourceops layer that this
	// agent call is already allowed by the entry-point gate, such as low-risk,
	// trusted auto-run, or danger-mode bypass. Agent-supplied approval flags do
	// not reach this path.
	execCtx := datasourceops.WithUserApproved(ctx)
	result, err := def.Call(execCtx, service, params)
	if err != nil {
		errAttribution := agentaudit.AttributionFromError(err)
		_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, err.Error(), errAttribution)
		// Block path: surface as ToolError so the wire layer doesn't lose the
		// original message, and carry safe matched-rule attribution for callers
		// that need machine-readable regression evidence.
		return nil, nil, toolErrorFromError(err, errAttribution)
	}

	// 5) Post-execution revoke recheck. The result has already escaped the
	// sandbox if we got here, but we still want the audit trail to reflect a
	// revocation that happened mid-flight rather than a clean success.
	if _, recheckErr := agentaudit.CheckAccess(in.DataPath, in.AccessKey); recheckErr != nil {
		if errors.Is(recheckErr, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(in.DataPath, service, string(source), in.AccessKey, def.Name, params, "access revoked during execution")
			return nil, nil, ipc.NewError(ipc.CodeAccessKeyRevoked, recheckErr.Error())
		}
		if errors.Is(recheckErr, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(in.DataPath, nil, string(source), in.AccessKey, def.Name, params, agentaudit.StatusError, "access expired during execution")
			return nil, nil, ipc.NewError(ipc.CodeAccessKeyExpired, recheckErr.Error())
		}
	}

	// 6) Schema egress gate (post-execution). See SchemaEgressPostflight for
	// why this re-reads the datasource fresh rather than reusing the
	// preflight copy.
	if schemaEgressRecheck {
		if pErr := SchemaEgressPostflight(ctx, gateInput, schemaEgressDSID, schemaEgressTrigger); pErr != nil {
			return nil, nil, ipc.NewError(ipc.CodeAgentForbidden, pErr.Error())
		}
	}

	_ = agentaudit.AppendToolCall(in.DataPath, service, string(source), in.AccessKey, def.Name, params, agentaudit.StatusSuccess, "")
	return &Result{
		ToolName: def.Name,
		Result:   datasourceops.RedactValue(result),
	}, nil, nil
}

func writePreviewRequiresElevatedApproval(preview *console.WritePreview) bool {
	return preview != nil && preview.RequiresElevatedApproval
}

// sensitivityGrantTools is the closed set of tool names that mutate the
// user's active masking policy and therefore require an explicit per-agent
// grant. Read-path tools (get_sensitivity_config, get_sensitivity_report)
// are intentionally absent — agents need to inspect schema metadata to
// produce useful proposals, but should not be able to change policy
// without the user opting in. A new sensitivity write tool must be added
// here on its registration commit.
var sensitivityGrantTools = map[string]struct{}{
	"set_sensitivity_custom_rules": {},
	"save_sensitivity_report":      {},
	"delete_sensitivity_report":    {},
}

func requiresSensitivityGrant(toolName string) bool {
	_, ok := sensitivityGrantTools[strings.TrimSpace(toolName)]
	return ok
}

// RequiresSignedInUser reports whether a tool mutates policy/configuration
// that is only editable after a desktop user signs in. The agent grant still
// gates which identities may run these tools, but it is not a substitute for
// the active user session required by the desktop UI/backend gates.
func RequiresSignedInUser(toolName string) bool {
	return requiresSensitivityGrant(toolName) || requiresRiskRuleGrant(toolName)
}

// riskRuleGrantTools is the closed set of tool names that mutate the
// daemon's user-rule store or built-in / probe-catalog overrides. All are
// gated by the explicit RiskRuleManagementGrant on the agent identity.
// list_risk_rules is intentionally absent — read access is needed for the
// management UI and for the harness's post-seed sanity check. A new
// risk-rule write tool MUST be added to this map on the same commit that
// registers it; otherwise an unprivileged agent could mutate the live
// rule cache by name alone.
var riskRuleGrantTools = map[string]struct{}{
	"set_risk_rule":                    {},
	"delete_risk_rule":                 {},
	"set_builtin_risk_rule_enabled":    {},
	"set_builtin_risk_rule_thresholds": {},
}

func requiresRiskRuleGrant(toolName string) bool {
	_, ok := riskRuleGrantTools[strings.TrimSpace(toolName)]
	return ok
}

// datasourceManagementGrantTools is intentionally limited to create aliases.
// Updating or deleting existing datasources remains approval-gated because
// those operations can repoint or remove an existing trust boundary.
var datasourceManagementGrantTools = map[string]struct{}{
	"create_datasource": {},
	"add_datasource":    {},
}

func requiresDatasourceManagementGrant(toolName string) bool {
	_, ok := datasourceManagementGrantTools[strings.TrimSpace(toolName)]
	return ok
}
