package riskengine

import "strings"

type SuggestedRewrite struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	RewriteHint      string   `json:"rewriteHint,omitempty"`
	SuggestedTools   []string `json:"suggestedTools,omitempty"`
	RequiresApproval bool     `json:"requiresApproval,omitempty"`
}

func SuggestedRewritesForAssessment(a RiskAssessment) []SuggestedRewrite {
	switch strings.ToUpper(strings.TrimSpace(a.RuleCode)) {
	case "PRB-003":
		return []SuggestedRewrite{
			{
				ID:             "inspect_indexes_first",
				Title:          "Inspect indexes before retrying",
				Description:    "Use live schema metadata to identify indexed columns for the target entity.",
				RewriteHint:    "Call describe_entity for the target table or collection, then retry with an indexed WHERE predicate.",
				SuggestedTools: []string{"describe_entity"},
			},
			{
				ID:          "add_indexed_filter_and_smaller_limit",
				Title:       "Add an indexed filter and smaller LIMIT",
				Description: "Avoid a broad scan by constraining the query with an indexed equality/range predicate and a smaller sample.",
				RewriteHint: "Rewrite as SELECT ... WHERE <indexed_column> = ? LIMIT 20, or ask the user for a selective key value.",
			},
			{
				ID:               "request_full_scan_approval",
				Title:            "Ask before scanning broadly",
				Description:      "Only proceed with a full scan when the user explicitly accepts the cost/risk.",
				RewriteHint:      "If a full scan is truly required, explain why and let the approval gate control execution.",
				RequiresApproval: true,
			},
		}
	case "PRB-004":
		return []SuggestedRewrite{
			{
				ID:          "increase_selectivity",
				Title:       "Increase WHERE selectivity",
				Description: "Reduce examined rows by adding a narrower predicate, preferably on an indexed/key column.",
				RewriteHint: "Add a selective WHERE clause before retrying; avoid repeating the same broad statement.",
			},
			{
				ID:          "lower_limit",
				Title:       "Lower the LIMIT",
				Description: "Return a smaller first page while you verify the shape of the data.",
				RewriteHint: "Retry with LIMIT 20 or a smaller pageSize.",
			},
			{
				ID:          "use_keyset_pagination",
				Title:       "Use keyset pagination",
				Description: "Page from an indexed cursor instead of offsetting or scanning a wide range.",
				RewriteHint: "Use WHERE <pk_or_index> > last_seen_value ORDER BY <pk_or_index> LIMIT 20.",
			},
		}
	case "PRB-005":
		if assessmentMentions(a, "filesort") {
			return []SuggestedRewrite{
				{
					ID:             "order_by_pk_or_index",
					Title:          "Order by a primary key or indexed column",
					Description:    "Avoid filesort by using the primary key or an existing index as the ORDER BY column.",
					RewriteHint:    "Rewrite ORDER BY to use the primary key or an indexed column verified by describe_entity.",
					SuggestedTools: []string{"describe_entity"},
				},
				{
					ID:          "sample_without_order_by",
					Title:       "Drop nonessential ORDER BY for sampling",
					Description: "If ordering is not required for the question, remove ORDER BY and take a small LIMIT sample first.",
					RewriteHint: "Retry as SELECT ... FROM ... WHERE ... LIMIT 20 without the nonessential ORDER BY.",
				},
				{
					ID:               "request_index_for_required_order",
					Title:            "Request an index for required ordering",
					Description:      "If business logic requires that sort column, ask the user to add an index instead of auto-creating one.",
					RewriteHint:      "Explain that the query needs an index on the sort column; do not create the index automatically.",
					RequiresApproval: true,
				},
			}
		}
		return []SuggestedRewrite{
			{
				ID:             "inspect_access_path",
				Title:          "Inspect the access path",
				Description:    "Use schema/index metadata to find a simpler access path before retrying.",
				RewriteHint:    "Call describe_entity, then choose indexed filters or keyset pagination.",
				SuggestedTools: []string{"describe_entity"},
			},
			{
				ID:          "simplify_plan",
				Title:       "Simplify the plan",
				Description: "Reduce joins, temporary grouping/sorting, or unindexed predicates before execution.",
				RewriteHint: "Break the query into a smaller indexed lookup or add a selective WHERE and lower LIMIT.",
			},
		}
	default:
		return nil
	}
}

func assessmentMentions(a RiskAssessment, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, reason := range a.Reasons {
		if strings.Contains(strings.ToLower(reason), needle) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(a.RuleDescription), needle)
}
