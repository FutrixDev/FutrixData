package riskengine

func builtinDynamoDBRules() []Rule {
	types := []string{"dynamodb"}
	return []Rule{
		// === LOW RISK: Read ===
		{
			ID:          "dynamodb-allow-select",
			Code:        "DYN-001",
			Description: "Allow SELECT — read-only query",
			Scope:       RuleScope{DsTypes: types},
			Priority:    10,
			Action:      ActionAllow,
			Reason:      "read-only operation",
			When:        RuleCondition{Command: []string{"select"}},
		},
		{
			ID:              "dynamodb-allow-insert",
			Code:            "DYN-002",
			Description:     "Allow ordinary INSERT statements when explicitly enabled",
			Scope:           RuleScope{DsTypes: types},
			Priority:        60,
			Action:          ActionAllow,
			Reason:          "ordinary INSERT allowed",
			When:            RuleCondition{Command: []string{"insert"}},
			defaultDisabled: true,
		},

		// === HIGH RISK: Destructive without WHERE ===
		{
			ID:          "dynamodb-block-delete-no-where",
			Code:        "DYN-003",
			Description: "Block DELETE without WHERE — full table delete",
			Scope:       RuleScope{DsTypes: types},
			Priority:    90,
			Action:      ActionBlock,
			Reason:      "DELETE without WHERE",
			When:        RuleCondition{Command: []string{"delete"}, HasWhere: boolPtr(false)},
		},
		{
			ID:          "dynamodb-block-update-no-where",
			Code:        "DYN-004",
			Description: "Block UPDATE without WHERE — full table update",
			Scope:       RuleScope{DsTypes: types},
			Priority:    90,
			Action:      ActionBlock,
			Reason:      "UPDATE without WHERE",
			When:        RuleCondition{Command: []string{"update"}, HasWhere: boolPtr(false)},
		},

		// === MEDIUM RISK: Write with conditions ===
		{
			ID:          "dynamodb-warn-delete",
			Code:        "DYN-005",
			Description: "Warn on DELETE with WHERE",
			Scope:       RuleScope{DsTypes: types},
			Priority:    50,
			Action:      ActionWarn,
			Reason:      "DELETE",
			When:        RuleCondition{Command: []string{"delete"}, HasWhere: boolPtr(true)},
		},
		{
			ID:          "dynamodb-warn-update",
			Code:        "DYN-006",
			Description: "Warn on UPDATE with WHERE",
			Scope:       RuleScope{DsTypes: types},
			Priority:    50,
			Action:      ActionWarn,
			Reason:      "UPDATE",
			When:        RuleCondition{Command: []string{"update"}, HasWhere: boolPtr(true)},
		},
		{
			ID:          "dynamodb-warn-insert",
			Code:        "DYN-007",
			Description: "Warn on INSERT — write operation",
			Scope:       RuleScope{DsTypes: types},
			Priority:    40,
			Action:      ActionWarn,
			Reason:      "INSERT",
			When:        RuleCondition{Command: []string{"insert"}},
		},
	}
}
