package console

import (
	"testing"

	"futrixdata/platform/internal/console/window"
)

func TestApplySQLLimitPolicy_NoLimit(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "SELECT * FROM users"
	got, decision := applySQLLimitPolicy(stmt, policy)
	want := "SELECT * FROM users LIMIT 2001"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if decision.Effective != 2000 || decision.Fetch != 2001 || !decision.Enforced {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestApplySQLLimitPolicy_LimitUnderMax(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "SELECT * FROM users LIMIT 100"
	got, decision := applySQLLimitPolicy(stmt, policy)
	if got != stmt {
		t.Fatalf("expected query unchanged, got %q", got)
	}
	if decision.Effective != 100 || decision.Fetch != 100 || decision.Enforced {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestApplySQLLimitPolicy_LimitOverMax(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "SELECT * FROM users LIMIT 5000"
	got, decision := applySQLLimitPolicy(stmt, policy)
	if got != stmt {
		t.Fatalf("expected query unchanged, got %q", got)
	}
	if decision.Effective != 5000 || decision.Fetch != 5000 || decision.Enforced {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestFindTopLevelLimit_IgnoresSubquery(t *testing.T) {
	stmt := "SELECT * FROM (SELECT * FROM t LIMIT 5) sub"
	info := findTopLevelLimit(stmt)
	if info.found {
		t.Fatalf("expected no top-level limit, got %+v", info)
	}
}

func TestFindTopLevelLimit_OverflowIsUnparsed(t *testing.T) {
	stmt := "SELECT * FROM users LIMIT 18446744073709551615"
	info := findTopLevelLimit(stmt)
	if !info.found {
		t.Fatal("expected top-level limit to be found")
	}
	if info.parsed {
		t.Fatalf("overflowed LIMIT must not parse as %d", info.count)
	}
}

func TestRewriteSQLLimit_OffsetForms(t *testing.T) {
	stmt := "SELECT * FROM t LIMIT 5 OFFSET 10"
	info := findTopLevelLimit(stmt)
	if !info.found || !info.parsed {
		t.Fatalf("expected limit found")
	}
	got := rewriteSQLLimit(stmt, info, 2001)
	want := "SELECT * FROM t LIMIT 2001 OFFSET 10"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	stmt = "SELECT * FROM t LIMIT 10, 20"
	info = findTopLevelLimit(stmt)
	got = rewriteSQLLimit(stmt, info, 2001)
	want = "SELECT * FROM t LIMIT 10, 2001"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAppendSQLLimit_PreservesSemicolon(t *testing.T) {
	stmt := "SELECT * FROM t;"
	got := appendSQLLimit(stmt, 2001)
	want := "SELECT * FROM t LIMIT 2001;"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPrepareSQLQuery_NoRewriteForShow(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "SHOW TABLES"
	got, decision := prepareSQLQuery(stmt, policy)
	if got != stmt {
		t.Fatalf("expected query unchanged, got %q", got)
	}
	if !decision.Enforced {
		t.Fatalf("expected enforced decision for show")
	}
}

func TestPrepareSQLQuery_WithStatement(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "WITH t AS (SELECT 1) SELECT * FROM t"
	got, decision := prepareSQLQuery(stmt, policy)
	want := "WITH t AS (SELECT 1) SELECT * FROM t LIMIT 2001"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if !decision.Enforced {
		t.Fatalf("expected enforced decision for with statement")
	}
}

func TestPrepareSQLQuery_LeadingCommentSelect(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "-- warm-up comment\nSELECT * FROM users"
	got, decision := prepareSQLQuery(stmt, policy)
	want := "-- warm-up comment\nSELECT * FROM users LIMIT 2001"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if !decision.Enforced {
		t.Fatalf("expected enforced decision for commented select")
	}
}

func TestPrepareSQLQuery_WithDeleteIsNotReadQuery(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	stmt := "WITH doomed AS (SELECT id FROM users) DELETE FROM users WHERE id IN (SELECT id FROM doomed)"
	got, _ := prepareSQLQuery(stmt, policy)
	if got != stmt {
		t.Fatalf("expected statement unchanged, got %q", got)
	}
}
