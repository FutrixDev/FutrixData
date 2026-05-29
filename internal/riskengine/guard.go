package riskengine

import (
	"context"
	"errors"
	"strings"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
)

// Guard implements console.ExecuteInterceptor using the risk engine.
type Guard struct {
	engine        *Engine
	probeProvider ProbeProvider
}

// NewGuard creates a new Guard that intercepts Execute calls.
func NewGuard(engine *Engine) *Guard {
	return &Guard{engine: engine}
}

// SetProbeProvider sets the probe provider for dynamic risk assessment.
func (g *Guard) SetProbeProvider(p ProbeProvider) {
	g.probeProvider = p
}

func (g *Guard) ApplyExecuteOptionsCaps(_ context.Context, ds datasource.DataSource, statement string, opts *console.ExecuteOptions) error {
	if g == nil || g.engine == nil || opts == nil || ds.Type != datasource.TypeDynamoDB || !opts.Bounds.Enabled() {
		return nil
	}
	policy := g.engine.ProbePolicyForParsed(ParseStatement(string(ds.Type), ds.ID, statement))
	return ApplyDynamoDBExecutionPolicyCaps(ds, opts, policy)
}

func ApplyDynamoDBExecutionPolicyCaps(ds datasource.DataSource, opts *console.ExecuteOptions, policy ProbePolicy) error {
	if opts == nil || ds.Type != datasource.TypeDynamoDB || !opts.Bounds.Enabled() {
		return nil
	}
	opts.CaptureRequestedBounds()
	requested := opts.RequestedExecutionBounds()
	effective := opts.Bounds
	violations := make([]console.ExecutionLimitViolation, 0, 2)

	if policy.MaxDynamoDBPages > 0 {
		switch {
		case opts.Bounds.MaxPages > policy.MaxDynamoDBPages:
			if opts.Bounds.StrictLimits {
				violations = append(violations, console.ExecutionLimitViolation{Name: "maxPages", Requested: opts.Bounds.MaxPages, Maximum: policy.MaxDynamoDBPages})
			} else {
				opts.Bounds.MaxPages = policy.MaxDynamoDBPages
				opts.AddClampedLimit("maxPages")
			}
		case opts.Bounds.MaxPages <= 0:
			opts.Bounds.MaxPages = policy.MaxDynamoDBPages
		}
	}
	if policy.MaxDynamoDBEvaluatedItems > 0 {
		switch {
		case opts.Bounds.MaxEvaluatedItems > policy.MaxDynamoDBEvaluatedItems:
			if opts.Bounds.StrictLimits {
				violations = append(violations, console.ExecutionLimitViolation{Name: "maxEvaluatedItems", Requested: opts.Bounds.MaxEvaluatedItems, Maximum: policy.MaxDynamoDBEvaluatedItems})
			} else {
				opts.Bounds.MaxEvaluatedItems = policy.MaxDynamoDBEvaluatedItems
				opts.AddClampedLimit("maxEvaluatedItems")
			}
		case opts.Bounds.MaxEvaluatedItems <= 0:
			opts.Bounds.MaxEvaluatedItems = policy.MaxDynamoDBEvaluatedItems
		}
	}
	if len(violations) > 0 {
		effective.MaxPages = cappedPositiveInt(effective.MaxPages, policy.MaxDynamoDBPages)
		effective.MaxEvaluatedItems = cappedPositiveInt(effective.MaxEvaluatedItems, policy.MaxDynamoDBEvaluatedItems)
		return &console.ExecutionLimitError{
			Kind:           "dynamodb-execution-limits",
			DatasourceType: string(ds.Type),
			Requested:      requested,
			Effective:      effective,
			Violations:     violations,
		}
	}
	return nil
}

func cappedPositiveInt(value, cap int) int {
	if cap <= 0 {
		return value
	}
	if value <= 0 || value > cap {
		return cap
	}
	return value
}

// Assess returns the full risk assessment plus any probe results. It is the
// pure-fact entrypoint consumed by callers that need to combine the assessment
// with a trust level via DecideGate (AI Chat, MCP, CLI). It never wraps the
// result in an error — callers that enforce the assessment as an interceptor
// should use BeforeExecute instead.
func (g *Guard) Assess(ctx context.Context, ds datasource.DataSource, statement string) (RiskAssessment, *console.ExplainResult, string) {
	if g.engine == nil {
		return RiskAssessment{Level: RiskLow, Action: ActionAllow}, nil, ""
	}

	ps := ParseStatement(string(ds.Type), ds.ID, statement)
	assessment := g.engine.AssessParsed(ps)
	probePolicy := g.engine.ProbePolicyForParsed(ps)
	probePS := ps
	var explainResult *console.ExplainResult

	// Probe low and medium-risk statements so direct execution can fail closed
	// when the execution path is still too expensive or unverifiable.
	if (assessment.Action == ActionAllow || assessment.Action == ActionWarn) && g.probeProvider != nil {
		probe := g.runProbe(ctx, ds, ps)
		explainResult = probe.ExplainResult
		if resolvedPS, ok := resolvedParsedStatementForView(ps, probe); ok {
			probePS = resolvedPS
			assessment = stricterAssessment(assessment, g.engine.AssessParsed(resolvedPS))
			probePolicy = stricterProbePolicy(probePolicy, g.engine.ProbePolicyForParsed(resolvedPS))
		}
		assessment = AssessWithProbePolicy(assessment, probePS, probe, probePolicy)
	}

	return assessment, explainResult, ps.TargetEntity
}

// BeforeExecute evaluates the statement against risk rules and wraps the
// result in a BlockedError when the engine wants the execution stopped. It
// implements the console.ExecuteInterceptor contract for Manager.Execute; AI
// Chat, MCP, and CLI paths should prefer Assess + DecideGate instead.
func (g *Guard) BeforeExecute(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) error {
	assessment, explainResult, targetEntity := g.Assess(ctx, ds, statement)
	switch assessment.Action {
	case ActionWarn, ActionBlock, ActionRequireApproval:
		if console.AllowsInteractiveApprovalBypass(opts) {
			return nil
		}
		return &BlockedError{Assessment: assessment, TargetEntity: targetEntity, Explain: explainResult}
	default:
		return nil
	}
}

func (g *Guard) runProbe(ctx context.Context, ds datasource.DataSource, ps ParsedStatement) ProbeResult {
	result := ProbeResult{}
	dsType := strings.ToLower(strings.TrimSpace(string(ds.Type)))

	switch dsType {
	case "mysql", "postgresql", "d1", "mongodb":
		if dsType == "d1" && ps.TargetEntity != "" {
			describe, err := g.probeProvider.DescribeEntity(ctx, ds, ps.TargetEntity)
			if err != nil {
				result.DescribeErr = err
			} else {
				result.DescribeResult = &describe
				if strings.EqualFold(describe.EntityKind, "view") && strings.TrimSpace(describe.DefinitionSQL) != "" {
					viewFacts, factsErr := console.SQLViewRiskFactsForDefinition(ps.TargetEntity, describe.DefinitionSQL, ps.DsType)
					if factsErr != nil {
						result.ViewParseErr = factsErr
					} else {
						result.ViewResult = &ViewProbeResult{
							ViewEntity:    viewFacts.ViewEntity,
							EntityKind:    describe.EntityKind,
							DefinitionSQL: describe.DefinitionSQL,
							BaseEntities:  append([]string(nil), viewFacts.BaseEntities...),
							EntityNameMap: cloneStringMap(viewFacts.EntityNameMap),
							InnerFacts:    viewFacts.Inner,
						}
					}
				}
			}
		}
		if !supportsExplainProbe(ps) {
			result.ExplainSkipped = true
			return result
		}
		explain, err := g.probeProvider.Explain(ctx, ds, ps.Raw)
		if err != nil {
			if errors.Is(err, console.ErrUnsupported) {
				result.ExplainSkipped = true
			} else {
				result.ExplainErr = err
			}
		} else {
			result.ExplainResult = &explain
		}
	case "dynamodb":
		if ps.TargetEntity != "" {
			describe, err := g.probeProvider.DescribeEntity(ctx, ds, ps.TargetEntity)
			if err != nil {
				result.DescribeErr = err
			} else {
				result.DescribeResult = &describe
				result.DescribeEntity = ps.TargetEntity
			}
		}
	case "elasticsearch":
		// ES probe is done via static analysis in AssessWithProbe, no network call needed
	}

	return result
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func supportsExplainProbe(ps ParsedStatement) bool {
	switch ps.DsType {
	case "mysql", "postgresql", "d1":
		switch strings.ToLower(strings.TrimSpace(ps.FirstKeyword)) {
		case "select", "insert", "update", "delete", "replace":
			return true
		default:
			return false
		}
	case "mongodb":
		switch NormalizeMongoAction(ps.MongoAction) {
		case "find", "aggregate", "updateone", "updatemany", "replaceone", "deleteone", "deletemany", "findoneandupdate", "findoneandreplace", "findoneanddelete":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func resolvedParsedStatementForView(ps ParsedStatement, probe ProbeResult) (ParsedStatement, bool) {
	if probe.ViewResult == nil || len(probe.ViewResult.BaseEntities) == 0 {
		return ps, false
	}
	resolved := ps
	entities := make([]string, 0, 1+len(probe.ViewResult.BaseEntities))
	if ps.TargetEntity != "" {
		entities = append(entities, ps.TargetEntity)
	}
	for _, entity := range probe.ViewResult.BaseEntities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		duplicate := false
		for _, existing := range entities {
			if strings.EqualFold(existing, entity) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			entities = append(entities, entity)
		}
	}
	if len(entities) == 0 {
		return ps, false
	}
	resolved.TargetEntities = entities
	return resolved, true
}

func stricterAssessment(current, candidate RiskAssessment) RiskAssessment {
	if riskActionSeverity(candidate.Action) > riskActionSeverity(current.Action) {
		return candidate
	}
	if riskActionSeverity(candidate.Action) < riskActionSeverity(current.Action) {
		return current
	}
	if current.RuleID == "" && candidate.RuleID != "" {
		current.RuleID = candidate.RuleID
	}
	current.Reasons = mergeAssessmentReasons(current.Reasons, candidate.Reasons)
	current.Level = actionToRiskLevel(current.Action)
	return current
}

func mergeAssessmentReasons(current, candidate []string) []string {
	if len(candidate) == 0 {
		return current
	}
	merged := append([]string{}, current...)
	for _, reason := range candidate {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		found := false
		for _, existing := range merged {
			if existing == reason {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, reason)
		}
	}
	return merged
}

func stricterProbePolicy(current, candidate ProbePolicy) ProbePolicy {
	current = current.normalized()
	candidate = candidate.normalized()
	if candidate.MaxExaminedRows < current.MaxExaminedRows {
		current.MaxExaminedRows = candidate.MaxExaminedRows
	}
	if candidate.MaxJoinCount < current.MaxJoinCount {
		current.MaxJoinCount = candidate.MaxJoinCount
	}
	if candidate.MaxFullScans < current.MaxFullScans {
		current.MaxFullScans = candidate.MaxFullScans
	}
	if candidate.MaxEstimatedJoinRows < current.MaxEstimatedJoinRows {
		current.MaxEstimatedJoinRows = candidate.MaxEstimatedJoinRows
	}
	if candidate.SeqScanRowsThreshold < current.SeqScanRowsThreshold {
		current.SeqScanRowsThreshold = candidate.SeqScanRowsThreshold
	}
	if candidate.CostThreshold < current.CostThreshold {
		current.CostThreshold = candidate.CostThreshold
	}
	if candidate.MaxDynamoDBPages < current.MaxDynamoDBPages {
		current.MaxDynamoDBPages = candidate.MaxDynamoDBPages
	}
	if candidate.MaxDynamoDBEvaluatedItems < current.MaxDynamoDBEvaluatedItems {
		current.MaxDynamoDBEvaluatedItems = candidate.MaxDynamoDBEvaluatedItems
	}
	if !candidate.AllowSafeSeqScan {
		current.AllowSafeSeqScan = false
	}
	return current
}

func riskActionSeverity(action Action) int {
	switch action {
	case ActionAllow:
		return 0
	case ActionWarn:
		return 1
	case ActionRequireApproval:
		return 2
	case ActionBlock:
		return 3
	default:
		return 1
	}
}

// Ensure Guard implements console.ExecuteInterceptor at compile time.
var _ console.ExecuteInterceptor = (*Guard)(nil)
