package console

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

var errRedisSentinelUnsupported = errors.New("redis sentinel not supported")

func parseClusterNodes(raw string) []redisClusterNode {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	nodes := make([]redisClusterNode, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		flags := strings.Split(fields[2], ",")
		if hasClusterFlag(flags, "fail") || hasClusterFlag(flags, "fail?") || hasClusterFlag(flags, "handshake") || hasClusterFlag(flags, "noaddr") {
			continue
		}
		addr := strings.Split(fields[1], "@")[0]
		role := "unknown"
		if hasClusterFlag(flags, "master") {
			role = "master"
		} else if hasClusterFlag(flags, "slave") || hasClusterFlag(flags, "replica") {
			role = "replica"
		}
		nodes = append(nodes, redisClusterNode{
			ID:   fields[0],
			Addr: addr,
			Role: role,
		})
	}
	return nodes
}

func hasClusterFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if flag == target {
			return true
		}
	}
	return false
}

func detectRedisMode(ctx context.Context, client *redis.Client, ds datasource.DataSource) (redisConnInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	role, err := client.Do(ctx, "ROLE").Result()
	if err != nil {
		return redisConnInfo{}, err
	}
	if roleName := redisRoleName(role); roleName == "sentinel" {
		return redisConnInfo{}, errRedisSentinelUnsupported
	}

	info, err := client.Info(ctx, "cluster").Result()
	if err == nil && redisInfoClusterEnabled(info) {
		nodesRaw, err := client.ClusterNodes(ctx).Result()
		if err != nil {
			return redisConnInfo{Mode: redisModeCluster, Nodes: redisNodesFromOptions(ds)}, nil
		}
		parsed := parseClusterNodes(nodesRaw)
		nodes := clusterAddrs(parsed)
		return redisConnInfo{Mode: redisModeCluster, Nodes: nodes}, nil
	}
	return redisConnInfo{Mode: redisModeStandalone}, nil
}

func redisRoleName(role any) string {
	values, ok := role.([]any)
	if !ok || len(values) == 0 {
		return ""
	}
	switch v := values[0].(type) {
	case string:
		return strings.ToLower(v)
	case []byte:
		return strings.ToLower(string(v))
	default:
		return strings.ToLower(fmt.Sprint(v))
	}
}

func redisInfoClusterEnabled(info string) bool {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cluster_enabled:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "cluster_enabled:"))
			return value == "1"
		}
	}
	return false
}

func clusterAddrs(nodes []redisClusterNode) []string {
	masters := make([]string, 0, len(nodes))
	others := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.Addr) == "" {
			continue
		}
		others = append(others, node.Addr)
		if node.Role == "master" {
			masters = append(masters, node.Addr)
		}
	}
	if len(masters) > 0 {
		return normalizeRedisNodes(masters)
	}
	return normalizeRedisNodes(others)
}

func redisNodesFromOptions(ds datasource.DataSource) []string {
	if ds.Options == nil {
		return nil
	}
	nodesRaw, ok := ds.Options["nodes"]
	if !ok {
		return nil
	}
	switch v := nodesRaw.(type) {
	case []any:
		nodes := make([]string, 0, len(v))
		for _, item := range v {
			node := strings.TrimSpace(fmt.Sprint(item))
			if node != "" {
				nodes = append(nodes, node)
			}
		}
		return nodes
	case []string:
		nodes := make([]string, 0, len(v))
		for _, node := range v {
			node = strings.TrimSpace(node)
			if node != "" {
				nodes = append(nodes, node)
			}
		}
		return nodes
	case string:
		return splitRedisNodes(v)
	}
	return nil
}

func splitRedisNodes(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ' ', '\t', '\n', ';':
			return true
		default:
			return false
		}
	})
	nodes := make([]string, 0, len(parts))
	for _, part := range parts {
		node := strings.TrimSpace(part)
		if node != "" {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func normalizeRedisNodes(nodes []string) []string {
	if len(nodes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(nodes))
	unique := make([]string, 0, len(nodes))
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		if node == "" {
			continue
		}
		node = stripRedisBusPort(node)
		if node == "" {
			continue
		}
		if _, ok := seen[node]; ok {
			continue
		}
		seen[node] = struct{}{}
		unique = append(unique, node)
	}
	sort.Strings(unique)
	return unique
}

func stripRedisBusPort(node string) string {
	node = strings.TrimSpace(node)
	if node == "" {
		return ""
	}
	if idx := strings.Index(node, "@"); idx != -1 {
		return strings.TrimSpace(node[:idx])
	}
	return node
}

func redisProbeAddr(ds datasource.DataSource) (string, error) {
	host := strings.TrimSpace(ds.Host)
	if host != "" && ds.Port != 0 {
		// DataSource.Host may already contain IPv6 brackets (e.g. "[::1]").
		// JoinHostPort expects the raw host part and will add brackets as needed.
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
		return net.JoinHostPort(host, fmt.Sprintf("%d", ds.Port)), nil
	}
	nodes := normalizeRedisNodes(redisNodesFromOptions(ds))
	if len(nodes) > 0 {
		return nodes[0], nil
	}
	return "", errors.New("redis address required")
}

func redisAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "noauth"),
		strings.Contains(msg, "wrongpass"),
		strings.Contains(msg, "authentication"),
		strings.Contains(msg, "auth failed"),
		strings.Contains(msg, "client sent auth"),
		strings.Contains(msg, "no password is set"):
		return true
	default:
		return false
	}
}
