package console

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

type RedisAdapter struct {
	mu           sync.Mutex
	clients      map[string]redis.UniversalClient
	byID         map[string]string
	fingerprints map[string]string
	infoByID     map[string]redisConnInfo
}

func NewRedisAdapter() *RedisAdapter {
	return &RedisAdapter{
		clients:      make(map[string]redis.UniversalClient),
		byID:         make(map[string]string),
		fingerprints: make(map[string]string),
		infoByID:     make(map[string]redisConnInfo),
	}
}

func (r *RedisAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	client, _, err := r.clientFor(ctx, ds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.Ping(ctx).Err()
}

func (r *RedisAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	client, info, err := r.clientFor(ctx, ds)
	if err != nil {
		return nil, err
	}
	pattern := opts.Pattern
	if pattern == "" {
		pattern = "*"
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}

	if info.Mode == redisModeCluster {
		clusterClient, ok := client.(*redis.ClusterClient)
		if !ok {
			return nil, errors.New("cluster client not available")
		}
		acc := newRedisClusterScanAccumulator()
		err := clusterClient.ForEachMaster(ctx, func(ctx context.Context, client *redis.Client) error {
			var cursor uint64
			for {
				batch, next, err := client.Scan(ctx, cursor, pattern, int64(limit)).Result()
				if err != nil {
					return err
				}
				count := acc.addKeys(batch)
				cursor = next
				if cursor == 0 || count >= limit {
					break
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return acc.keyList(), nil
	}

	var cursor uint64
	var results []string
	for {
		batch, next, err := client.Scan(ctx, cursor, pattern, int64(limit)).Result()
		if err != nil {
			return nil, err
		}
		results = append(results, batch...)
		cursor = next
		if cursor == 0 || len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (r *RedisAdapter) ScanKeys(ctx context.Context, ds datasource.DataSource, pattern string, cursor RedisScanCursor) ([]string, RedisScanCursor, bool, error) {
	client, info, err := r.clientFor(ctx, ds)
	if err != nil {
		return nil, RedisScanCursor{}, false, err
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "*"
	}
	count := redisScanCount(pattern)
	maxLoops := 1
	if redisPrefixPattern(pattern) {
		maxLoops = 20
	}

	if info.Mode == redisModeCluster {
		clusterClient, ok := client.(*redis.ClusterClient)
		if !ok {
			return nil, RedisScanCursor{}, false, errors.New("cluster client not available")
		}
		acc := newRedisClusterScanAccumulator()
		err := clusterClient.ForEachMaster(ctx, func(ctx context.Context, client *redis.Client) error {
			addr := client.Options().Addr
			var start uint64
			if cursor.Cursors != nil {
				if value, ok := cursor.Cursors[addr]; ok {
					start = value
				}
			}
			cursorValue := start
			for i := 0; i < maxLoops; i++ {
				batch, next, err := client.Scan(ctx, cursorValue, pattern, count).Result()
				if err != nil {
					return err
				}
				acc.addKeys(batch)
				cursorValue = next
				if cursorValue == 0 {
					break
				}
			}
			acc.setCursor(addr, cursorValue)
			return nil
		})
		if err != nil {
			return nil, RedisScanCursor{}, false, err
		}
		result := acc.keysSorted()
		nextCursor := acc.cursor()
		return result, nextCursor, redisCursorDone(nextCursor), nil
	}

	start := cursor.Cursor
	cursorValue := start
	var batch []string
	for i := 0; i < maxLoops; i++ {
		step, next, err := client.Scan(ctx, cursorValue, pattern, count).Result()
		if err != nil {
			return nil, RedisScanCursor{}, false, err
		}
		batch = append(batch, step...)
		cursorValue = next
		if cursorValue == 0 {
			break
		}
	}
	nextCursor := RedisScanCursor{Cursor: cursorValue}
	return batch, nextCursor, cursorValue == 0, nil
}

type redisClusterScanAccumulator struct {
	mu      sync.Mutex
	keys    map[string]struct{}
	cursors map[string]uint64
}

func newRedisClusterScanAccumulator() *redisClusterScanAccumulator {
	return &redisClusterScanAccumulator{
		keys:    make(map[string]struct{}),
		cursors: make(map[string]uint64),
	}
}

func (a *redisClusterScanAccumulator) addKeys(keys []string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, key := range keys {
		a.keys[key] = struct{}{}
	}
	return len(a.keys)
}

func (a *redisClusterScanAccumulator) setCursor(addr string, cursor uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cursors[addr] = cursor
}

func (a *redisClusterScanAccumulator) keyList() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.keys))
	for key := range a.keys {
		out = append(out, key)
	}
	return out
}

func (a *redisClusterScanAccumulator) keysSorted() []string {
	out := a.keyList()
	sort.Strings(out)
	return out
}

func (a *redisClusterScanAccumulator) cursor() RedisScanCursor {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := RedisScanCursor{Cursors: make(map[string]uint64, len(a.cursors))}
	for addr, cursor := range a.cursors {
		out.Cursors[addr] = cursor
	}
	return out
}

func (r *RedisAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	client, _, err := r.clientFor(ctx, ds)
	if err != nil {
		return DescribeResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	keyType, err := client.Type(ctx, name).Result()
	if err != nil {
		return DescribeResult{}, err
	}

	details := []DetailItem{{Label: "Type", Value: keyType}}
	var size int64

	ttl, err := client.TTL(ctx, name).Result()
	if err == nil {
		details = append(details, DetailItem{Label: "TTL", Value: formatTTL(ttl)})
	}

	if mem, err := client.Do(ctx, "MEMORY", "USAGE", name).Int64(); err == nil {
		details = append(details, DetailItem{Label: "Memory Usage", Value: mem})
	}

	if enc, err := client.Do(ctx, "OBJECT", "ENCODING", name).Result(); err == nil && enc != nil {
		details = append(details, DetailItem{Label: "Encoding", Value: fmt.Sprint(enc)})
	}

	if value, ok := redisKeySize(ctx, client, keyType, name); ok {
		size = value
		details = append(details, DetailItem{Label: "Size", Value: size})
	}

	result := DescribeResult{Details: details}
	if preview, err := redisPreview(ctx, client, keyType, name, size); err == nil && preview != nil {
		result.Preview = preview
	}
	return result, nil
}

func (r *RedisAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	args, err := parseRedisCommand(statement)
	if err != nil {
		return QueryResult{}, err
	}
	argv := make([]string, len(args))
	for i, arg := range args {
		argv[i] = fmt.Sprint(arg)
	}
	return r.ExecuteRedisCommand(ctx, ds, argv, opts)
}

func (r *RedisAdapter) ExecuteRedisCommand(ctx context.Context, ds datasource.DataSource, args []string, opts ExecuteOptions) (QueryResult, error) {
	client, _, err := r.clientFor(ctx, ds)
	if err != nil {
		return QueryResult{}, err
	}
	_ = opts
	argv, err := redisArgsToAny(args)
	if err != nil {
		return QueryResult{}, err
	}

	start := time.Now()
	result, err := client.Do(ctx, argv...).Result()
	if err != nil {
		return QueryResult{}, err
	}
	result = normalizeRedisResultForJSON(result)

	row := map[string]any{"result": result}
	return QueryResult{Columns: []string{"result"}, Rows: []map[string]any{row}, RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
}

func (r *RedisAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	return ExplainResult{}, ErrUnsupported
}

func normalizeRedisResultForJSON(value any) any {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []byte:
		return string(typed)
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Map:
		iter := rv.MapRange()
		out := make(map[string]any, rv.Len())
		for iter.Next() {
			key := iter.Key().Interface()
			val := iter.Value().Interface()
			// Convert []byte keys to string (common with RESP3 maps)
			var keyStr string
			if keyBytes, ok := key.([]byte); ok {
				keyStr = string(keyBytes)
			} else {
				keyStr = fmt.Sprint(key)
			}
			out[keyStr] = normalizeRedisResultForJSON(val)
		}
		return out
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, normalizeRedisResultForJSON(rv.Index(i).Interface()))
		}
		return out
	default:
		return value
	}
}
