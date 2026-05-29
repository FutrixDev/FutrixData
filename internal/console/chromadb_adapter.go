package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"futrixdata/platform/internal/datasource"
)

type ChromaDBAdapter struct {
	client *chromaDBClient
}

type chromaDBCollection struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Metadata  map[string]any `json:"metadata"`
	Dimension any            `json:"dimension"`
}

const chromaDBCollectionsPageSize = 1000
const chromaDBCollectionsMaxPages = 100

func NewChromaDBAdapter() *ChromaDBAdapter {
	return &ChromaDBAdapter{client: newChromaDBClient()}
}

func (a *ChromaDBAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	_, _, err := a.client.do(ctx, ds, httpMethodGet, chromaDBAPIPrefix(ds)+"/collections?limit=1&offset=0", "")
	return err
}

func (a *ChromaDBAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	collections, err := a.listCollections(ctx, ds)
	if err != nil {
		return nil, err
	}
	pattern := strings.ToLower(strings.TrimSpace(opts.Pattern))
	out := make([]string, 0, len(collections))
	for _, collection := range collections {
		name := strings.TrimSpace(collection.Name)
		if name == "" {
			continue
		}
		if pattern != "" && !strings.Contains(strings.ToLower(name), pattern) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (a *ChromaDBAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	collection, err := a.collectionByNameOrID(ctx, ds, name)
	if err != nil {
		return DescribeResult{}, err
	}
	count := a.countBestEffort(ctx, ds, collection.ID)
	preview := a.previewBestEffort(ctx, ds, collection.ID)

	details := []DetailItem{
		{Label: "Collection", Value: collection.Name},
		{Label: "ID", Value: collection.ID},
	}
	if collection.Dimension != nil {
		details = append(details, DetailItem{Label: "Dimension", Value: collection.Dimension})
	}
	if count != nil {
		details = append(details, DetailItem{Label: "Records", Value: *count})
	}
	if len(collection.Metadata) > 0 {
		details = append(details, DetailItem{Label: "Metadata", Value: collection.Metadata})
	}

	return DescribeResult{
		Columns:    nil,
		Indexes:    nil,
		Details:    details,
		EntityKind: "collection",
		Preview:    preview,
	}, nil
}

func (a *ChromaDBAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	_ = opts
	start := time.Now()
	stmt, err := parseChromaDBStatement(statement)
	if err != nil {
		return QueryResult{}, err
	}
	targetPath, sourceEntity, err := a.resolveReadPath(ctx, ds, stmt)
	if err != nil {
		return QueryResult{}, err
	}
	// Strip max_distance from the body before sending to ChromaDB (it's a
	// local post-filter, not a native ChromaDB parameter).
	sendBody := chromaDBStripCustomFields(stmt.Body)
	raw, _, err := a.client.do(ctx, ds, stmt.Method, targetPath, sendBody)
	if err != nil {
		return QueryResult{}, chromaDBHumanizeError(err)
	}
	rows, detail := chromaDBRowsFromResponse(raw)

	// Apply max_distance filter using the original body (which still has the field).
	rows = chromaDBFilterByMaxDistance(rows, stmt.Body)

	rowCount := int64(len(rows))

	if sourceEntity != "" && chromaDBShouldUseCollectionCount(targetPath, stmt.Body) {
		collection, err := a.collectionByNameOrID(ctx, ds, sourceEntity)
		if err == nil {
			if total := a.countBestEffort(ctx, ds, collection.ID); total != nil && *total > rowCount {
				rowCount = *total
			}
		}
	}

	return QueryResult{
		Columns:      nil,
		Rows:         rows,
		RowCount:     rowCount,
		ElapsedMs:    time.Since(start).Milliseconds(),
		Detail:       detail,
		SourceEntity: sourceEntity,
	}, nil
}

func (a *ChromaDBAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	_ = ctx
	_ = ds
	_ = statement
	return ExplainResult{}, ErrUnsupported
}

func (a *ChromaDBAdapter) listCollections(ctx context.Context, ds datasource.DataSource) ([]chromaDBCollection, error) {
	collections := []chromaDBCollection{}
	for pageIndex := 0; pageIndex < chromaDBCollectionsMaxPages; pageIndex++ {
		offset := pageIndex * chromaDBCollectionsPageSize
		path := fmt.Sprintf("%s/collections?limit=%d&offset=%d", chromaDBAPIPrefix(ds), chromaDBCollectionsPageSize, offset)
		raw, _, err := a.client.do(ctx, ds, httpMethodGet, path, "")
		if err != nil {
			return nil, err
		}
		pageItems, err := parseChromaDBCollections(raw)
		if err != nil {
			return nil, err
		}
		collections = append(collections, pageItems...)
		if len(pageItems) < chromaDBCollectionsPageSize {
			return collections, nil
		}
	}

	// Probe one extra page so an exact multiple of the page size does not
	// trip the safety cap when there is simply no more data to fetch.
	offset := chromaDBCollectionsMaxPages * chromaDBCollectionsPageSize
	path := fmt.Sprintf("%s/collections?limit=%d&offset=%d", chromaDBAPIPrefix(ds), chromaDBCollectionsPageSize, offset)
	raw, _, err := a.client.do(ctx, ds, httpMethodGet, path, "")
	if err != nil {
		return nil, err
	}
	pageItems, err := parseChromaDBCollections(raw)
	if err != nil {
		return nil, err
	}
	collections = append(collections, pageItems...)
	if len(pageItems) < chromaDBCollectionsPageSize {
		return collections, nil
	}
	return nil, fmt.Errorf("chromadb collection pagination exceeded %d full pages", chromaDBCollectionsMaxPages)
}

func parseChromaDBCollections(raw []byte) ([]chromaDBCollection, error) {
	var direct []chromaDBCollection
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Collections []chromaDBCollection `json:"collections"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Collections, nil
}

func (a *ChromaDBAdapter) collectionByNameOrID(ctx context.Context, ds datasource.DataSource, nameOrID string) (chromaDBCollection, error) {
	target := strings.TrimSpace(nameOrID)
	if target == "" {
		return chromaDBCollection{}, errors.New("collection is required")
	}
	collections, err := a.listCollections(ctx, ds)
	if err != nil {
		return chromaDBCollection{}, err
	}
	for _, collection := range collections {
		if strings.EqualFold(strings.TrimSpace(collection.Name), target) || strings.TrimSpace(collection.ID) == target {
			if strings.TrimSpace(collection.ID) == "" {
				return chromaDBCollection{}, errors.New("collection id is missing")
			}
			return collection, nil
		}
	}
	return chromaDBCollection{}, fmt.Errorf("collection not found: %s", target)
}

func (a *ChromaDBAdapter) countBestEffort(ctx context.Context, ds datasource.DataSource, collectionID string) *int64 {
	path := chromaDBAPIPrefix(ds) + "/collections/" + url.PathEscape(collectionID) + "/count"
	raw, _, err := a.client.do(ctx, ds, httpMethodGet, path, "")
	if err != nil {
		return nil
	}
	if count, ok := chromaDBCountFromResponse(raw); ok {
		return &count
	}
	return nil
}

func (a *ChromaDBAdapter) previewBestEffort(ctx context.Context, ds datasource.DataSource, collectionID string) any {
	path := chromaDBAPIPrefix(ds) + "/collections/" + url.PathEscape(collectionID) + "/get"
	raw, _, err := a.client.do(ctx, ds, httpMethodPost, path, `{"limit":1,"include":["documents","metadatas"]}`)
	if err != nil {
		return nil
	}
	var preview any
	if err := json.Unmarshal(raw, &preview); err != nil {
		return nil
	}
	return preview
}

func chromaDBCountFromResponse(raw []byte) (int64, bool) {
	var count int64
	if err := json.Unmarshal(raw, &count); err == nil {
		return count, true
	}
	var wrapped struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Count, true
	}
	return 0, false
}

func (a *ChromaDBAdapter) resolveReadPath(ctx context.Context, ds datasource.DataSource, stmt chromaDBStatement) (string, string, error) {
	path := strings.TrimSpace(stmt.Path)
	if path == "" {
		return "", "", errors.New("path is required")
	}
	if strings.HasPrefix(path, "/api/v2/") {
		if err := chromaDBEnsureReadOnly(stmt.Method, path); err != nil {
			return "", "", err
		}
		if !chromaDBPathTargetsConfiguredScope(ds, path) {
			return "", "", fmt.Errorf("api path must target configured tenant/database: %s", chromaDBAPIPrefix(ds))
		}
		return path, chromaDBSourceEntityFromAPIPath(path), nil
	}
	if strings.HasPrefix(path, "/collections/") {
		parts := strings.Split(strings.TrimPrefix(path, "/collections/"), "/")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" && len(parts) > 1 {
			if err := chromaDBEnsureReadOnly(stmt.Method, path); err != nil {
				return "", "", err
			}
			collectionRef, _ := url.PathUnescape(parts[0])
			if chromaDBLooksLikeCollectionID(collectionRef) {
				parts[0] = url.PathEscape(collectionRef)
				path = "/collections/" + strings.Join(parts, "/")
				return chromaDBAPIPrefix(ds) + path, collectionRef, nil
			}
			collection, err := a.collectionByNameOrID(ctx, ds, parts[0])
			if err != nil {
				return "", "", err
			}
			parts[0] = url.PathEscape(collection.ID)
			path = "/collections/" + strings.Join(parts, "/")
			return chromaDBAPIPrefix(ds) + path, collection.Name, nil
		}
	}
	if err := chromaDBEnsureReadOnly(stmt.Method, path); err != nil {
		return "", "", err
	}
	return chromaDBAPIPrefix(ds) + path, chromaDBSourceEntityFromAPIPath(path), nil
}

func chromaDBPathTargetsConfiguredScope(ds datasource.DataSource, path string) bool {
	prefix := chromaDBAPIPrefix(ds)
	return path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"?")
}

func chromaDBLooksLikeCollectionID(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) != 36 {
		return false
	}
	for i, ch := range normalized {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func chromaDBEnsureReadOnly(method string, path string) error {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	normalizedPath := strings.TrimSpace(path)
	if idx := strings.Index(normalizedPath, "?"); idx >= 0 {
		normalizedPath = normalizedPath[:idx]
	}
	normalizedPath = strings.TrimRight(normalizedPath, "/")
	if normalizedMethod == httpMethodGet || normalizedMethod == "HEAD" {
		if strings.Contains(normalizedPath, "/collections") {
			return nil
		}
	}
	if normalizedMethod == httpMethodPost {
		if strings.HasSuffix(normalizedPath, "/get") || strings.HasSuffix(normalizedPath, "/query") {
			return nil
		}
	}
	return fmt.Errorf("chromadb adapter is read-only in this release: %s %s", method, path)
}

func chromaDBRowsFromResponse(raw []byte) ([]map[string]any, any) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		text := strings.TrimSpace(string(raw))
		if text == "" {
			return nil, nil
		}
		return []map[string]any{{"result": text}}, text
	}
	switch typed := parsed.(type) {
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			} else {
				rows = append(rows, map[string]any{"result": item})
			}
		}
		return rows, parsed
	case map[string]any:
		// Detect ChromaDB columnar format: {ids: [], documents: [], metadatas: [], ...}
		// Transpose into row-based format for table display.
		if rows := chromaDBTransposeColumnar(typed); rows != nil {
			return rows, parsed
		}
		return []map[string]any{{"result": typed}}, parsed
	default:
		return []map[string]any{{"result": typed}}, parsed
	}
}

// chromaDBTransposeColumnar detects ChromaDB's columnar response format
// (from /get and /query endpoints) and transposes it into per-document rows.
//
// /get returns:  {ids: ["a","b"], documents: ["d1","d2"], metadatas: [{},{}], ...}
// /query returns: {ids: [["a","b"]], documents: [["d1","d2"]], distances: [[0.1,0.2]], ...}
func chromaDBTransposeColumnar(m map[string]any) []map[string]any {
	idsRaw, hasIDs := m["ids"]
	if !hasIDs {
		return nil
	}

	// Determine if this is a /query response (nested arrays) or /get response (flat arrays).
	ids, isQuery := chromaDBResolveIDColumn(idsRaw)
	if ids == nil {
		return nil
	}
	n := len(ids)
	if n == 0 {
		return []map[string]any{}
	}

	rows := make([]map[string]any, n)
	for i := range n {
		rows[i] = map[string]any{"id": ids[i]}
	}

	// Columns to extract from the response.
	columnKeys := []struct {
		src string
		dst string
	}{
		{"documents", "document"},
		{"metadatas", "metadata"},
		{"embeddings", "embedding"},
		{"distances", "distance"},
		{"uris", "uri"},
		{"data", "data"},
	}

	for _, col := range columnKeys {
		raw, ok := m[col.src]
		if !ok || raw == nil {
			continue
		}
		values := chromaDBResolveColumn(raw, isQuery)
		if values == nil || len(values) != n {
			continue
		}
		for i, v := range values {
			if v != nil {
				rows[i][col.dst] = v
			}
		}
	}

	return rows
}

// chromaDBResolveIDColumn returns the flat id list and whether the response is
// from a /query endpoint (nested arrays).
// For multi-query responses (e.g. multiple query_texts), all batches are concatenated.
func chromaDBResolveIDColumn(raw any) ([]any, bool) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	if len(arr) == 0 {
		return arr, false // empty but still columnar
	}
	// /query: ids is [[id1, id2, ...], [id3, ...]] — elements are arrays.
	if _, ok := arr[0].([]any); ok {
		return chromaDBFlattenNested(arr), true
	}
	// /get: ids is [id1, id2, ...]
	return arr, false
}

// chromaDBResolveColumn extracts the flat value slice from either a /get or /query column.
// For multi-query responses, all batches are concatenated.
func chromaDBResolveColumn(raw any, isQuery bool) []any {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	if isQuery {
		return chromaDBFlattenNested(arr)
	}
	return arr
}

// chromaDBFlattenNested concatenates all inner arrays from a nested array (e.g. [[a,b],[c,d]] → [a,b,c,d]).
func chromaDBFlattenNested(arr []any) []any {
	var out []any
	for _, item := range arr {
		inner, ok := item.([]any)
		if !ok {
			continue
		}
		out = append(out, inner...)
	}
	return out
}

func chromaDBSourceEntityFromAPIPath(path string) string {
	normalized := strings.Trim(strings.TrimSpace(path), "/")
	if normalized == "" {
		return ""
	}
	parts := strings.Split(normalized, "/")
	for i, part := range parts {
		if part == "collections" && i+1 < len(parts) {
			unescaped, err := url.PathUnescape(parts[i+1])
			if err == nil {
				return unescaped
			}
			return parts[i+1]
		}
	}
	return ""
}

// chromaDBShouldUseCollectionCount decides whether the collection-wide /count
// is a valid proxy for the total result count of the given request.
//
// It returns true only when BOTH conditions are met:
//  1. The path targets a paginated document endpoint (/get or /query).
//     Metadata lookups, /count itself, etc. return single-row responses where
//     overriding RowCount would produce bogus page counts.
//  2. The request body has no filter keys (ids, where, where_document,
//     query_texts, query_embeddings). Filtered queries return a subset, so the
//     collection-wide total would be misleading.
func chromaDBShouldUseCollectionCount(path string, body string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(path), "/")
	if !strings.HasSuffix(normalized, "/get") && !strings.HasSuffix(normalized, "/query") {
		return false
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || trimmed == "{}" {
		return true
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return false
	}
	filterKeys := []string{"ids", "where", "where_document", "query_texts", "query_embeddings"}
	for _, key := range filterKeys {
		if v, ok := parsed[key]; ok && v != nil {
			return false
		}
	}
	return true
}

// chromaDBHumanizeError rewrites known cryptic ChromaDB API errors into
// actionable messages for the GUI user.
func chromaDBHumanizeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "query_embeddings") && strings.Contains(msg, "Field required") {
		return fmt.Errorf("ChromaDB REST API requires pre-computed embedding vectors for similarity search. " +
			"Use the Vector input field to provide query_embeddings. " +
			"Example: [0.1, 0.2, 0.3, ...]")
	}
	return err
}

// chromaDBFilterByMaxDistance removes rows whose "distance" exceeds the
// "max_distance" threshold specified in the request body.  ChromaDB itself
// chromaDBStripCustomFields removes custom fields (like max_distance) from the
// request body before sending to ChromaDB, since they are not part of the
// ChromaDB API and may cause validation errors on strict deployments.
func chromaDBStripCustomFields(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || trimmed[0] != '{' {
		return body
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return body
	}
	delete(parsed, "max_distance")
	out, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return string(out)
}

// does not support a max_distance parameter, so this is a server-side filter
// applied after the API call returns.  If max_distance is absent or the rows
// have no distance column, the rows are returned unchanged.
func chromaDBFilterByMaxDistance(rows []map[string]any, body string) []map[string]any {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || trimmed == "{}" {
		return rows
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return rows
	}
	raw, ok := parsed["max_distance"]
	if !ok || raw == nil {
		return rows
	}
	maxDist, ok := chromaDBToFloat64(raw)
	if !ok {
		return rows
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		dist, ok := chromaDBToFloat64(row["distance"])
		if !ok {
			filtered = append(filtered, row)
			continue
		}
		if dist <= maxDist {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func chromaDBToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	}
	return 0, false
}

const (
	httpMethodGet  = "GET"
	httpMethodPost = "POST"
)
