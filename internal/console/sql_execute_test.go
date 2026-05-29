package console

import "testing"

func TestIsQueryStatement_LeadingCommentSelect(t *testing.T) {
	if !isQueryStatement("-- preflight\nSELECT * FROM users", "postgres") {
		t.Fatalf("expected commented SELECT to be treated as query")
	}
}

func TestIsQueryStatement_WithDeleteIsNotQuery(t *testing.T) {
	stmt := "WITH doomed AS (SELECT id FROM users) DELETE FROM users WHERE id IN (SELECT id FROM doomed)"
	if isQueryStatement(stmt, "postgres") {
		t.Fatalf("expected WITH ... DELETE to be treated as non-query")
	}
}

func TestLeadingSQLKeyword_WithSelectReturnsSelect(t *testing.T) {
	stmt := "WITH seeded AS (SELECT 1) SELECT * FROM seeded"
	if got := leadingSQLKeyword(stmt); got != "select" {
		t.Fatalf("expected WITH ... SELECT fallback keyword to be select, got %q", got)
	}
}

func TestSQLStatementHasWhereClause_FallsBackWhenMySQLParseFails(t *testing.T) {
	stmt := "UPDATE users FORCE INDEX (PRIMARY) SET active = 0 WHERE id > 10"
	if !SQLStatementHasWhereClause(stmt, "mysql") {
		t.Fatalf("expected MySQL UPDATE fallback to detect top-level WHERE")
	}
}
