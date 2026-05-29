package console

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/rediscmd"
)

var redisNewClient = func(opts *redis.Options) *redis.Client {
	return redis.NewClient(opts)
}

var redisDetectMode = detectRedisMode

func (r *RedisAdapter) clientFor(ctx context.Context, ds datasource.DataSource) (redis.UniversalClient, redisConnInfo, error) {
	fingerprint := redisFingerprint(ds)

	r.mu.Lock()
	if key, ok := r.byID[ds.ID]; ok && r.fingerprints[ds.ID] == fingerprint {
		if client, ok := r.clients[key]; ok {
			info := r.infoByID[ds.ID]
			r.mu.Unlock()
			return client, info, nil
		}
	}
	prevKey := r.byID[ds.ID]
	r.mu.Unlock()

	client, info, err := r.newRedisClient(ctx, ds)
	if err != nil {
		return nil, redisConnInfo{}, err
	}
	info.Nodes = normalizeRedisNodes(info.Nodes)
	key := redisKey(ds, info)

	r.mu.Lock()
	defer r.mu.Unlock()

	if prevKey != "" && (r.fingerprints[ds.ID] != fingerprint || prevKey != key) {
		if old, ok := r.clients[prevKey]; ok {
			_ = old.Close()
			delete(r.clients, prevKey)
		}
	}
	if cached, ok := r.clients[key]; ok {
		_ = client.Close()
		client = cached
	} else {
		r.clients[key] = client
	}
	r.byID[ds.ID] = key
	r.fingerprints[ds.ID] = fingerprint
	r.infoByID[ds.ID] = info
	return client, info, nil
}

func (r *RedisAdapter) newRedisClient(ctx context.Context, ds datasource.DataSource) (redis.UniversalClient, redisConnInfo, error) {
	addr, err := redisProbeAddr(ds)
	if err != nil {
		return nil, redisConnInfo{}, err
	}
	username := ds.Username
	password := ds.Password
	db := redisDB(ds.Database)
	forceStandalone := redisForceStandalone(ds)
	probe := redisNewClient(&redis.Options{Addr: addr, Username: username, Password: password, DB: db})
	info, err := redisDetectMode(ctx, probe, ds)
	if err != nil && password != "" && redisAuthError(err) {
		_ = probe.Close()
		password = ""
		probe = redisNewClient(&redis.Options{Addr: addr, Username: username, Password: password, DB: db})
		info, err = redisDetectMode(ctx, probe, ds)
	}
	if forceStandalone {
		return probe, redisConnInfo{Mode: redisModeStandalone}, nil
	}
	if err != nil {
		if errors.Is(err, errRedisSentinelUnsupported) {
			_ = probe.Close()
			return nil, redisConnInfo{}, err
		}
		info = redisConnInfo{Mode: redisModeStandalone}
	}
	if info.Mode == redisModeCluster {
		_ = probe.Close()
		nodes := info.Nodes
		if len(nodes) == 0 {
			nodes = redisNodesFromOptions(ds)
		}
		if len(nodes) == 0 {
			nodes = []string{addr}
		}
		info.Nodes = nodes
		client := redis.NewClusterClient(&redis.ClusterOptions{Addrs: nodes, Username: username, Password: password})
		return client, info, nil
	}
	return probe, info, nil
}

func parseRedisCommand(statement string) ([]any, error) {
	parts, err := rediscmd.Parse(statement)
	if err != nil {
		return nil, err
	}
	args := make([]any, len(parts))
	for i, part := range parts {
		args[i] = part
	}
	return args, nil
}

func redisArgsToAny(args []string) ([]any, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return nil, errors.New("redis command args required")
	}
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = arg
	}
	out[0] = strings.TrimSpace(args[0])
	return out, nil
}

func RedisCommandStatement(args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", errors.New("redis command args required")
	}
	parts := make([]string, len(args))
	for i, arg := range args {
		if i == 0 {
			arg = strings.TrimSpace(arg)
		}
		parts[i] = quoteRedisStatementArg(arg)
	}
	return strings.Join(parts, " "), nil
}

func quoteRedisStatementArg(arg string) string {
	if arg == "" {
		return strconv.Quote(arg)
	}
	for _, ch := range arg {
		if ch <= ' ' || ch == '"' || ch == '\\' {
			return strconv.Quote(arg)
		}
	}
	return arg
}

func redisDB(value string) int {
	if value == "" {
		return 0
	}
	db, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return db
}

func redisForceStandalone(ds datasource.DataSource) bool {
	if ds.Options == nil {
		return false
	}
	raw, ok := ds.Options["__forceStandalone"]
	if !ok {
		return false
	}
	return anyBool(raw)
}

func anyBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		switch normalized {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	default:
		return false
	}
}

func redisFingerprint(ds datasource.DataSource) string {
	builder := strings.Builder{}
	builder.WriteString(ds.Host)
	builder.WriteString(":")
	builder.WriteString(strconv.Itoa(ds.Port))
	builder.WriteString("|")
	builder.WriteString(ds.Database)
	builder.WriteString("|")
	builder.WriteString(ds.Username)
	builder.WriteString("|")
	builder.WriteString(ds.Password)
	builder.WriteString("|")
	nodes := normalizeRedisNodes(redisNodesFromOptions(ds))
	if len(nodes) > 0 {
		builder.WriteString(strings.Join(nodes, ","))
		builder.WriteString("|")
	}
	return builder.String()
}

func redisKey(ds datasource.DataSource, info redisConnInfo) string {
	builder := strings.Builder{}
	builder.WriteString(ds.ID)
	builder.WriteString("|")
	builder.WriteString(string(info.Mode))
	builder.WriteString("|")
	if info.Mode == redisModeCluster {
		nodes := normalizeRedisNodes(info.Nodes)
		builder.WriteString(strings.Join(nodes, ","))
		builder.WriteString("|")
	} else {
		builder.WriteString(ds.Host)
		builder.WriteString(":")
		builder.WriteString(strconv.Itoa(ds.Port))
		builder.WriteString("|")
		builder.WriteString(ds.Database)
		builder.WriteString("|")
	}
	builder.WriteString(ds.Username)
	builder.WriteString("|")
	builder.WriteString(ds.Password)
	return builder.String()
}
