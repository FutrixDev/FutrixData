package aichat

import "testing"

func TestMakeStoredAnalysisResult_PrefersMaskedAgentView(t *testing.T) {
	effect := ConsoleResultEffect{
		DatasourceID:   "ds_test",
		DatasourceType: "mysql",
		Database:       "appdb",
		Statement:      "SELECT email FROM users",
		Result: QueryResult{
			Columns:  []string{"email"},
			Rows:     []map[string]any{{"email": "user@example.com"}},
			RowCount: 1,
			AgentView: &QueryResult{
				Columns:  []string{"email"},
				Rows:     []map[string]any{{"email": "masked:abc123"}},
				RowCount: 1,
			},
		},
	}

	stored := makeStoredAnalysisResult(effect)
	if len(stored.Rows) != 1 {
		t.Fatalf("expected one stored row, got %d", len(stored.Rows))
	}
	if got, _ := stored.Rows[0]["email"].(string); got != "masked:abc123" {
		t.Fatalf("expected analysis store to keep masked value, got %q", got)
	}
}
