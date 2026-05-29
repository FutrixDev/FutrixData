package toolreg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

// DatasourceIDFromParams extracts the target datasource identifier from a tool
// call argument map. Both `datasourceId` and `id` are accepted — scoped-by-id
// tools (execute_statement, list_entities, etc.) use the former, while tools
// that operate on a single datasource (get_datasource, delete_datasource) use
// the latter. Returns the empty string when neither key is present.
func DatasourceIDFromParams(p map[string]any) string {
	for _, key := range []string{"datasourceId", "id"} {
		v, ok := p[key]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s != "" {
			return s
		}
	}
	return ""
}

// DatasourceIDFromToolDef extracts a datasource target only for tools whose
// declared schema actually has a datasourceId parameter. This avoids treating
// unrelated `id` fields, such as risk-rule ids, as datasource ids when
// enforcing agent datasource scopes.
func DatasourceIDFromToolDef(def ToolDef, p map[string]any) string {
	for _, param := range def.Params {
		if param.Name == "datasourceId" {
			return DatasourceIDFromParams(p)
		}
	}
	return ""
}

// ApprovalDecision is the structured result of an approval evaluation. It
// carries both the boolean gate signal and the underlying risk assessment
// (when one was produced) so audit-writing call sites can persist the
// matched rule alongside the decision. Assessment is nil when no risk
// evaluation was performed — for example when the statement or datasource
// id is missing and the decision falls back to a safe-default approval.
type ApprovalDecision struct {
	NeedsApproval bool
	// Blocked is true when the risk engine returned a hard block that should
	// not be converted into an approval prompt. Danger-mode datasources keep
	// their existing bypass behavior and therefore do not set this flag.
	Blocked bool
	// Assessment — the winning rule + reasons + level. Pointer because
	// "no assessment" is meaningfully distinct from "evaluator returned
	// the zero assessment" (the latter is the fallback "no matching rule"
	// state, which still has Action/Reasons set).
	Assessment *riskengine.RiskAssessment
	// BlockAssessment carries the full risk result for hard blocks, including
	// fallback block reasons that may not have a rule id. Assessment remains
	// focused on attribution-worthy matched rules.
	BlockAssessment *riskengine.RiskAssessment
	WritePreview    *console.WritePreview
	// WritePreviewUnavailable means the statement looked previewable, but the
	// count query failed. Keep approval fail-closed without turning transient
	// preview failures into fatal tool errors.
	WritePreviewUnavailable bool
}

type writePreviewStatementService interface {
	PreviewWriteStatement(context.Context, string, string, string, string) (console.WritePreview, error)
}

// BlockedErrorFromDecision converts a hard-block approval decision into the
// same error shape produced by the risk-engine guard, so CLI/MCP/tool-call
// paths report consistent wording and audit attribution.
func BlockedErrorFromDecision(decision ApprovalDecision) *riskengine.BlockedError {
	assessment := riskengine.RiskAssessment{
		Level:   riskengine.RiskHigh,
		Action:  riskengine.ActionBlock,
		Reasons: []string{"blocked by risk rules"},
	}
	if decision.BlockAssessment != nil {
		assessment = *decision.BlockAssessment
	} else if decision.Assessment != nil {
		assessment = *decision.Assessment
	}
	return &riskengine.BlockedError{Assessment: assessment}
}

// AssessStatementApproval is the structured-return counterpart to
// ShouldRequireStatementApproval. Both go through the same evaluator and
// gate-decision logic; this variant additionally surfaces the underlying
// RiskAssessment so audit logs can record which rule matched.
//
// Callers writing audit entries should prefer this; callers that only need
// the boolean (approval prompt rendering, dry-run inspection) can keep
// using the wrapper for clarity.
func AssessStatementApproval(ctx context.Context, svc Service, datasourceID, statement, database, executionMode string) (ApprovalDecision, error) {
	if strings.TrimSpace(datasourceID) == "" {
		return ApprovalDecision{NeedsApproval: true}, nil
	}
	if strings.TrimSpace(statement) == "" {
		return ApprovalDecision{NeedsApproval: true}, nil
	}
	ds, err := svc.GetDatasource(ctx, datasourceID)
	if err != nil {
		return ApprovalDecision{}, err
	}

	assessment, err := svc.AssessStatement(ctx, datasourceID, statement, database, executionMode)
	if err != nil {
		return ApprovalDecision{}, err
	}

	gate := riskengine.DecideGate(ds.TrustLevel(), assessment)
	decision := ApprovalDecision{NeedsApproval: gate == riskengine.GateRequireApproval}
	if ds.TrustLevel() != datasource.TrustDanger && assessment.Action == riskengine.ActionBlock {
		blockAssessment := assessment
		decision.Blocked = true
		decision.BlockAssessment = &blockAssessment
	}
	// Surface the assessment when a real rule signalled risk — i.e. the rule
	// has an ID AND its action is anything other than Allow. Both conditions
	// are required:
	//   - RuleID guards against the engine's synthesised fallbacks (e.g. raw
	//     DDL with no matching rule returns Warn/Medium with empty RuleID;
	//     attributing those to a rule would render "Matched rule: -").
	//   - Action != Allow guards against the TrustApproval-on-allow case
	//     (e.g. SELECT 1 matches sql-allow-read; the rule auto-runs at every
	//     other trust level, so when TrustApproval gates it, trust policy is
	//     the cause, not the rule). Attributing those to risk_engine would
	//     contradict the row's own rule-action display.
	// Warn / RequireApproval / Block rules that match are surfaced — DecideGate
	// then projects (level, trust) onto the gate, so the rule materially drove
	// the decision (e.g. a Warn rule on TrustCautious gates because the rule's
	// Medium level is above TrustCautious's Low threshold).
	if assessment.RuleID != "" && assessment.Action != riskengine.ActionAllow {
		decision.Assessment = &assessment
	}
	if assessment.Action != riskengine.ActionBlock {
		preview, previewUnavailable, err := statementWritePreview(ctx, svc, datasourceID, statement, database, executionMode)
		if err != nil {
			return ApprovalDecision{}, err
		}
		if previewUnavailable {
			decision.NeedsApproval = true
			decision.WritePreviewUnavailable = true
		}
		if preview != nil {
			decision.WritePreview = preview
			if preview.RequiresElevatedApproval {
				decision.NeedsApproval = true
			}
		}
	}
	return decision, nil
}

func AssessRedisCommandApproval(ctx context.Context, svc Service, datasourceID string, args []string, database, executionMode string) (ApprovalDecision, error) {
	if strings.TrimSpace(datasourceID) == "" {
		return ApprovalDecision{NeedsApproval: true}, nil
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return ApprovalDecision{NeedsApproval: true}, nil
	}
	ds, err := svc.GetDatasource(ctx, datasourceID)
	if err != nil {
		return ApprovalDecision{}, err
	}
	if ds.Type != datasource.TypeRedis && ds.Type != datasource.TypeRedisCluster {
		return ApprovalDecision{NeedsApproval: true}, nil
	}

	assessment, err := svc.AssessRedisCommand(ctx, datasourceID, args, database, executionMode)
	if err != nil {
		return ApprovalDecision{}, err
	}

	gate := riskengine.DecideGate(ds.TrustLevel(), assessment)
	decision := ApprovalDecision{NeedsApproval: gate == riskengine.GateRequireApproval}
	if ds.TrustLevel() != datasource.TrustDanger && assessment.Action == riskengine.ActionBlock {
		blockAssessment := assessment
		decision.Blocked = true
		decision.BlockAssessment = &blockAssessment
	}
	if assessment.RuleID != "" && assessment.Action != riskengine.ActionAllow {
		decision.Assessment = &assessment
	}
	return decision, nil
}

func statementWritePreview(ctx context.Context, svc Service, datasourceID, statement, database, executionMode string) (*console.WritePreview, bool, error) {
	previewSvc, ok := svc.(writePreviewStatementService)
	if !ok {
		return nil, false, nil
	}
	preview, err := previewSvc.PreviewWriteStatement(ctx, datasourceID, statement, database, executionMode)
	if err != nil {
		if errors.Is(err, console.ErrUnsupported) {
			return nil, false, nil
		}
		return nil, true, nil
	}
	return &preview, false, nil
}

// ShouldRequireStatementApproval decides whether an execute_statement call
// needs explicit approval. Thin wrapper kept for callers that only need
// the boolean (approval prompt rendering, dry-run inspection) and don't
// write audit entries.
//
// Combines the riskengine assessment with the datasource's trust level via
// riskengine.DecideGate — so MCP, CLI, and AI Chat all agree on outcomes,
// including cases where a statement only reveals its true cost via EXPLAIN.
func ShouldRequireStatementApproval(ctx context.Context, svc Service, datasourceID, statement, database, executionMode string) (bool, error) {
	decision, err := AssessStatementApproval(ctx, svc, datasourceID, statement, database, executionMode)
	if err != nil {
		return false, err
	}
	return decision.NeedsApproval, nil
}

// DatasourceTrustLevel looks up the trust level of the referenced datasource.
// Returns TrustApproval (the safest default) when the id is blank, the
// service is nil, or the datasource cannot be resolved. Callers that need to
// know whether a datasource is in danger mode should compare the result to
// datasource.TrustDanger instead of relying on a deleted IsDangerous helper.
func DatasourceTrustLevel(ctx context.Context, svc Service, id string) datasource.TrustLevel {
	id = strings.TrimSpace(id)
	if id == "" || svc == nil {
		return datasource.TrustApproval
	}
	ds, err := svc.GetDatasource(ctx, id)
	if err != nil {
		return datasource.TrustApproval
	}
	return ds.TrustLevel()
}

// IsDatasourceDangerous is a convenience used by the MCP surface to gate the
// guard-bypass context flag. Kept as a thin wrapper so the call site reads
// naturally.
func IsDatasourceDangerous(ctx context.Context, svc Service, id string) bool {
	return DatasourceTrustLevel(ctx, svc, id) == datasource.TrustDanger
}
