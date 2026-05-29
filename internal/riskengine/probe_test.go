package riskengine

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
)

func TestAssessWithProbe_SQLNoIndex(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mysql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{UsesIndex: false},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn", result.Action)
	}
	if result.Level != RiskMedium {
		t.Errorf("Level = %s, want medium", result.Level)
	}
}

func TestAssessWithProbe_SeqScanSmallTable(t *testing.T) {
	// PG optimizer chooses Seq Scan on a small table (18 rows, cost 2.6).
	// This is a safe optimization decision and should NOT trigger a warning.
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			TotalDocsExamined: 18,
			MaxSeqScanRows:    18,
			TotalCost:         2.6,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Errorf("small table Seq Scan: Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_SeqScanLargeTable(t *testing.T) {
	// Seq Scan on a large table (50,000 rows) should still trigger a warning.
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			TotalDocsExamined: 50000,
			MaxSeqScanRows:    50000,
			TotalCost:         5000,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("large table Seq Scan: Action = %s, want warn", result.Action)
	}
}

func TestAssessWithProbe_SeqScanMidRangeTable(t *testing.T) {
	// Seq Scan on a 5,000-row table with low cost. This is in the range
	// between MaxExaminedRows (1,000) and SeqScanRowsThreshold (10,000).
	// The seq-scan exemption should also exempt the examined-rows gate.
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			TotalDocsExamined: 5000,
			MaxSeqScanRows:    5000,
			TotalCost:         50,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Errorf("mid-range table Seq Scan (5000 rows, low cost): Action = %s, want allow; reasons: %v", result.Action, result.Reasons)
	}
}

func TestAssessWithProbe_SeqScanSmallRowsHighCost(t *testing.T) {
	// Small row count but high cost (e.g., expensive expression evaluation).
	// Should still trigger a warning because cost exceeds threshold.
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			TotalDocsExamined: 500,
			MaxSeqScanRows:    500,
			TotalCost:         1500,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("small rows + high cost Seq Scan: Action = %s, want warn", result.Action)
	}
}

func TestAssessWithProbe_SeqScanSmallTable_ToggleDisabled(t *testing.T) {
	// Same small-table Seq Scan as TestAssessWithProbe_SeqScanSmallTable,
	// but with AllowSafeSeqScan disabled. Should trigger "no index detected".
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			TotalDocsExamined: 18,
			MaxSeqScanRows:    18,
			TotalCost:         2.6,
		},
	}
	policy := DefaultProbePolicy()
	policy.AllowSafeSeqScan = false
	result := AssessWithProbePolicy(base, ps, probe, policy)
	if result.Action != ActionWarn {
		t.Errorf("safe seq scan disabled: Action = %s, want warn", result.Action)
	}
	hasNoIndex := false
	for _, r := range result.Reasons {
		if contains(r, "no index") {
			hasNoIndex = true
		}
	}
	if !hasNoIndex {
		t.Errorf("expected 'no index detected' reason when AllowSafeSeqScan=false, got %v", result.Reasons)
	}
}

func TestAssessWithProbe_SQLWithIndex(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mysql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         true,
			TotalDocsExamined: 50,
			TotalKeysExamined: 50,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_SQLExaminedOverThreshold(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         true,
			TotalDocsExamined: 5000,
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn", result.Action)
	}
}

func TestAssessWithProbePolicy_UsesConfiguredExaminedThreshold(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "postgresql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         true,
			TotalDocsExamined: 500,
		},
	}

	allowResult := AssessWithProbePolicy(base, ps, probe, ProbePolicy{MaxExaminedRows: 1000, MaxJoinCount: 4, MaxFullScans: 1, MaxEstimatedJoinRows: 10000})
	if allowResult.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", allowResult.Action)
	}

	warnResult := AssessWithProbePolicy(base, ps, probe, ProbePolicy{MaxExaminedRows: 200, MaxJoinCount: 4, MaxFullScans: 1, MaxEstimatedJoinRows: 10000})
	if warnResult.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", warnResult.Action)
	}
}

func TestAssessWithProbe_SQLExplainError(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mysql"}
	probe := ProbeResult{
		ExplainErr: fmt.Errorf("explain failed"),
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn", result.Action)
	}
}

func TestAssessWithProbe_SQLExplainSkipped(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mysql"}
	probe := ProbeResult{ExplainSkipped: true}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow (skipped)", result.Action)
	}
}

func TestAssessWithProbe_HighRiskPassthrough(t *testing.T) {
	base := RiskAssessment{Level: RiskHigh, Action: ActionBlock, Reasons: []string{"DROP"}}
	ps := ParsedStatement{DsType: "mysql"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{UsesIndex: true, TotalDocsExamined: 1},
	}
	// High risk should not be downgraded by probe
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionBlock {
		t.Errorf("Action = %s, want block (high risk should pass through)", result.Action)
	}
}

func TestAssessWithProbe_MongoDB(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mongodb"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{UsesIndex: false},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (no index)", result.Action)
	}
}

func TestAssessWithProbe_MongoDBCollectionScanReason(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "mongodb", MongoAction: "find"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex:         false,
			Stages:            []string{"COLLSCAN", "SORT"},
			TotalDocsExamined: 2500,
			Detail: map[string]any{
				"queryPlanner":   map[string]any{"winningPlan": map[string]any{"stage": "COLLSCAN"}},
				"executionStats": map[string]any{"totalDocsExamined": 2500},
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	joined := strings.Join(result.Reasons, " | ")
	if !contains(joined, "collection scan over 1000 docs") {
		t.Fatalf("Reasons = %#v, want collection scan reason", result.Reasons)
	}
	if !contains(joined, "blocking sort over 1000 docs") {
		t.Fatalf("Reasons = %#v, want blocking sort reason", result.Reasons)
	}
}

func TestAnalyzeExplainDetail_PostgresSortStageUsesPostgresPath(t *testing.T) {
	explain := console.ExplainResult{
		Stages: []string{"Sort"},
		Detail: []any{
			map[string]any{
				"Plan": map[string]any{
					"Node Type": "Nested Loop",
					"Plans": []any{
						map[string]any{"Node Type": "Seq Scan", "Plan Rows": float64(100)},
						map[string]any{"Node Type": "Seq Scan", "Plan Rows": float64(200)},
					},
				},
			},
		},
	}

	risks := AnalyzeExplainDetail(explain)
	joined := strings.Join(risks, " | ")
	if !contains(joined, "2 sequential scans") {
		t.Fatalf("Risks = %#v, want postgres sequential scan analysis", risks)
	}
}

func TestAnalyzeExplainDetail_MongoCollectionScanCountUsesRawDetail(t *testing.T) {
	explain := console.ExplainResult{
		Stages:            []string{"COLLSCAN"},
		TotalDocsExamined: 50,
		Detail: map[string]any{
			"queryPlanner": map[string]any{
				"winningPlan": map[string]any{
					"stage": "COLLSCAN",
					"inputStage": map[string]any{
						"stage": "COLLSCAN",
					},
				},
			},
		},
	}

	risks := AnalyzeExplainDetailWithPolicy(explain, ProbePolicy{
		MaxExaminedRows:      1000,
		MaxJoinCount:         4,
		MaxFullScans:         1,
		MaxEstimatedJoinRows: 10000,
	})
	joined := strings.Join(risks, " | ")
	if !contains(joined, "2 collection scans in MongoDB plan") {
		t.Fatalf("Risks = %#v, want repeated collection scan warning", risks)
	}
}

func TestAnalyzeExplainDetail_MongoRejectedPlansDoNotInflateStageCounts(t *testing.T) {
	explain := console.ExplainResult{
		Stages:            []string{"IXSCAN"},
		TotalDocsExamined: 50,
		Detail: map[string]any{
			"queryPlanner": map[string]any{
				"winningPlan": map[string]any{
					"stage": "IXSCAN",
				},
				"rejectedPlans": []any{
					map[string]any{"stage": "COLLSCAN"},
					map[string]any{"stage": "COLLSCAN"},
				},
			},
		},
	}

	risks := AnalyzeExplainDetailWithPolicy(explain, ProbePolicy{
		MaxExaminedRows:      1000,
		MaxJoinCount:         4,
		MaxFullScans:         1,
		MaxEstimatedJoinRows: 10000,
	})
	joined := strings.Join(risks, " | ")
	if contains(joined, "collection scans in MongoDB plan") {
		t.Fatalf("Risks = %#v, rejected plans should not count toward collection-scan warnings", risks)
	}
}

func TestAnalyzeExplainDetail_MongoExecutionStatsDoNotDoubleCountWinningPlan(t *testing.T) {
	explain := console.ExplainResult{
		Stages:            []string{"COLLSCAN"},
		TotalDocsExamined: 50,
		Detail: map[string]any{
			"queryPlanner": map[string]any{
				"winningPlan": map[string]any{
					"stage": "COLLSCAN",
				},
			},
			"executionStats": map[string]any{
				"executionStages": map[string]any{
					"stage": "COLLSCAN",
				},
			},
		},
	}

	risks := AnalyzeExplainDetailWithPolicy(explain, ProbePolicy{
		MaxExaminedRows:      1000,
		MaxJoinCount:         4,
		MaxFullScans:         1,
		MaxEstimatedJoinRows: 10000,
	})
	joined := strings.Join(risks, " | ")
	if contains(joined, "2 collection scans in MongoDB plan") {
		t.Fatalf("Risks = %#v, winning plan should not be double-counted across queryPlanner and executionStats", risks)
	}
}

func TestAssessWithProbe_D1ViewScanHeavyQueryWarns(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "conversion_stats"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex: false,
			Stages: []string{
				"CO-ROUTINE conversion_stats",
				"SCAN conversions",
				"USE TEMP B-TREE FOR GROUP BY",
				"SCAN conversion_stats",
			},
		},
		ViewResult: &ViewProbeResult{
			ViewEntity:    "conversion_stats",
			EntityKind:    "view",
			DefinitionSQL: "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format",
			BaseEntities:  []string{"conversions"},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if !contains(strings.Join(result.Reasons, " | "), "scan on conversions") {
		t.Fatalf("Reasons = %#v, want scan reason", result.Reasons)
	}
	if !contains(strings.Join(result.Reasons, " | "), "temporary grouping structure") {
		t.Fatalf("Reasons = %#v, want temp grouping reason", result.Reasons)
	}
}

func TestAssessWithProbe_D1ViewIndexedQueryCanStayAllow(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "active_users"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH users USING INDEX idx_users_status (status=?)",
			},
			TotalDocsExamined: 20,
		},
		ViewResult: &ViewProbeResult{
			ViewEntity:    "active_users",
			EntityKind:    "view",
			DefinitionSQL: "CREATE VIEW active_users AS SELECT * FROM users WHERE status = 'active'",
			BaseEntities:  []string{"users"},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_D1ViewAliasedScanWarns(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "order_summary"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SCAN o",
				"SEARCH users USING INDEX idx_users_id (id=?)",
			},
		},
		ViewResult: &ViewProbeResult{
			ViewEntity:    "order_summary",
			EntityKind:    "view",
			DefinitionSQL: "CREATE VIEW order_summary AS SELECT * FROM users u JOIN orders o ON o.user_id = u.id",
			BaseEntities:  []string{"users", "orders"},
			EntityNameMap: map[string]string{"u": "users", "users": "users", "o": "orders", "orders": "orders"},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if !contains(strings.Join(result.Reasons, " | "), "scan on orders") {
		t.Fatalf("Reasons = %#v, want aliased scan reason", result.Reasons)
	}
}

func TestAssessWithProbe_D1IndexedTableQueryWithoutRowEstimateCanStayAllow(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "conversions"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_D1MixedScanAndIndexPlanWarns(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "users"}
	probe := ProbeResult{
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH users USING INDEX idx_users_id (id=?)",
				"SCAN orders",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if !contains(strings.Join(result.Reasons, " | "), "execution path not verified") {
		t.Fatalf("Reasons = %#v, want execution-path reason", result.Reasons)
	}
}

func TestAssessWithProbe_D1DescribeFailureWarnsEvenWhenIndexed(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "conversion_stats"}
	probe := ProbeResult{
		DescribeErr: fmt.Errorf("describe failed"),
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"CO-ROUTINE conversion_stats",
				"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if !contains(strings.Join(result.Reasons, " | "), "view definition not verified") {
		t.Fatalf("Reasons = %#v, want view verification reason", result.Reasons)
	}
}

func TestAssessWithProbe_D1TableDescribeFailureIndexedQueryCanStayAllow(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{DsType: "d1", TargetEntity: "conversions"}
	probe := ProbeResult{
		DescribeErr: fmt.Errorf("describe failed"),
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH conversions USING INTEGER PRIMARY KEY (rowid=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_D1FlattenedViewDescribeFailureWarns(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "d1",
		TargetEntity: "conversion_stats",
		Raw:          "SELECT * FROM conversion_stats LIMIT 50",
	}
	probe := ProbeResult{
		DescribeErr: fmt.Errorf("describe failed"),
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH conversions USING INDEX idx_conversions_format (format=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if !contains(strings.Join(result.Reasons, " | "), "view definition not verified") {
		t.Fatalf("Reasons = %#v, want view verification reason", result.Reasons)
	}
}

func TestAssessWithProbe_D1TableAliasDescribeFailureIndexedQueryCanStayAllow(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "d1",
		TargetEntity: "conversions",
		Raw:          "SELECT * FROM conversions c WHERE c.id = 1",
	}
	probe := ProbeResult{
		DescribeErr: fmt.Errorf("describe failed"),
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH c USING INTEGER PRIMARY KEY (rowid=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_D1BacktickTableAliasDescribeFailureIndexedQueryCanStayAllow(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "d1",
		TargetEntity: "conversions",
		Raw:          "SELECT * FROM `conversions` c WHERE c.id = 1",
	}
	probe := ProbeResult{
		DescribeErr: fmt.Errorf("describe failed"),
		ExplainResult: &console.ExplainResult{
			UsesIndex: true,
			Stages: []string{
				"SEARCH c USING INTEGER PRIMARY KEY (rowid=?)",
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAssessWithProbe_D1DescribeFailureWithoutExplainEvidenceDoesNotWarn(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "d1",
		TargetEntity: "conversions",
		Raw:          "SELECT * FROM conversions",
	}
	probe := ProbeResult{
		DescribeErr:    fmt.Errorf("describe failed"),
		ExplainSkipped: true,
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow", result.Action)
	}
}

func TestAnalyzeExplainDetail_MySQLMultipleFullScans(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: false,
		Detail: []any{
			map[string]any{"select_type": "SIMPLE", "table": "orders", "type": "ALL", "rows": "50000", "Extra": ""},
			map[string]any{"select_type": "SIMPLE", "table": "users", "type": "ALL", "rows": "10000", "Extra": "Using join buffer"},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	hasCartesian := false
	for _, r := range risks {
		if contains(r, "full table scans") {
			hasCartesian = true
		}
	}
	if !hasCartesian {
		t.Errorf("expected cartesian product warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_MySQLDependentSubquery(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: true,
		Detail: []any{
			map[string]any{"select_type": "PRIMARY", "table": "orders", "type": "ref", "rows": "100", "key": "idx_user", "Extra": ""},
			map[string]any{"select_type": "DEPENDENT SUBQUERY", "table": "users", "type": "eq_ref", "rows": "1", "key": "PRIMARY", "Extra": ""},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	hasDependent := false
	for _, r := range risks {
		if contains(r, "dependent subquery") {
			hasDependent = true
		}
	}
	if !hasDependent {
		t.Errorf("expected dependent subquery warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_MySQLTempTableFilesort(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: true,
		Detail: []any{
			map[string]any{"select_type": "SIMPLE", "table": "orders", "type": "ref", "rows": "5000", "key": "idx_date", "Extra": "Using temporary; Using filesort"},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	hasTemp := false
	hasFilesort := false
	for _, r := range risks {
		if contains(r, "temporary table") {
			hasTemp = true
		}
		if contains(r, "filesort") {
			hasFilesort = true
		}
	}
	if !hasTemp {
		t.Errorf("expected temporary table warning, got %v", risks)
	}
	if !hasFilesort {
		t.Errorf("expected filesort warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_MySQLManyJoins(t *testing.T) {
	rows := make([]any, 6)
	for i := range rows {
		rows[i] = map[string]any{
			"select_type": "SIMPLE",
			"table":       fmt.Sprintf("t%d", i),
			"type":        "ref",
			"rows":        "10",
			"key":         "idx",
			"Extra":       "",
		}
	}
	explain := console.ExplainResult{UsesIndex: true, Detail: rows}
	risks := AnalyzeExplainDetail(explain)
	hasManyTables := false
	for _, r := range risks {
		if contains(r, "tables joined") {
			hasManyTables = true
		}
	}
	if !hasManyTables {
		t.Errorf("expected many tables warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_MySQLJoinEstimateOverflowStillWarns(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: true,
		Detail: []any{
			map[string]any{"select_type": "SIMPLE", "table": "a", "type": "ref", "rows": fmt.Sprintf("%d", math.MaxInt64), "key": "idx_a"},
			map[string]any{"select_type": "SIMPLE", "table": "b", "type": "ref", "rows": "2", "key": "idx_b"},
		},
	}
	risks := AnalyzeExplainDetailWithPolicy(explain, ProbePolicy{
		MaxExaminedRows:      1000,
		MaxJoinCount:         4,
		MaxFullScans:         1,
		MaxEstimatedJoinRows: 10000,
	})
	found := false
	for _, risk := range risks {
		if contains(risk, "estimated") && contains(risk, "rows across joins") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected overflow-safe join estimate warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_MySQLSafeQuery(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: true,
		Detail: []any{
			map[string]any{"select_type": "SIMPLE", "table": "users", "type": "ref", "rows": "1", "key": "PRIMARY", "Extra": "Using where"},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	if len(risks) > 0 {
		t.Errorf("expected no risks for safe query, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_PostgresJoinsAndSeqScans(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: false,
		Detail: []any{
			map[string]any{
				"Plan": map[string]any{
					"Node Type": "Nested Loop",
					"Plan Rows": float64(5000),
					"Plans": []any{
						map[string]any{
							"Node Type": "Seq Scan",
							"Plan Rows": float64(100),
						},
						map[string]any{
							"Node Type": "Seq Scan",
							"Plan Rows": float64(50),
						},
					},
				},
			},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	hasSeqScans := false
	for _, r := range risks {
		if contains(r, "sequential scans") {
			hasSeqScans = true
		}
	}
	if !hasSeqScans {
		t.Errorf("expected sequential scans warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_PostgresSortLargeSet(t *testing.T) {
	explain := console.ExplainResult{
		UsesIndex: true,
		Detail: []any{
			map[string]any{
				"Plan": map[string]any{
					"Node Type": "Sort",
					"Plan Rows": float64(50000),
					"Plans": []any{
						map[string]any{
							"Node Type":  "Index Scan",
							"Plan Rows":  float64(50000),
							"Index Name": "idx_created",
						},
					},
				},
			},
		},
	}
	risks := AnalyzeExplainDetail(explain)
	hasSort := false
	for _, r := range risks {
		if contains(r, "sort on large") {
			hasSort = true
		}
	}
	if !hasSort {
		t.Errorf("expected sort warning, got %v", risks)
	}
}

func TestAnalyzeExplainDetail_NilDetail(t *testing.T) {
	explain := console.ExplainResult{UsesIndex: true, Detail: nil}
	risks := AnalyzeExplainDetail(explain)
	if len(risks) != 0 {
		t.Errorf("expected no risks for nil detail, got %v", risks)
	}
}

func TestAssessWithProbe_ES_LowRisk(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"term":{"status":"error"}},"size":10}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow (bounded ES search)", result.Action)
	}
}

func TestAssessWithProbe_ES_UnboundedSearch(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (unbounded ES search)", result.Action)
	}
}

func TestAssessWithProbe_ES_DeepPagination(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"term":{"status":"ok"}},"from":15000,"size":10}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (deep pagination)", result.Action)
	}
}

func TestAssessWithProbe_ES_ScriptQuery(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"script_score":{"query":{"match_all":{}},"script":{"source":"_score * 2"}}},"size":10}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (script query)", result.Action)
	}
}

func TestAssessWithProbe_ES_FuzzyQuery(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"fuzzy":{"name":{"value":"test"}}},"size":10}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (fuzzy query)", result.Action)
	}
}

func TestAssessWithProbe_ES_DeepAggs(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body: `{"query":{"match_all":{}},"size":0,"aggs":{
			"l1":{"terms":{"field":"a","size":10},"aggs":{
				"l2":{"terms":{"field":"b","size":10},"aggs":{
					"l3":{"terms":{"field":"c","size":10},"aggs":{
						"l4":{"terms":{"field":"d","size":10}}
					}}
				}}
			}}
		}}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (deep aggs nesting)", result.Action)
	}
}

func TestAssessWithProbe_ES_TermsNoSize(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"term":{"status":"ok"}},"size":0,"aggs":{"by_user":{"terms":{"field":"user_id"}}}}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (terms agg without size)", result.Action)
	}
}

func TestAssessWithProbe_ES_BareAggsNoQuery(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"size":0,"aggs":{"total":{"value_count":{"field":"_id"}}}}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (aggs without query)", result.Action)
	}
}

func TestAssessWithProbe_ES_GetDoc(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "GET",
		URLPath:    "/users/_doc/123",
		Body:       "",
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow (GET doc by ID)", result.Action)
	}
}

func TestAssessWithProbe_ES_GetSearchQueryStringRisks(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "GET",
		URLPath:    "/logs/_search?q=*:*&size=20000",
		Body:       "",
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
}

func TestAssessWithProbe_ES_SafeAggs(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"term":{"status":"ok"}},"size":0,"aggs":{"by_status":{"terms":{"field":"status","size":10}}}}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow (safe aggs with query and size)", result.Action)
	}
}

func TestAssessWithProbe_ES_LargeSize(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:     "elasticsearch",
		HTTPMethod: "POST",
		URLPath:    "/logs/_search",
		Body:       `{"query":{"term":{"status":"ok"}},"size":50000}`,
	}
	result := AssessWithProbe(base, ps, ProbeResult{})
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (size > 10000)", result.Action)
	}
}

func TestAssessWithProbe_DynamoDB_WithPartitionKey(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "dynamodb",
		TargetEntity: "Orders",
		Raw:          "SELECT * FROM Orders WHERE pk = 'abc'",
	}
	probe := ProbeResult{
		DescribeResult: &console.DescribeResult{
			Details: []console.DetailItem{
				{Label: "Partition Key", Value: "pk"},
			},
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Errorf("Action = %s, want allow (has partition key)", result.Action)
	}
}

func TestAssessWithProbe_DynamoDB_NoPartitionKey(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "dynamodb",
		TargetEntity: "Orders",
		Raw:          "SELECT * FROM Orders WHERE status = 'active'",
	}
	probe := ProbeResult{
		DescribeResult: &console.DescribeResult{
			Details: []console.DetailItem{
				{Label: "Partition Key", Value: "pk"},
			},
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Errorf("Action = %s, want warn (no partition key)", result.Action)
	}
}

func TestAssessWithProbe_DynamoDB_IndexAccess(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "dynamodb",
		TargetEntity: "Orders",
		DynamoTable:  "Orders",
		DynamoIndex:  "GenreAndPriceIndex",
		Raw:          `SELECT * FROM "Orders"."GenreAndPriceIndex" WHERE genre = 'rock' AND price BETWEEN 1 AND 10`,
	}
	probe := ProbeResult{
		DescribeEntity: "Orders",
		DescribeResult: &console.DescribeResult{
			Details: []console.DetailItem{
				{Label: "Partition Key", Value: "pk"},
				{Label: "Sort Key", Value: "sk"},
			},
			Indexes: []console.IndexInfo{
				{Name: "GenreAndPriceIndex", Definition: "genre=HASH | price=RANGE | projection=ALL"},
			},
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow (index key access)", result.Action)
	}
}

func TestAssessWithProbe_DynamoDB_UnknownExplicitIndexNamesRequestedAndKnownIndexes(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "dynamodb",
		TargetEntity: "Orders",
		DynamoTable:  "Orders",
		DynamoIndex:  "MissingIndex",
		Raw:          `SELECT * FROM "Orders"."MissingIndex" WHERE genre = 'rock'`,
	}
	probe := ProbeResult{
		DescribeEntity: "Orders",
		DescribeResult: &console.DescribeResult{
			Details: []console.DetailItem{
				{Label: "Partition Key", Value: "pk"},
			},
			Indexes: []console.IndexInfo{
				{Name: "GenreAndPriceIndex", Definition: "genre=HASH | price=RANGE | projection=ALL"},
				{Name: "CustomerIndex", Definition: "customer_id=HASH | created_at=RANGE | projection=ALL"},
			},
		},
	}

	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	got := strings.Join(result.Reasons, "\n")
	for _, want := range []string{"requested index MissingIndex", "known indexes: GenreAndPriceIndex, CustomerIndex"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Reasons = %v, want to contain %q", result.Reasons, want)
		}
	}
}

func TestAssessWithProbe_DynamoDB_SortKeyWithoutPartitionKeyWarns(t *testing.T) {
	base := RiskAssessment{Level: RiskLow, Action: ActionAllow}
	ps := ParsedStatement{
		DsType:       "dynamodb",
		TargetEntity: "Orders",
		DynamoTable:  "Orders",
		Raw:          "SELECT * FROM Orders WHERE sk BETWEEN 100 AND 200",
	}
	probe := ProbeResult{
		DescribeResult: &console.DescribeResult{
			Details: []console.DetailItem{
				{Label: "Partition Key", Value: "pk"},
				{Label: "Sort Key", Value: "sk"},
			},
		},
	}
	result := AssessWithProbe(base, ps, probe)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want warn", result.Action)
	}
	if len(result.Reasons) == 0 || result.Reasons[len(result.Reasons)-1] != "sort key filter without partition key equality looks scan-like" {
		t.Fatalf("Reasons = %v, want scan-like sort key reason", result.Reasons)
	}
}
