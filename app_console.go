package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
)

func (a *App) GetRedisCommandDocs(id string) (console.RedisCommandDocsEntry, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return console.RedisCommandDocsEntry{}, errors.New("datasource not found")
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.get_redis_command_docs", ds, "", console.ExecuteOptions{}, false)
	var err error
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "app.redis_docs.adapter_lookup")
	adapter, err := a.manager.AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return console.RedisCommandDocsEntry{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	redisAdapter, ok := adapter.(*console.RedisAdapter)
	if !ok {
		err = errors.New("redis adapter not available")
		return console.RedisCommandDocsEntry{}, err
	}
	done = console.DatasourceTimingStage(ctx, "redis.command_docs_store_get")
	entry, err := a.redisDocs.Get(ctx, ds.ID, func(ctx context.Context) (map[string]any, error) {
		// Resolve SecretRef-backed credentials; this path connects directly via the
		// adapter and would otherwise use the empty password from the stored record.
		resolved, err := a.manager.ResolveDatasource(ctx, ds)
		if err != nil {
			return nil, err
		}
		return console.FetchRedisCommandDocs(ctx, redisAdapter, resolved)
	})
	status := "ok"
	if err != nil {
		status = "error"
	}
	done(console.DatasourceTimingKV("status", status), console.DatasourceTimingKV("commands", len(entry.Commands)))
	return entry, err
}

type RedisKeyPage struct {
	Keys   []string `json:"keys"`
	Cursor string   `json:"cursor"`
	Done   bool     `json:"done"`
}

const (
	elasticsearchIndicesMetaStatement = "GET /_cat/indices?format=json&h=index,health,store.size"
	entitySchemaCacheRefreshInterval  = 30 * time.Second
)

func (a *App) ListEntities(id, pattern, database, executionMode string, forceRefresh bool) ([]string, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.list_entities", ds, "", console.ExecuteOptions{}, false)
	var err error
	defer func() { finishTiming(err) }()

	cacheKey := entitySchemaCacheKey(ds)
	if supportsEntitySchemaCache(ds) && a.entityCache != nil {
		if forceRefresh {
			items, listErr := a.manager.ListEntities(ctx, ds, console.ListOptions{})
			err = listErr
			if err == nil {
				a.upsertEntityCacheEntities(ds, cacheKey, items, nil)
				_ = a.entityCache.ClearDescribe(cacheKey)
				_ = a.entityCache.MarkStale(cacheKey)
				return filterCachedEntityNames(items, pattern), nil
			}
			if cached, _, ok := a.entityCache.GetEntities(cacheKey); ok {
				err = nil
				return filterCachedEntityNames(cached, pattern), nil
			}
			return nil, err
		}
		if cached, _, ok := a.entityCache.GetEntities(cacheKey); ok {
			done := console.DatasourceTimingStage(ctx, "app.entity_cache")
			done(console.DatasourceTimingKV("status", "hit"), console.DatasourceTimingKV("items", len(cached)))
			a.refreshEntitySchemaCacheAsync(ds, cacheKey)
			return filterCachedEntityNames(cached, pattern), nil
		}
		items, listErr := a.manager.ListEntities(ctx, ds, console.ListOptions{})
		err = listErr
		if err != nil {
			return nil, err
		}
		a.upsertEntityCacheEntities(ds, cacheKey, items, nil)
		a.refreshEntitySchemaCacheAsync(ds, cacheKey)
		return filterCachedEntityNames(items, pattern), nil
	}

	result, listErr := a.manager.ListEntities(ctx, ds, console.ListOptions{Pattern: pattern})
	err = listErr
	return result, err
}

func (a *App) ListEntitiesPage(id, pattern, database, cursor string, limit int, executionMode string, forceRefresh bool) (console.EntityPage, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return console.EntityPage{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.list_entities_page", ds, "", console.ExecuteOptions{PagingToken: cursor, PageSize: limit}, false)
	var err error
	defer func() { finishTiming(err) }()

	cacheKey := entitySchemaCacheKey(ds)
	if supportsEntitySchemaCache(ds) && a.entityCache != nil {
		if forceRefresh {
			items, kinds, listErr := a.listEntitiesWithKinds(ctx, ds)
			err = listErr
			if err == nil {
				a.upsertEntityCacheEntitiesWithKinds(ds, cacheKey, items, kinds, nil)
				_ = a.entityCache.ClearDescribe(cacheKey)
				_ = a.entityCache.MarkStale(cacheKey)
				return paginateCachedEntityNames(items, pattern, cursor, limit, nil, kinds), nil
			}
			if cached, _, ok := a.entityCache.GetEntities(cacheKey); ok {
				err = nil
				details := a.cachedDescribeDetailsForEntities(cacheKey, cached)
				cachedKinds := a.entityCache.GetKinds(cacheKey)
				return paginateCachedEntityNames(cached, pattern, cursor, limit, details, cachedKinds), nil
			}
			return console.EntityPage{}, err
		}
		if cached, _, ok := a.entityCache.GetEntities(cacheKey); ok {
			done := console.DatasourceTimingStage(ctx, "app.entity_cache")
			done(console.DatasourceTimingKV("status", "hit"), console.DatasourceTimingKV("items", len(cached)))
			a.refreshEntitySchemaCacheAsync(ds, cacheKey)
			details := a.cachedDescribeDetailsForEntities(cacheKey, cached)
			kinds := a.entityCache.GetKinds(cacheKey)
			if len(kinds) == 0 && datasourceSupportsKinds(ds.Type) {
				kinds = a.collectEntityKinds(ctx, ds)
				if len(kinds) > 0 {
					a.upsertEntityCacheEntitiesWithKinds(ds, cacheKey, cached, kinds, nil)
				}
			}
			return paginateCachedEntityNames(cached, pattern, cursor, limit, details, kinds), nil
		}
		items, kinds, listErr := a.listEntitiesWithKinds(ctx, ds)
		err = listErr
		if err != nil {
			return console.EntityPage{}, err
		}
		a.upsertEntityCacheEntitiesWithKinds(ds, cacheKey, items, kinds, nil)
		a.refreshEntitySchemaCacheAsync(ds, cacheKey)
		details := a.cachedDescribeDetailsForEntities(cacheKey, items)
		return paginateCachedEntityNames(items, pattern, cursor, limit, details, kinds), nil
	}

	result, listErr := a.manager.ListEntitiesPage(ctx, ds, console.ListOptions{Limit: limit, Pattern: pattern}, cursor)
	err = listErr
	return result, err
}

// datasourceSupportsKinds reports whether the datasource type emits entity kind
// metadata (table vs view). Only SQL-based adapters populate EntityPage.Kinds.
func datasourceSupportsKinds(dsType datasource.DataSourceType) bool {
	switch dsType {
	case datasource.TypeMySQL, datasource.TypePostgreSQL, datasource.TypeD1:
		return true
	}
	return false
}

// listEntitiesWithKinds fetches the full entity list together with kind metadata.
// For adapters that support kinds (MySQL, PostgreSQL, D1), it uses ListEntitiesPage
// which returns both items and kinds in a single call, avoiding a second round-trip.
func (a *App) listEntitiesWithKinds(ctx context.Context, ds datasource.DataSource) ([]string, map[string]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !datasourceSupportsKinds(ds.Type) {
		items, err := a.manager.ListEntities(ctx, ds, console.ListOptions{})
		return items, nil, err
	}
	// Use ListEntitiesPage to get items and kinds in one pass.
	var allItems []string
	allKinds := make(map[string]string)
	cursor := ""
	const pageLimit = 10000
	for {
		page, err := a.manager.ListEntitiesPage(ctx, ds, console.ListOptions{Limit: pageLimit}, cursor)
		if err != nil {
			if len(allItems) > 0 {
				break
			}
			// Fallback to ListEntities if ListEntitiesPage is not supported.
			items, listErr := a.manager.ListEntities(ctx, ds, console.ListOptions{})
			if listErr != nil {
				return nil, nil, listErr
			}
			return items, nil, nil
		}
		allItems = append(allItems, page.Items...)
		for k, v := range page.Kinds {
			allKinds[k] = v
		}
		if page.Done || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}
	return allItems, allKinds, nil
}

// collectEntityKinds pages through ListEntitiesPage to accumulate kind metadata.
// Always returns a non-nil map so callers can distinguish "no views" from "not checked".
func (a *App) collectEntityKinds(ctx context.Context, ds datasource.DataSource) map[string]string {
	const pageLimit = 10000
	allKinds := make(map[string]string)
	cursor := ""
	for {
		page, err := a.manager.ListEntitiesPage(ctx, ds, console.ListOptions{Limit: pageLimit}, cursor)
		if err != nil {
			break
		}
		for k, v := range page.Kinds {
			allKinds[k] = v
		}
		if page.Done || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}
	return allKinds
}

func (a *App) ScanRedisKeys(id, pattern, cursor string) (RedisKeyPage, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return RedisKeyPage{}, errors.New("datasource not found")
	}
	if ds.Type != datasource.TypeRedis {
		return RedisKeyPage{}, errors.New("redis datasource required")
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.scan_redis_keys", ds, "", console.ExecuteOptions{PagingToken: cursor}, false)
	var err error
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "app.redis_scan.adapter_lookup")
	adapter, err := a.manager.AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	scanner, ok := adapter.(console.KeyScanner)
	if !ok {
		err = console.ErrUnsupported
		return RedisKeyPage{}, err
	}
	// Resolve SecretRef-backed credentials; this scan bypasses the manager dispatch
	// path that normally resolves secrets, so an unresolved ds would authenticate
	// with an empty password.
	done = console.DatasourceTimingStage(ctx, "app.redis_scan.resolve_datasource")
	ds, err = a.manager.ResolveDatasource(ctx, ds)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	done = console.DatasourceTimingStage(ctx, "app.redis_scan.decode_cursor")
	start, err := console.DecodeRedisCursor(cursor)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	done = console.DatasourceTimingStage(ctx, "redis.scan_keys")
	keys, next, scanDone, err := scanner.ScanKeys(ctx, ds, pattern, start)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("keys", len(keys)), console.DatasourceTimingKV("done", scanDone))
	done = console.DatasourceTimingStage(ctx, "app.redis_scan.encode_cursor")
	encoded, err := console.EncodeRedisCursor(next)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	return RedisKeyPage{Keys: keys, Cursor: encoded, Done: scanDone}, nil
}

func (a *App) GetRedisKeyMeta(id string, keys []string) (map[string]console.RedisKeyMetaItem, error) {
	if len(keys) == 0 {
		return map[string]console.RedisKeyMetaItem{}, nil
	}
	ds, ok := a.store.Get(id)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	if ds.Type != datasource.TypeRedis {
		return nil, errors.New("redis datasource required")
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.get_redis_key_meta", ds, "", console.ExecuteOptions{}, false)
	var err error
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "app.redis_key_meta.adapter_lookup")
	adapter, err := a.manager.AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	provider, ok := adapter.(console.RedisKeyMetaProvider)
	if !ok {
		err = console.ErrUnsupported
		return nil, err
	}
	// Resolve SecretRef-backed credentials; this direct adapter call bypasses the
	// manager dispatch path that normally resolves secrets.
	done = console.DatasourceTimingStage(ctx, "app.redis_key_meta.resolve_datasource")
	ds, err = a.manager.ResolveDatasource(ctx, ds)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	done = console.DatasourceTimingStage(ctx, "redis.key_meta")
	result, err := provider.GetKeyMeta(ctx, ds, keys)
	status := "ok"
	if err != nil {
		status = "error"
	}
	done(console.DatasourceTimingKV("status", status), console.DatasourceTimingKV("keys", len(keys)), console.DatasourceTimingKV("items", len(result)))
	return result, err
}

func (a *App) DescribeEntity(id, name, database, executionMode string) (console.DescribeResult, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return console.DescribeResult{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.describe_entity", ds, "", console.ExecuteOptions{}, false)
	var err error
	defer func() { finishTiming(err) }()

	entityName := strings.TrimSpace(name)
	cacheKey := entitySchemaCacheKey(ds)
	if supportsEntitySchemaCache(ds) && a.entityCache != nil {
		if cached, ok := a.entityCache.GetDescribe(cacheKey, entityName); ok {
			if !a.entityCache.ShouldRefresh(cacheKey, entitySchemaCacheRefreshInterval) {
				done := console.DatasourceTimingStage(ctx, "app.describe_cache")
				done(console.DatasourceTimingKV("status", "hit"))
				a.refreshEntityDescribeCacheAsync(ds, cacheKey, entityName)
				return cached, nil
			}
			result, describeErr := a.manager.DescribeEntity(ctx, ds, entityName)
			err = describeErr
			if err == nil {
				a.upsertEntityCacheDescribe(ds, cacheKey, entityName, result)
				return result, nil
			}
			err = nil
			return cached, nil
		}
	}

	result, describeErr := a.manager.DescribeEntity(ctx, ds, entityName)
	err = describeErr
	if err != nil {
		if supportsEntitySchemaCache(ds) && a.entityCache != nil {
			if cached, ok := a.entityCache.GetDescribe(cacheKey, entityName); ok {
				err = nil
				return cached, nil
			}
		}
		return console.DescribeResult{}, err
	}

	if supportsEntitySchemaCache(ds) && a.entityCache != nil {
		a.upsertEntityCacheDescribe(ds, cacheKey, entityName, result)
	}
	return result, nil
}

func (a *App) ExecuteStatement(id, statement, database, pagingToken string, pageSize int, executionMode string, approved bool, dynamoMaxReturnedRows, dynamoMaxPages, dynamoMaxEvaluatedItems int) (out console.QueryResult, err error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return console.QueryResult{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)

	normalizedStatement := normalizeStatement(statement)
	isElasticsearchIndicesMeta := ds.Type == datasource.TypeElasticsearch && isElasticsearchIndicesMetaRequest(normalizedStatement)
	cacheKey := entitySchemaCacheKey(ds)

	if isElasticsearchIndicesMeta && a.entityCache != nil {
		if entities, meta, ok := a.entityCache.GetEntities(cacheKey); ok && len(entities) > 0 {
			a.refreshEntitySchemaCacheAsync(ds, cacheKey)
			return buildCachedElasticsearchIndicesResult(entities, meta), nil
		}
	}

	execOpts := console.ExecuteOptions{
		PagingToken: pagingToken,
		PageSize:    pageSize,
		Bounds: console.ExecuteBounds{
			MaxReturnedRows:   dynamoMaxReturnedRows,
			MaxPages:          dynamoMaxPages,
			MaxEvaluatedItems: dynamoMaxEvaluatedItems,
		},
	}
	ctx := context.Background()
	finishTiming := func(error) {}
	ctx, finishTiming = a.beginDatasourceTiming(ctx, "app.execute_statement", ds, statement, execOpts, approved)
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "app.apply_dynamodb_caps")
	if err := a.applyDynamoDBRiskExecutionCaps(ds, statement, &execOpts); err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return console.QueryResult{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	var result console.QueryResult
	if approved {
		result, err = a.manager.ExecuteWithInteractiveApproval(ctx, ds, statement, execOpts)
	} else {
		result, err = a.manager.Execute(ctx, ds, statement, execOpts)
	}
	if err != nil {
		if riskInfo, ok := console.RiskInfoFromError(err); ok {
			return console.QueryResult{RiskInfo: &riskInfo}, nil
		}
		if isElasticsearchIndicesMeta && a.entityCache != nil {
			if entities, meta, ok := a.entityCache.GetEntities(cacheKey); ok && len(entities) > 0 {
				return buildCachedElasticsearchIndicesResult(entities, meta), nil
			}
		}
		return console.QueryResult{}, err
	}

	if isElasticsearchIndicesMeta && a.entityCache != nil {
		entities, meta := parseElasticsearchIndicesMetaRows(result.Rows)
		if len(entities) > 0 {
			a.upsertEntityCacheEntities(ds, cacheKey, entities, meta)
		}
	}
	return result, nil
}

func (a *App) applyDynamoDBRiskExecutionCaps(ds datasource.DataSource, statement string, opts *console.ExecuteOptions) error {
	if a == nil || a.riskEngine == nil || opts == nil || ds.Type != datasource.TypeDynamoDB || !opts.Bounds.Enabled() {
		return nil
	}
	policy := a.riskEngine.ProbePolicyForParsed(riskengine.ParseStatement(string(ds.Type), ds.ID, statement))
	return riskengine.ApplyDynamoDBExecutionPolicyCaps(ds, opts, policy)
}

func (a *App) ExplainStatement(id, statement string, analyze bool, database, executionMode string) (console.ExplainResult, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return console.ExplainResult{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx := context.Background()
	finishTiming := func(error) {}
	ctx, finishTiming = a.beginDatasourceTiming(ctx, "app.explain_statement", ds, statement, console.ExecuteOptions{}, false)
	statement = console.PrepareExplainStatement(statement, analyze, ds.Type)
	result, err := a.manager.Explain(ctx, ds, statement)
	finishTiming(err)
	return result, err
}

func (a *App) ListDatabases(id, pattern, executionMode string) ([]string, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.list_databases", ds, "", console.ExecuteOptions{}, false)
	result, err := a.manager.ListDatabases(ctx, ds, console.ListOptions{Pattern: pattern})
	finishTiming(err)
	return result, err
}

func (a *App) D1DeployMigrations(id string) (bool, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return false, errors.New("datasource not found")
	}
	if ds.Type != datasource.TypeD1 {
		return false, errors.New("d1 datasource required")
	}
	if !d1DatasourceSupportsDev(ds.Options) {
		return false, errors.New("dev mode is not supported for this datasource")
	}
	ctx, finishTiming := a.beginDatasourceTiming(context.Background(), "app.d1_deploy_migrations", ds, "", console.ExecuteOptions{}, false)
	var err error
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "app.d1_migrations.adapter_lookup")
	adapter, err := a.manager.AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return false, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	d1Adapter, ok := adapter.(*console.D1Adapter)
	if !ok {
		err = errors.New("d1 adapter not available")
		return false, err
	}
	done = console.DatasourceTimingStage(ctx, "d1.deploy_migrations")
	if err = d1Adapter.DeployMigrations(ctx, ds); err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return false, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	return true, nil
}

func (a *App) ExportQueryResult(fileName, content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", errors.New("export content is empty")
	}

	exportDir, err := resolveExportDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", err
	}

	safeName := sanitizeExportFileName(fileName)
	exportPath, err := nextExportFilePath(exportDir, safeName)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(exportPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return exportPath, nil
}

func resolveExportDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	downloads := filepath.Join(home, "Downloads")
	info, err := os.Stat(downloads)
	if err == nil && info.IsDir() {
		return downloads, nil
	}
	return home, nil
}

func sanitizeExportFileName(fileName string) string {
	trimmed := strings.TrimSpace(fileName)
	if trimmed == "" {
		return fmt.Sprintf("query-result-%d.json", time.Now().Unix())
	}
	base := filepath.Base(trimmed)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return fmt.Sprintf("query-result-%d.json", time.Now().Unix())
	}
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			if r < 32 {
				return -1
			}
			return r
		}
	}, base)
	clean = strings.TrimSpace(clean)
	if clean == "" || clean == "." || clean == ".." {
		return fmt.Sprintf("query-result-%d.json", time.Now().Unix())
	}
	return clean
}

func nextExportFilePath(dir, fileName string) (string, error) {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	candidate := filepath.Join(dir, fileName)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	}
	for i := 1; i <= 9999; i++ {
		next := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(next); errors.Is(err, os.ErrNotExist) {
			return next, nil
		}
	}
	return "", errors.New("unable to allocate export file path")
}

func supportsEntitySchemaCache(ds datasource.DataSource) bool {
	if ds.Type == datasource.TypeRedis {
		return false
	}
	if ds.Type == datasource.TypeD1 && isD1LocalExecutionMode(ds) {
		return false
	}
	return true
}

func entitySchemaCacheKey(ds datasource.DataSource) string {
	key := strings.TrimSpace(ds.ID)
	if key == "" {
		key = "unknown"
	}
	if ds.Type == datasource.TypeMongoDB {
		db := strings.ToLower(strings.TrimSpace(ds.Database))
		if db != "" {
			key = key + "::db::" + db
		}
	}
	if ds.Type == datasource.TypeD1 {
		key = key + "::mode::" + d1ExecutionModeForCache(ds)
	}
	return key
}

func d1ExecutionModeForCache(ds datasource.DataSource) string {
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(ds.Options, "executionMode")))
	if mode == "dev" || mode == "remote" {
		return mode
	}
	legacyMode := strings.ToLower(strings.TrimSpace(optionAnyString(ds.Options, "mode")))
	if legacyMode == "local" {
		return "dev"
	}
	return "remote"
}

func isD1LocalExecutionMode(ds datasource.DataSource) bool {
	if ds.Type != datasource.TypeD1 {
		return false
	}
	return d1ExecutionModeForCache(ds) == "dev"
}

func filterCachedEntityNames(items []string, pattern string) []string {
	normalized := normalizeCachedEntityNames(items)
	trimmedPattern := strings.ToLower(strings.TrimSpace(pattern))
	if trimmedPattern == "" {
		return normalized
	}
	out := make([]string, 0, len(normalized))
	for _, item := range normalized {
		if strings.Contains(strings.ToLower(item), trimmedPattern) {
			out = append(out, item)
		}
	}
	return out
}

func paginateCachedEntityNames(
	items []string,
	pattern, cursor string,
	limit int,
	details map[string]console.DescribeResult,
	kinds map[string]string,
) console.EntityPage {
	filtered := filterCachedEntityNames(items, pattern)
	if limit <= 0 {
		limit = 100
	}
	start := 0
	trimmedCursor := strings.TrimSpace(cursor)
	if trimmedCursor != "" {
		idx := sort.SearchStrings(filtered, trimmedCursor)
		if idx < len(filtered) && filtered[idx] == trimmedCursor {
			start = idx + 1
		} else {
			start = idx
		}
	}
	if start >= len(filtered) {
		return console.EntityPage{Items: []string{}, Cursor: "", Done: true}
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	pageItems := append([]string(nil), filtered[start:end]...)
	done := end >= len(filtered)
	nextCursor := ""
	if !done && len(pageItems) > 0 {
		nextCursor = pageItems[len(pageItems)-1]
	}
	page := console.EntityPage{Items: pageItems, Cursor: nextCursor, Done: done}
	if len(kinds) > 0 && len(pageItems) > 0 {
		pageKinds := make(map[string]string)
		for _, item := range pageItems {
			if k, ok := kinds[item]; ok {
				pageKinds[item] = k
			}
		}
		if len(pageKinds) > 0 {
			page.Kinds = pageKinds
		}
	}
	if len(details) == 0 || len(pageItems) == 0 {
		return page
	}
	page.Details = make(map[string]console.DescribeResult, len(pageItems))
	for _, item := range pageItems {
		if detail, ok := details[item]; ok {
			page.Details[item] = detail
		}
	}
	if len(page.Details) == 0 {
		page.Details = nil
	}
	return page
}

func (a *App) cachedDescribeDetailsForEntities(cacheKey string, entities []string) map[string]console.DescribeResult {
	if a.entityCache == nil || len(entities) == 0 {
		return nil
	}
	entry, ok := a.entityCache.GetEntry(cacheKey)
	if !ok || len(entry.Details) == 0 {
		return nil
	}
	details := make(map[string]console.DescribeResult, len(entities))
	for _, name := range entities {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}
		detail, exists := entry.Details[trimmedName]
		if !exists {
			continue
		}
		details[trimmedName] = detail
	}
	if len(details) == 0 {
		return nil
	}
	return details
}

func normalizeCachedEntityNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func normalizeStatement(statement string) string {
	fields := strings.Fields(strings.TrimSpace(statement))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func isElasticsearchIndicesMetaRequest(statement string) bool {
	return strings.EqualFold(statement, elasticsearchIndicesMetaStatement)
}

func parseElasticsearchIndicesMetaRows(rows []map[string]any) ([]string, map[string]console.ElasticsearchIndexMeta) {
	if len(rows) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(rows))
	meta := make(map[string]console.ElasticsearchIndexMeta, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(row["index"]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; !ok {
			names = append(names, name)
			seen[name] = struct{}{}
		}
		meta[name] = console.ElasticsearchIndexMeta{
			Health:    strings.TrimSpace(fmt.Sprint(row["health"])),
			StoreSize: strings.TrimSpace(fmt.Sprint(row["store.size"])),
		}
	}
	sort.Strings(names)
	return names, meta
}

func buildCachedElasticsearchIndicesResult(entities []string, meta map[string]console.ElasticsearchIndexMeta) console.QueryResult {
	names := normalizeCachedEntityNames(entities)
	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		m := meta[name]
		row := map[string]any{"index": name}
		if strings.TrimSpace(m.Health) != "" {
			row["health"] = m.Health
		}
		if strings.TrimSpace(m.StoreSize) != "" {
			row["store.size"] = m.StoreSize
		}
		rows = append(rows, row)
	}
	return console.QueryResult{
		Columns:   []string{"index", "health", "store.size"},
		Rows:      rows,
		RowCount:  int64(len(rows)),
		HasMore:   false,
		NextToken: "",
		PrevToken: "",
		ElapsedMs: 0,
	}
}

func (a *App) refreshEntitySchemaCacheAsync(ds datasource.DataSource, cacheKey string) {
	if a.entityCache == nil || !supportsEntitySchemaCache(ds) {
		return
	}
	if !a.entityCache.ShouldRefresh(cacheKey, entitySchemaCacheRefreshInterval) {
		return
	}
	if !a.entityCache.TryBeginRefresh(cacheKey) {
		return
	}
	go func() {
		defer a.entityCache.EndRefresh(cacheKey)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		if ds.Type == datasource.TypeElasticsearch {
			result, err := a.manager.ExecuteInternal(ctx, ds, elasticsearchIndicesMetaStatement, console.ExecuteOptions{})
			if err == nil {
				entities, meta := parseElasticsearchIndicesMetaRows(result.Rows)
				if len(entities) > 0 {
					a.upsertEntityCacheEntities(ds, cacheKey, entities, meta)
					return
				}
			}
		}
		items, err := a.manager.ListEntities(ctx, ds, console.ListOptions{})
		if err == nil && len(items) > 0 {
			var kinds map[string]string
			if datasourceSupportsKinds(ds.Type) {
				kinds = a.collectEntityKinds(ctx, ds)
			}
			a.upsertEntityCacheEntitiesWithKinds(ds, cacheKey, items, kinds, nil)
		}
	}()
}

func (a *App) refreshEntityDescribeCacheAsync(ds datasource.DataSource, cacheKey, entityName string) {
	if a.entityCache == nil || !supportsEntitySchemaCache(ds) {
		return
	}
	if !a.entityCache.ShouldRefresh(cacheKey, entitySchemaCacheRefreshInterval) {
		return
	}
	refreshKey := cacheKey + "::describe::" + entityName
	if !a.entityCache.TryBeginRefresh(refreshKey) {
		return
	}
	go func() {
		defer a.entityCache.EndRefresh(refreshKey)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		result, err := a.manager.DescribeEntity(ctx, ds, entityName)
		if err != nil {
			return
		}
		a.upsertEntityCacheDescribe(ds, cacheKey, entityName, result)
	}()
}

func (a *App) upsertEntityCacheEntities(ds datasource.DataSource, cacheKey string, items []string, meta map[string]console.ElasticsearchIndexMeta) {
	if a.entityCache == nil {
		return
	}
	if err := a.entityCache.UpsertEntities(cacheKey, items, meta); err != nil {
		return
	}
	a.syncSchemaKnowledgeAsync(ds, cacheKey)
}

func (a *App) upsertEntityCacheEntitiesWithKinds(ds datasource.DataSource, cacheKey string, items []string, kinds map[string]string, meta map[string]console.ElasticsearchIndexMeta) {
	if a.entityCache == nil {
		return
	}
	if err := a.entityCache.UpsertEntitiesWithKinds(cacheKey, items, kinds, meta); err != nil {
		return
	}
	a.syncSchemaKnowledgeAsync(ds, cacheKey)
}

func (a *App) upsertEntityCacheDescribe(ds datasource.DataSource, cacheKey, entityName string, result console.DescribeResult) {
	if a.entityCache == nil {
		return
	}
	if err := a.entityCache.UpsertDescribe(cacheKey, entityName, result); err != nil {
		return
	}
	a.syncSchemaKnowledgeAsync(ds, cacheKey)
}

func (a *App) syncSchemaKnowledgeAsync(ds datasource.DataSource, cacheKey string) {
	if a.entityCache == nil || a.schemaKB == nil {
		return
	}
	if !a.schemaKB.TryBegin(cacheKey) {
		return
	}
	go func() {
		defer a.schemaKB.End(cacheKey)
		entry, ok := a.entityCache.GetEntry(cacheKey)
		if !ok || len(entry.Entities) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = a.schemaKB.SyncFromCache(ctx, ds, cacheKey, entry)
	}()
}

func datasourceWithDatabaseOverride(ds datasource.DataSource, database string) datasource.DataSource {
	if ds.Type != datasource.TypeMongoDB {
		return ds
	}
	trimmed := strings.TrimSpace(database)
	if trimmed == "" {
		return ds
	}
	ds.Database = trimmed
	return ds
}

func datasourceWithD1ExecutionModeOverride(ds datasource.DataSource, executionMode string) datasource.DataSource {
	if ds.Type != datasource.TypeD1 {
		return ds
	}
	mode := strings.ToLower(strings.TrimSpace(executionMode))
	if mode == "dev" && !d1DatasourceSupportsDev(ds.Options) {
		mode = "remote"
	}
	if mode != "dev" && mode != "remote" {
		return ds
	}
	next := ds
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["executionMode"] = mode
	return next
}
