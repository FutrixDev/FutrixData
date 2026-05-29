package console

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"futrixdata/platform/internal/datasource"
)

const stubMySQLDriverName = "stub-mysql-schema"
const stubPostgresDriverName = "stub-postgres-schema"

var registerStubMySQLDriverOnce sync.Once
var registerStubPostgresDriverOnce sync.Once

func registerStubMySQLDriver() {
	registerStubMySQLDriverOnce.Do(func() {
		sql.Register(stubMySQLDriverName, stubMySQLDriver{})
	})
}

func registerStubPostgresDriver() {
	registerStubPostgresDriverOnce.Do(func() {
		sql.Register(stubPostgresDriverName, stubPostgresDriver{})
	})
}

type stubMySQLDriver struct{}

func (stubMySQLDriver) Open(name string) (driver.Conn, error) {
	return &stubMySQLConn{}, nil
}

type stubPostgresDriver struct{}

func (stubPostgresDriver) Open(name string) (driver.Conn, error) {
	return &stubPostgresConn{}, nil
}

type stubMySQLConn struct{}

var _ driver.Conn = (*stubMySQLConn)(nil)
var _ driver.QueryerContext = (*stubMySQLConn)(nil)

func (c *stubMySQLConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported: %s", query)
}

func (c *stubMySQLConn) Close() error { return nil }

func (c *stubMySQLConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("tx not supported")
}

func (c *stubMySQLConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	_ = ctx
	lower := strings.ToLower(query)

	switch {
	case strings.Contains(lower, "from information_schema.tables"):
		if !strings.Contains(lower, "order by table_name") {
			return nil, fmt.Errorf("expected ORDER BY table_name, got %s", query)
		}
		var cursor string
		var limit int64
		for _, arg := range args {
			switch v := arg.Value.(type) {
			case string:
				cursor = v
			case int64:
				limit = v
			case int32:
				limit = int64(v)
			case int:
				limit = int64(v)
			}
		}
		if cursor != "" && !strings.Contains(lower, "table_name >") {
			return nil, fmt.Errorf("expected cursor query with table_name >, got %s", query)
		}
		type stubEntity struct {
			name      string
			tableType string
		}
		all := []stubEntity{
			{"alpha", "BASE TABLE"},
			{"beta", "BASE TABLE"},
			{"gamma", "BASE TABLE"},
			{"omega", "VIEW"},
		}
		filtered := make([]stubEntity, 0, len(all))
		for _, e := range all {
			if cursor == "" || e.name > cursor {
				filtered = append(filtered, e)
			}
		}
		if limit <= 0 {
			limit = int64(len(filtered))
		}
		if int64(len(filtered)) > limit {
			filtered = filtered[:limit]
		}
		includeType := strings.Contains(lower, "table_type")
		values := make([][]driver.Value, 0, len(filtered))
		for _, e := range filtered {
			if includeType {
				values = append(values, []driver.Value{e.name, e.tableType})
			} else {
				values = append(values, []driver.Value{e.name})
			}
		}
		if includeType {
			return &stubRows{
				columns: []string{"table_name", "table_type"},
				values:  values,
			}, nil
		}
		return &stubRows{
			columns: []string{"table_name"},
			values:  values,
		}, nil
	case strings.Contains(lower, "from information_schema.columns"):
		dataType := "char"
		if strings.Contains(lower, "column_type") {
			dataType = "char(5)"
		}
		return &stubRows{
			columns: []string{"column_name", "data_type", "is_nullable", "column_default"},
			values: [][]driver.Value{
				{"c_char_05", dataType, "NO", nil},
			},
		}, nil
	case strings.Contains(lower, "from information_schema.statistics"):
		return &stubRows{
			columns: []string{"index_name", "column_name", "non_unique", "seq_in_index"},
			values: [][]driver.Value{
				{"PRIMARY", "c_char_05", int64(0), int64(1)},
			},
		}, nil
	case strings.Contains(lower, "from information_schema.key_column_usage"):
		return &stubRows{
			columns: []string{"column_name"},
			values: [][]driver.Value{
				{"id"},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type stubPostgresConn struct{}

var _ driver.Conn = (*stubPostgresConn)(nil)
var _ driver.QueryerContext = (*stubPostgresConn)(nil)

func (c *stubPostgresConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare not supported: %s", query)
}

func (c *stubPostgresConn) Close() error { return nil }

func (c *stubPostgresConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("tx not supported")
}

func (c *stubPostgresConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	_ = ctx
	lower := strings.ToLower(query)

	switch {
	case strings.Contains(lower, "from information_schema.tables"):
		if !strings.Contains(lower, "table_schema not in ('pg_catalog','information_schema')") {
			return nil, fmt.Errorf("expected system schema exclusion, got %s", query)
		}
		if !strings.Contains(lower, "order by table_schema, table_name") {
			return nil, fmt.Errorf("expected ORDER BY table_schema, table_name, got %s", query)
		}

		stringArgs := make([]string, 0, 4)
		limit := int64(0)
		for _, arg := range args {
			switch v := arg.Value.(type) {
			case string:
				stringArgs = append(stringArgs, v)
			case int64:
				limit = v
			case int32:
				limit = int64(v)
			case int:
				limit = int64(v)
			}
		}
		if limit <= 0 {
			limit = 100
		}

		pattern := ""
		cursorSchema := ""
		cursorTable := ""
		idx := 0

		if strings.Contains(lower, "ilike") {
			if len(stringArgs) < 1 {
				return nil, fmt.Errorf("expected pattern arg, got %v", args)
			}
			pattern = stringArgs[0]
			idx = 1
		}
		if strings.Contains(lower, "(table_schema, table_name) >") {
			if len(stringArgs) < idx+2 {
				return nil, fmt.Errorf("expected schema+table cursor args, got %v", args)
			}
			cursorSchema = stringArgs[idx]
			cursorTable = stringArgs[idx+1]
		} else if len(stringArgs) > idx {
			return nil, fmt.Errorf("expected composite cursor query, got %s", query)
		}

		type entry struct{ schema, table, tableType string }
		all := []entry{
			{schema: "audit", table: "users", tableType: "BASE TABLE"},
			{schema: "audit", table: "zeta", tableType: "VIEW"},
			{schema: "public", table: "users", tableType: "BASE TABLE"},
			{schema: "public", table: "zeta", tableType: "BASE TABLE"},
		}

		filtered := make([]entry, 0, len(all))
		for _, item := range all {
			if cursorSchema != "" || cursorTable != "" {
				if item.schema < cursorSchema || (item.schema == cursorSchema && item.table <= cursorTable) {
					continue
				}
			}
			if pattern != "" {
				needle := strings.Trim(strings.ToLower(pattern), "%")
				combined := strings.ToLower(item.schema + "." + item.table)
				if needle != "" && !strings.Contains(combined, needle) {
					continue
				}
			}
			filtered = append(filtered, item)
		}
		if int64(len(filtered)) > limit {
			filtered = filtered[:limit]
		}

		includeType := strings.Contains(lower, "table_type")
		values := make([][]driver.Value, 0, len(filtered))
		for _, item := range filtered {
			if includeType {
				values = append(values, []driver.Value{item.schema, item.table, item.tableType})
			} else {
				values = append(values, []driver.Value{item.schema, item.table})
			}
		}

		if includeType {
			return &stubRows{columns: []string{"table_schema", "table_name", "table_type"}, values: values}, nil
		}
		return &stubRows{columns: []string{"table_schema", "table_name"}, values: values}, nil
	case strings.Contains(lower, "from information_schema.columns"):
		if strings.Contains(lower, "current_schema()") {
			if len(args) != 1 {
				return nil, fmt.Errorf("expected 1 arg for current_schema() describe, got %v", args)
			}
		} else if !strings.Contains(lower, "table_schema = $1 and table_name = $2") {
			return nil, fmt.Errorf("expected schema-qualified describe, got %s", query)
		}
		return &stubRows{
			columns: []string{"column_name", "data_type", "is_nullable", "column_default"},
			values: [][]driver.Value{
				{"id", "bigint", "NO", nil},
			},
		}, nil
	case strings.Contains(lower, "from pg_indexes"):
		if strings.Contains(lower, "current_schema()") {
			if len(args) != 1 {
				return nil, fmt.Errorf("expected 1 arg for current_schema() indexes, got %v", args)
			}
		} else if !strings.Contains(lower, "schemaname = $1 and tablename = $2") {
			return nil, fmt.Errorf("expected schema-qualified pg_indexes query, got %s", query)
		}
		return &stubRows{
			columns: []string{"indexname", "indexdef"},
			values: [][]driver.Value{
				{"idx_alpha", "CREATE INDEX idx_alpha ON alpha (id)"},
			},
		}, nil
	case strings.Contains(lower, "from pg_constraint"):
		if strings.Contains(lower, "current_schema()") {
			if len(args) != 1 {
				return nil, fmt.Errorf("expected 1 arg for current_schema() primary key query, got %v", args)
			}
		} else if !strings.Contains(lower, "n.nspname = $1 and t.relname = $2") {
			return nil, fmt.Errorf("expected schema-qualified primary key query, got %s", query)
		}
		return &stubRows{
			columns: []string{"conname", "attname"},
			values: [][]driver.Value{
				{"custom_pkey", "id"},
			},
		}, nil
	case strings.Contains(lower, "from pg_index"):
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 arg for pg_index primary key lookup, got %v", args)
		}
		return &stubRows{
			columns: []string{"attname"},
			values: [][]driver.Value{
				{"id"},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type stubRows struct {
	columns []string
	values  [][]driver.Value
	idx     int
}

var _ driver.Rows = (*stubRows)(nil)

func (r *stubRows) Columns() []string { return r.columns }

func (r *stubRows) Close() error { return nil }

func (r *stubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.idx])
	r.idx++
	return nil
}

func TestSQLAdapter_DescribeEntity_MySQL_IncludesColumnLength(t *testing.T) {
	registerStubMySQLDriver()

	adapter := &SQLAdapter{
		driver:  stubMySQLDriverName,
		dialect: "mysql",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}

	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}
	got, err := adapter.DescribeEntity(context.Background(), ds, "tbl")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if len(got.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(got.Columns))
	}
	if got.Columns[0].DataType != "char(5)" {
		t.Fatalf("expected dataType to include length, got %q", got.Columns[0].DataType)
	}
}

func TestSQLAdapter_ListEntitiesPage_MySQL(t *testing.T) {
	registerStubMySQLDriver()

	adapter := &SQLAdapter{
		driver:  stubMySQLDriverName,
		dialect: "mysql",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}

	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypeMySQL}

	page1, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page1.Items) != 2 || page1.Items[0] != "alpha" || page1.Items[1] != "beta" {
		t.Fatalf("unexpected page1 items: %v", page1.Items)
	}
	if page1.Cursor != "beta" || page1.Done {
		t.Fatalf("expected cursor=beta done=false, got cursor=%q done=%v", page1.Cursor, page1.Done)
	}

	page2, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, page1.Cursor)
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page2.Items) != 2 || page2.Items[0] != "gamma" || page2.Items[1] != "omega" {
		t.Fatalf("unexpected page2 items: %v", page2.Items)
	}
	if page2.Cursor != "" || !page2.Done {
		t.Fatalf("expected cursor='' done=true, got cursor=%q done=%v", page2.Cursor, page2.Done)
	}
	if page2.Kinds == nil || page2.Kinds["omega"] != "view" {
		t.Fatalf("expected omega to be a view, got kinds=%v", page2.Kinds)
	}
	if _, ok := page2.Kinds["gamma"]; ok {
		t.Fatalf("expected gamma NOT in kinds (table), got kinds=%v", page2.Kinds)
	}
}

func TestSQLAdapter_ListEntitiesPage_Postgres_SchemaQualified(t *testing.T) {
	registerStubPostgresDriver()

	adapter := &SQLAdapter{
		driver:  stubPostgresDriverName,
		dialect: "postgres",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}

	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypePostgreSQL}

	page1, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page1.Items) != 2 || page1.Items[0] != "audit.users" || page1.Items[1] != "audit.zeta" {
		t.Fatalf("unexpected page1 items: %v", page1.Items)
	}
	if page1.Cursor != "audit.zeta" || page1.Done {
		t.Fatalf("expected cursor=audit.zeta done=false, got cursor=%q done=%v", page1.Cursor, page1.Done)
	}
	if page1.Kinds == nil || page1.Kinds["audit.zeta"] != "view" {
		t.Fatalf("expected audit.zeta to be a view, got kinds=%v", page1.Kinds)
	}
	if _, ok := page1.Kinds["audit.users"]; ok {
		t.Fatalf("expected audit.users NOT in kinds (table), got kinds=%v", page1.Kinds)
	}

	page2, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, page1.Cursor)
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page2.Items) != 2 || page2.Items[0] != "public.users" || page2.Items[1] != "public.zeta" {
		t.Fatalf("unexpected page2 items: %v", page2.Items)
	}
	if page2.Cursor != "" || !page2.Done {
		t.Fatalf("expected cursor='' done=true, got cursor=%q done=%v", page2.Cursor, page2.Done)
	}
	if len(page2.Kinds) != 0 {
		t.Fatalf("expected no views in page2, got kinds=%v", page2.Kinds)
	}
}

func TestSQLAdapter_ListEntitiesPage_Postgres_PatternMatchesSchemaQualified(t *testing.T) {
	registerStubPostgresDriver()

	adapter := &SQLAdapter{
		driver:  stubPostgresDriverName,
		dialect: "postgres",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}

	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypePostgreSQL}

	page, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 10, Pattern: "public.users"}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != "public.users" {
		t.Fatalf("unexpected items: %v", page.Items)
	}
	if page.Cursor != "" || !page.Done {
		t.Fatalf("expected cursor='' done=true, got cursor=%q done=%v", page.Cursor, page.Done)
	}
}

func TestSQLAdapter_DescribeEntity_Postgres_SchemaQualified(t *testing.T) {
	registerStubPostgresDriver()

	adapter := &SQLAdapter{
		driver:  stubPostgresDriverName,
		dialect: "postgres",
		dsn: func(datasource.DataSource) (string, error) {
			return "stub", nil
		},
		pools: make(map[string]*sql.DB),
		byID:  make(map[string]string),
	}

	ds := datasource.DataSource{ID: "ds1", Type: datasource.TypePostgreSQL}

	got, err := adapter.DescribeEntity(context.Background(), ds, "audit.users")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if len(got.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(got.Columns))
	}
	if got.Columns[0].Name != "id" {
		t.Fatalf("expected column id, got %q", got.Columns[0].Name)
	}
	if len(got.Indexes) != 2 {
		t.Fatalf("expected 2 indexes, got %d", len(got.Indexes))
	}
	if got.Indexes[0].Name != "idx_alpha" {
		t.Fatalf("expected idx_alpha, got %q", got.Indexes[0].Name)
	}
	if got.Indexes[1].Name != "PRIMARY" {
		t.Fatalf("expected PRIMARY, got %q", got.Indexes[1].Name)
	}
	if got.Indexes[1].Column != "id" {
		t.Fatalf("expected PRIMARY column id, got %q", got.Indexes[1].Column)
	}
}

func TestInferSQLSortKeys_IgnoresCTEInternalJoinForTopLevelSingleTable(t *testing.T) {
	registerStubPostgresDriver()

	db, err := sql.Open(stubPostgresDriverName, "stub")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	stmt := `WITH joined AS (SELECT u.id FROM users u JOIN orders o ON u.id = o.user_id) SELECT id FROM public.users`
	keys, err := inferSQLSortKeys(context.Background(), db, "postgres", stmt)
	if err != nil {
		t.Fatalf("inferSQLSortKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Column != "id" || keys[0].Desc {
		t.Fatalf("expected inferred primary-key sort on id, got %+v", keys)
	}
}

func TestInferSQLSortKeys_FallsBackToOrderByWhenMySQLParseFails(t *testing.T) {
	stmt := "SELECT * FROM users FORCE INDEX (PRIMARY) ORDER BY created_at DESC"
	keys, err := inferSQLSortKeys(context.Background(), nil, "mysql", stmt)
	if err != nil {
		t.Fatalf("inferSQLSortKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Column != "created_at" || !keys[0].Desc {
		t.Fatalf("expected fallback ORDER BY sort on created_at desc, got %+v", keys)
	}
}

func TestInferSQLSortKeys_FallsBackToPrimaryKeyWhenMySQLParseFails(t *testing.T) {
	registerStubMySQLDriver()

	db, err := sql.Open(stubMySQLDriverName, "stub")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	stmt := "SELECT * FROM users FORCE INDEX (PRIMARY)"
	keys, err := inferSQLSortKeys(context.Background(), db, "mysql", stmt)
	if err != nil {
		t.Fatalf("inferSQLSortKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Column != "id" || keys[0].Desc {
		t.Fatalf("expected fallback primary-key sort on id, got %+v", keys)
	}
}
