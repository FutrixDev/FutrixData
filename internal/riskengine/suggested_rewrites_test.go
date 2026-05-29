package riskengine

import "testing"

func TestSuggestedRewritesForAssessment_ProbeRules(t *testing.T) {
	cases := []struct {
		name       string
		assessment RiskAssessment
		wantIDs    []string
	}{
		{
			name:       "PRB-003 no index",
			assessment: RiskAssessment{RuleCode: "PRB-003", Reasons: []string{"no index detected"}},
			wantIDs:    []string{"inspect_indexes_first", "add_indexed_filter_and_smaller_limit", "request_full_scan_approval"},
		},
		{
			name:       "PRB-004 wide scan",
			assessment: RiskAssessment{RuleCode: "PRB-004", Reasons: []string{"examined over 1000 rows"}},
			wantIDs:    []string{"increase_selectivity", "lower_limit", "use_keyset_pagination"},
		},
		{
			name:       "PRB-005 filesort",
			assessment: RiskAssessment{RuleCode: "PRB-005", Reasons: []string{"uses filesort"}},
			wantIDs:    []string{"order_by_pk_or_index", "sample_without_order_by", "request_index_for_required_order"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SuggestedRewritesForAssessment(tc.assessment)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("rewrite count = %d; want %d: %#v", len(got), len(tc.wantIDs), got)
			}
			for i, want := range tc.wantIDs {
				if got[i].ID != want {
					t.Fatalf("rewrite[%d].ID = %q; want %q", i, got[i].ID, want)
				}
				if got[i].Title == "" || got[i].Description == "" {
					t.Fatalf("rewrite[%d] should be machine-readable and human-readable: %#v", i, got[i])
				}
			}
		})
	}
}
