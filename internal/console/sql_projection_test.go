package console

import (
	"context"
	"fmt"
	"testing"

	"futrixdata/platform/internal/datasource"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
)

func TestBuildSQLResultColumns_JoinProjectionTracksOrigins(t *testing.T) {
	ds := datasource.DataSource{ID: "ds_sql", Type: datasource.TypeMySQL}
	describe := func(_ context.Context, _ datasource.DataSource, name string) (DescribeResult, error) {
		switch name {
		case "users":
			return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "email"}}}, nil
		case "orders":
			return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "user_id"}, {Name: "total"}}}, nil
		default:
			return DescribeResult{}, fmt.Errorf("unexpected table %q", name)
		}
	}

	columns, err := buildSQLResultColumns(
		context.Background(),
		ds,
		"mysql",
		"SELECT u.email AS user_email, o.total, LOWER(u.email) AS email_norm FROM users u JOIN orders o ON u.id = o.user_id",
		[]string{"user_email", "total", "email_norm"},
		describe,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 3 {
		t.Fatalf("expected 3 result columns, got %d", len(columns))
	}
	if columns[0].Name != "user_email" || columns[0].Origins[0].Table != "users" || columns[0].Origins[0].Column != "email" {
		t.Fatalf("unexpected first column: %+v", columns[0])
	}
	if columns[1].Name != "total" || columns[1].Origins[0].Table != "orders" || columns[1].Origins[0].Column != "total" {
		t.Fatalf("unexpected second column: %+v", columns[1])
	}
	if columns[2].Name != "email_norm" {
		t.Fatalf("unexpected third column name: %+v", columns[2])
	}
	if columns[2].ConservativeMask {
		t.Fatalf("expected resolved expression to avoid conservative masking: %+v", columns[2])
	}
	if len(columns[2].Origins) != 1 || columns[2].Origins[0].Table != "users" || columns[2].Origins[0].Column != "email" {
		t.Fatalf("unexpected third column origin: %+v", columns[2])
	}
}

func TestBuildSQLResultColumns_SelectStarPreservesJoinOrder(t *testing.T) {
	ds := datasource.DataSource{ID: "ds_sql", Type: datasource.TypePostgreSQL}
	describe := func(_ context.Context, _ datasource.DataSource, name string) (DescribeResult, error) {
		switch name {
		case "public.users":
			return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "email"}}}, nil
		case "public.orders":
			return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "total"}}}, nil
		default:
			return DescribeResult{}, fmt.Errorf("unexpected table %q", name)
		}
	}

	columns, err := buildSQLResultColumns(
		context.Background(),
		ds,
		"postgres",
		"SELECT * FROM public.users u JOIN public.orders o ON u.id = o.user_id",
		[]string{"id", "email", "id", "total"},
		describe,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 4 {
		t.Fatalf("expected 4 result columns, got %d", len(columns))
	}
	if columns[0].Origins[0].Table != "users" || columns[0].Origins[0].Column != "id" {
		t.Fatalf("unexpected first origin: %+v", columns[0])
	}
	if columns[1].Origins[0].Table != "users" || columns[1].Origins[0].Column != "email" {
		t.Fatalf("unexpected second origin: %+v", columns[1])
	}
	if columns[2].Origins[0].Table != "orders" || columns[2].Origins[0].Column != "id" {
		t.Fatalf("unexpected third origin: %+v", columns[2])
	}
	if columns[3].Origins[0].Table != "orders" || columns[3].Origins[0].Column != "total" {
		t.Fatalf("unexpected fourth origin: %+v", columns[3])
	}
}

func TestBuildSQLResultColumns_MySQLQualifiedStarUsesBareDescribeName(t *testing.T) {
	ds := datasource.DataSource{ID: "ds_sql", Type: datasource.TypeMySQL, Database: "app"}
	describeCalls := make([]string, 0, 1)
	describe := func(_ context.Context, _ datasource.DataSource, name string) (DescribeResult, error) {
		describeCalls = append(describeCalls, name)
		if name != "users" {
			return DescribeResult{}, fmt.Errorf("expected bare mysql table name, got %q", name)
		}
		return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "email"}}}, nil
	}

	columns, err := buildSQLResultColumns(
		context.Background(),
		ds,
		"mysql",
		"SELECT * FROM app.users",
		[]string{"id", "email"},
		describe,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(describeCalls) != 1 || describeCalls[0] != "users" {
		t.Fatalf("expected one bare mysql describe call, got %v", describeCalls)
	}
	if len(columns) != 2 {
		t.Fatalf("expected 2 result columns, got %d", len(columns))
	}
	if columns[0].ConservativeMask || columns[1].ConservativeMask {
		t.Fatalf("expected mysql qualified star to avoid conservative fallback: %+v", columns)
	}
	if columns[0].Origins[0].Schema != "app" || columns[0].Origins[0].Table != "users" {
		t.Fatalf("unexpected first origin: %+v", columns[0])
	}
}

func TestBuildSQLResultColumns_MySQLCrossDatabaseStarFallsBackConservatively(t *testing.T) {
	ds := datasource.DataSource{ID: "ds_sql", Type: datasource.TypeMySQL, Database: "app"}
	describeCalls := make([]string, 0, 1)
	describe := func(_ context.Context, _ datasource.DataSource, name string) (DescribeResult, error) {
		describeCalls = append(describeCalls, name)
		return DescribeResult{Columns: []ColumnInfo{{Name: "id"}, {Name: "email"}}}, nil
	}

	columns, err := buildSQLResultColumns(
		context.Background(),
		ds,
		"mysql",
		"SELECT * FROM other_db.users",
		[]string{"id", "email"},
		describe,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(describeCalls) != 0 {
		t.Fatalf("expected cross-database mysql star to skip describe, got %v", describeCalls)
	}
	if len(columns) != 2 {
		t.Fatalf("expected 2 fallback columns, got %d", len(columns))
	}
	for _, column := range columns {
		if !column.ConservativeMask || column.SourceKind != "unknown" {
			t.Fatalf("expected conservative fallback for cross-database mysql star, got %+v", column)
		}
	}
}

func TestBuildSQLResultColumns_FallsBackWhenParserPanics(t *testing.T) {
	originalParse := parseSQLStatements
	parseSQLStatements = func(string) (parser.Statements, error) {
		panic("boom")
	}
	t.Cleanup(func() {
		parseSQLStatements = originalParse
	})

	columns, err := buildSQLResultColumns(
		context.Background(),
		datasource.DataSource{ID: "ds_sql", Type: datasource.TypeMySQL},
		"mysql",
		"SELECT id FROM users",
		[]string{"id"},
		func(_ context.Context, _ datasource.DataSource, _ string) (DescribeResult, error) {
			return DescribeResult{Columns: []ColumnInfo{{Name: "id"}}}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 1 {
		t.Fatalf("expected fallback result column, got %d", len(columns))
	}
	if !columns[0].ConservativeMask || columns[0].SourceKind != "unknown" {
		t.Fatalf("expected conservative fallback after panic, got %+v", columns[0])
	}
}

func TestBuildSQLResultColumns_UnionFallsBackConservatively(t *testing.T) {
	columns, err := buildSQLResultColumns(
		context.Background(),
		datasource.DataSource{ID: "ds_sql", Type: datasource.TypePostgreSQL},
		"postgres",
		"SELECT email FROM users UNION ALL SELECT email FROM archived_users",
		[]string{"email"},
		func(_ context.Context, _ datasource.DataSource, _ string) (DescribeResult, error) {
			return DescribeResult{Columns: []ColumnInfo{{Name: "email"}}}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 1 {
		t.Fatalf("expected fallback result column, got %d", len(columns))
	}
	if !columns[0].ConservativeMask || columns[0].SourceKind != "unknown" {
		t.Fatalf("expected conservative fallback for union query, got %+v", columns[0])
	}
}

func TestBuildSQLResultColumns_WithoutDescribeFallsBackConservatively(t *testing.T) {
	columns, err := buildSQLResultColumns(
		context.Background(),
		datasource.DataSource{ID: "ds_sql", Type: datasource.TypePostgreSQL},
		"postgres",
		"SELECT email FROM users",
		[]string{"email"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(columns) != 1 {
		t.Fatalf("expected one fallback result column, got %d", len(columns))
	}
	if !columns[0].ConservativeMask || columns[0].SourceKind != "unknown" {
		t.Fatalf("expected conservative fallback without describe, got %+v", columns[0])
	}
}
