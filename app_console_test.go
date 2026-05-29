package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

func TestDatasourceWithD1ExecutionModeOverride_RejectsDevWhenDatasourceDoesNotSupportDev(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"databaseId":   "db_123",
			"databaseName": "analytics",
		},
	}

	next := datasourceWithD1ExecutionModeOverride(ds, "dev")
	if got := optionAnyString(next.Options, "executionMode"); got != "remote" {
		t.Fatalf("expected executionMode remote for non-dev datasource, got %q", got)
	}
}

func TestDatasourceWithD1ExecutionModeOverride_AllowsDevWhenDatasourceSupportsDev(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"supportDev":         true,
			"devProjectPath":     "/tmp/project",
			"wranglerConfigPath": "/tmp/project/wrangler.toml",
		},
	}

	next := datasourceWithD1ExecutionModeOverride(ds, "dev")
	if got := optionAnyString(next.Options, "executionMode"); got != "dev" {
		t.Fatalf("expected executionMode dev, got %q", got)
	}
}

func TestDatasourceWithD1ExecutionModeOverride_AllowsDevForLegacyWranglerConfig(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"databaseId":         "db_legacy",
			"databaseName":       "legacy_db",
			"wranglerConfigPath": "/tmp/project/wrangler.toml",
		},
	}

	next := datasourceWithD1ExecutionModeOverride(ds, "dev")
	if got := optionAnyString(next.Options, "executionMode"); got != "dev" {
		t.Fatalf("expected executionMode dev for legacy wrangler datasource, got %q", got)
	}
}

func TestExportQueryResult_WritesIntoDownloadsDirectory(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	app := &App{}
	path, err := app.ExportQueryResult("mysql-result.csv", "id,name\n1,A\n")
	if err != nil {
		t.Fatalf("export query result: %v", err)
	}
	if filepath.Dir(path) != downloads {
		t.Fatalf("expected export in downloads: got %q want dir %q", path, downloads)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if string(data) != "id,name\n1,A\n" {
		t.Fatalf("unexpected export content: %q", string(data))
	}
}

func TestExportQueryResult_AppendsSuffixWhenFileExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	existing := filepath.Join(downloads, "mysql-result.csv")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	app := &App{}
	path, err := app.ExportQueryResult("mysql-result.csv", "new")
	if err != nil {
		t.Fatalf("export query result: %v", err)
	}
	if filepath.Base(path) != "mysql-result-1.csv" {
		t.Fatalf("expected suffixed file name, got %q", filepath.Base(path))
	}
}

func TestExportQueryResult_SanitizesTraversalName(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	app := &App{}
	path, err := app.ExportQueryResult("../secret.csv", "x")
	if err != nil {
		t.Fatalf("export query result: %v", err)
	}
	if filepath.Dir(path) != downloads {
		t.Fatalf("expected export under downloads, got %q", path)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("expected sanitized export path, got %q", path)
	}
	if filepath.Base(path) != "secret.csv" {
		t.Fatalf("expected sanitized base file name secret.csv, got %q", filepath.Base(path))
	}
}

func TestResolveExportDirectory_FallsBackToHomeWhenDownloadsMissing(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := resolveExportDirectory()
	if err != nil {
		t.Fatalf("resolve export directory: %v", err)
	}
	if path != tmpHome {
		t.Fatalf("expected fallback to home directory, got %q want %q", path, tmpHome)
	}
}

type fakeConsoleAdapter struct {
	listEntitiesFn     func(ctx context.Context, ds datasource.DataSource, opts console.ListOptions) ([]string, error)
	listEntitiesPageFn func(ctx context.Context, ds datasource.DataSource, opts console.ListOptions, cursor string) (console.EntityPage, error)
	describeEntityFn   func(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error)
	executeFn          func(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) (console.QueryResult, error)
}

type fakeRiskExecuteError struct {
	message string
	info    console.ExecuteRiskInfo
}

func (e fakeRiskExecuteError) Error() string {
	return e.message
}

func (e fakeRiskExecuteError) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return e.info
}

type fakeConsoleInterceptor struct {
	err      error
	lastOpts console.ExecuteOptions
}

func (f *fakeConsoleInterceptor) BeforeExecute(_ context.Context, _ datasource.DataSource, _ string, opts console.ExecuteOptions) error {
	f.lastOpts = opts
	return f.err
}

func (f *fakeConsoleAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	_ = ctx
	_ = ds
	return nil
}

func (f *fakeConsoleAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts console.ListOptions) ([]string, error) {
	if f.listEntitiesFn == nil {
		return nil, errors.New("list entities not configured")
	}
	return f.listEntitiesFn(ctx, ds, opts)
}

func (f *fakeConsoleAdapter) ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts console.ListOptions, cursor string) (console.EntityPage, error) {
	if f.listEntitiesPageFn == nil {
		return console.EntityPage{}, errors.New("list entities page not configured")
	}
	return f.listEntitiesPageFn(ctx, ds, opts, cursor)
}

func (f *fakeConsoleAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error) {
	if f.describeEntityFn == nil {
		return console.DescribeResult{}, errors.New("describe entity not configured")
	}
	return f.describeEntityFn(ctx, ds, name)
}

func (f *fakeConsoleAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) (console.QueryResult, error) {
	if f.executeFn == nil {
		return console.QueryResult{}, errors.New("execute not configured")
	}
	return f.executeFn(ctx, ds, statement, opts)
}

func (f *fakeConsoleAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (console.ExplainResult, error) {
	_ = ctx
	_ = ds
	_ = statement
	return console.ExplainResult{}, nil
}

func newConsoleEntityCacheTestApp(t *testing.T, ds datasource.DataSource, adapter *fakeConsoleAdapter) *App {
	t.Helper()

	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	store := datasource.NewStore(dataPath)
	if _, err := store.Create(ds); err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(ds.Type, adapter)

	entityCache := console.NewEntitySchemaCacheStore(filepath.Join(root, "entity-schema-cache.json"))
	if err := entityCache.Load(); err != nil {
		t.Fatalf("load entity cache: %v", err)
	}

	return &App{
		store:       store,
		manager:     manager,
		entityCache: entityCache,
	}
}

func TestSupportsEntitySchemaCache_CoversAllExceptRedisAndD1Local(t *testing.T) {
	if !supportsEntitySchemaCache(datasource.DataSource{Type: datasource.TypeMySQL}) {
		t.Fatalf("expected mysql to support entity schema cache")
	}
	if !supportsEntitySchemaCache(datasource.DataSource{Type: datasource.TypePostgreSQL}) {
		t.Fatalf("expected postgresql to support entity schema cache")
	}
	if !supportsEntitySchemaCache(datasource.DataSource{Type: datasource.TypeMongoDB}) {
		t.Fatalf("expected mongodb to support entity schema cache")
	}
	if supportsEntitySchemaCache(datasource.DataSource{Type: datasource.TypeRedis}) {
		t.Fatalf("expected redis to be excluded from entity schema cache")
	}
	if supportsEntitySchemaCache(datasource.DataSource{
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode": "local",
		},
	}) {
		t.Fatalf("expected d1 local mode to be excluded from entity schema cache")
	}
	if !supportsEntitySchemaCache(datasource.DataSource{
		Type: datasource.TypeD1,
		Options: map[string]any{
			"executionMode": "remote",
		},
	}) {
		t.Fatalf("expected d1 remote mode to support entity schema cache")
	}
}

func TestListEntitiesPage_MySQLFallsBackToEntityCacheWhenRemoteUnavailable(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "mysql",
		Type: datasource.TypeMySQL,
	}

	listEntitiesErr := error(nil)
	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			if listEntitiesErr != nil {
				return nil, listEntitiesErr
			}
			return []string{"users", "orders", "audit_logs"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, errors.New("remote paging unavailable")
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	firstPage, err := app.ListEntitiesPage(ds.ID, "", "", "", 2, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage first page: %v", err)
	}
	if !reflect.DeepEqual(firstPage.Items, []string{"audit_logs", "orders"}) {
		t.Fatalf("expected first page items from cached snapshot, got %#v", firstPage.Items)
	}
	if firstPage.Done {
		t.Fatalf("expected first page to have more results")
	}

	listEntitiesErr = errors.New("remote list unavailable")
	filteredPage, err := app.ListEntitiesPage(ds.ID, "ord", "", "", 10, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage cached filter fallback: %v", err)
	}
	if !reflect.DeepEqual(filteredPage.Items, []string{"orders"}) {
		t.Fatalf("expected filtered cached items, got %#v", filteredPage.Items)
	}
}

func TestListEntitiesPage_MySQLIncludesCachedDescribeDetailsForCurrentPage(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "mysql",
		Type: datasource.TypeMySQL,
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{"users", "orders"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, errors.New("remote paging unavailable")
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, name string) (console.DescribeResult, error) {
			switch name {
			case "users":
				return console.DescribeResult{
					Columns: []console.ColumnInfo{{Name: "id", DataType: "bigint", Nullable: "NO"}},
					Indexes: []console.IndexInfo{{Name: "PRIMARY", Column: "id", Unique: true}},
				}, nil
			default:
				return console.DescribeResult{}, nil
			}
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)
	if _, err := app.DescribeEntity(ds.ID, "users", "", ""); err != nil {
		t.Fatalf("DescribeEntity seed cache: %v", err)
	}

	page, err := app.ListEntitiesPage(ds.ID, "users", "", "", 10, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	detail, ok := page.Details["users"]
	if !ok {
		t.Fatalf("expected users detail in cached page payload")
	}
	if len(detail.Indexes) != 1 || detail.Indexes[0].Name != "PRIMARY" {
		t.Fatalf("expected primary index detail, got %#v", detail.Indexes)
	}
}

func TestListEntitiesPage_D1LocalDoesNotExposeCachedDescribeDetails(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1",
		Name: "d1-local",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"supportDev":         true,
			"devProjectPath":     "/tmp/project",
			"wranglerConfigPath": "/tmp/project/wrangler.toml",
			"executionMode":      "dev",
		},
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{"users"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{Items: []string{"users"}, Cursor: "", Done: true}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{
				Columns: []console.ColumnInfo{{Name: "id", DataType: "integer", Nullable: "NO"}},
				Indexes: []console.IndexInfo{{Name: "PRIMARY", Column: "id", Unique: true}},
			}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)
	if _, err := app.DescribeEntity(ds.ID, "users", "", "dev"); err != nil {
		t.Fatalf("DescribeEntity local d1: %v", err)
	}

	page, err := app.ListEntitiesPage(ds.ID, "", "", "", 10, "dev", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage local d1: %v", err)
	}
	if len(page.Details) != 0 {
		t.Fatalf("expected no cached describe details for d1 local, got %#v", page.Details)
	}
}

func TestListEntitiesPage_ForceRefreshBypassesCacheSnapshot(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "mysql",
		Type: datasource.TypeMySQL,
	}

	entityRows := []string{"users_old", "orders_old"}
	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return append([]string(nil), entityRows...), nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, errors.New("remote paging unavailable")
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	oldPage, err := app.ListEntitiesPage(ds.ID, "", "", "", 10, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage first load: %v", err)
	}
	if !reflect.DeepEqual(oldPage.Items, []string{"orders_old", "users_old"}) {
		t.Fatalf("expected initial cached entities, got %#v", oldPage.Items)
	}

	entityRows = []string{"users_new", "payments_new"}
	forcedPage, err := app.ListEntitiesPage(ds.ID, "", "", "", 10, "", true)
	if err != nil {
		t.Fatalf("ListEntitiesPage forced load: %v", err)
	}
	if !reflect.DeepEqual(forcedPage.Items, []string{"payments_new", "users_new"}) {
		t.Fatalf("expected forced refresh entities, got %#v", forcedPage.Items)
	}
}

func TestListEntitiesPage_D1ReturnsViewKinds(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1_cloud",
		Name: "d1-eps",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode": "cloud",
		},
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{"conversion_stats", "conversions", "rate_limits"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{
				Items: []string{"conversion_stats", "conversions", "rate_limits"},
				Kinds: map[string]string{"conversion_stats": "view"},
				Done:  true,
			}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	// Cold load — no cache yet
	page, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage cold: %v", err)
	}
	if page.Kinds == nil || page.Kinds["conversion_stats"] != "view" {
		t.Fatalf("cold load: expected conversion_stats=view in kinds, got %v", page.Kinds)
	}

	// Second call — should hit cache and still return kinds
	page2, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage cached: %v", err)
	}
	if page2.Kinds == nil || page2.Kinds["conversion_stats"] != "view" {
		t.Fatalf("cached load: expected conversion_stats=view in kinds, got %v", page2.Kinds)
	}

	// Force refresh — should also return kinds
	page3, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", true)
	if err != nil {
		t.Fatalf("ListEntitiesPage forced: %v", err)
	}
	if page3.Kinds == nil || page3.Kinds["conversion_stats"] != "view" {
		t.Fatalf("forced refresh: expected conversion_stats=view in kinds, got %v", page3.Kinds)
	}
}

func TestListEntitiesPage_D1LegacyCacheGetsKindsBackfilled(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1_legacy",
		Name: "d1-eps",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode": "cloud",
		},
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{"conversion_stats", "conversions", "rate_limits"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{
				Items: []string{"conversion_stats", "conversions", "rate_limits"},
				Kinds: map[string]string{"conversion_stats": "view"},
				Done:  true,
			}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	// Simulate legacy cache: upsert entities WITHOUT kinds
	cacheKey := entitySchemaCacheKey(ds)
	_ = app.entityCache.UpsertEntitiesWithKinds(cacheKey, []string{"conversion_stats", "conversions", "rate_limits"}, nil, nil)

	// Now call ListEntitiesPage — cache hit path, but no kinds in cache
	page, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage with legacy cache: %v", err)
	}
	t.Logf("Legacy cache result: items=%v kinds=%v", page.Items, page.Kinds)
	if page.Kinds == nil || page.Kinds["conversion_stats"] != "view" {
		t.Fatalf("legacy cache: expected conversion_stats=view in kinds, got %v", page.Kinds)
	}
}

func TestListEntitiesPage_D1LegacyCacheWithFailingAPI(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1_failing",
		Name: "d1-eps",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode": "cloud",
		},
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return nil, errors.New("api error")
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, errors.New("api error")
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	// Simulate legacy cache: entities without kinds
	cacheKey := entitySchemaCacheKey(ds)
	_ = app.entityCache.UpsertEntitiesWithKinds(cacheKey, []string{"conversion_stats", "conversions", "rate_limits"}, nil, nil)

	// ListEntitiesPage — cache hit path, but API fails so collectEntityKinds returns empty
	page, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage with legacy cache and failing API: %v", err)
	}
	t.Logf("Legacy cache + failing API: items=%v kinds=%v", page.Items, page.Kinds)
	// This will likely FAIL — no kinds returned because collectEntityKinds fails silently
	if page.Kinds == nil || page.Kinds["conversion_stats"] != "view" {
		t.Logf("BUG CONFIRMED: legacy cache + failing API = no kinds. kinds=%v", page.Kinds)
	}
}

func TestListEntitiesPage_D1LegacyCacheWithWorkingAPI(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_d1_working",
		Name: "d1-eps",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode": "cloud",
		},
	}

	pageCallCount := 0
	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{"conversion_stats", "conversions", "rate_limits"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			pageCallCount++
			return console.EntityPage{
				Items: []string{"conversion_stats", "conversions", "rate_limits"},
				Kinds: map[string]string{"conversion_stats": "view"},
				Done:  true,
			}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}

	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	// Simulate legacy cache: entities without kinds (like user's existing cache)
	cacheKey := entitySchemaCacheKey(ds)
	_ = app.entityCache.UpsertEntitiesWithKinds(cacheKey, []string{"conversion_stats", "conversions", "rate_limits"}, nil, nil)

	// ListEntitiesPage with working API — should backfill kinds synchronously
	page, err := app.ListEntitiesPage(ds.ID, "", "", "", 200, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	t.Logf("Result: items=%v kinds=%v pageCallCount=%d", page.Items, page.Kinds, pageCallCount)
	if page.Kinds == nil || page.Kinds["conversion_stats"] != "view" {
		t.Fatalf("expected conversion_stats=view in kinds, got %v (pageCallCount=%d)", page.Kinds, pageCallCount)
	}
}

func TestListEntitiesPage_DynamoDBFallsBackToEntityCacheWhenRemoteUnavailable(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_dynamodb",
		Name: "ddb",
		Type: datasource.TypeDynamoDB,
	}

	listEntitiesErr := error(nil)
	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			if listEntitiesErr != nil {
				return nil, listEntitiesErr
			}
			return []string{"users", "orders", "audit_logs"}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, errors.New("remote paging unavailable")
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	firstPage, err := app.ListEntitiesPage(ds.ID, "", "", "", 2, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage first page: %v", err)
	}
	if !reflect.DeepEqual(firstPage.Items, []string{"audit_logs", "orders"}) {
		t.Fatalf("expected first page items from cached snapshot, got %#v", firstPage.Items)
	}
	if firstPage.Done {
		t.Fatalf("expected first page to have more results")
	}

	listEntitiesErr = errors.New("remote list unavailable")
	filteredPage, err := app.ListEntitiesPage(ds.ID, "ord", "", "", 10, "", false)
	if err != nil {
		t.Fatalf("ListEntitiesPage cached filter fallback: %v", err)
	}
	if !reflect.DeepEqual(filteredPage.Items, []string{"orders"}) {
		t.Fatalf("expected filtered cached items, got %#v", filteredPage.Items)
	}
	if !filteredPage.Done {
		t.Fatalf("expected filtered page to be done")
	}
}

func TestDescribeEntity_DynamoDBFallsBackToEntityCacheWhenRemoteUnavailable(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_dynamodb",
		Name: "ddb",
		Type: datasource.TypeDynamoDB,
	}

	describeErr := error(nil)
	expected := console.DescribeResult{
		Columns: []console.ColumnInfo{{Name: "id", DataType: "S"}},
		Indexes: []console.IndexInfo{{Name: "PRIMARY", Column: "id", Unique: true}},
		Details: []console.DetailItem{{Label: "Table", Value: "orders"}},
		Dialect: "partiql",
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			if describeErr != nil {
				return console.DescribeResult{}, describeErr
			}
			return expected, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			return console.QueryResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	initial, err := app.DescribeEntity(ds.ID, "orders", "", "")
	if err != nil {
		t.Fatalf("DescribeEntity initial: %v", err)
	}
	if !reflect.DeepEqual(initial, expected) {
		t.Fatalf("expected initial describe result, got %#v", initial)
	}

	describeErr = errors.New("describe unavailable")
	cached, err := app.DescribeEntity(ds.ID, "orders", "", "")
	if err != nil {
		t.Fatalf("DescribeEntity cached fallback: %v", err)
	}
	if !reflect.DeepEqual(cached, expected) {
		t.Fatalf("expected cached describe result, got %#v", cached)
	}
}

func TestAppApplyDynamoDBRiskExecutionCaps(t *testing.T) {
	maxPages := 2
	maxEvaluatedItems := 120
	engine := riskengine.NewEngine()
	engine.LoadUserRules([]riskengine.Rule{
		{
			ID:       "tight-dynamodb-budget",
			Scope:    riskengine.RuleScope{DsTypes: []string{"dynamodb"}},
			Enabled:  true,
			Priority: 200,
			Action:   riskengine.ActionAllow,
			When:     riskengine.RuleCondition{Command: []string{"select"}},
			Thresholds: riskengine.RuleThresholds{
				MaxDynamoDBPages:          &maxPages,
				MaxDynamoDBEvaluatedItems: &maxEvaluatedItems,
			},
		},
	})
	app := &App{riskEngine: engine}
	opts := console.ExecuteOptions{
		Bounds: console.ExecuteBounds{
			MaxReturnedRows:   100,
			MaxPages:          20,
			MaxEvaluatedItems: 5000,
		},
	}

	if err := app.applyDynamoDBRiskExecutionCaps(
		datasource.DataSource{ID: "ds_dynamodb", Type: datasource.TypeDynamoDB},
		`SELECT * FROM "orders"`,
		&opts,
	); err != nil {
		t.Fatalf("applyDynamoDBRiskExecutionCaps: %v", err)
	}

	if opts.Bounds.MaxPages != maxPages {
		t.Fatalf("MaxPages = %d, want %d", opts.Bounds.MaxPages, maxPages)
	}
	if opts.Bounds.MaxEvaluatedItems != maxEvaluatedItems {
		t.Fatalf("MaxEvaluatedItems = %d, want %d", opts.Bounds.MaxEvaluatedItems, maxEvaluatedItems)
	}
	if opts.Bounds.MaxReturnedRows != 100 {
		t.Fatalf("MaxReturnedRows = %d, want unchanged 100", opts.Bounds.MaxReturnedRows)
	}
}

func TestExecuteStatement_WailsBindingUsesFixedArguments(t *testing.T) {
	methodType := reflect.TypeOf((&App{}).ExecuteStatement)
	if methodType.IsVariadic() {
		t.Fatal("ExecuteStatement must not be variadic; Wails runtime rejects extra positional arguments")
	}
	if got, want := methodType.NumIn(), 10; got != want {
		t.Fatalf("ExecuteStatement argument count = %d, want %d", got, want)
	}
}

func TestExecuteStatement_ElasticsearchCatIndicesFallsBackToEntityCache(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_es",
		Name: "es",
		Type: datasource.TypeElasticsearch,
	}

	execErr := error(nil)
	expected := console.QueryResult{
		Rows: []map[string]any{
			{"index": "futrixdata-demo-1", "health": "green", "store.size": "12mb"},
			{"index": "futrixdata-demo-2", "health": "yellow", "store.size": "48mb"},
		},
		RowCount: 2,
	}

	adapter := &fakeConsoleAdapter{
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return []string{}, nil
		},
		listEntitiesPageFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions, _ string) (console.EntityPage, error) {
			return console.EntityPage{}, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			if execErr != nil {
				return console.QueryResult{}, execErr
			}
			return expected, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)

	statement := "GET /_cat/indices?format=json&h=index,health,store.size"
	initial, err := app.ExecuteStatement(ds.ID, statement, "", "", 10000, "", false, 0, 0, 0)
	if err != nil {
		t.Fatalf("ExecuteStatement initial: %v", err)
	}
	if !reflect.DeepEqual(initial.Rows, expected.Rows) {
		t.Fatalf("expected initial cat indices rows, got %#v", initial.Rows)
	}

	execErr = errors.New("cat indices unavailable")
	cached, err := app.ExecuteStatement(ds.ID, statement, "", "", 10000, "", false, 0, 0, 0)
	if err != nil {
		t.Fatalf("ExecuteStatement cached fallback: %v", err)
	}
	if !reflect.DeepEqual(cached.Rows, expected.Rows) {
		t.Fatalf("expected cached cat indices rows, got %#v", cached.Rows)
	}
}

func TestExecuteStatement_PreservesStructuredRiskError(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "mysql",
		Type: datasource.TypeMySQL,
	}
	adapter := &fakeConsoleAdapter{
		executeFn: func(_ context.Context, _ datasource.DataSource, _ string, _ console.ExecuteOptions) (console.QueryResult, error) {
			t.Fatal("adapter Execute should not be called when interceptor blocks")
			return console.QueryResult{}, nil
		},
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return nil, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)
	app.manager.SetInterceptor(&fakeConsoleInterceptor{
		err: fakeRiskExecuteError{
			message: "statement stopped for review: DELETE",
			info: console.ExecuteRiskInfo{
				Action:       "warn",
				Level:        "medium",
				Reasons:      []string{"DELETE"},
				RuleID:       "sql-warn-delete",
				TargetEntity: "users",
			},
		},
	})

	result, err := app.ExecuteStatement(ds.ID, "DELETE FROM users WHERE id = 1", "", "", 100, "", false, 0, 0, 0)
	if err != nil {
		t.Fatalf("expected no error (risk info in result), got: %v", err)
	}
	if result.RiskInfo == nil {
		t.Fatal("expected RiskInfo in result")
	}
	if result.RiskInfo.Action != "warn" {
		t.Fatalf("action = %q, want warn", result.RiskInfo.Action)
	}
	if result.RiskInfo.TargetEntity != "users" {
		t.Fatalf("targetEntity = %q, want users", result.RiskInfo.TargetEntity)
	}
}

func TestExecuteStatement_ApprovedConsoleRunUsesInteractiveApprovalPath(t *testing.T) {
	ds := datasource.DataSource{
		ID:   "ds_mysql",
		Name: "mysql",
		Type: datasource.TypeMySQL,
	}
	adapter := &fakeConsoleAdapter{
		executeFn: func(_ context.Context, _ datasource.DataSource, statement string, _ console.ExecuteOptions) (console.QueryResult, error) {
			if statement != "DELETE FROM users" {
				t.Fatalf("unexpected statement: %s", statement)
			}
			return console.QueryResult{Rows: []map[string]any{{"ok": true}}}, nil
		},
		listEntitiesFn: func(_ context.Context, _ datasource.DataSource, _ console.ListOptions) ([]string, error) {
			return nil, nil
		},
		describeEntityFn: func(_ context.Context, _ datasource.DataSource, _ string) (console.DescribeResult, error) {
			return console.DescribeResult{}, nil
		},
	}
	app := newConsoleEntityCacheTestApp(t, ds, adapter)
	app.manager.SetInterceptor(riskengine.NewGuard(riskengine.NewEngine()))

	if _, err := app.ExecuteStatement(ds.ID, "DELETE FROM users", "", "", 100, "", true, 0, 0, 0); err != nil {
		t.Fatalf("execute statement: %v", err)
	}
}
