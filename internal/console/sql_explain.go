package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"futrixdata/platform/internal/datasource"
)

func (a *SQLAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return ExplainResult{}, err
	}

	done := DatasourceTimingStage(ctx, "sql.explain_build_query")
	query, err := a.explainQuery(statement)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return ExplainResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))

	done = DatasourceTimingStage(ctx, "sql.explain_query_context")
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return ExplainResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	defer rows.Close()

	switch a.dialect {
	case "mysql":
		done = DatasourceTimingStage(ctx, "sql.explain_read_plan")
		_, data, err := readRows(rows)
		if err != nil {
			done(DatasourceTimingKV("status", "error"))
			return ExplainResult{}, err
		}
		stages := make([]string, 0, 4)
		indexes := make([]string, 0, 2)
		stageSeen := make(map[string]bool)
		indexSeen := make(map[string]bool)
		hasFullScan := false
		var maxRows int64
		var maxFullScanRows int64

		for _, row := range data {
			keyName := strings.TrimSpace(asString(row["key"]))
			if keyName != "" && !indexSeen[keyName] {
				indexSeen[keyName] = true
				indexes = append(indexes, keyName)
			}

			rowsStr := strings.TrimSpace(asString(row["rows"]))
			var rowsValue int64
			if rowsStr != "" {
				if parsed, err := strconv.ParseInt(rowsStr, 10, 64); err == nil {
					rowsValue = parsed
					if rowsValue > maxRows {
						maxRows = rowsValue
					}
				}
			}

			typ := strings.ToUpper(strings.TrimSpace(asString(row["type"])))
			if typ != "" {
				stage := mysqlTypeStage(typ)
				if stage != "" && !stageSeen[stage] {
					stageSeen[stage] = true
					stages = append(stages, stage)
				}
			}

			if typ == "ALL" {
				hasFullScan = true
				if rowsValue > maxFullScanRows {
					maxFullScanRows = rowsValue
				}
			}
		}

		result := ExplainResult{
			UsesIndex:         !hasFullScan && len(indexes) > 0,
			Indexes:           indexes,
			Stages:            stages,
			TotalDocsExamined: maxRows,
			MaxSeqScanRows:    maxFullScanRows,
			Detail:            data,
		}
		done(
			DatasourceTimingKV("status", "ok"),
			DatasourceTimingKV("rows", len(data)),
			DatasourceTimingKV("uses_index", result.UsesIndex),
			DatasourceTimingKV("indexes", len(indexes)),
			DatasourceTimingKV("max_rows", maxRows),
			DatasourceTimingKV("max_full_scan_rows", maxFullScanRows),
		)
		return result, nil
	case "postgres":
		done = DatasourceTimingStage(ctx, "sql.explain_read_plan")
		_, data, err := readRows(rows)
		if err != nil {
			done(DatasourceTimingKV("status", "error"))
			return ExplainResult{}, err
		}
		detail, err := decodePostgresExplainJSON(data)
		if err != nil {
			done(DatasourceTimingKV("status", "error"))
			return ExplainResult{}, err
		}
		stats := parsePostgresExplainJSON(detail)
		result := ExplainResult{
			UsesIndex:         stats.usesIndex,
			Indexes:           stats.indexes,
			Stages:            stats.stages,
			TotalDocsExamined: stats.maxRows,
			MaxSeqScanRows:    stats.maxSeqScanRows,
			TotalCost:         stats.totalCost,
			Detail:            detail,
		}
		done(
			DatasourceTimingKV("status", "ok"),
			DatasourceTimingKV("rows", len(data)),
			DatasourceTimingKV("uses_index", result.UsesIndex),
			DatasourceTimingKV("indexes", len(result.Indexes)),
			DatasourceTimingKV("max_rows", result.TotalDocsExamined),
			DatasourceTimingKV("max_full_scan_rows", result.MaxSeqScanRows),
		)
		return result, nil
	default:
		return ExplainResult{}, ErrUnsupported
	}
}

func (a *SQLAdapter) explainQuery(statement string) (string, error) {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return "", errors.New("empty statement")
	}
	if a.dialect != "postgres" {
		return "EXPLAIN " + trimmed, nil
	}

	analyze := false
	if rest, ok := cutLeadingKeyword(trimmed, "analyze"); ok {
		analyze = true
		trimmed = strings.TrimSpace(rest)
	}
	if trimmed == "" {
		return "", errors.New("empty statement")
	}

	options := "FORMAT JSON"
	if analyze {
		options = "ANALYZE TRUE, FORMAT JSON"
	}
	return fmt.Sprintf("EXPLAIN (%s) %s", options, trimmed), nil
}

func mysqlTypeStage(typ string) string {
	switch strings.ToUpper(strings.TrimSpace(typ)) {
	case "ALL":
		return "FULL TABLE SCAN"
	case "INDEX":
		return "FULL INDEX SCAN"
	case "RANGE":
		return "RANGE SCAN"
	case "REF":
		return "INDEX LOOKUP"
	case "EQ_REF":
		return "UNIQUE LOOKUP"
	case "CONST":
		return "CONST"
	case "SYSTEM":
		return "SYSTEM"
	default:
		return strings.ToUpper(strings.TrimSpace(typ))
	}
}

func decodePostgresExplainJSON(rows []map[string]any) (any, error) {
	if len(rows) == 0 {
		return []any{}, nil
	}

	raw := ""
	for _, row := range rows {
		for _, value := range row {
			serialized := strings.TrimSpace(asString(value))
			if serialized == "" {
				continue
			}
			raw = serialized
			break
		}
		if raw != "" {
			break
		}
	}
	if raw == "" {
		return []any{}, nil
	}

	var detail any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&detail); err != nil {
		return nil, fmt.Errorf("parse postgres explain json: %w", err)
	}
	return detail, nil
}

type postgresExplainStats struct {
	indexes        []string
	stages         []string
	usesIndex      bool
	maxRows        int64
	maxSeqScanRows int64
	totalCost      float64
}

func parsePostgresExplainJSON(detail any) postgresExplainStats {
	indexes := make([]string, 0, 2)
	stages := make([]string, 0, 4)
	indexSeen := make(map[string]bool)
	stageSeen := make(map[string]bool)
	hasSeqScan := false
	hasIndexAccess := false
	var maxRows int64
	var maxSeqScanRows int64
	var totalCost float64
	rootSeen := false

	walkPostgresExplainPlans(detail, func(node map[string]any) {
		nodeType := strings.TrimSpace(asString(node["Node Type"]))
		lowerNode := strings.ToLower(nodeType)
		if nodeType != "" {
			stageName := postgresNodeStage(nodeType, node["Scan Direction"])
			if stageName != "" && !stageSeen[stageName] {
				stageSeen[stageName] = true
				stages = append(stages, stageName)
			}
			if strings.Contains(lowerNode, "seq scan") {
				hasSeqScan = true
				if planRows, ok := toInt64(node["Plan Rows"]); ok && planRows > maxSeqScanRows {
					maxSeqScanRows = planRows
				}
			}
			if strings.Contains(lowerNode, "index scan") || strings.Contains(lowerNode, "index only scan") {
				hasIndexAccess = true
			}
		}

		indexName := strings.TrimSpace(asString(node["Index Name"]))
		if indexName != "" {
			hasIndexAccess = true
			if !indexSeen[indexName] {
				indexSeen[indexName] = true
				indexes = append(indexes, indexName)
			}
		}

		// Use the root node's row estimate as the effective examined scope,
		// and extract Total Cost from the root node.
		// Child nodes' Plan Rows don't reflect actual scope when parent
		// nodes (e.g., Limit) constrain execution.
		if !rootSeen {
			rootSeen = true
			for _, key := range []string{"Actual Rows", "Plan Rows"} {
				rawRows, ok := node[key]
				if !ok {
					continue
				}
				rowsValue, ok := toInt64(rawRows)
				if ok && rowsValue > maxRows {
					maxRows = rowsValue
				}
			}
			if cost, ok := toFloat64(node["Total Cost"]); ok {
				totalCost = cost
			}
		}
	})

	return postgresExplainStats{
		indexes:        indexes,
		stages:         stages,
		usesIndex:      !hasSeqScan && hasIndexAccess,
		maxRows:        maxRows,
		maxSeqScanRows: maxSeqScanRows,
		totalCost:      totalCost,
	}
}

func postgresNodeStage(nodeType string, scanDirection any) string {
	normalized := strings.TrimSpace(nodeType)
	if normalized == "" {
		return ""
	}
	direction := strings.TrimSpace(asString(scanDirection))
	if strings.EqualFold(direction, "backward") {
		switch strings.ToLower(normalized) {
		case "index scan", "index only scan":
			return normalized + " Backward"
		}
	}
	return normalized
}

func walkPostgresExplainPlans(detail any, visit func(map[string]any)) {
	switch typed := detail.(type) {
	case []any:
		for _, item := range typed {
			walkPostgresExplainPlans(item, visit)
		}
	case map[string]any:
		if plan, ok := typed["Plan"]; ok {
			if root, ok := plan.(map[string]any); ok {
				walkPostgresExplainNode(root, visit)
				return
			}
		}
		if _, ok := typed["Node Type"]; ok {
			walkPostgresExplainNode(typed, visit)
			return
		}
		for _, value := range typed {
			walkPostgresExplainPlans(value, visit)
		}
	}
}

func walkPostgresExplainNode(node map[string]any, visit func(map[string]any)) {
	visit(node)

	children, ok := node["Plans"]
	if !ok {
		return
	}
	list, ok := children.([]any)
	if !ok {
		return
	}
	for _, child := range list {
		childNode, ok := child.(map[string]any)
		if !ok {
			continue
		}
		walkPostgresExplainNode(childNode, visit)
	}
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed, true
		}
		if parsed, err := v.Float64(); err == nil {
			return int64(parsed), true
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return parsed, true
		}
		if parsed, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return int64(parsed), true
		}
	}
	return 0, false
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed, true
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
