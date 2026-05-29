package console

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"futrixdata/platform/internal/datasource"
)

const DefaultWritePreviewElevatedApprovalThreshold int64 = 100

type WritePreviewOptions struct {
	ElevatedApprovalThreshold int64
}

type WritePreview struct {
	Operation                string           `json:"operation"`
	TargetEntity             string           `json:"targetEntity"`
	EstimatedAffectedRows    int64            `json:"estimatedAffectedRows"`
	SampleRows               []map[string]any `json:"sampleRows,omitempty"`
	RequiresElevatedApproval bool             `json:"requiresElevatedApproval"`
	ThresholdRows            int64            `json:"thresholdRows,omitempty"`
}

type WritePreviewer interface {
	PreviewWrite(ctx context.Context, ds datasource.DataSource, statement string, opts WritePreviewOptions) (WritePreview, error)
}

func (m *Manager) PreviewWrite(ctx context.Context, ds datasource.DataSource, statement string, opts WritePreviewOptions) (WritePreview, error) {
	done := DatasourceTimingStage(ctx, "manager.preview_write.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return WritePreview{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.preview_write.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return WritePreview{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	previewer, ok := adapter.(WritePreviewer)
	if !ok {
		return WritePreview{}, ErrUnsupported
	}
	done = DatasourceTimingStage(ctx, "manager.preview_write.adapter_call")
	preview, err := previewer.PreviewWrite(ctx, ds, statement, opts)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return WritePreview{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("estimated_rows", preview.EstimatedAffectedRows))
	return preview, nil
}

func (a *SQLAdapter) PreviewWrite(ctx context.Context, ds datasource.DataSource, statement string, opts WritePreviewOptions) (WritePreview, error) {
	if ds.Type != datasource.TypeMySQL || normalizeSQLDialectName(a.dialect) != "mysql" {
		return WritePreview{}, ErrUnsupported
	}
	done := DatasourceTimingStage(ctx, "sql.write_preview_parse")
	spec, err := mysqlWritePreviewSpec(statement)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return WritePreview{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("operation", spec.operation), DatasourceTimingKV("target_entity", spec.targetEntity))
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return WritePreview{}, err
	}
	done = DatasourceTimingStage(ctx, "sql.write_preview_count")
	estimated, err := querySingleCount(ctx, db, spec.countStatement)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return WritePreview{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("estimated_rows", estimated))
	if spec.hasLimit && estimated > spec.limitRows {
		estimated = spec.limitRows
	}
	threshold := opts.ElevatedApprovalThreshold
	if threshold <= 0 {
		threshold = DefaultWritePreviewElevatedApprovalThreshold
	}
	return WritePreview{
		Operation:                spec.operation,
		TargetEntity:             spec.targetEntity,
		EstimatedAffectedRows:    estimated,
		RequiresElevatedApproval: estimated > threshold,
		ThresholdRows:            threshold,
	}, nil
}

type mysqlWritePreviewStatementSpec struct {
	operation      string
	targetEntity   string
	countStatement string
	hasLimit       bool
	limitRows      int64
}

func mysqlWritePreviewSpec(statement string) (mysqlWritePreviewStatementSpec, error) {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return mysqlWritePreviewStatementSpec{}, errors.New("statement required")
	}
	if countTopLevelSQLStatements(trimmed, "mysql") != 1 {
		return mysqlWritePreviewStatementSpec{}, ErrUnsupported
	}
	tokens := scanTopLevelTokens(trimmed)
	operation := mysqlWritePreviewOperation(tokens)
	if operation != "delete" && operation != "update" {
		return mysqlWritePreviewStatementSpec{}, ErrUnsupported
	}
	whereIdx := tokenIndex(tokens, "where", 0)
	if whereIdx < 0 {
		return mysqlWritePreviewStatementSpec{}, fmt.Errorf("%s without WHERE is not eligible for write preview", strings.ToUpper(operation))
	}
	analysis, _ := AnalyzeSQL(mysqlWritePreviewParseStatement(trimmed), "mysql")
	whereToken := tokens[whereIdx]
	targetSQL, err := mysqlWritePreviewTargetSQL(trimmed, tokens, operation, whereIdx)
	if err != nil {
		return mysqlWritePreviewStatementSpec{}, err
	}
	countPrefix := mysqlWritePreviewCountPrefix(trimmed, tokens, operation)
	whereEnd := len(trimmed)
	for _, keyword := range []string{"order", "limit"} {
		if idx := tokenIndex(tokens, keyword, whereIdx+1); idx >= 0 && tokens[idx].start < whereEnd {
			whereEnd = tokens[idx].start
		}
	}
	whereSQL := strings.TrimSpace(trimmed[whereToken.end:whereEnd])
	whereSQL = strings.TrimSpace(strings.TrimSuffix(whereSQL, ";"))
	if whereSQL == "" {
		return mysqlWritePreviewStatementSpec{}, fmt.Errorf("%s without WHERE is not eligible for write preview", strings.ToUpper(operation))
	}
	targetEntity := ""
	if analysis != nil {
		targetEntity = strings.TrimSpace(analysis.PrimaryTable)
	}
	if targetEntity == "" {
		targetEntity = mysqlWritePreviewEntityFromTargetSQL(targetSQL)
	}
	limitRows := int64(0)
	hasLimit := false
	limitInfo := findTopLevelLimit(trimmed)
	if limitInfo.found && limitInfo.parsed && limitInfo.count >= 0 {
		hasLimit = true
		limitRows = limitInfo.count
	}
	return mysqlWritePreviewStatementSpec{
		operation:      operation,
		targetEntity:   strings.Trim(targetEntity, "`"),
		countStatement: strings.TrimSpace(fmt.Sprintf("%s SELECT COUNT(*) AS estimatedAffectedRows FROM %s WHERE %s", countPrefix, targetSQL, whereSQL)),
		hasLimit:       hasLimit,
		limitRows:      limitRows,
	}, nil
}

func mysqlWritePreviewOperation(tokens []sqlToken) string {
	if len(tokens) == 0 {
		return ""
	}
	rootIdx := 0
	if tokens[0].value == "with" {
		rootIdx = -1
		for i := 1; i < len(tokens); i++ {
			if tokens[i].value == "update" || tokens[i].value == "delete" {
				rootIdx = i
				break
			}
		}
		if rootIdx < 0 {
			return ""
		}
	}
	switch tokens[rootIdx].value {
	case "update", "delete":
		return tokens[rootIdx].value
	default:
		return ""
	}
}

func mysqlWritePreviewTargetSQL(statement string, tokens []sqlToken, operation string, whereIdx int) (string, error) {
	if len(tokens) == 0 {
		return "", ErrUnsupported
	}
	var start, end int
	switch operation {
	case "delete":
		deleteIdx := tokenIndex(tokens, "delete", 0)
		if deleteIdx < 0 || deleteIdx >= whereIdx {
			return "", ErrUnsupported
		}
		fromIdx := tokenIndex(tokens, "from", deleteIdx+1)
		if fromIdx < 0 || fromIdx >= whereIdx {
			return "", ErrUnsupported
		}
		if usingIdx := tokenIndex(tokens, "using", fromIdx+1); usingIdx >= 0 && usingIdx < whereIdx {
			return "", ErrUnsupported
		}
		start = tokens[fromIdx].end
		end = tokens[whereIdx].start
		if mysqlWritePreviewHasMultiTableTarget(statement[start:end], tokens, fromIdx+1, whereIdx) {
			return "", ErrUnsupported
		}
	case "update":
		updateIdx := tokenIndex(tokens, "update", 0)
		if updateIdx < 0 || updateIdx >= whereIdx {
			return "", ErrUnsupported
		}
		setIdx := tokenIndex(tokens, "set", updateIdx+1)
		if setIdx < 0 || setIdx >= whereIdx {
			return "", ErrUnsupported
		}
		targetIdx := updateIdx + 1
		for targetIdx < setIdx && isMySQLUpdateModifier(tokens[targetIdx].value) {
			targetIdx++
		}
		if targetIdx >= setIdx {
			return "", ErrUnsupported
		}
		start = tokens[targetIdx].start
		end = tokens[setIdx].start
		if mysqlWritePreviewHasMultiTableTarget(statement[start:end], tokens, targetIdx, setIdx) {
			return "", ErrUnsupported
		}
	default:
		return "", ErrUnsupported
	}
	targetSQL := strings.TrimSpace(statement[start:end])
	targetSQL = strings.TrimSpace(strings.TrimSuffix(targetSQL, ";"))
	if targetSQL == "" {
		return "", ErrUnsupported
	}
	return targetSQL, nil
}

func mysqlWritePreviewHasMultiTableTarget(targetSQL string, tokens []sqlToken, startIdx, endIdx int) bool {
	if hasTopLevelComma(targetSQL) {
		return true
	}
	for i := startIdx; i < endIdx && i < len(tokens); i++ {
		switch tokens[i].value {
		case "join", "straight_join", "left", "right", "inner", "outer", "cross", "natural":
			if tokens[i].value == "join" && isMySQLIndexHintJoinToken(tokens, i, startIdx) {
				continue
			}
			return true
		}
	}
	return false
}

func isMySQLIndexHintJoinToken(tokens []sqlToken, idx, startIdx int) bool {
	if idx-1 < startIdx || tokens[idx-1].value != "for" {
		return false
	}
	for i := idx - 2; i >= startIdx; i-- {
		switch tokens[i].value {
		case "index", "key":
			if i-1 >= startIdx {
				switch tokens[i-1].value {
				case "use", "ignore", "force":
					return true
				}
			}
			return false
		case "join", "straight_join", "left", "right", "inner", "outer", "cross", "natural", ",":
			return false
		}
	}
	return false
}

func hasTopLevelComma(sql string) bool {
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false
	for i := 0; i < len(sql); {
		ch := sql[i]
		if inSingle {
			if ch == '\\' && i+1 < len(sql) {
				i += 2
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			if ch == '\\' && i+1 < len(sql) {
				i += 2
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			i++
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			i++
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inBacktick = true
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return true
			}
		}
		i++
	}
	return false
}

func mysqlWritePreviewParseStatement(statement string) string {
	tokens := scanTopLevelTokens(statement)
	if len(tokens) < 3 {
		return statement
	}
	opIdx := tokenIndex(tokens, "update", 0)
	if opIdx < 0 {
		opIdx = tokenIndex(tokens, "delete", 0)
	}
	if opIdx < 0 || opIdx+1 >= len(tokens) {
		return statement
	}
	isModifier := isMySQLUpdateModifier
	if tokens[opIdx].value == "delete" {
		isModifier = isMySQLDeleteModifier
	}
	targetIdx := opIdx + 1
	for targetIdx < len(tokens) && isModifier(tokens[targetIdx].value) {
		targetIdx++
	}
	if targetIdx == opIdx+1 || targetIdx >= len(tokens) {
		return statement
	}
	return strings.TrimSpace(statement[:tokens[opIdx].end]) + " " + statement[tokens[targetIdx].start:]
}

func mysqlWritePreviewCountPrefix(statement string, tokens []sqlToken, operation string) string {
	if len(tokens) == 0 || tokens[0].value != "with" {
		return ""
	}
	opIdx := tokenIndex(tokens, operation, 0)
	if opIdx <= 0 {
		return ""
	}
	return strings.TrimSpace(statement[:tokens[opIdx].start])
}

func isMySQLUpdateModifier(token string) bool {
	switch token {
	case "low_priority", "ignore":
		return true
	default:
		return false
	}
}

func isMySQLDeleteModifier(token string) bool {
	switch token {
	case "low_priority", "quick", "ignore":
		return true
	default:
		return false
	}
}

func tokenIndex(tokens []sqlToken, value string, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].value == value {
			return i
		}
	}
	return -1
}

func mysqlWritePreviewEntityFromTargetSQL(targetSQL string) string {
	fields := strings.Fields(targetSQL)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "`")
}

func querySingleCount(ctx context.Context, db *sql.DB, statement string) (int64, error) {
	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errors.New("write preview count returned no rows")
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
