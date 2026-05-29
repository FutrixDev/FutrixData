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

type ElasticsearchAdapter struct {
	client *elasticsearchClient
}

func NewElasticsearchAdapter() *ElasticsearchAdapter {
	return &ElasticsearchAdapter{client: newElasticsearchClient()}
}

func (a *ElasticsearchAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	_, _, err := a.client.do(ctx, ds, "GET", "/", "")
	return err
}

func (a *ElasticsearchAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	payload, _, err := a.client.do(ctx, ds, "GET", "/_cat/indices?format=json&h=index", "")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Index string `json:"index"`
	}
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	pattern := strings.ToLower(strings.TrimSpace(opts.Pattern))
	for _, row := range rows {
		name := strings.TrimSpace(row.Index)
		if name == "" {
			continue
		}
		if pattern != "" && !strings.Contains(strings.ToLower(name), pattern) {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

func (a *ElasticsearchAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	index := strings.TrimSpace(name)
	if index == "" {
		return DescribeResult{}, errors.New("index is required")
	}

	mappingRaw, _, err := a.client.do(ctx, ds, "GET", fmt.Sprintf("/%s/_mapping", urlPathEscape(index)), "")
	if err != nil {
		return DescribeResult{}, err
	}

	var mapping any
	if err := json.Unmarshal(mappingRaw, &mapping); err != nil {
		return DescribeResult{}, err
	}

	columns := flattenElasticsearchMappingColumns(mapping)
	sort.Slice(columns, func(i, j int) bool { return columns[i].Name < columns[j].Name })

	details := a.describeIndexDetailsBestEffort(ctx, ds, index)
	preview := a.describeIndexPreviewBestEffort(ctx, ds, index)

	return DescribeResult{
		Columns: columns,
		Indexes: []IndexInfo{},
		Details: details,
		Preview: preview,
	}, nil
}

func (a *ElasticsearchAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	_ = opts
	start := time.Now()

	stmt, err := parseElasticsearchStatement(statement)
	if err != nil {
		return QueryResult{}, err
	}
	stmt.Path, stmt.Body = ensureElasticsearchTrackTotalHits(stmt.Path, stmt.Body)
	stmt.Body = ensureElasticsearchTimeout(stmt.Path, stmt.Body)
	raw, _, err := a.client.do(ctx, ds, stmt.Method, stmt.Path, stmt.Body)
	if err != nil {
		return QueryResult{}, err
	}

	rows, meta := extractElasticsearchRowsAndMeta(raw)
	rowCount := int64(len(rows))
	elapsedMs := time.Since(start).Milliseconds()
	if meta != nil {
		if meta.TotalHits != nil {
			rowCount = *meta.TotalHits
		}
		if meta.TookMs != nil && *meta.TookMs > 0 {
			elapsedMs = *meta.TookMs
		}
	}
	var detail map[string]any
	if meta != nil && meta.PitID != nil && strings.TrimSpace(*meta.PitID) != "" {
		detail = map[string]any{"pitId": strings.TrimSpace(*meta.PitID)}
	}
	var sourceEntity string
	if targets := extractElasticsearchTargetsFromPath(stmt.Path); len(targets) > 0 {
		sourceEntity = strings.Join(targets, ",")
	}
	return QueryResult{
		Columns:      nil,
		Rows:         rows,
		RowCount:     rowCount,
		HasMore:      false,
		NextToken:    "",
		PrevToken:    "",
		ElapsedMs:    elapsedMs,
		Detail:       detail,
		SourceEntity: sourceEntity,
	}, nil
}

func ensureElasticsearchTrackTotalHits(path string, body string) (string, string) {
	parsed, err := url.Parse(path)
	if err != nil {
		return path, body
	}
	cleanPath := strings.TrimSpace(parsed.Path)
	if cleanPath == "" {
		candidate := strings.TrimSpace(path)
		if idx := strings.Index(candidate, "?"); idx >= 0 {
			candidate = candidate[:idx]
		}
		cleanPath = candidate
	}

	normalizedPath := strings.TrimPrefix(cleanPath, "/")
	if normalizedPath != "_search" && !strings.HasSuffix(normalizedPath, "/_search") {
		return path, body
	}

	params := parsed.Query()
	if params.Has("track_total_hits") {
		return path, body
	}

	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		params.Set("track_total_hits", "true")
		parsed.RawQuery = params.Encode()
		return parsed.String(), body
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return path, body
	}
	if obj == nil {
		return path, body
	}
	if _, ok := obj["track_total_hits"]; ok {
		return path, body
	}
	obj["track_total_hits"] = json.RawMessage("true")
	out, err := json.Marshal(obj)
	if err != nil {
		return path, body
	}
	return path, string(out)
}

// ensureElasticsearchTimeout injects a "timeout" field into _search request bodies
// if the user hasn't specified one. This prevents slow queries from consuming
// cluster resources indefinitely.
func ensureElasticsearchTimeout(path string, body string) string {
	normalizedPath := strings.TrimPrefix(strings.TrimSpace(path), "/")
	if idx := strings.Index(normalizedPath, "?"); idx >= 0 {
		normalizedPath = normalizedPath[:idx]
	}
	if normalizedPath != "_search" && !strings.HasSuffix(normalizedPath, "/_search") {
		return body
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return body
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return body
	}
	if _, ok := obj["timeout"]; ok {
		return body // user already specified timeout
	}
	obj["timeout"] = json.RawMessage(`"30s"`)
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return string(out)
}

func (a *ElasticsearchAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	_ = ctx
	_ = ds
	_ = statement
	return ExplainResult{}, ErrUnsupported
}

func (a *ElasticsearchAdapter) describeIndexDetailsBestEffort(ctx context.Context, ds datasource.DataSource, index string) []DetailItem {
	path := fmt.Sprintf("/_cat/indices/%s?format=json&h=index,docs.count,store.size,health,status", urlPathEscape(index))
	payload, _, err := a.client.do(ctx, ds, "GET", path, "")
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	out := make([]DetailItem, 0, 6)
	appendField := func(label, key string) {
		val, ok := row[key]
		if !ok {
			return
		}
		out = append(out, DetailItem{Label: label, Value: val})
	}
	appendField("Index", "index")
	appendField("Health", "health")
	appendField("Status", "status")
	appendField("Docs", "docs.count")
	appendField("Store", "store.size")
	return out
}

func (a *ElasticsearchAdapter) describeIndexPreviewBestEffort(ctx context.Context, ds datasource.DataSource, index string) any {
	path := fmt.Sprintf("/%s/_search?size=1&sort=_doc", urlPathEscape(index))
	payload, _, err := a.client.do(ctx, ds, "GET", path, "")
	if err != nil {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil
	}
	hits, _ := doc["hits"].(map[string]any)
	hitsArr, _ := hits["hits"].([]any)
	if len(hitsArr) == 0 {
		return nil
	}
	first, _ := hitsArr[0].(map[string]any)
	if first == nil {
		return nil
	}
	if source, ok := first["_source"]; ok {
		return source
	}
	return first
}

type elasticsearchSearchMeta struct {
	TotalHits *int64
	TookMs    *int64
	PitID     *string
}

func extractElasticsearchRowsAndMeta(raw []byte) ([]map[string]any, *elasticsearchSearchMeta) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return []map[string]any{{"result": trimmed}}, nil
	}

	switch v := payload.(type) {
	case []any:
		rows := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			} else {
				rows = append(rows, map[string]any{"value": item})
			}
		}
		return rows, nil
	case map[string]any:
		if hitsRaw, ok := v["hits"].(map[string]any); ok {
			if hitsArr, ok := hitsRaw["hits"].([]any); ok {
				rows := make([]map[string]any, 0, len(hitsArr))
				for _, hit := range hitsArr {
					if row, ok := hit.(map[string]any); ok {
						rows = append(rows, row)
					} else {
						rows = append(rows, map[string]any{"value": hit})
					}
				}
				meta := &elasticsearchSearchMeta{}
				if totalRaw, ok := hitsRaw["total"]; ok {
					switch total := totalRaw.(type) {
					case float64:
						val := int64(total)
						meta.TotalHits = &val
					case map[string]any:
						if valueRaw, ok := total["value"].(float64); ok {
							val := int64(valueRaw)
							meta.TotalHits = &val
						}
					}
				}
				if tookRaw, ok := v["took"].(float64); ok {
					val := int64(tookRaw)
					meta.TookMs = &val
				}
				if pitIDRaw, ok := v["pit_id"].(string); ok && strings.TrimSpace(pitIDRaw) != "" {
					pitID := strings.TrimSpace(pitIDRaw)
					meta.PitID = &pitID
				}
				return rows, meta
			}
		}
		return []map[string]any{v}, nil
	default:
		return []map[string]any{{"value": payload}}, nil
	}
}

func flattenElasticsearchMappingColumns(payload any) []ColumnInfo {
	root, ok := payload.(map[string]any)
	if !ok {
		return nil
	}

	props := findElasticsearchProperties(root)
	if props == nil {
		return nil
	}

	out := make([]ColumnInfo, 0)
	flattenElasticsearchProperties("", props, &out)
	return out
}

func findElasticsearchProperties(root map[string]any) map[string]any {
	for _, indexValue := range root {
		indexMap, ok := indexValue.(map[string]any)
		if !ok {
			continue
		}
		mappings, _ := indexMap["mappings"].(map[string]any)
		if mappings == nil {
			continue
		}
		if props, ok := mappings["properties"].(map[string]any); ok {
			return props
		}
		for _, v := range mappings {
			sub, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if props, ok := sub["properties"].(map[string]any); ok {
				return props
			}
		}
	}
	return nil
}

func flattenElasticsearchProperties(prefix string, props map[string]any, out *[]ColumnInfo) {
	keys := make([]string, 0, len(props))
	for key := range props {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		raw := props[key]
		fieldDef, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}

		typ := ""
		if rawType, ok := fieldDef["type"]; ok {
			typ = strings.TrimSpace(fmt.Sprint(rawType))
		} else if _, ok := fieldDef["properties"].(map[string]any); ok {
			typ = "object"
		}

		*out = append(*out, ColumnInfo{Name: name, DataType: typ, Nullable: "-"})

		if fieldsRaw, ok := fieldDef["fields"].(map[string]any); ok {
			flattenElasticsearchProperties(name, fieldsRaw, out)
		}
		if nestedProps, ok := fieldDef["properties"].(map[string]any); ok {
			flattenElasticsearchProperties(name, nestedProps, out)
		}
	}
}

func urlPathEscape(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(url.QueryEscape(part), "+", "%20")
	}
	return strings.Join(parts, "/")
}
