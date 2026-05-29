package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestElasticsearchAdapter_TestConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tagline":"You Know, for Search"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	ds := elasticsearchDatasourceFromURL(t, srv.URL)
	adapter := NewElasticsearchAdapter()

	if err := adapter.TestConnection(context.Background(), ds); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestElasticsearchAdapter_ListEntities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_cat/indices" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("format") != "json" {
			t.Fatalf("expected format=json, got %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("h") != "index" {
			t.Fatalf("expected h=index, got %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"index": "futrixdata-demo-1"},
			{"index": "futrixdata-demo-2"},
			{"index": "other-index"},
		})
	}))
	t.Cleanup(srv.Close)

	ds := elasticsearchDatasourceFromURL(t, srv.URL)
	adapter := NewElasticsearchAdapter()

	items, err := adapter.ListEntities(context.Background(), ds, ListOptions{Pattern: "demo"})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 indices, got %d (%v)", len(items), items)
	}
	if items[0] != "futrixdata-demo-1" || items[1] != "futrixdata-demo-2" {
		t.Fatalf("unexpected indices: %v", items)
	}
}

func TestElasticsearchAdapter_DescribeEntity_FlattensMapping(t *testing.T) {
	index := "futrixdata-demo-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+index+"/_mapping":
			_ = json.NewEncoder(w).Encode(map[string]any{
				index: map[string]any{
					"mappings": map[string]any{
						"properties": map[string]any{
							"user": map[string]any{
								"properties": map[string]any{
									"id": map[string]any{"type": "keyword"},
									"meta": map[string]any{
										"properties": map[string]any{
											"region": map[string]any{"type": "keyword"},
										},
									},
								},
							},
							"title": map[string]any{
								"type": "text",
								"fields": map[string]any{
									"keyword": map[string]any{"type": "keyword"},
								},
							},
							"created_at": map[string]any{"type": "date"},
						},
					},
				},
			})
			return
		case r.URL.Path == "/_cat/indices/"+index:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"index":      index,
					"docs.count": "100000",
					"store.size": "12mb",
					"health":     "green",
					"status":     "open",
				},
			})
			return
		case r.URL.Path == "/"+index+"/_search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hits": map[string]any{
					"hits": []any{
						map[string]any{"_source": map[string]any{"title": "hello"}},
					},
				},
			})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	ds := elasticsearchDatasourceFromURL(t, srv.URL)
	adapter := NewElasticsearchAdapter()

	result, err := adapter.DescribeEntity(context.Background(), ds, index)
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	has := func(name string) bool {
		for _, col := range result.Columns {
			if col.Name == name {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"created_at", "title", "title.keyword", "user.id", "user.meta.region"} {
		if !has(want) {
			t.Fatalf("expected field %q in columns; got=%v", want, result.Columns)
		}
	}
}

func TestElasticsearchAdapter_Execute_TextAndSearchHits(t *testing.T) {
	index := "futrixdata-demo-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
			case r.URL.Path == "/_cat/indices" && r.URL.Query().Has("v"):
				_, _ = w.Write([]byte("index docs.count\nfutrixdata-demo-1 100000\n"))
				return
			case r.URL.Path == "/"+index+"/_search":
				if r.Method == http.MethodGet {
					if got := r.URL.Query().Get("q"); got != "foo" {
						t.Fatalf("expected q=foo, got %q", got)
					}
					if got := r.URL.Query().Get("size"); got != "2" {
						t.Fatalf("expected size=2, got %q", got)
					}
					if got := r.URL.Query().Get("track_total_hits"); got != "true" {
						t.Fatalf("expected track_total_hits=true, got %q", got)
					}
					raw, _ := io.ReadAll(r.Body)
					if strings.TrimSpace(string(raw)) != "" {
						t.Fatalf("expected empty request body for GET, got %q", string(raw))
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"took": 5,
						"hits": map[string]any{
							"total": map[string]any{
								"value":    12402,
								"relation": "eq",
							},
							"hits": []any{
								map[string]any{"_id": "1", "_source": map[string]any{"title": "a"}},
							},
						},
					})
					return
				}
				raw, _ := io.ReadAll(r.Body)
				body := string(raw)
				if r.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", r.Method)
				}
			if !strings.Contains(body, "match_all") {
				t.Fatalf("expected request body to include match_all, got %q", body)
			}
			if !strings.Contains(body, "\"track_total_hits\":true") {
				t.Fatalf("expected request body to include track_total_hits, got %q", body)
			}
			if !strings.Contains(body, "9223372036854775807") {
				t.Fatalf("expected request body to preserve large integer, got %q", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pit_id": "pit-1",
				"took": 23,
				"hits": map[string]any{
					"total": map[string]any{
						"value":    12402,
						"relation": "eq",
					},
					"hits": []any{
						map[string]any{"_id": "1", "_source": map[string]any{"title": "a"}},
						map[string]any{"_id": "2", "_source": map[string]any{"title": "b"}},
					},
				},
			})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	ds := elasticsearchDatasourceFromURL(t, srv.URL)
	adapter := NewElasticsearchAdapter()

	textResult, err := adapter.Execute(context.Background(), ds, "GET /_cat/indices?v", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute text: %v", err)
	}
	if textResult.RowCount != 1 || len(textResult.Rows) != 1 {
		t.Fatalf("expected 1 row for text response, got rowCount=%d rows=%d", textResult.RowCount, len(textResult.Rows))
	}
	if got := strings.TrimSpace(fmt.Sprint(textResult.Rows[0]["result"])); !strings.Contains(got, "futrixdata-demo-1") {
		t.Fatalf("expected text response to include index name, got %q", got)
	}

	searchResult, err := adapter.Execute(context.Background(), ds, "POST /futrixdata-demo-1/_search\n{\"query\":{\"match_all\":{}},\"size\":2,\"from\":9223372036854775807}", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute search: %v", err)
	}
	if searchResult.RowCount != 12402 || len(searchResult.Rows) != 2 {
		t.Fatalf("expected rowCount=12402 and 2 hits rows, got rowCount=%d rows=%d", searchResult.RowCount, len(searchResult.Rows))
	}
	if searchResult.ElapsedMs != 23 {
		t.Fatalf("expected took/elapsedMs=23, got %d", searchResult.ElapsedMs)
	}
	detail, _ := searchResult.Detail.(map[string]any)
	if got := strings.TrimSpace(fmt.Sprint(detail["pitId"])); got != "pit-1" {
		t.Fatalf("expected pitId pit-1, got %#v", searchResult.Detail)
	}

	bodylessResult, err := adapter.Execute(
		context.Background(),
		ds,
		"GET /futrixdata-demo-1/_search?q=foo&size=2",
		ExecuteOptions{},
	)
	if err != nil {
		t.Fatalf("Execute bodyless search: %v", err)
	}
	if bodylessResult.RowCount != 12402 || len(bodylessResult.Rows) != 1 {
		t.Fatalf("expected rowCount=12402 and 1 hit row, got rowCount=%d rows=%d", bodylessResult.RowCount, len(bodylessResult.Rows))
	}
}

func elasticsearchDatasourceFromURL(t *testing.T, rawURL string) datasource.DataSource {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host := parsed.Hostname()
	portStr := parsed.Port()
	if host == "" || portStr == "" {
		t.Fatalf("expected host:port, got %q", rawURL)
	}
	parsedPort, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return datasource.DataSource{
		ID:      "ds_test",
		Type:    datasource.TypeElasticsearch,
		Host:    host,
		Port:    parsedPort,
		Options: map[string]any{"scheme": parsed.Scheme},
	}
}
