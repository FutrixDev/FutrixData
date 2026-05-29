package riskengine

func builtinElasticsearchRules() []Rule {
	types := []string{"elasticsearch"}
	return []Rule{
		// === HIGH RISK: Destructive operations ===
		{
			ID:          "es-block-delete",
			Code:        "ELS-001",
			Description: "Block DELETE requests — index/document deletion",
			Scope:       RuleScope{DsTypes: types},
			Priority:    100,
			Action:      ActionBlock,
			Reason:      "DELETE",
			When:        RuleCondition{HTTPMethod: []string{"DELETE"}},
		},
		{
			ID:          "es-block-delete-by-query",
			Code:        "ELS-002",
			Description: "Block _delete_by_query requests — destructive bulk deletion",
			Scope:       RuleScope{DsTypes: types},
			Priority:    110,
			Action:      ActionBlock,
			Reason:      "DELETE BY QUERY",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/_delete_by_query(?:\?.*)?$`,
			},
		},
		{
			ID:          "es-require-approval-bulk-write",
			Code:        "ELS-003",
			Description: "Require approval for bulk, reindex, and update-by-query operations",
			Scope:       RuleScope{DsTypes: types},
			Priority:    80,
			Action:      ActionRequireApproval,
			Reason:      "bulk write request",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/(_bulk|_reindex|_update_by_query)(?:\?.*)?$`,
			},
		},
		{
			ID:          "es-require-approval-index-admin",
			Code:        "ELS-004",
			Description: "Require approval for index lifecycle and topology changes",
			Scope:       RuleScope{DsTypes: types},
			Priority:    80,
			Action:      ActionRequireApproval,
			Reason:      "index admin request",
			When: RuleCondition{
				HTTPMethod: []string{"POST", "PUT"},
				PathPattern: `(?i)/(_forcemerge|_close|_open|_shrink|_split|_clone|_rollover|_restore|_snapshot)(?:/|$|\?)`,
			},
		},
		{
			ID:          "es-require-approval-config-write",
			Code:        "ELS-005",
			Description: "Require approval for index and cluster configuration writes",
			Scope:       RuleScope{DsTypes: types},
			Priority:    75,
			Action:      ActionRequireApproval,
			Reason:      "configuration write request",
			When: RuleCondition{
				Any: []RuleCondition{
					{HTTPMethod: []string{"PUT", "POST", "PATCH"}, PathPattern: `(?i)/(_settings|_mapping|_aliases)(?:/|$|\?)`},
					{HTTPMethod: []string{"PUT", "POST", "PATCH"}, PathPattern: `(?i)/(_component_template|_index_template|_template|_ingest/pipeline|_ilm|_slm)(?:/|$|\?)`},
				},
			},
		},

		// === MEDIUM RISK: Write operations ===
		{
			ID:          "es-warn-put-patch",
			Code:        "ELS-006",
			Description: "Warn on PUT/PATCH — write request",
			Scope:       RuleScope{DsTypes: types},
			Priority:    50,
			Action:      ActionWarn,
			Reason:      "WRITE REQUEST",
			When:        RuleCondition{HTTPMethod: []string{"PUT", "PATCH"}},
		},
		{
			ID:          "es-warn-post-write",
			Code:        "ELS-007",
			Description: "Warn on POST (non-search) — write request",
			Scope:       RuleScope{DsTypes: types},
			Priority:    50,
			Action:      ActionWarn,
			Reason:      "WRITE REQUEST",
			When: RuleCondition{
				HTTPMethod: []string{"POST"},
				Not: &RuleCondition{
					PathPattern: `(?i)/(_search|_count)(?:\?.*)?$`,
				},
			},
		},
		{
			ID:          "es-warn-refresh-flush",
			Code:        "ELS-008",
			Description: "Warn on refresh and flush operations",
			Scope:       RuleScope{DsTypes: types},
			Priority:    55,
			Action:      ActionWarn,
			Reason:      "index maintenance request",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/(_refresh|_flush)(?:/|$|\?)`,
			},
		},

		// === LOW RISK: Read operations ===
		{
			ID:          "es-allow-search",
			Code:        "ELS-009",
			Description: "Allow POST _search — read-only search",
			Scope:       RuleScope{DsTypes: types},
			Priority:    10,
			Action:      ActionAllow,
			Reason:      "read-only search",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/_search(?:\?.*)?$`,
			},
		},
		{
			ID:          "es-allow-count",
			Code:        "ELS-010",
			Description: "Allow POST _count — read-only count",
			Scope:       RuleScope{DsTypes: types},
			Priority:    10,
			Action:      ActionAllow,
			Reason:      "read-only count",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/_count(?:\?.*)?$`,
			},
		},
		{
			ID:          "es-allow-get-head",
			Code:        "ELS-011",
			Description: "Allow GET/HEAD — read-only",
			Scope:       RuleScope{DsTypes: types},
			Priority:    10,
			Action:      ActionAllow,
			Reason:      "read-only operation",
			When:        RuleCondition{HTTPMethod: []string{"GET", "HEAD"}},
		},
	}
}
