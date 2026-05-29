package console

import (
	"reflect"
	"testing"
)

func TestSQLViewRiskFacts_AggregateView(t *testing.T) {
	definition := "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format"

	facts, err := SQLViewRiskFactsForDefinition("conversion_stats", definition, "sqlite")
	if err != nil {
		t.Fatalf("SQLViewRiskFactsForDefinition() error = %v", err)
	}
	if facts.ViewEntity != "conversion_stats" {
		t.Fatalf("ViewEntity = %q, want conversion_stats", facts.ViewEntity)
	}
	if !reflect.DeepEqual(facts.BaseEntities, []string{"conversions"}) {
		t.Fatalf("BaseEntities = %#v, want conversions", facts.BaseEntities)
	}
	if facts.Inner.Verb != "select" {
		t.Fatalf("Inner.Verb = %q, want select", facts.Inner.Verb)
	}
	if facts.Inner.TargetEntity != "conversions" {
		t.Fatalf("Inner.TargetEntity = %q, want conversions", facts.Inner.TargetEntity)
	}
	if facts.Inner.HasJoin {
		t.Fatal("expected HasJoin=false")
	}
}

func TestSQLViewRiskFacts_JoinViewIncludesAllBaseEntities(t *testing.T) {
	definition := `CREATE VIEW order_summary AS
SELECT u.id, o.id AS order_id, p.id AS payment_id
FROM users u
JOIN orders o ON o.user_id = u.id
JOIN payments p ON p.order_id = o.id`

	facts, err := SQLViewRiskFactsForDefinition("order_summary", definition, "sqlite")
	if err != nil {
		t.Fatalf("SQLViewRiskFactsForDefinition() error = %v", err)
	}
	want := []string{"users", "orders", "payments"}
	if !reflect.DeepEqual(facts.BaseEntities, want) {
		t.Fatalf("BaseEntities = %#v, want %#v", facts.BaseEntities, want)
	}
	if !facts.Inner.HasJoin {
		t.Fatal("expected HasJoin=true")
	}
	if facts.Inner.JoinCount != 2 {
		t.Fatalf("JoinCount = %d, want 2", facts.Inner.JoinCount)
	}
	if got := facts.EntityNameMap["o"]; got != "orders" {
		t.Fatalf("EntityNameMap[o] = %q, want orders", got)
	}
	if got := facts.EntityNameMap["users"]; got != "users" {
		t.Fatalf("EntityNameMap[users] = %q, want users", got)
	}
	if got := facts.EntityNameMap["p"]; got != "payments" {
		t.Fatalf("EntityNameMap[p] = %q, want payments", got)
	}
}

func TestSQLViewRiskFacts_CreateViewIfNotExists(t *testing.T) {
	definition := "CREATE VIEW IF NOT EXISTS conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format"

	facts, err := SQLViewRiskFactsForDefinition("conversion_stats", definition, "sqlite")
	if err != nil {
		t.Fatalf("SQLViewRiskFactsForDefinition() error = %v", err)
	}
	if !reflect.DeepEqual(facts.BaseEntities, []string{"conversions"}) {
		t.Fatalf("BaseEntities = %#v, want conversions", facts.BaseEntities)
	}
}

func TestSQLViewRiskFacts_CreateTempView(t *testing.T) {
	definition := "CREATE TEMP VIEW active_users AS SELECT * FROM users WHERE status = 'active'"

	facts, err := SQLViewRiskFactsForDefinition("active_users", definition, "sqlite")
	if err != nil {
		t.Fatalf("SQLViewRiskFactsForDefinition() error = %v", err)
	}
	if !reflect.DeepEqual(facts.BaseEntities, []string{"users"}) {
		t.Fatalf("BaseEntities = %#v, want users", facts.BaseEntities)
	}
}

func TestSQLViewRiskFacts_CreateTemporaryViewIfNotExists(t *testing.T) {
	definition := "CREATE TEMPORARY VIEW IF NOT EXISTS active_users AS SELECT * FROM users WHERE status = 'active'"

	facts, err := SQLViewRiskFactsForDefinition("active_users", definition, "sqlite")
	if err != nil {
		t.Fatalf("SQLViewRiskFactsForDefinition() error = %v", err)
	}
	if !reflect.DeepEqual(facts.BaseEntities, []string{"users"}) {
		t.Fatalf("BaseEntities = %#v, want users", facts.BaseEntities)
	}
}

func TestSQLViewRiskFacts_InvalidDefinition(t *testing.T) {
	_, err := SQLViewRiskFactsForDefinition("broken_view", "CREATE VIEW broken_view AS PRAGMA table_info('users')", "sqlite")
	if err == nil {
		t.Fatal("expected error for unsupported view definition")
	}
}
