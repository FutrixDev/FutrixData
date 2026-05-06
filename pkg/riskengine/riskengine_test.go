package riskengine

import "testing"

func TestSQLBuiltinRules(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		stmt string
		want Action
	}{
		{"select * from users where id = 1", ActionAllow},
		{"delete from users", ActionBlock},
		{"update users set name = 'x' where id = 1", ActionWarn},
		{"drop table users", ActionBlock},
	}
	for _, tc := range cases {
		got := engine.Assess("postgresql", "prod", tc.stmt)
		if got.Action != tc.want {
			t.Fatalf("%q action = %s, want %s (%+v)", tc.stmt, got.Action, tc.want, got)
		}
	}
}

func TestUserRuleOverridesBuiltinByScopeAndPriority(t *testing.T) {
	engine := NewEngine()
	engine.LoadUserRules([]Rule{{
		ID:          "prod-users-read-approval",
		Description: "Require approval for users reads in prod",
		Scope:       RuleScope{DatasourceID: "prod", Entity: "users"},
		Enabled:     true,
		Priority:    500,
		Action:      ActionRequireApproval,
		Reason:      "sensitive table",
		When:        RuleCondition{Command: []string{"select"}},
	}})
	got := engine.Assess("postgresql", "prod", "select * from users")
	if got.Action != ActionRequireApproval || got.RuleID != "prod-users-read-approval" {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}
