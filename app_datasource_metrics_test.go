package main

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
)

type metricsStubAdapter struct {
	execute func(ds datasource.DataSource, statement string) (console.QueryResult, error)
}

func (m metricsStubAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	_ = ctx
	_ = ds
	return nil
}

func (m metricsStubAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts console.ListOptions) ([]string, error) {
	_ = ctx
	_ = ds
	_ = opts
	return nil, nil
}

func (m metricsStubAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error) {
	_ = ctx
	_ = ds
	_ = name
	return console.DescribeResult{}, nil
}

func (m metricsStubAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) (console.QueryResult, error) {
	_ = ctx
	_ = opts
	if m.execute == nil {
		return console.QueryResult{}, errors.New("stub execute not configured")
	}
	return m.execute(ds, statement)
}

func (m metricsStubAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (console.ExplainResult, error) {
	_ = ctx
	_ = ds
	_ = statement
	return console.ExplainResult{}, console.ErrUnsupported
}

func TestGetDatasourceMetrics_UnknownDatasource(t *testing.T) {
	app := &App{
		store: datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json")),
	}
	_, err := app.GetDatasourceMetrics("missing")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestGetDatasourceMetrics_Redis_ParsesMemoryAndCPU(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-metrics",
		Type: datasource.TypeRedis,
		Host: "127.0.0.1",
		Port: 6379,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			_ = ds
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "INFO MEMORY":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": "# Memory\nused_memory:1048576\nmaxmemory:2097152\n",
						},
					},
				}, nil
			case "INFO CPU":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": "# CPU\nused_cpu_user:11.25\nused_cpu_sys:4.5\n",
						},
					},
				}, nil
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetrics(created.ID)
	if err != nil {
		t.Fatalf("GetDatasourceMetrics: %v", err)
	}

	if !metrics.MemoryAvailable {
		t.Fatalf("expected memory to be available: %#v", metrics)
	}
	if !metrics.CPUAvailable {
		t.Fatalf("expected cpu to be available: %#v", metrics)
	}
	if metrics.MemoryUsedBytes != 1_048_576 {
		t.Fatalf("expected memory used 1048576, got %d", metrics.MemoryUsedBytes)
	}
	if metrics.MemoryTotalBytes != 2_097_152 {
		t.Fatalf("expected memory total 2097152, got %d", metrics.MemoryTotalBytes)
	}
	if math.Abs(metrics.CPUUserSeconds-11.25) > 0.0001 {
		t.Fatalf("expected cpu user seconds 11.25, got %f", metrics.CPUUserSeconds)
	}
	if math.Abs(metrics.CPUSystemSeconds-4.5) > 0.0001 {
		t.Fatalf("expected cpu system seconds 4.5, got %f", metrics.CPUSystemSeconds)
	}
}

func TestGetDatasourceMetricsByNode_RedisClusterTargetsRequestedNode(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-metrics",
		Type: datasource.TypeRedis,
		Host: "10.0.0.1",
		Port: 7000,
		Options: map[string]any{
			"nodes": []string{"10.0.0.1:7000", "10.0.0.2:7001"},
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	var targetNodeHits int
	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			case "INFO MEMORY":
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					targetNodeHits++
					if forceStandalone, ok := ds.Options["__forceStandalone"].(bool); !ok || !forceStandalone {
						t.Fatalf("expected __forceStandalone=true for node metrics datasource, got %#v", ds.Options["__forceStandalone"])
					}
					return console.QueryResult{
						Rows: []map[string]any{
							{
								"result": "# Memory\nused_memory:2097152\nmaxmemory:4194304\n",
							},
						},
					}, nil
				}
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": "# Memory\nused_memory:1048576\nmaxmemory:2097152\n",
						},
					},
				}, nil
			case "INFO CPU":
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					return console.QueryResult{
						Rows: []map[string]any{
							{
								"result": "# CPU\nused_cpu_user:21.0\nused_cpu_sys:6.5\n",
							},
						},
					}, nil
				}
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": "# CPU\nused_cpu_user:11.0\nused_cpu_sys:4.0\n",
						},
					},
				}, nil
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetricsByNode(created.ID, "10.0.0.2:7001")
	if err != nil {
		t.Fatalf("GetDatasourceMetricsByNode: %v", err)
	}
	if targetNodeHits == 0 {
		t.Fatalf("expected INFO queries to target requested node")
	}
	if metrics.Node != "10.0.0.2:7001" {
		t.Fatalf("expected selected node 10.0.0.2:7001, got %q", metrics.Node)
	}
	if len(metrics.Nodes) != 2 || metrics.Nodes[0] != "10.0.0.1:7000" || metrics.Nodes[1] != "10.0.0.2:7001" {
		t.Fatalf("unexpected cluster node list: %#v", metrics.Nodes)
	}
	if metrics.MemoryUsedBytes != 2_097_152 {
		t.Fatalf("expected memory used 2097152, got %d", metrics.MemoryUsedBytes)
	}
	if metrics.MemoryTotalBytes != 4_194_304 {
		t.Fatalf("expected memory total 4194304, got %d", metrics.MemoryTotalBytes)
	}
	if math.Abs(metrics.CPUUserSeconds-21.0) > 0.0001 {
		t.Fatalf("expected cpu user seconds 21.0, got %f", metrics.CPUUserSeconds)
	}
	if math.Abs(metrics.CPUSystemSeconds-6.5) > 0.0001 {
		t.Fatalf("expected cpu system seconds 6.5, got %f", metrics.CPUSystemSeconds)
	}
}

func TestGetDatasourceMetrics_RedisClusterFallbackKeepsNodeUnsetWhenCollectionIsNotHostPinned(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-fallback",
		Type: datasource.TypeRedis,
		Host: "10.0.0.2",
		Port: 7001,
		Options: map[string]any{
			"nodes": []string{"10.0.0.1:7000", "10.0.0.2:7001"},
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	var autoSelectedNodeInfoCalls int
	var fallbackNodeInfoCalls int

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			case "INFO MEMORY":
				if ds.Host == "10.0.0.1" && ds.Port == 7000 {
					autoSelectedNodeInfoCalls++
					return console.QueryResult{}, errors.New("dial tcp 10.0.0.1:7000: connect: connection refused")
				}
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					fallbackNodeInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# Memory\nused_memory:3145728\nmaxmemory:6291456\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected datasource target for INFO MEMORY")
			case "INFO CPU":
				if ds.Host == "10.0.0.1" && ds.Port == 7000 {
					autoSelectedNodeInfoCalls++
					return console.QueryResult{}, errors.New("ERR unknown command `INFO cpu`")
				}
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					fallbackNodeInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# CPU\nused_cpu_user:31.5\nused_cpu_sys:10.0\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected datasource target for INFO CPU")
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetrics(created.ID)
	if err != nil {
		t.Fatalf("GetDatasourceMetrics: %v", err)
	}

	if autoSelectedNodeInfoCalls == 0 {
		t.Fatalf("expected to attempt metrics collection on auto-selected node first")
	}
	if fallbackNodeInfoCalls == 0 {
		t.Fatalf("expected fallback metrics collection via datasource connection")
	}
	if !metrics.MemoryAvailable || !metrics.CPUAvailable {
		t.Fatalf("expected fallback metrics to be available, got %#v", metrics)
	}
	if metrics.MemoryUsedBytes != 3_145_728 || metrics.MemoryTotalBytes != 6_291_456 {
		t.Fatalf("unexpected memory values after fallback: used=%d total=%d", metrics.MemoryUsedBytes, metrics.MemoryTotalBytes)
	}
	if math.Abs(metrics.CPUUserSeconds-31.5) > 0.0001 || math.Abs(metrics.CPUSystemSeconds-10.0) > 0.0001 {
		t.Fatalf("unexpected cpu values after fallback: user=%f system=%f", metrics.CPUUserSeconds, metrics.CPUSystemSeconds)
	}
	if metrics.Node != "" {
		t.Fatalf("expected node to remain empty when fallback collection is not host-pinned, got %q", metrics.Node)
	}
	if len(metrics.Warnings) == 0 {
		t.Fatalf("expected warnings about the failed auto-selected node")
	}
}

func TestGetDatasourceMetrics_RedisClusterFallbackKeepsNodeUnsetWithBracketedIPv6DatasourceHost(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-fallback-ipv6-bracketed-host",
		Type: datasource.TypeRedis,
		Host: "[2001:db8::2]",
		Port: 7001,
		Options: map[string]any{
			"nodes": []string{"[2001:db8::1]:7000", "[2001:db8::2]:7001"},
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	var autoSelectedNodeInfoCalls int
	var fallbackNodeInfoCalls int

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a [2001:db8::1]:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b [2001:db8::2]:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			case "INFO MEMORY":
				if ds.Host == "2001:db8::1" && ds.Port == 7000 {
					autoSelectedNodeInfoCalls++
					return console.QueryResult{}, errors.New("dial tcp [2001:db8::1]:7000: connect: connection refused")
				}
				if ds.Host == "[2001:db8::2]" && ds.Port == 7001 {
					fallbackNodeInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# Memory\nused_memory:3145728\nmaxmemory:6291456\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected datasource target for INFO MEMORY")
			case "INFO CPU":
				if ds.Host == "2001:db8::1" && ds.Port == 7000 {
					autoSelectedNodeInfoCalls++
					return console.QueryResult{}, errors.New("ERR unknown command `INFO cpu`")
				}
				if ds.Host == "[2001:db8::2]" && ds.Port == 7001 {
					fallbackNodeInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# CPU\nused_cpu_user:31.5\nused_cpu_sys:10.0\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected datasource target for INFO CPU")
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetrics(created.ID)
	if err != nil {
		t.Fatalf("GetDatasourceMetrics: %v", err)
	}

	if autoSelectedNodeInfoCalls == 0 {
		t.Fatalf("expected to attempt metrics collection on auto-selected node first")
	}
	if fallbackNodeInfoCalls == 0 {
		t.Fatalf("expected fallback metrics collection via datasource connection")
	}
	if !metrics.MemoryAvailable || !metrics.CPUAvailable {
		t.Fatalf("expected fallback metrics to be available, got %#v", metrics)
	}
	if metrics.Node != "" {
		t.Fatalf("expected node to remain empty when fallback collection is not host-pinned, got %q", metrics.Node)
	}
}

func TestGetDatasourceMetricsByNode_DoesNotFallbackWhenRequestedNodeUnavailable(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-no-fallback-on-explicit-node",
		Type: datasource.TypeRedis,
		Host: "10.0.0.2",
		Port: 7001,
		Options: map[string]any{
			"nodes": []string{"10.0.0.1:7000", "10.0.0.2:7001"},
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	var fallbackNodeInfoCalls int

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			case "INFO MEMORY", "INFO CPU":
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					fallbackNodeInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# Memory\nused_memory:3145728\nmaxmemory:6291456\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("node unavailable")
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetricsByNode(created.ID, "10.0.0.1:7000")
	if err != nil {
		t.Fatalf("GetDatasourceMetricsByNode: %v", err)
	}

	if fallbackNodeInfoCalls > 0 {
		t.Fatalf("did not expect fallback collection when a specific node was requested")
	}
	if metrics.Node != "10.0.0.1:7000" {
		t.Fatalf("expected selected node to remain explicit request, got %q", metrics.Node)
	}
	if metrics.MemoryAvailable || metrics.CPUAvailable {
		t.Fatalf("expected unavailable metrics when requested node cannot serve INFO, got %#v", metrics)
	}
	if len(metrics.Warnings) == 0 {
		t.Fatalf("expected warnings when requested node is unavailable")
	}
}

func TestGetDatasourceMetricsByNode_DoesNotFallbackWhenRequestedNodeMissingFromTopology(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-no-fallback-on-missing-requested-node",
		Type: datasource.TypeRedis,
		Host: "10.0.0.2",
		Port: 7001,
		Options: map[string]any{
			"nodes": []string{"10.0.0.1:7000", "10.0.0.2:7001"},
		},
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{
					Rows: []map[string]any{
						{
							"result": strings.Join([]string{
								"node-a 10.0.0.1:7000@17000 master - 0 1700000000000 1 connected 0-5460",
								"node-b 10.0.0.2:7001@17001 master - 0 1700000000000 2 connected 5461-10922",
							}, "\n"),
						},
					},
				}, nil
			case "INFO MEMORY":
				return console.QueryResult{
					Rows: []map[string]any{
						{"result": "# Memory\nused_memory:3145728\nmaxmemory:6291456\n"},
					},
				}, nil
			case "INFO CPU":
				return console.QueryResult{
					Rows: []map[string]any{
						{"result": "# CPU\nused_cpu_user:31.5\nused_cpu_sys:10.0\n"},
					},
				}, nil
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	missingNode := "10.0.0.9:7009"
	metrics, err := app.GetDatasourceMetricsByNode(created.ID, missingNode)
	if err != nil {
		t.Fatalf("GetDatasourceMetricsByNode: %v", err)
	}

	if metrics.Node != missingNode {
		t.Fatalf("expected selected node to stay at missing requested node, got %q", metrics.Node)
	}
	if metrics.MemoryAvailable || metrics.CPUAvailable {
		t.Fatalf("expected unavailable metrics when requested node is missing from topology, got %#v", metrics)
	}
	if len(metrics.Warnings) == 0 {
		t.Fatalf("expected warning when requested node is missing from topology")
	}
	if !strings.Contains(strings.ToLower(strings.Join(metrics.Warnings, " ")), "not found") {
		t.Fatalf("expected warning to mention requested node not found, got %v", metrics.Warnings)
	}
}

func TestGetDatasourceMetricsByNode_DoesNotFallbackToDatasourceWhenTopologyUnavailable(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "redis-cluster-no-fallback-when-topology-unavailable",
		Type: datasource.TypeRedis,
		Host: "10.0.0.2",
		Port: 7001,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	const requestedNode = "10.0.0.1:7000"

	var (
		defaultInfoCalls   int
		requestedInfoCalls int
	)

	manager := console.NewManager()
	manager.Register(datasource.TypeRedis, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			switch strings.ToUpper(strings.TrimSpace(statement)) {
			case "CLUSTER NODES":
				return console.QueryResult{}, errors.New("ERR unknown command 'CLUSTER'")
			case "INFO MEMORY":
				if ds.Host == "10.0.0.1" && ds.Port == 7000 {
					requestedInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# Memory\nused_memory:1048576\nmaxmemory:2097152\n"},
						},
					}, nil
				}
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					defaultInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# Memory\nused_memory:3145728\nmaxmemory:6291456\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected host for INFO memory")
			case "INFO CPU":
				if ds.Host == "10.0.0.1" && ds.Port == 7000 {
					requestedInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# CPU\nused_cpu_user:9.5\nused_cpu_sys:2.5\n"},
						},
					}, nil
				}
				if ds.Host == "10.0.0.2" && ds.Port == 7001 {
					defaultInfoCalls++
					return console.QueryResult{
						Rows: []map[string]any{
							{"result": "# CPU\nused_cpu_user:31.5\nused_cpu_sys:10.0\n"},
						},
					}, nil
				}
				return console.QueryResult{}, errors.New("unexpected host for INFO cpu")
			default:
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetricsByNode(created.ID, requestedNode)
	if err != nil {
		t.Fatalf("GetDatasourceMetricsByNode: %v", err)
	}

	if defaultInfoCalls > 0 {
		t.Fatalf("did not expect fallback collection via datasource default connection when topology is unavailable")
	}
	if requestedInfoCalls == 0 {
		t.Fatalf("expected direct collection attempts on requested node when topology is unavailable")
	}
	if metrics.Node != requestedNode {
		t.Fatalf("expected metrics node to remain explicit request, got %q", metrics.Node)
	}
	if !metrics.MemoryAvailable || !metrics.CPUAvailable {
		t.Fatalf("expected metrics to be collected from requested node, got %#v", metrics)
	}
	if metrics.MemoryUsedBytes != 1_048_576 || metrics.MemoryTotalBytes != 2_097_152 {
		t.Fatalf("unexpected memory values from requested node: used=%d total=%d", metrics.MemoryUsedBytes, metrics.MemoryTotalBytes)
	}
}

func TestGetDatasourceMetrics_Elasticsearch_AggregatesNodeStats(t *testing.T) {
	store := datasource.NewStore(filepath.Join(t.TempDir(), "datasources.json"))
	created, err := store.Create(datasource.DataSource{
		Name: "es-metrics",
		Type: datasource.TypeElasticsearch,
		Host: "127.0.0.1",
		Port: 9200,
	})
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}

	manager := console.NewManager()
	manager.Register(datasource.TypeElasticsearch, metricsStubAdapter{
		execute: func(ds datasource.DataSource, statement string) (console.QueryResult, error) {
			_ = ds
			if !strings.Contains(strings.ToLower(statement), "_nodes/stats") {
				return console.QueryResult{}, errors.New("unexpected statement: " + statement)
			}
			return console.QueryResult{
				Rows: []map[string]any{
					{
						"nodes": map[string]any{
							"node-a": map[string]any{
								"process": map[string]any{
									"cpu": map[string]any{"percent": 32.0},
								},
								"jvm": map[string]any{
									"mem": map[string]any{
										"heap_used_in_bytes":     100.0,
										"non_heap_used_in_bytes": 20.0,
										"heap_max_in_bytes":      200.0,
									},
								},
							},
							"node-b": map[string]any{
								"process": map[string]any{
									"cpu": map[string]any{"percent": 48.0},
								},
								"jvm": map[string]any{
									"mem": map[string]any{
										"heap_used_in_bytes":     300.0,
										"non_heap_used_in_bytes": 30.0,
										"heap_max_in_bytes":      400.0,
									},
								},
							},
						},
					},
				},
			}, nil
		},
	})

	app := &App{store: store, manager: manager}
	metrics, err := app.GetDatasourceMetrics(created.ID)
	if err != nil {
		t.Fatalf("GetDatasourceMetrics: %v", err)
	}

	if !metrics.CPUAvailable {
		t.Fatalf("expected cpu available: %#v", metrics)
	}
	if !metrics.MemoryAvailable {
		t.Fatalf("expected memory available: %#v", metrics)
	}
	if math.Abs(metrics.CPUPercent-40.0) > 0.0001 {
		t.Fatalf("expected cpu percent 40.0, got %f", metrics.CPUPercent)
	}
	if metrics.MemoryUsedBytes != 450 {
		t.Fatalf("expected memory used 450, got %d", metrics.MemoryUsedBytes)
	}
	if metrics.MemoryTotalBytes != 600 {
		t.Fatalf("expected memory total 600, got %d", metrics.MemoryTotalBytes)
	}
}
