package console

import (
	"context"
	"strings"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestSQLPagingQueryNext(t *testing.T) {
	keys := []sqlSortKey{{Column: "id", Desc: false}}
	query := buildSQLPagingQuery("select * from users", keys, []any{int64(10)}, DirectionNext, 51)
	if query == "" {
		t.Fatalf("expected paging query")
	}
}

func TestSQLPagingLimitCap(t *testing.T) {
	fetch := sqlFetchLimit(50, 100, 80)
	if fetch != 20 {
		t.Fatalf("expected fetch limit 20, got %d", fetch)
	}
	fetch = sqlFetchLimit(50, 100, 0)
	if fetch != 51 {
		t.Fatalf("expected fetch limit 51, got %d", fetch)
	}
}

func TestSQLPagingLimitZero(t *testing.T) {
	fetch := sqlFetchLimit(50, 0, 0)
	if fetch != 0 {
		t.Fatalf("expected fetch limit 0, got %d", fetch)
	}
}

func TestPagingOffsets(t *testing.T) {
	if pagingNextOffset(0, 50) != 50 {
		t.Fatalf("expected next offset 50")
	}
	if pagingPrevOffset(50, 50) != 0 {
		t.Fatalf("expected prev offset 0")
	}
}

func TestBuildSQLPagingQueryDetailed_CTEInnerWhereUsesOuterWhereDetection(t *testing.T) {
	statement := `WITH cte AS (SELECT id FROM users WHERE active = true) SELECT id FROM cte`
	query, err := buildSQLPagingQueryDetailed(statement, "postgres", []sqlSortKey{{Column: "id"}}, []any{int64(10)}, DirectionNext, 51)
	if err != nil {
		t.Fatalf("buildSQLPagingQueryDetailed: %v", err)
	}
	if strings.Contains(query, " FROM cte AND ") {
		t.Fatalf("expected outer query without WHERE to append WHERE, got %q", query)
	}
	if !strings.Contains(query, " FROM cte WHERE ") {
		t.Fatalf("expected outer query to append WHERE, got %q", query)
	}
}

func TestSQLCursorValues_UsesOrderedSQLValuesForQualifiedJoinKey(t *testing.T) {
	values, err := sqlCursorValues(
		map[string]any{
			"id":    1,
			"id__2": 9,
		},
		[]any{1, 9},
		[]ResultColumn{
			{
				Key:      "id",
				Name:     "id",
				Position: 0,
				Origins:  []ResultColumnOrigin{{Alias: "u", Table: "users", Column: "id"}},
			},
			{
				Key:      "id__2",
				Name:     "id",
				Position: 1,
				Origins:  []ResultColumnOrigin{{Alias: "o", Table: "orders", Column: "id"}},
			},
		},
		[]sqlSortKey{{Column: "o.id", Desc: false}},
		"SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id ORDER BY o.id",
		"postgres",
	)
	if err != nil {
		t.Fatalf("sqlCursorValues: %v", err)
	}
	if len(values) != 1 || values[0] != 9 {
		t.Fatalf("expected cursor to use second ordered id, got %#v", values)
	}
}

func TestSQLCursorValues_UsesAliasToDisambiguateSelfJoin(t *testing.T) {
	values, err := sqlCursorValues(
		map[string]any{
			"id":    1,
			"id__2": 9,
		},
		[]any{1, 9},
		[]ResultColumn{
			{
				Key:      "id",
				Name:     "id",
				Position: 0,
				Origins:  []ResultColumnOrigin{{Alias: "u", Table: "users", Column: "id"}},
			},
			{
				Key:      "id__2",
				Name:     "id",
				Position: 1,
				Origins:  []ResultColumnOrigin{{Alias: "v", Table: "users", Column: "id"}},
			},
		},
		[]sqlSortKey{{Column: "v.id", Desc: false}},
		"SELECT u.id, v.id FROM users u JOIN users v ON u.referrer_id = v.id ORDER BY v.id",
		"postgres",
	)
	if err != nil {
		t.Fatalf("sqlCursorValues: %v", err)
	}
	if len(values) != 1 || values[0] != 9 {
		t.Fatalf("expected cursor to use aliased self-join id, got %#v", values)
	}
}

func TestSQLCursorValues_RejectsAmbiguousQualifiedFallback(t *testing.T) {
	_, err := sqlCursorValues(
		map[string]any{
			"id":    1,
			"id__2": 9,
		},
		[]any{1, 9},
		[]ResultColumn{
			{
				Key:              "id",
				Name:             "id",
				Position:         0,
				ConservativeMask: true,
			},
			{
				Key:              "id__2",
				Name:             "id",
				Position:         1,
				ConservativeMask: true,
			},
		},
		[]sqlSortKey{{Column: "o.id", Desc: false}},
		"SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id ORDER BY o.id",
		"postgres",
	)
	if err == nil || !strings.Contains(err.Error(), "ambiguous cursor value for qualified key o.id") {
		t.Fatalf("expected ambiguous qualified cursor error, got %v", err)
	}
}

func TestSQLCursorValues_AllowsQualifiedFallbackWhenRowValueIsUnique(t *testing.T) {
	values, err := sqlCursorValues(
		map[string]any{
			"id": 9,
		},
		[]any{9},
		[]ResultColumn{
			{
				Key:              "id",
				Name:             "id",
				Position:         0,
				ConservativeMask: true,
			},
		},
		[]sqlSortKey{{Column: "o.id", Desc: false}},
		"SELECT o.id FROM orders o ORDER BY o.id",
		"postgres",
	)
	if err != nil {
		t.Fatalf("sqlCursorValues: %v", err)
	}
	if len(values) != 1 || values[0] != 9 {
		t.Fatalf("expected unique qualified fallback cursor, got %#v", values)
	}
}

func TestSQLCursorValues_PreservesRealColumnNamesEndingWithDuplicateSuffix(t *testing.T) {
	values, err := sqlCursorValues(
		map[string]any{
			"code__2": "A-9",
		},
		[]any{"A-9"},
		[]ResultColumn{
			{
				Key:              "code__2",
				Name:             "code__2",
				Position:         0,
				ConservativeMask: true,
			},
		},
		[]sqlSortKey{{Column: "o.code__2", Desc: false}},
		"SELECT o.code__2 FROM orders o ORDER BY o.code__2",
		"postgres",
	)
	if err != nil {
		t.Fatalf("sqlCursorValues: %v", err)
	}
	if len(values) != 1 || values[0] != "A-9" {
		t.Fatalf("expected real __2 column to survive qualified fallback, got %#v", values)
	}
}

func TestSQLCursorValues_QualifiedFallbackIgnoresRealSuffixStyleSiblingColumns(t *testing.T) {
	values, err := sqlCursorValues(
		map[string]any{
			"id":    7,
			"id__2": 42,
		},
		[]any{7, 42},
		[]ResultColumn{
			{
				Key:              "id",
				Name:             "id",
				Position:         0,
				ConservativeMask: true,
			},
			{
				Key:              "id__2",
				Name:             "id__2",
				Position:         1,
				ConservativeMask: true,
			},
		},
		[]sqlSortKey{{Column: "o.id", Desc: false}},
		"SELECT o.id, o.id__2 FROM orders o ORDER BY o.id",
		"postgres",
	)
	if err != nil {
		t.Fatalf("sqlCursorValues: %v", err)
	}
	if len(values) != 1 || values[0] != 7 {
		t.Fatalf("expected qualified cursor to use real id column, got %#v", values)
	}
}

func TestSQLAdapter_BuildSQLQueryResult_UsesConservativeFallbackOnProjectionError(t *testing.T) {
	adapter := &SQLAdapter{dialect: "postgres"}
	result := adapter.buildSQLQueryResult(
		context.Background(),
		datasource.DataSource{ID: "ds_sql", Type: datasource.TypePostgreSQL},
		"SELECT [",
		sqlRowBatch{
			ColumnNames: []string{"email"},
			ColumnKeys:  []string{"email"},
			Rows:        []map[string]any{{"email": "user@example.com"}},
			RowValues:   [][]any{{"user@example.com"}},
		},
		false,
		3,
	)
	if len(result.ColumnMeta) != 1 {
		t.Fatalf("expected one result column, got %#v", result.ColumnMeta)
	}
	if !result.ColumnMeta[0].ConservativeMask {
		t.Fatalf("expected conservative fallback when SQL projection parsing fails, got %#v", result.ColumnMeta[0])
	}
}
