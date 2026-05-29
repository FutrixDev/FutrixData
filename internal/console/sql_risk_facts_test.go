package console

import (
	"reflect"
	"testing"
)

func TestSQLRiskFacts_CTEWriteTargetsActualEntity(t *testing.T) {
	tests := []struct {
		name               string
		dialect            string
		statement          string
		wantVerb           string
		wantTarget         string
		wantJoinCount      int
		wantHasJoin        bool
		wantHasWhere       bool
		wantEqualityFields []string
	}{
		{
			name:               "with update",
			dialect:            "postgres",
			statement:          "WITH src AS (SELECT id FROM staging) UPDATE target SET active = true WHERE id IN (SELECT id FROM src)",
			wantVerb:           "update",
			wantTarget:         "target",
			wantHasWhere:       true,
			wantEqualityFields: nil,
		},
		{
			name:               "with insert",
			dialect:            "postgres",
			statement:          "WITH src AS (SELECT * FROM raw) INSERT INTO target SELECT * FROM src",
			wantVerb:           "insert",
			wantTarget:         "target",
			wantHasWhere:       false,
			wantEqualityFields: nil,
		},
		{
			name:               "cte update with outer select",
			dialect:            "postgres",
			statement:          "WITH changed AS (UPDATE users SET active = true WHERE id = 7 RETURNING id) SELECT * FROM tmp_logs",
			wantVerb:           "update",
			wantTarget:         "users",
			wantHasWhere:       true,
			wantEqualityFields: nil,
		},
		{
			name:               "schema update",
			dialect:            "postgres",
			statement:          "UPDATE schema_a.orders SET status = 'done' WHERE id = 42",
			wantVerb:           "update",
			wantTarget:         "schema_a.orders",
			wantHasWhere:       true,
			wantEqualityFields: []string{"id"},
		},
		{
			name:               "multi join select",
			dialect:            "postgres",
			statement:          "SELECT u.id, o.id, p.id FROM users u JOIN orders o ON o.user_id = u.id JOIN payments p ON p.order_id = o.id WHERE u.id = 7",
			wantVerb:           "select",
			wantTarget:         "users",
			wantJoinCount:      2,
			wantHasJoin:        true,
			wantHasWhere:       true,
			wantEqualityFields: []string{"id"},
		},
		{
			name:               "multiple cte writes prefer matching verb target",
			dialect:            "postgres",
			statement:          "WITH audit AS (INSERT INTO audit_logs(id) VALUES (1) RETURNING id), cleanup AS (DELETE FROM users WHERE id = 1 RETURNING id) SELECT * FROM reports",
			wantVerb:           "delete",
			wantTarget:         "users",
			wantHasWhere:       true,
			wantEqualityFields: nil,
		},
		{
			name:               "top level delete keeps delete verb and where",
			dialect:            "postgres",
			statement:          "WITH cleanup AS (DELETE FROM archive WHERE id = 1 RETURNING id) DELETE FROM users",
			wantVerb:           "delete",
			wantTarget:         "users",
			wantHasWhere:       false,
			wantEqualityFields: nil,
		},
		{
			name:               "top level update wins over delete cte",
			dialect:            "postgres",
			statement:          "WITH cleanup AS (DELETE FROM archive WHERE id = 1 RETURNING id) UPDATE users SET active = false",
			wantVerb:           "update",
			wantTarget:         "users",
			wantHasWhere:       false,
			wantEqualityFields: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := SQLRiskFactsForStatement(tt.statement, tt.dialect)
			if err != nil {
				t.Fatalf("SQLRiskFactsForStatement() error = %v", err)
			}
			if facts.Verb != tt.wantVerb {
				t.Fatalf("Verb = %q, want %q", facts.Verb, tt.wantVerb)
			}
			if facts.TargetEntity != tt.wantTarget {
				t.Fatalf("TargetEntity = %q, want %q", facts.TargetEntity, tt.wantTarget)
			}
			if tt.wantTarget != "" && len(facts.TargetEntities) == 0 {
				t.Fatalf("TargetEntities = %#v, want non-empty", facts.TargetEntities)
			}
			if facts.JoinCount != tt.wantJoinCount {
				t.Fatalf("JoinCount = %d, want %d", facts.JoinCount, tt.wantJoinCount)
			}
			if facts.HasJoin != tt.wantHasJoin {
				t.Fatalf("HasJoin = %v, want %v", facts.HasJoin, tt.wantHasJoin)
			}
			if facts.HasWhere != tt.wantHasWhere {
				t.Fatalf("HasWhere = %v, want %v", facts.HasWhere, tt.wantHasWhere)
			}
			if !reflect.DeepEqual(facts.EqualityFields, tt.wantEqualityFields) {
				t.Fatalf("EqualityFields = %#v, want %#v", facts.EqualityFields, tt.wantEqualityFields)
			}
		})
	}
}

func TestSQLRiskFacts_FallbackSkipsOptionalDDLKeywords(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		wantVerb  string
		wantTable string
	}{
		{
			name:      "drop table if exists",
			statement: "DROP TABLE IF EXISTS users",
			wantVerb:  "drop",
			wantTable: "users",
		},
		{
			name:      "create table if not exists",
			statement: "CREATE TABLE IF NOT EXISTS audit_logs (id bigint)",
			wantVerb:  "create",
			wantTable: "audit_logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := SQLRiskFactsForStatement(tt.statement, "mysql")
			if err != nil {
				t.Fatalf("SQLRiskFactsForStatement() error = %v", err)
			}
			if facts.Verb != tt.wantVerb {
				t.Fatalf("Verb = %q, want %q", facts.Verb, tt.wantVerb)
			}
			if facts.TargetEntity != tt.wantTable {
				t.Fatalf("TargetEntity = %q, want %q", facts.TargetEntity, tt.wantTable)
			}
		})
	}
}

func TestSQLRiskFacts_CTEWriteTargetsIncludeAllEntities(t *testing.T) {
	facts, err := SQLRiskFactsForStatement(
		"WITH audit AS (INSERT INTO audit_logs(id) VALUES (1) RETURNING id), cleanup AS (DELETE FROM users WHERE id = 1 RETURNING id) SELECT * FROM reports",
		"postgres",
	)
	if err != nil {
		t.Fatalf("SQLRiskFactsForStatement() error = %v", err)
	}
	want := []string{"audit_logs", "users"}
	if !reflect.DeepEqual(facts.TargetEntities, want) {
		t.Fatalf("TargetEntities = %#v, want %#v", facts.TargetEntities, want)
	}
}
