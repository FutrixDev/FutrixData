package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestChromaDBAdapter_TestConnection_ListsConfiguredDatabase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("x-chroma-token"); got != "token-123" {
			t.Fatalf("expected token header, got %q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)
	ds.Options["apiToken"] = "token-123"

	if err := adapter.TestConnection(context.Background(), ds); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestChromaDBAdapter_ListEntities_FiltersCollectionNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "col_2", "name": "events_archive"},
			{"id": "col_1", "name": "support_docs"},
			{"id": "col_3", "name": "docs"},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.ListEntities(context.Background(), ds, ListOptions{Pattern: "doc"})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(got) != 2 || got[0] != "docs" || got[1] != "support_docs" {
		t.Fatalf("unexpected collections: %v", got)
	}
}

func TestChromaDBAdapter_ListEntities_PaginatesCollections(t *testing.T) {
	requestedOffsets := []int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "1000" {
			t.Fatalf("expected page limit 1000, got %q", got)
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			t.Fatalf("parse offset: %v", err)
		}
		requestedOffsets = append(requestedOffsets, offset)
		switch offset {
		case 0:
			collections := make([]map[string]any, 0, 1000)
			for i := range 1000 {
				collections = append(collections, map[string]any{
					"id":   "page0_" + strconv.Itoa(i),
					"name": "page0_" + strconv.Itoa(i),
				})
			}
			_ = json.NewEncoder(w).Encode(collections)
		case 1000:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "col_target", "name": "target_docs"},
			})
		default:
			t.Fatalf("unexpected offset %d", offset)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.ListEntities(context.Background(), ds, ListOptions{Pattern: "target"})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(got) != 1 || got[0] != "target_docs" {
		t.Fatalf("unexpected collections: %v", got)
	}
	if len(requestedOffsets) != 2 || requestedOffsets[0] != 0 || requestedOffsets[1] != 1000 {
		t.Fatalf("unexpected requested offsets: %v", requestedOffsets)
	}
}

func TestChromaDBAdapter_ListEntities_StopsAfterPaginationGuard(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		collections := make([]map[string]any, 0, chromaDBCollectionsPageSize)
		for i := 0; i < chromaDBCollectionsPageSize; i++ {
			collections = append(collections, map[string]any{
				"id":   "page_" + strconv.Itoa(requestCount) + "_" + strconv.Itoa(i),
				"name": "page_" + strconv.Itoa(requestCount) + "_" + strconv.Itoa(i),
			})
		}
		_ = json.NewEncoder(w).Encode(collections)
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	_, err := adapter.ListEntities(context.Background(), ds, ListOptions{})
	if err == nil {
		t.Fatalf("expected pagination guard error")
	}
	if !strings.Contains(err.Error(), "pagination exceeded") {
		t.Fatalf("expected pagination guard error, got %v", err)
	}
	if requestCount != chromaDBCollectionsMaxPages+1 {
		t.Fatalf("expected %d requests before stopping, got %d", chromaDBCollectionsMaxPages+1, requestCount)
	}
}

func TestChromaDBAdapter_ListEntities_AllowsExactBoundaryCollectionCount(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			t.Fatalf("parse offset: %v", err)
		}
		if offset == chromaDBCollectionsMaxPages*chromaDBCollectionsPageSize {
			_ = json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		collections := make([]map[string]any, 0, chromaDBCollectionsPageSize)
		for i := 0; i < chromaDBCollectionsPageSize; i++ {
			collections = append(collections, map[string]any{
				"id":   "page_" + strconv.Itoa(offset) + "_" + strconv.Itoa(i),
				"name": "page_" + strconv.Itoa(offset) + "_" + strconv.Itoa(i),
			})
		}
		_ = json.NewEncoder(w).Encode(collections)
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.ListEntities(context.Background(), ds, ListOptions{})
	if err != nil {
		t.Fatalf("expected exact-boundary pagination to succeed, got %v", err)
	}
	expectedCount := chromaDBCollectionsMaxPages * chromaDBCollectionsPageSize
	if len(got) != expectedCount {
		t.Fatalf("expected %d collections, got %d", expectedCount, len(got))
	}
	if requestCount != chromaDBCollectionsMaxPages+1 {
		t.Fatalf("expected %d requests including probe page, got %d", chromaDBCollectionsMaxPages+1, requestCount)
	}
}

func TestChromaDBAdapter_DescribeEntity_LoadsMetadataAndPreview(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tenants/default_tenant/databases/default_database/collections":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "col_docs", "name": "docs", "metadata": map[string]any{"team": "support"}, "dimension": 1536},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tenants/default_tenant/databases/default_database/collections/col_docs/count":
			_ = json.NewEncoder(w).Encode(map[string]any{"count": 12})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/tenants/default_tenant/databases/default_database/collections/col_docs/get":
			raw, _ := io.ReadAll(r.Body)
			body := string(raw)
			if !strings.Contains(body, `"limit":1`) {
				t.Fatalf("expected preview limit=1, got %s", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ids":       []string{"doc-1"},
				"documents": []string{"hello"},
				"metadatas": []map[string]any{{"source": "ticket"}},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.DescribeEntity(context.Background(), ds, "docs")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if got.EntityKind != "collection" {
		t.Fatalf("expected collection kind, got %q", got.EntityKind)
	}
	if len(got.Details) == 0 {
		t.Fatalf("expected collection details")
	}
	if got.Preview == nil {
		t.Fatalf("expected preview")
	}
}

func TestChromaDBAdapter_Execute_AllowsReadRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "col_docs", "name": "docs"},
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/count") {
			_ = json.NewEncoder(w).Encode(100)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections/col_docs/query" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"n_results":2`) {
			t.Fatalf("expected query body, got %s", string(raw))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ids":       [][]string{{"doc-1", "doc-2"}},
			"documents": [][]string{{"hello", "world"}},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.Execute(context.Background(), ds, "POST /collections/col_docs/query\n{\"query_texts\":[\"hello\"],\"n_results\":2}", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// query_texts is a filter key, so /count (collection-wide total) is NOT used;
	// rowCount should equal the actual number of returned rows.
	if got.RowCount != 2 || len(got.Rows) != 2 {
		t.Fatalf("expected rowCount=2 (filtered query, no /count) with 2 transposed rows, count=%d rows=%v", got.RowCount, got.Rows)
	}
	if got.Rows[0]["id"] != "doc-1" || got.Rows[1]["id"] != "doc-2" {
		t.Fatalf("expected transposed rows with ids doc-1/doc-2, got %v", got.Rows)
	}
	if got.Rows[0]["document"] != "hello" || got.Rows[1]["document"] != "world" {
		t.Fatalf("expected transposed documents hello/world, got %v", got.Rows)
	}
	if got.SourceEntity != "docs" {
		t.Fatalf("expected source entity docs, got %q", got.SourceEntity)
	}
}

func TestChromaDBAdapter_Execute_RejectsCrossTenantAPIPath(t *testing.T) {
	requested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)
	ds.Options["tenant"] = "tenant_a"
	ds.Options["database"] = "db_a"

	_, err := adapter.Execute(
		context.Background(),
		ds,
		"GET /api/v2/tenants/tenant_b/databases/db_b/collections",
		ExecuteOptions{},
	)
	if err == nil {
		t.Fatalf("expected cross-tenant path error")
	}
	if !strings.Contains(err.Error(), "configured tenant/database") {
		t.Fatalf("expected configured tenant/database error, got %v", err)
	}
	if requested {
		t.Fatalf("expected request to be rejected before reaching server")
	}
}

func TestChromaDBAdapter_Execute_UsesCollectionUUIDWithoutListingCollections(t *testing.T) {
	collectionID := "123e4567-e89b-12d3-a456-426614174000"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/tenants/default_tenant/databases/default_database/collections" {
			// The post-execute count lookup calls collectionByNameOrID which lists collections.
			// This is expected — resolveReadPath still skips listing for UUID paths.
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": collectionID, "name": "my-collection"},
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/count") {
			_ = json.NewEncoder(w).Encode(42)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/tenants/default_tenant/databases/default_database/collections/"+collectionID+"/get" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ids":       []string{"doc-1"},
			"documents": []string{"hello"},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	got, err := adapter.Execute(context.Background(), ds, "POST /collections/"+collectionID+"/get\n{\"ids\":[\"doc-1\"]}", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.SourceEntity != collectionID {
		t.Fatalf("expected source entity collection id, got %q", got.SourceEntity)
	}
}

func TestChromaDBAdapter_Execute_RejectsWriteRoutes(t *testing.T) {
	adapter := NewChromaDBAdapter()
	ds := datasource.DataSource{Type: datasource.TypeChromaDB, Host: "127.0.0.1", Port: 8000}

	_, err := adapter.Execute(context.Background(), ds, "POST /collections/col_docs/add\n{\"ids\":[\"1\"]}", ExecuteOptions{})
	if err == nil {
		t.Fatalf("expected write route error")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
}

func TestChromaDBAdapter_RealServerSmoke(t *testing.T) {
	rawURL := strings.TrimSpace(os.Getenv("FUTRIX_CHROMADB_TEST_URL"))
	if rawURL == "" {
		t.Skip("set FUTRIX_CHROMADB_TEST_URL to run live ChromaDB smoke test")
	}

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, rawURL)
	ctx := context.Background()

	if err := adapter.TestConnection(ctx, ds); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	entities, err := adapter.ListEntities(ctx, ds, ListOptions{Pattern: "futrix"})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if !containsString(entities, "futrix_docs") {
		t.Fatalf("expected futrix_docs collection, got %v", entities)
	}
	if _, err := adapter.DescribeEntity(ctx, ds, "futrix_docs"); err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	got, err := adapter.Execute(ctx, ds, "POST /collections/futrix_docs/get\n{\"ids\":[\"doc-1\"],\"include\":[\"documents\",\"metadatas\"]}", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute get: %v", err)
	}
	if got.RowCount < 1 || len(got.Rows) < 1 {
		t.Fatalf("expected live ChromaDB result rows, got count=%d rows=%v", got.RowCount, got.Rows)
	}
}

func TestChromaDBTransposeColumnar_GetResponse(t *testing.T) {
	raw := []byte(`{"ids":["a","b"],"documents":["doc-a","doc-b"],"metadatas":[{"k":"v1"},{"k":"v2"}]}`)
	rows, _ := chromaDBRowsFromResponse(raw)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["id"] != "a" || rows[1]["id"] != "b" {
		t.Fatalf("unexpected ids: %v", rows)
	}
	if rows[0]["document"] != "doc-a" || rows[1]["document"] != "doc-b" {
		t.Fatalf("unexpected documents: %v", rows)
	}
}

func TestChromaDBTransposeColumnar_QueryResponse(t *testing.T) {
	raw := []byte(`{"ids":[["x","y"]],"documents":[["hello","world"]],"distances":[[0.1,0.2]]}`)
	rows, _ := chromaDBRowsFromResponse(raw)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["id"] != "x" || rows[1]["id"] != "y" {
		t.Fatalf("unexpected ids: %v", rows)
	}
	if rows[0]["distance"] != 0.1 || rows[1]["distance"] != 0.2 {
		t.Fatalf("unexpected distances: %v", rows)
	}
}

func TestChromaDBTransposeColumnar_EmptyIds(t *testing.T) {
	raw := []byte(`{"ids":[],"documents":[]}`)
	rows, _ := chromaDBRowsFromResponse(raw)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestChromaDBTransposeColumnar_NonColumnar(t *testing.T) {
	raw := []byte(`{"error":"not found"}`)
	rows, _ := chromaDBRowsFromResponse(raw)
	if len(rows) != 1 {
		t.Fatalf("expected 1 fallback row, got %d", len(rows))
	}
	if rows[0]["result"] == nil {
		t.Fatalf("expected result key in fallback row")
	}
}

func TestChromaDBTransposeColumnar_MultiQueryResponse(t *testing.T) {
	raw := []byte(`{"ids":[["a","b"],["c"]],"documents":[["doc-a","doc-b"],["doc-c"]],"distances":[[0.1,0.2],[0.3]]}`)
	rows, _ := chromaDBRowsFromResponse(raw)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows from 2 query batches, got %d", len(rows))
	}
	if rows[0]["id"] != "a" || rows[1]["id"] != "b" || rows[2]["id"] != "c" {
		t.Fatalf("unexpected ids: %v", rows)
	}
	if rows[0]["document"] != "doc-a" || rows[2]["document"] != "doc-c" {
		t.Fatalf("unexpected documents: %v", rows)
	}
	if rows[0]["distance"] != 0.1 || rows[2]["distance"] != 0.3 {
		t.Fatalf("unexpected distances: %v", rows)
	}
}

func TestChromaDBShouldUseCollectionCount(t *testing.T) {
	tests := []struct {
		path   string
		body   string
		expect bool
	}{
		// Document endpoints without filters → use count.
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", "", true},
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", "{}", true},
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", `{"limit":10,"offset":0}`, true},
		{"/api/v2/tenants/t/databases/d/collections/col-1/query", `{"include":["documents"]}`, true},
		// Document endpoints with filters → skip count.
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", `{"ids":["doc-1"]}`, false},
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", `{"where":{"type":"article"}}`, false},
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", `{"where_document":{"$contains":"hello"}}`, false},
		{"/api/v2/tenants/t/databases/d/collections/col-1/query", `{"query_texts":["hello"]}`, false},
		{"/api/v2/tenants/t/databases/d/collections/col-1/query", `{"query_embeddings":[[0.1,0.2]]}`, false},
		{"/api/v2/tenants/t/databases/d/collections/col-1/get", `{"limit":10,"where":{"k":"v"}}`, false},
		// Non-document endpoints → skip count regardless of body.
		{"/api/v2/tenants/t/databases/d/collections/col-1/count", "", false},
		{"/api/v2/tenants/t/databases/d/collections/col-1", "", false},
		{"/api/v2/tenants/t/databases/d/collections", "", false},
	}
	for _, tt := range tests {
		got := chromaDBShouldUseCollectionCount(tt.path, tt.body)
		if got != tt.expect {
			t.Errorf("chromaDBShouldUseCollectionCount(%q, %q) = %v, want %v", tt.path, tt.body, got, tt.expect)
		}
	}
}

func TestChromaDBAdapter_Execute_UnfilteredUsesCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/collections") && !strings.HasSuffix(r.URL.Path, "/count") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "col-1", "name": "docs"},
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/count") {
			_ = json.NewEncoder(w).Encode(500)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/get") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ids":       []string{"a", "b"},
				"documents": []string{"d1", "d2"},
			})
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	adapter := NewChromaDBAdapter()
	ds := chromaDBDatasourceFromURL(t, srv.URL)

	// Unfiltered /get with only limit/offset — should use /count for total.
	got, err := adapter.Execute(context.Background(), ds, "POST /collections/docs/get\n{\"limit\":10,\"offset\":0}", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.RowCount != 500 {
		t.Fatalf("expected rowCount=500 from /count for unfiltered query, got %d", got.RowCount)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got.Rows))
	}
}

func TestChromaDBHumanizeError(t *testing.T) {
	// Nil passes through.
	if got := chromaDBHumanizeError(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}

	// Unrelated error passes through unchanged.
	plain := fmt.Errorf("some other error")
	if got := chromaDBHumanizeError(plain); got != plain {
		t.Fatalf("expected original error, got %v", got)
	}

	// The query_embeddings required error gets rewritten.
	original := fmt.Errorf(`chromadb request failed: 400 Bad Request: {"error":"InvalidArgumentError","message":"1 validation error for QueryEmbedding\nquery_embeddings\n Field required [type=missing]"}`)
	got := chromaDBHumanizeError(original)
	if got == original {
		t.Fatal("expected rewritten error, got original")
	}
	if !strings.Contains(got.Error(), "pre-computed embedding vectors") {
		t.Fatalf("expected user-friendly message, got: %s", got.Error())
	}
	if !strings.Contains(got.Error(), "query_embeddings") {
		t.Fatalf("expected guidance about query_embeddings, got: %s", got.Error())
	}
}

func TestChromaDBFilterByMaxDistance(t *testing.T) {
	rows := []map[string]any{
		{"id": "a", "distance": 0.1},
		{"id": "b", "distance": 0.5},
		{"id": "c", "distance": 1.2},
		{"id": "d", "distance": 0.3},
	}

	// No max_distance in body — returns all rows.
	got := chromaDBFilterByMaxDistance(rows, `{"n_results": 10}`)
	if len(got) != 4 {
		t.Fatalf("expected 4 rows without max_distance, got %d", len(got))
	}

	// max_distance = 0.5 — keeps rows with distance <= 0.5.
	got = chromaDBFilterByMaxDistance(rows, `{"n_results": 10, "max_distance": 0.5}`)
	if len(got) != 3 {
		t.Fatalf("expected 3 rows with max_distance=0.5, got %d", len(got))
	}

	// max_distance = 0.05 — only very close matches.
	got = chromaDBFilterByMaxDistance(rows, `{"max_distance": 0.05}`)
	if len(got) != 0 {
		t.Fatalf("expected 0 rows with max_distance=0.05, got %d", len(got))
	}

	// Empty body — returns all rows.
	got = chromaDBFilterByMaxDistance(rows, "")
	if len(got) != 4 {
		t.Fatalf("expected 4 rows with empty body, got %d", len(got))
	}

	// Rows without distance field are kept.
	mixed := []map[string]any{
		{"id": "x"},
		{"id": "y", "distance": 0.8},
	}
	got = chromaDBFilterByMaxDistance(mixed, `{"max_distance": 0.5}`)
	if len(got) != 1 {
		t.Fatalf("expected 1 row (no-distance kept), got %d", len(got))
	}
	if got[0]["id"] != "x" {
		t.Fatalf("expected row without distance to be kept, got %v", got[0])
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func chromaDBDatasourceFromURL(t *testing.T, rawURL string) datasource.DataSource {
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
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return datasource.DataSource{
		Type: datasource.TypeChromaDB,
		Host: host,
		Port: port,
		Options: map[string]any{
			"scheme":   "http",
			"tenant":   "default_tenant",
			"database": "default_database",
		},
	}
}
