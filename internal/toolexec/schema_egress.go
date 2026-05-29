package toolexec

import (
	"context"
	"fmt"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/toolreg"
)

// SchemaEgressGateInput carries the contextual fields needed to enforce the
// per-datasource schema egress policy on agent-originated tool calls.
//
// One enforcement helper backs both the daemon's tool.call IPC path
// (toolexec.Dispatch) and the direct-CLI agent path (cli.auditedCall) so
// tools added to schemaEgressTriggers are automatically gated regardless of
// which surface the agent reaches them from.
type SchemaEgressGateInput struct {
	DataPath  string
	Service   toolreg.Service
	Source    string
	AccessKey string
	ToolName  string
	Params    map[string]any
	Store     *schemaprivacy.AuditStore
}

// SchemaEgressTriggerFor returns the TriggerSource recorded for a tool name.
// Returns ok=false for tools outside the schema-egress trigger map (callers
// should skip the gate entirely in that case).
func SchemaEgressTriggerFor(toolName string) (schemaprivacy.TriggerSource, bool) {
	t, ok := schemaEgressTriggers[toolName]
	return t, ok
}

// SchemaEgressPreflight enforces the deny-only side of the gate before a tool
// runs. Returns (dsID, trigger, true, nil) when the call should proceed and
// the post-execution recheck must run; (_, _, false, nil) when no gate
// applies; (_, _, false, err) when the gate denies. On denial it writes both
// the schemaprivacy denial row (via Gate) and the agent-audit row.
//
// The preflight only enforces *deny* — on Allowed we deliberately defer the
// schemaprivacy audit row to SchemaEgressPostflight so the recorded decision
// reflects what actually shipped (a mid-flight revocation gets recorded as a
// denial, not retroactively as an allow).
//
// Fail-closed contract: when a tool is in schemaEgressTriggers and a dsID is
// present in params but GetDatasource errors, we deny the call rather than
// fall through. A transient datasource lookup failure must not be a free
// pass past the consent policy — without this, a brief read failure during
// preflight (followed by a successful read inside the tool itself) would
// silently emit schema metadata for a datasource whose consent we never
// confirmed. dsID-missing remains a no-op (the tool's own param validation
// surfaces the clearer error).
func SchemaEgressPreflight(ctx context.Context, in SchemaEgressGateInput) (dsID string, trigger schemaprivacy.TriggerSource, gated bool, err error) {
	t, ok := schemaEgressTriggers[in.ToolName]
	if !ok {
		return "", "", false, nil
	}
	id := toolreg.DatasourceIDFromParams(in.Params)
	if id == "" {
		return "", "", false, nil
	}
	ds, gErr := in.Service.GetDatasource(ctx, id)
	if gErr != nil {
		denyErr := fmt.Errorf("schema egress denied: datasource lookup failed: %w", gErr)
		attribution := agentaudit.PolicyAttribution("schema_egress_denied")
		_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, in.Service, in.Source, in.AccessKey, in.ToolName, in.Params, agentaudit.StatusError, denyErr.Error(), attribution)
		return "", "", false, denyErr
	}
	if schemaprivacy.ConsentOf(ds) == schemaprivacy.ConsentAllowed {
		return id, t, true, nil
	}
	gateErr := schemaprivacy.Gate(in.Store, ds, t, schemaprivacy.SendSummary{})
	attribution := agentaudit.PolicyAttribution("schema_egress_denied")
	_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, in.Service, in.Source, in.AccessKey, in.ToolName, in.Params, agentaudit.StatusError, gateErr.Error(), attribution)
	return "", "", false, gateErr
}

// SchemaEgressPostflight re-reads the datasource fresh and re-runs the
// consent check after a tool has executed but before the result is returned
// to the agent. A mid-flight consent flip (allowed → denied) is surfaced as
// an error and the result is withheld. Same for a datasource that was
// deleted during execution — fail closed.
//
// The empty SendSummary is a known limitation: extracting entity/field
// counts requires per-tool result inspection and is tracked separately.
func SchemaEgressPostflight(ctx context.Context, in SchemaEgressGateInput, dsID string, trigger schemaprivacy.TriggerSource) error {
	ds, gErr := in.Service.GetDatasource(ctx, dsID)
	if gErr != nil {
		attribution := agentaudit.PolicyAttribution("schema_egress_revoked_midflight")
		_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, in.Service, in.Source, in.AccessKey, in.ToolName, in.Params, agentaudit.StatusError, gErr.Error(), attribution)
		return gErr
	}
	if gateErr := schemaprivacy.Gate(in.Store, ds, trigger, schemaprivacy.SendSummary{}); gateErr != nil {
		attribution := agentaudit.PolicyAttribution("schema_egress_revoked_midflight")
		_ = agentaudit.AppendToolCallWithAttribution(in.DataPath, in.Service, in.Source, in.AccessKey, in.ToolName, in.Params, agentaudit.StatusError, gateErr.Error(), attribution)
		return gateErr
	}
	return nil
}
