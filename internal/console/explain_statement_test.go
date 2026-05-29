package console

import (
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestPrepareExplainStatement(t *testing.T) {
	t.Run("mysql strips explain wrapper", func(t *testing.T) {
		got := PrepareExplainStatement(" EXPLAIN SELECT * FROM users ", false, datasource.TypeMySQL)
		if got != "SELECT * FROM users" {
			t.Fatalf("expected mysql statement without explain prefix, got %q", got)
		}
	})

	t.Run("postgres strips explain format options", func(t *testing.T) {
		got := PrepareExplainStatement("EXPLAIN (FORMAT JSON) SELECT * FROM users", false, datasource.TypePostgreSQL)
		if got != "SELECT * FROM users" {
			t.Fatalf("expected postgres statement without explain options, got %q", got)
		}
	})

	t.Run("postgres strips explain analyze options and re-adds analyze marker", func(t *testing.T) {
		got := PrepareExplainStatement("EXPLAIN (ANALYZE, FORMAT JSON) SELECT * FROM users", true, datasource.TypePostgreSQL)
		if got != "ANALYZE SELECT * FROM users" {
			t.Fatalf("expected postgres analyze statement, got %q", got)
		}
	})

	t.Run("postgres analyze flag prefixes plain statement", func(t *testing.T) {
		got := PrepareExplainStatement("SELECT * FROM users", true, datasource.TypePostgreSQL)
		if got != "ANALYZE SELECT * FROM users" {
			t.Fatalf("expected postgres analyze statement, got %q", got)
		}
	})

	t.Run("postgres preserves explicit analyze prefix when explain wrapper is absent", func(t *testing.T) {
		got := PrepareExplainStatement("ANALYZE SELECT * FROM users", false, datasource.TypePostgreSQL)
		if got != "ANALYZE SELECT * FROM users" {
			t.Fatalf("expected explicit analyze prefix to be preserved, got %q", got)
		}
	})

	t.Run("postgres normalizes explain analyze wrapper to bare statement when analyze flag is false", func(t *testing.T) {
		got := PrepareExplainStatement("EXPLAIN ANALYZE SELECT * FROM users", false, datasource.TypePostgreSQL)
		if got != "SELECT * FROM users" {
			t.Fatalf("expected explain analyze wrapper to be stripped, got %q", got)
		}
	})
}
