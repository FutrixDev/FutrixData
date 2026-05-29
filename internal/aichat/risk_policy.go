package aichat

import (
	"context"

	"futrixdata/platform/internal/riskengine"
)

// ExecuteStatementPolicyInput carries the inputs required to decide whether a
// tool-invoked execute_statement call can bypass the approval gate.
type ExecuteStatementPolicyInput struct {
	Datasource DatasourceSummary
	Statement  string
	Database   string
	RiskGuard  RiskGuard
}

// ShouldAutoExecuteStatementPolicy returns true when the statement may run
// without a user approval prompt. The decision combines the riskengine
// assessment with the target datasource's trust level via
// riskengine.DecideGate — the single source of truth used by MCP, AI Chat, and
// CLI paths.
func ShouldAutoExecuteStatementPolicy(ctx context.Context, input ExecuteStatementPolicyInput) bool {
	if input.RiskGuard == nil {
		return false
	}
	ds := datasourceSummaryToDataSource(input.Datasource)
	assessment, _, _ := input.RiskGuard.Assess(ctx, ds, input.Statement)
	return riskengine.DecideGate(ds.TrustLevel(), assessment) == riskengine.GateAutoRun
}
