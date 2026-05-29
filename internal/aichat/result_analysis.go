package aichat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxStoredAnalysisRows  = 200
	maxStoredAnalysisBytes = 120_000
	maxStoredSummaryChars  = 4000
)

type storedAnalysisResult struct {
	CapturedAt time.Time

	DatasourceID   string
	DatasourceType string
	Database       string
	Statement      string

	RowCount      int64
	HasMore       bool
	NextToken     string
	PrevToken     string
	ElapsedMs     int64
	Columns       []string
	Rows          []map[string]any
	RowsTruncated bool

	ApproxBytes int
}

type analysisMemoryStore struct {
	mu      sync.Mutex
	results map[string]storedAnalysisResult
	summary map[string]string
}

func newAnalysisMemoryStore() *analysisMemoryStore {
	return &analysisMemoryStore{
		results: make(map[string]storedAnalysisResult),
		summary: make(map[string]string),
	}
}

func analysisQuestionFromArgs(req TurnRequest, args map[string]any) string {
	q := strings.TrimSpace(stringArg(args, "question", "prompt", "instruction"))
	if q != "" {
		return q
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if strings.ToLower(strings.TrimSpace(msg.Role)) != "user" {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func defaultAnalyzeResultAssistantMessage(locale uiLocale, stored storedAnalysisResult) string {
	rows := len(stored.Rows)
	if locale == uiLocaleZH {
		if stored.RowsTruncated {
			return fmt.Sprintf("我可以帮你分析刚才的查询结果（将发送前 %d 行给 AI 模型进行分析，需要你先在审批卡里确认）。", rows)
		}
		return fmt.Sprintf("我可以帮你分析刚才的查询结果（将发送 %d 行给 AI 模型进行分析，需要你先在审批卡里确认）。", rows)
	}
	if stored.RowsTruncated {
		return fmt.Sprintf("I can analyze the last query result (will send the first %d rows to the AI model; approval required).", rows)
	}
	return fmt.Sprintf("I can analyze the last query result (will send %d rows to the AI model; approval required).", rows)
}

func summarizeAnalyzeResult(locale uiLocale, stored storedAnalysisResult) string {
	rows := len(stored.Rows)
	if locale == uiLocaleZH {
		if stored.RowsTruncated {
			return fmt.Sprintf("将上次结果前 %d 行发送给 AI 进行分析（已截断）", rows)
		}
		return fmt.Sprintf("将上次结果 %d 行发送给 AI 进行分析", rows)
	}
	if stored.RowsTruncated {
		return fmt.Sprintf("Send first %d rows to AI for analysis (truncated)", rows)
	}
	return fmt.Sprintf("Send %d rows to AI for analysis", rows)
}

func sanitizeAnalyzeResultPayload(stored storedAnalysisResult) map[string]any {
	payload := map[string]any{
		"datasourceId": strings.TrimSpace(stored.DatasourceID),
		"database":     strings.TrimSpace(stored.Database),
		"rowCount":     stored.RowCount,
		"payloadRows":  len(stored.Rows),
		"truncated":    stored.RowsTruncated,
		"approxBytes":  stored.ApproxBytes,
	}
	if strings.TrimSpace(stored.DatasourceType) != "" {
		payload["datasourceType"] = strings.TrimSpace(stored.DatasourceType)
	}
	if !stored.CapturedAt.IsZero() {
		payload["capturedAt"] = stored.CapturedAt.Format(time.RFC3339)
	}
	return payload
}

func analysisSystemPrompt(locale uiLocale) string {
	if locale == uiLocaleZH {
		return strings.TrimSpace(`
你是 FutrixData 的数据分析助手。

用户已经明确批准把“上一次 AI 查询结果的部分行数据”发送给你做分析。

要求：
- 用中文输出
- 输出 Markdown（不要输出 JSON / 不要输出代码块包住整段回答）
- 不要泄露任何你看到的敏感原始值；如需举例，只能使用非常小的片段并进行适当脱敏
- 如果数据量被截断，明确说明“仅基于已提供的样本行”并给出后续建议（例如再筛选/聚合/增加字段等）
`)
	}

	return strings.TrimSpace(`
You are FutrixData's data analysis assistant.

The user has explicitly approved sending the (sampled) rows from the most recent AI query result for analysis.

Requirements:
- Respond in English
- Output Markdown (do not output JSON)
- Do not reveal sensitive raw values; if you must cite examples, keep them tiny and redact appropriately
- If rows are truncated, state that conclusions are based on the provided sample and suggest next steps
`)
}

func analysisUserContent(locale uiLocale, stored storedAnalysisResult, question string) string {
	rowsJSON, err := json.Marshal(stored.Rows)
	if err != nil {
		rowsJSON = []byte("[]")
	}

	q := strings.TrimSpace(question)
	if q == "" {
		if locale == uiLocaleZH {
			q = "请基于这些结果给出关键洞察、异常点、以及下一步建议。"
		} else {
			q = "Please provide key insights, anomalies, and next-step suggestions based on these results."
		}
	}

	var b strings.Builder
	if locale == uiLocaleZH {
		b.WriteString("用户问题：\n")
		b.WriteString(q + "\n\n")
		b.WriteString("结果元信息：\n")
		b.WriteString(fmt.Sprintf("- datasourceId: %s\n", stored.DatasourceID))
		if strings.TrimSpace(stored.Database) != "" {
			b.WriteString(fmt.Sprintf("- database: %s\n", stored.Database))
		}
		if stored.RowCount > 0 {
			b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
		}
		b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
		if stored.RowsTruncated {
			b.WriteString("- truncated: true\n")
		} else {
			b.WriteString("- truncated: false\n")
		}
		b.WriteString("\nrows(JSON)：\n")
		b.Write(rowsJSON)
		return strings.TrimSpace(b.String())
	}

	b.WriteString("User question:\n")
	b.WriteString(q + "\n\n")
	b.WriteString("Result metadata:\n")
	b.WriteString(fmt.Sprintf("- datasourceId: %s\n", stored.DatasourceID))
	if strings.TrimSpace(stored.Database) != "" {
		b.WriteString(fmt.Sprintf("- database: %s\n", stored.Database))
	}
	if stored.RowCount > 0 {
		b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
	}
	b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
	if stored.RowsTruncated {
		b.WriteString("- truncated: true\n")
	} else {
		b.WriteString("- truncated: false\n")
	}
	b.WriteString("\nrows(JSON):\n")
	b.Write(rowsJSON)
	return strings.TrimSpace(b.String())
}

func (s *analysisMemoryStore) PutResult(conversationID string, effect ConsoleResultEffect) {
	if s == nil {
		return
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return
	}

	stored := makeStoredAnalysisResult(effect)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[conversationID] = stored
}

func (s *analysisMemoryStore) GetResult(conversationID string) (storedAnalysisResult, bool) {
	if s == nil {
		return storedAnalysisResult{}, false
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return storedAnalysisResult{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.results[conversationID]
	return item, ok
}

func (s *analysisMemoryStore) PutSummary(conversationID string, summary string) {
	if s == nil {
		return
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}

	if len([]rune(summary)) > maxStoredSummaryChars {
		runes := []rune(summary)
		summary = string(runes[:maxStoredSummaryChars]) + "…"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary[conversationID] = summary
}

func (s *analysisMemoryStore) GetSummary(conversationID string) (string, bool) {
	if s == nil {
		return "", false
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.summary[conversationID]
	return value, ok
}

func makeStoredAnalysisResult(effect ConsoleResultEffect) storedAnalysisResult {
	source := effect.Result
	if effect.Result.AgentView != nil {
		source = *effect.Result.AgentView
	}

	rows := sanitizeRowsForJSON(source.Rows)
	columns := append([]string(nil), source.Columns...)

	cappedRows := rows
	if len(cappedRows) > maxStoredAnalysisRows {
		cappedRows = cappedRows[:maxStoredAnalysisRows]
	}
	cappedRows, approxBytes, truncatedByBytes := capRowsByJSONBytes(cappedRows, maxStoredAnalysisBytes)
	truncated := truncatedByBytes || len(cappedRows) < len(rows)

	return storedAnalysisResult{
		CapturedAt:     time.Now(),
		DatasourceID:   strings.TrimSpace(effect.DatasourceID),
		DatasourceType: strings.TrimSpace(effect.DatasourceType),
		Database:       strings.TrimSpace(effect.Database),
		Statement:      strings.TrimSpace(effect.Statement),
		RowCount:       source.RowCount,
		HasMore:        source.HasMore,
		NextToken:      strings.TrimSpace(source.NextToken),
		PrevToken:      strings.TrimSpace(source.PrevToken),
		ElapsedMs:      source.ElapsedMs,
		Columns:        columns,
		Rows:           cappedRows,
		RowsTruncated:  truncated,
		ApproxBytes:    approxBytes,
	}
}

func capRowsByJSONBytes(rows []map[string]any, maxBytes int) ([]map[string]any, int, bool) {
	if maxBytes <= 0 {
		return nil, 0, len(rows) > 0
	}
	if len(rows) == 0 {
		return rows, 0, false
	}

	encoded, err := json.Marshal(rows)
	if err == nil && len(encoded) <= maxBytes {
		return rows, len(encoded), false
	}

	// Reduce rows until it fits.
	lo := 1
	hi := len(rows)
	best := 1

	for lo <= hi {
		mid := (lo + hi) / 2
		slice := rows[:mid]
		encoded, err := json.Marshal(slice)
		if err != nil {
			// If encoding fails, shrink aggressively.
			hi = mid - 1
			continue
		}
		size := len(encoded)
		if size <= maxBytes {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if best < 1 {
		best = 1
	}
	encoded, err = json.Marshal(rows[:best])
	if err != nil {
		return rows[:1], 0, true
	}
	return rows[:best], len(encoded), true
}

func sanitizeRowsForJSON(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			out = append(out, map[string]any{})
			continue
		}
		sanitized := make(map[string]any, len(row))
		for k, v := range row {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			sanitized[key] = sanitizeValueForJSON(v, 0)
		}
		out = append(out, sanitized)
	}
	return out
}

func sanitizeValueForJSON(value any, depth int) any {
	if value == nil {
		return nil
	}
	if depth > 8 {
		return fmt.Sprint(value)
	}

	switch v := value.(type) {
	case string:
		return v
	case bool:
		return v
	case int:
		return v
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return uint64(v)
	case uint8:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint32:
		return uint64(v)
	case uint64:
		return v
	case float32:
		return float64(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprint(v)
		}
		return v
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case []byte:
		if len(v) == 0 {
			return ""
		}
		// Keep it readable-ish and JSON-safe.
		return base64.StdEncoding.EncodeToString(v)
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, vv := range v {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			m[key] = sanitizeValueForJSON(vv, depth+1)
		}
		return m
	case []any:
		arr := make([]any, 0, len(v))
		for _, item := range v {
			arr = append(arr, sanitizeValueForJSON(item, depth+1))
		}
		return arr
	default:
		if s, ok := value.(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprint(value)
	}
}

type numericStats struct {
	count int
	sum   float64
	min   float64
	max   float64
}

func localAnalyzeResultSummary(locale uiLocale, stored storedAnalysisResult) string {
	fields := make(map[string]struct{})
	for _, row := range stored.Rows {
		for key := range row {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			fields[key] = struct{}{}
		}
	}

	fieldList := make([]string, 0, len(fields))
	for key := range fields {
		fieldList = append(fieldList, key)
	}
	sort.Strings(fieldList)

	groupKey := ""
	if _, ok := fields["_id"]; ok {
		groupKey = "_id"
	} else if _, ok := fields["ownerId"]; ok {
		groupKey = "ownerId"
	} else {
		for _, candidate := range fieldList {
			if strings.HasSuffix(strings.ToLower(candidate), "id") {
				groupKey = candidate
				break
			}
		}
		if groupKey == "" && len(fieldList) > 0 {
			groupKey = fieldList[0]
		}
	}

	weightKey := ""
	for _, candidate := range []string{"count", "cnt", "total", "totalCount"} {
		if _, ok := fields[candidate]; ok {
			weightKey = candidate
			break
		}
	}

	stats := make(map[string]*numericStats, len(fieldList))
	for _, key := range fieldList {
		stats[key] = &numericStats{}
	}
	type groupItem struct {
		Key    string
		Weight float64
	}
	var groups []groupItem

	for _, row := range stored.Rows {
		for key, value := range row {
			num, ok := toFloat64(value)
			if !ok {
				continue
			}
			stat := stats[key]
			if stat.count == 0 {
				stat.min = num
				stat.max = num
			} else {
				if num < stat.min {
					stat.min = num
				}
				if num > stat.max {
					stat.max = num
				}
			}
			stat.count++
			stat.sum += num
		}

		if groupKey != "" && weightKey != "" {
			id := redactForSummary(row[groupKey])
			weight, ok := toFloat64(row[weightKey])
			if ok && id != "" {
				groups = append(groups, groupItem{Key: id, Weight: weight})
			}
		}
	}

	if len(groups) > 0 {
		sort.Slice(groups, func(i, j int) bool {
			if groups[i].Weight == groups[j].Weight {
				return groups[i].Key < groups[j].Key
			}
			return groups[i].Weight > groups[j].Weight
		})
	}

	var b strings.Builder
	if locale == uiLocaleZH {
		b.WriteString("### 本地摘要（AI 未返回可用内容）\n")
		if stored.RowCount > 0 {
			b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
		}
		b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
		if stored.RowsTruncated {
			b.WriteString("- truncated: true\n")
		}
		if len(fieldList) > 0 {
			b.WriteString("\n**字段**：")
			b.WriteString(strings.Join(fieldList, ", "))
			b.WriteString("\n")
		}
		if len(groups) > 0 {
			topN := 10
			if len(groups) < topN {
				topN = len(groups)
			}
			b.WriteString(fmt.Sprintf("\n**Top %s（按 %s）**：\n", groupKey, weightKey))
			for i := 0; i < topN; i++ {
				b.WriteString(fmt.Sprintf("- %s: %s=%.0f\n", groups[i].Key, weightKey, groups[i].Weight))
			}
			b.WriteString("\n建议：如果你希望按数量从大到小排序，可在聚合末尾使用 `{ $sort: { " + weightKey + ": -1 } }`。\n")
		}

		wroteStats := 0
		for _, key := range fieldList {
			stat := stats[key]
			if stat == nil || stat.count == 0 {
				continue
			}
			if wroteStats == 0 {
				b.WriteString("\n**数值字段范围（样本）**：\n")
			}
			avg := stat.sum / float64(stat.count)
			b.WriteString(fmt.Sprintf("- %s: min=%.3g max=%.3g avg=%.3g (n=%d)\n", key, stat.min, stat.max, avg, stat.count))
			wroteStats++
			if wroteStats >= 5 {
				break
			}
		}
		return strings.TrimSpace(b.String())
	}

	b.WriteString("### Local summary (AI returned empty)\n")
	if stored.RowCount > 0 {
		b.WriteString(fmt.Sprintf("- rowCount: %d\n", stored.RowCount))
	}
	b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(stored.Rows)))
	if stored.RowsTruncated {
		b.WriteString("- truncated: true\n")
	}
	if len(fieldList) > 0 {
		b.WriteString("\n**Fields**: ")
		b.WriteString(strings.Join(fieldList, ", "))
		b.WriteString("\n")
	}
	if len(groups) > 0 {
		topN := 10
		if len(groups) < topN {
			topN = len(groups)
		}
		b.WriteString(fmt.Sprintf("\n**Top %s (by %s)**:\n", groupKey, weightKey))
		for i := 0; i < topN; i++ {
			b.WriteString(fmt.Sprintf("- %s: %s=%.0f\n", groups[i].Key, weightKey, groups[i].Weight))
		}
		b.WriteString(fmt.Sprintf("\nTip: if you want to rank by %s desc, add `{ $sort: { %s: -1 } }` to the end of the pipeline.\n", weightKey, weightKey))
	}

	wroteStats := 0
	for _, key := range fieldList {
		stat := stats[key]
		if stat == nil || stat.count == 0 {
			continue
		}
		if wroteStats == 0 {
			b.WriteString("\n**Numeric ranges (sample)**:\n")
		}
		avg := stat.sum / float64(stat.count)
		b.WriteString(fmt.Sprintf("- %s: min=%.3g max=%.3g avg=%.3g (n=%d)\n", key, stat.min, stat.max, avg, stat.count))
		wroteStats++
		if wroteStats >= 5 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	default:
		return 0, false
	}
}

func redactForSummary(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return redactString(v)
	default:
		return redactString(fmt.Sprint(v))
	}
}

func redactString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 10 {
		return value
	}
	pfx := string(runes[:4])
	sfx := string(runes[len(runes)-2:])
	return pfx + "…" + sfx
}
