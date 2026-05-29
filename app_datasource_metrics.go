package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
)

type DatasourceMetrics struct {
	DatasourceID   string                    `json:"datasourceId"`
	DatasourceType datasource.DataSourceType `json:"datasourceType"`
	CollectedAt    int64                     `json:"collectedAt"`
	Node           string                    `json:"node,omitempty"`
	Nodes          []string                  `json:"nodes,omitempty"`

	CPUAvailable     bool    `json:"cpuAvailable"`
	CPUPercent       float64 `json:"cpuPercent,omitempty"`
	CPUUserSeconds   float64 `json:"cpuUserSeconds,omitempty"`
	CPUSystemSeconds float64 `json:"cpuSystemSeconds,omitempty"`

	MemoryAvailable  bool   `json:"memoryAvailable"`
	MemoryUsedBytes  int64  `json:"memoryUsedBytes,omitempty"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes,omitempty"`
	MemoryUsedText   string `json:"memoryUsedText,omitempty"`
	MemoryTotalText  string `json:"memoryTotalText,omitempty"`

	Warnings []string       `json:"warnings,omitempty"`
	Raw      map[string]any `json:"raw,omitempty"`
}

func (a *App) GetDatasourceMetrics(id string) (DatasourceMetrics, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return DatasourceMetrics{}, errors.New("datasource not found")
	}
	return a.collectDatasourceMetrics(context.Background(), ds, ""), nil
}

func (a *App) GetDatasourceMetricsByNode(id string, node string) (DatasourceMetrics, error) {
	ds, ok := a.store.Get(id)
	if !ok {
		return DatasourceMetrics{}, errors.New("datasource not found")
	}
	return a.collectDatasourceMetrics(context.Background(), ds, node), nil
}

func (a *App) collectDatasourceMetrics(ctx context.Context, ds datasource.DataSource, redisNode string) DatasourceMetrics {
	metrics := DatasourceMetrics{
		DatasourceID:   ds.ID,
		DatasourceType: ds.Type,
		CollectedAt:    time.Now().UnixMilli(),
		Warnings:       make([]string, 0, 4),
		Raw:            map[string]any{},
	}

	appendWarning := func(message string) {
		msg := strings.TrimSpace(message)
		if msg == "" {
			return
		}
		metrics.Warnings = append(metrics.Warnings, msg)
	}

	switch ds.Type {
	case datasource.TypeRedis:
		a.collectRedisMetrics(ctx, ds, redisNode, &metrics, appendWarning)
	case datasource.TypeElasticsearch:
		a.collectElasticsearchMetrics(ctx, ds, &metrics, appendWarning)
	case datasource.TypeMongoDB:
		a.collectMongoMetrics(ctx, ds, &metrics, appendWarning)
	case datasource.TypeMySQL:
		a.collectMySQLMetrics(ctx, ds, &metrics, appendWarning)
	case datasource.TypePostgreSQL:
		a.collectPostgresMetrics(ctx, ds, &metrics, appendWarning)
	default:
		appendWarning(fmt.Sprintf("metrics collection is not implemented for datasource type %q", ds.Type))
	}

	if metrics.MemoryAvailable {
		metrics.MemoryUsedText = formatMetricBytes(metrics.MemoryUsedBytes)
		if metrics.MemoryTotalBytes > 0 {
			metrics.MemoryTotalText = formatMetricBytes(metrics.MemoryTotalBytes)
		}
	}

	if len(metrics.Raw) == 0 {
		metrics.Raw = nil
	}
	if len(metrics.Warnings) == 0 {
		metrics.Warnings = nil
	}
	return metrics
}

func (a *App) collectRedisMetrics(ctx context.Context, ds datasource.DataSource, requestedNode string, metrics *DatasourceMetrics, warn func(string)) {
	requestedNode = normalizeRedisNodeAddress(requestedNode)
	explicitNodeRequested := requestedNode != ""

	clusterNodes := a.loadRedisClusterNodes(ctx, ds)
	if len(clusterNodes) == 0 {
		clusterNodes = redisNodesFromDatasourceOptions(ds)
	}
	if len(clusterNodes) > 0 {
		metrics.Nodes = clusterNodes
		if explicitNodeRequested && !containsRedisNode(clusterNodes, requestedNode) {
			metrics.Node = requestedNode
			warn("redis node metrics unavailable: requested node not found in cluster topology")
			return
		}
		selectedNode := selectRedisMetricsNode(requestedNode, clusterNodes)
		if selectedNode != "" {
			metrics.Node = selectedNode
			metricsDS, err := datasourceWithRedisNode(ds, selectedNode)
			if err == nil {
				a.collectRedisMetricsCore(ctx, metricsDS, selectedNode, metrics, warn)
				if explicitNodeRequested || metrics.MemoryAvailable || metrics.CPUAvailable {
					return
				}
				warn("redis node metrics unavailable on selected node; retrying via datasource connection")
				metrics.Node = ""
			} else {
				warn("redis node metrics unavailable: " + err.Error())
				if explicitNodeRequested {
					return
				}
				metrics.Node = ""
			}
		}
	}

	if explicitNodeRequested {
		metrics.Node = requestedNode
		metricsDS, err := datasourceWithRedisNode(ds, requestedNode)
		if err != nil {
			warn("redis node metrics unavailable: " + err.Error())
			return
		}
		warn("redis cluster topology unavailable: collecting metrics directly from requested node")
		a.collectRedisMetricsCore(ctx, metricsDS, requestedNode, metrics, warn)
		return
	}

	a.collectRedisMetricsCore(ctx, ds, "", metrics, warn)
}

func (a *App) collectRedisMetricsCore(ctx context.Context, ds datasource.DataSource, selectedNode string, metrics *DatasourceMetrics, warn func(string)) {
	memResult, err := a.manager.ExecuteInternal(ctx, ds, "INFO memory", console.ExecuteOptions{})
	if err != nil {
		warn("redis memory metrics unavailable: " + err.Error())
	} else if raw, ok := queryResultText(memResult); ok {
		info := parseRedisInfo(raw)
		if used, ok := parseInt64(info["used_memory"]); ok {
			metrics.MemoryUsedBytes = used
			metrics.MemoryAvailable = true
		}
		if total, ok := parseInt64(info["maxmemory"]); ok && total > 0 {
			metrics.MemoryTotalBytes = total
		} else if total, ok := parseInt64(info["total_system_memory"]); ok && total > 0 {
			metrics.MemoryTotalBytes = total
		}
		if selectedNode != "" {
			info["node"] = selectedNode
		}
		metrics.Raw["redis_memory"] = info
	} else {
		warn("redis memory metrics unavailable: INFO memory returned no payload")
	}

	cpuResult, err := a.manager.ExecuteInternal(ctx, ds, "INFO cpu", console.ExecuteOptions{})
	if err != nil {
		warn("redis cpu metrics unavailable: " + err.Error())
		return
	}
	raw, ok := queryResultText(cpuResult)
	if !ok {
		warn("redis cpu metrics unavailable: INFO cpu returned no payload")
		return
	}
	info := parseRedisInfo(raw)
	if user, ok := parseFloat64(info["used_cpu_user"]); ok {
		metrics.CPUUserSeconds = user
		metrics.CPUAvailable = true
	}
	if system, ok := parseFloat64(info["used_cpu_sys"]); ok {
		metrics.CPUSystemSeconds = system
		metrics.CPUAvailable = true
	}
	if selectedNode != "" {
		info["node"] = selectedNode
	}
	metrics.Raw["redis_cpu"] = info
}

func (a *App) collectElasticsearchMetrics(ctx context.Context, ds datasource.DataSource, metrics *DatasourceMetrics, warn func(string)) {
	statement := "GET /_nodes/stats/process,jvm?filter_path=nodes.*.name,nodes.*.process.cpu.percent,nodes.*.jvm.mem.heap_used_in_bytes,nodes.*.jvm.mem.non_heap_used_in_bytes,nodes.*.jvm.mem.heap_max_in_bytes"
	result, err := a.manager.ExecuteInternal(ctx, ds, statement, console.ExecuteOptions{})
	if err != nil {
		warn("elasticsearch metrics unavailable: " + err.Error())
		return
	}
	row, ok := firstRow(result)
	if !ok {
		warn("elasticsearch metrics unavailable: empty response")
		return
	}
	nodes, ok := anyMap(row["nodes"])
	if !ok || len(nodes) == 0 {
		warn("elasticsearch metrics unavailable: missing nodes payload")
		return
	}

	var (
		cpuSum     float64
		cpuCount   float64
		memoryUsed int64
		memoryMax  int64
	)
	for _, nodeAny := range nodes {
		node, ok := anyMap(nodeAny)
		if !ok {
			continue
		}
		if cpu, ok := nestedNumber(node, "process", "cpu", "percent"); ok {
			cpuSum += cpu
			cpuCount++
		}
		heapUsed, hasHeapUsed := nestedInt64(node, "jvm", "mem", "heap_used_in_bytes")
		nonHeapUsed, hasNonHeapUsed := nestedInt64(node, "jvm", "mem", "non_heap_used_in_bytes")
		if hasHeapUsed || hasNonHeapUsed {
			memoryUsed += heapUsed + nonHeapUsed
		}
		if heapMax, ok := nestedInt64(node, "jvm", "mem", "heap_max_in_bytes"); ok {
			memoryMax += heapMax
		}
	}

	if cpuCount > 0 {
		metrics.CPUPercent = cpuSum / cpuCount
		metrics.CPUAvailable = true
	}
	if memoryUsed > 0 {
		metrics.MemoryUsedBytes = memoryUsed
		metrics.MemoryAvailable = true
	}
	if memoryMax > 0 {
		metrics.MemoryTotalBytes = memoryMax
	}
	metrics.Raw["elasticsearch_nodes"] = nodes
}

func (a *App) collectMongoMetrics(ctx context.Context, ds datasource.DataSource, metrics *DatasourceMetrics, warn func(string)) {
	result, err := a.manager.ExecuteInternal(ctx, ds, "db.serverStatus()", console.ExecuteOptions{})
	if err != nil {
		warn("mongodb metrics unavailable: " + err.Error())
		return
	}
	row, ok := firstRow(result)
	if !ok {
		warn("mongodb metrics unavailable: empty response")
		return
	}

	if residentMB, ok := nestedNumber(row, "mem", "resident"); ok && residentMB > 0 {
		metrics.MemoryUsedBytes = int64(math.Round(residentMB * 1024 * 1024))
		metrics.MemoryAvailable = true
	}
	if virtualMB, ok := nestedNumber(row, "mem", "virtual"); ok && virtualMB > 0 {
		metrics.MemoryTotalBytes = int64(math.Round(virtualMB * 1024 * 1024))
	}
	if userUS, ok := nestedNumber(row, "extra_info", "user_time_us"); ok && userUS >= 0 {
		metrics.CPUUserSeconds = userUS / 1_000_000
		metrics.CPUAvailable = true
	}
	if systemUS, ok := nestedNumber(row, "extra_info", "system_time_us"); ok && systemUS >= 0 {
		metrics.CPUSystemSeconds = systemUS / 1_000_000
		metrics.CPUAvailable = true
	}
	metrics.Raw["mongodb_server_status"] = row
}

func (a *App) collectMySQLMetrics(ctx context.Context, ds datasource.DataSource, metrics *DatasourceMetrics, warn func(string)) {
	result, err := a.manager.ExecuteInternal(ctx, ds, "SELECT COALESCE(SUM(CURRENT_NUMBER_OF_BYTES_USED),0) AS memory_used_bytes FROM performance_schema.memory_summary_global_by_event_name", console.ExecuteOptions{})
	if err == nil {
		if row, ok := firstRow(result); ok {
			if memoryUsed, ok := rowNumber(row, "memory_used_bytes"); ok {
				metrics.MemoryUsedBytes = int64(memoryUsed)
				metrics.MemoryAvailable = metrics.MemoryUsedBytes > 0
			}
		}
	} else {
		warn("mysql used memory metrics unavailable: " + err.Error())
	}

	totalResult, err := a.manager.ExecuteInternal(ctx, ds, "SELECT @@innodb_buffer_pool_size AS memory_total_bytes", console.ExecuteOptions{})
	if err == nil {
		if row, ok := firstRow(totalResult); ok {
			if memoryTotal, ok := rowNumber(row, "memory_total_bytes", "@@innodb_buffer_pool_size"); ok {
				metrics.MemoryTotalBytes = int64(memoryTotal)
				if metrics.MemoryTotalBytes > 0 {
					metrics.MemoryAvailable = true
				}
			}
		}
	} else {
		warn("mysql total memory metrics unavailable: " + err.Error())
	}

	warn("mysql cpu percent is not available via standard SQL without extra instrumentation")
}

func (a *App) collectPostgresMetrics(ctx context.Context, ds datasource.DataSource, metrics *DatasourceMetrics, warn func(string)) {
	usedResult, err := a.manager.ExecuteInternal(ctx, ds, "SELECT COALESCE(SUM(total_bytes),0) AS memory_used_bytes FROM pg_backend_memory_contexts", console.ExecuteOptions{})
	if err == nil {
		if row, ok := firstRow(usedResult); ok {
			if used, ok := rowNumber(row, "memory_used_bytes"); ok {
				metrics.MemoryUsedBytes = int64(used)
				metrics.MemoryAvailable = metrics.MemoryUsedBytes > 0
			}
		}
	} else {
		warn("postgres memory_used metrics unavailable: " + err.Error())
	}

	totalResult, err := a.manager.ExecuteInternal(ctx, ds, "SELECT pg_size_bytes(current_setting('shared_buffers')) AS memory_total_bytes", console.ExecuteOptions{})
	if err == nil {
		if row, ok := firstRow(totalResult); ok {
			if total, ok := rowNumber(row, "memory_total_bytes"); ok {
				metrics.MemoryTotalBytes = int64(total)
				if metrics.MemoryTotalBytes > 0 {
					metrics.MemoryAvailable = true
				}
			}
		}
	} else {
		warn("postgres memory_total metrics unavailable: " + err.Error())
	}

	cpuResult, err := a.manager.ExecuteInternal(ctx, ds, "SELECT COALESCE(SUM(user_time + system_time),0) AS cpu_seconds FROM pg_stat_kcache", console.ExecuteOptions{})
	if err == nil {
		if row, ok := firstRow(cpuResult); ok {
			if seconds, ok := rowNumber(row, "cpu_seconds"); ok {
				metrics.CPUUserSeconds = seconds
				metrics.CPUAvailable = seconds > 0
			}
		}
	} else {
		warn("postgres cpu metrics require pg_stat_kcache extension: " + err.Error())
	}
}

func (a *App) loadRedisClusterNodes(ctx context.Context, ds datasource.DataSource) []string {
	result, err := a.manager.ExecuteInternal(ctx, ds, "CLUSTER NODES", console.ExecuteOptions{})
	if err != nil {
		return nil
	}
	raw, ok := queryResultText(result)
	if !ok {
		return nil
	}
	return parseRedisClusterNodes(raw)
}

func parseRedisClusterNodes(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	nodes := make([]string, 0, len(lines))
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
		if hasRedisClusterFlag(flags, "fail") || hasRedisClusterFlag(flags, "fail?") || hasRedisClusterFlag(flags, "handshake") || hasRedisClusterFlag(flags, "noaddr") {
			continue
		}
		addr := normalizeRedisNodeAddress(fields[1])
		if addr != "" {
			nodes = append(nodes, addr)
		}
	}
	return uniqueRedisNodes(nodes)
}

func hasRedisClusterFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if strings.TrimSpace(flag) == target {
			return true
		}
	}
	return false
}

func redisNodesFromDatasourceOptions(ds datasource.DataSource) []string {
	if ds.Options == nil {
		return nil
	}
	raw, ok := ds.Options["nodes"]
	if !ok {
		return nil
	}
	nodes := make([]string, 0, 4)
	switch typed := raw.(type) {
	case []any:
		for _, node := range typed {
			addr := normalizeRedisNodeAddress(fmt.Sprint(node))
			if addr != "" {
				nodes = append(nodes, addr)
			}
		}
	case []string:
		for _, node := range typed {
			addr := normalizeRedisNodeAddress(node)
			if addr != "" {
				nodes = append(nodes, addr)
			}
		}
	case string:
		parts := strings.FieldsFunc(typed, func(r rune) bool {
			switch r {
			case ',', ';', '\n', '\r', '\t', ' ':
				return true
			default:
				return false
			}
		})
		for _, part := range parts {
			addr := normalizeRedisNodeAddress(part)
			if addr != "" {
				nodes = append(nodes, addr)
			}
		}
	}
	return uniqueRedisNodes(nodes)
}

func selectRedisMetricsNode(requestedNode string, nodes []string) string {
	normalized := normalizeRedisNodeAddress(requestedNode)
	if normalized != "" {
		for _, node := range nodes {
			if node == normalized {
				return node
			}
		}
	}
	if len(nodes) == 0 {
		return normalized
	}
	return nodes[0]
}

func datasourceWithRedisNode(ds datasource.DataSource, node string) (datasource.DataSource, error) {
	addr := normalizeRedisNodeAddress(node)
	host, port, err := redisNodeHostPort(addr)
	if err != nil {
		return datasource.DataSource{}, err
	}
	next := ds
	next.ID = fmt.Sprintf("%s|metrics|%s", ds.ID, addr)
	next.Host = host
	next.Port = port
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["__forceStandalone"] = true
	delete(next.Options, "nodes")
	return next, nil
}

func copyDatasourceOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(options)+1)
	for key, value := range options {
		out[key] = value
	}
	return out
}

func redisNodeHostPort(node string) (string, int, error) {
	node = strings.TrimSpace(node)
	if node == "" {
		return "", 0, errors.New("redis node address is required")
	}
	host, portText, err := net.SplitHostPort(node)
	if err != nil {
		idx := strings.LastIndex(node, ":")
		if idx <= 0 || idx+1 >= len(node) {
			return "", 0, fmt.Errorf("invalid redis node address %q", node)
		}
		host = strings.TrimSpace(node[:idx])
		portText = strings.TrimSpace(node[idx+1:])
	}
	port, convErr := strconv.Atoi(portText)
	if convErr != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid redis node address %q", node)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", 0, fmt.Errorf("invalid redis node address %q", node)
	}
	return host, port, nil
}

func normalizeRedisNodeAddress(value string) string {
	node := strings.TrimSpace(value)
	if node == "" {
		return ""
	}
	if idx := strings.Index(node, "@"); idx >= 0 {
		node = strings.TrimSpace(node[:idx])
	}
	return node
}

func uniqueRedisNodes(nodes []string) []string {
	if len(nodes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(nodes))
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		normalized := normalizeRedisNodeAddress(node)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func containsRedisNode(nodes []string, target string) bool {
	target = normalizeRedisNodeAddress(target)
	if target == "" {
		return false
	}
	for _, node := range nodes {
		if normalizeRedisNodeAddress(node) == target {
			return true
		}
	}
	return false
}

func firstRow(result console.QueryResult) (map[string]any, bool) {
	if len(result.Rows) == 0 || result.Rows[0] == nil {
		return nil, false
	}
	return result.Rows[0], true
}

func queryResultText(result console.QueryResult) (string, bool) {
	row, ok := firstRow(result)
	if !ok {
		return "", false
	}
	if value, ok := row["result"]; ok {
		switch typed := value.(type) {
		case string:
			return typed, true
		case []byte:
			return string(typed), true
		default:
			return fmt.Sprint(typed), true
		}
	}
	return "", false
}

func parseRedisInfo(raw string) map[string]string {
	out := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func parseInt64(value string) (int64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseFloat64(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func rowNumber(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := row[key]
		if !ok {
			continue
		}
		if number, ok := anyNumber(value); ok {
			return number, true
		}
	}
	return 0, false
}

func anyNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case json.Number:
		if number, err := typed.Float64(); err == nil {
			return number, true
		}
	case string:
		if number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			return number, true
		}
	case []byte:
		if number, err := strconv.ParseFloat(strings.TrimSpace(string(typed)), 64); err == nil {
			return number, true
		}
	}
	return 0, false
}

func anyMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func nestedNumber(root map[string]any, path ...string) (float64, bool) {
	value, ok := nestedValue(root, path...)
	if !ok {
		return 0, false
	}
	return anyNumber(value)
}

func nestedInt64(root map[string]any, path ...string) (int64, bool) {
	number, ok := nestedNumber(root, path...)
	if !ok {
		return 0, false
	}
	return int64(number), true
}

func nestedValue(root map[string]any, path ...string) (any, bool) {
	var current any = root
	for _, key := range path {
		node, ok := anyMap(current)
		if !ok {
			return nil, false
		}
		next, ok := node[key]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func formatMetricBytes(value int64) string {
	if value < 0 {
		return ""
	}
	const unit = 1024.0
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	f := float64(value)
	for _, suffix := range suffixes {
		f = f / unit
		if f < unit {
			if f >= 100 {
				return fmt.Sprintf("%.0f %s", f, suffix)
			}
			if f >= 10 {
				return fmt.Sprintf("%.1f %s", f, suffix)
			}
			return fmt.Sprintf("%.2f %s", f, suffix)
		}
	}
	return fmt.Sprintf("%.2f PB", f)
}
