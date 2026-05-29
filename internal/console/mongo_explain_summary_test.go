package console

import (
	"reflect"
	"testing"
)

func TestMongoExplainPlanSummary_prefersExecutionStages(t *testing.T) {
	summary := MongoExplainPlanSummaryForResult(ExplainResult{
		Stages: []string{"COLLSCAN"},
		Detail: map[string]any{
			"queryPlanner": map[string]any{
				"winningPlan": map[string]any{
					"stage": "COLLSCAN",
				},
			},
			"executionStats": map[string]any{
				"executionStages": map[string]any{
					"stage": "FETCH",
					"inputStage": map[string]any{
						"stage": "IXSCAN",
					},
				},
			},
		},
	})

	if !reflect.DeepEqual(summary.Stages, []string{"FETCH", "IXSCAN", "COLLSCAN"}) {
		t.Fatalf("Stages = %#v, want execution-stage order with fallback merge", summary.Stages)
	}
	if summary.StageCounts["FETCH"] != 1 || summary.StageCounts["IXSCAN"] != 1 {
		t.Fatalf("StageCounts = %#v, want execution-stage counts", summary.StageCounts)
	}
	if summary.StageCounts["COLLSCAN"] != 1 {
		t.Fatalf("StageCounts = %#v, want fallback stage preserved once", summary.StageCounts)
	}
}

func TestMongoExplainPlanSummary_skipsRejectedPlans(t *testing.T) {
	summary := MongoExplainPlanSummaryForResult(ExplainResult{
		Stages: []string{"IXSCAN"},
		Detail: map[string]any{
			"queryPlanner": map[string]any{
				"winningPlan": map[string]any{"stage": "IXSCAN"},
				"rejectedPlans": []any{
					map[string]any{"stage": "COLLSCAN"},
					map[string]any{"stage": "COLLSCAN"},
				},
			},
		},
	})

	if summary.StageCounts["COLLSCAN"] != 0 {
		t.Fatalf("StageCounts = %#v, rejected plans should be skipped", summary.StageCounts)
	}
	if summary.StageCounts["IXSCAN"] != 1 {
		t.Fatalf("StageCounts = %#v, winning plan should still count", summary.StageCounts)
	}
}
