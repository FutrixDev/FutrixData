package sensitivity

import (
	"sort"
	"strings"

	"futrixdata/platform/internal/console"
)

// ApplyQueryResultMasking mutates a query result in place for programmatic
// consumers that must see masked data by default.
func ApplyQueryResultMasking(mp *MaskingProcessor, datasourceID string, result *console.QueryResult) {
	if mp == nil || result == nil {
		return
	}

	var masked []string
	usedSQLMetadata := len(result.ColumnMeta) > 0 && len(result.RowValues) > 0
	if len(result.ColumnMeta) > 0 && len(result.RowValues) > 0 {
		masked = mp.MaskSQLQueryResult(datasourceID, result.ColumnMeta, result.RowValues)
		if len(masked) > 0 {
			applyMaskedColumnState(result.ColumnMeta, masked)
			result.Rows = rebuildRowsFromOrderedValues(result.ColumnMeta, result.RowValues)
		}
	}
	if len(masked) == 0 && len(result.Rows) > 0 && (!usedSQLMetadata || sqlMaskingMetadataIncomplete(result.ColumnMeta)) {
		entityHint := strings.TrimSpace(result.SourceEntity)
		if entityHint == "" {
			entityHint = inferEntityHintFromColumnMeta(result.ColumnMeta)
		}
		if entityHint != "" {
			masked = mp.MaskQueryResult(datasourceID, entityHint, result.Columns, result.Rows)
		} else {
			masked = maskQueryResultAcrossAllEntities(mp, datasourceID, result.Columns, result.Rows)
		}
		if len(masked) > 0 {
			applyMaskedColumnState(result.ColumnMeta, masked)
			if len(result.ColumnMeta) > 0 && len(result.RowValues) > 0 {
				result.RowValues = rebuildOrderedValuesFromRows(result.ColumnMeta, result.Rows)
			}
		}
	}
	if len(masked) > 0 {
		result.MaskedColumns = masked
	}
}

func applyMaskedColumnState(columns []console.ResultColumn, masked []string) {
	if len(columns) == 0 || len(masked) == 0 {
		return
	}
	maskSet := make(map[string]struct{}, len(masked))
	for _, key := range masked {
		maskSet[key] = struct{}{}
	}
	for i := range columns {
		_, columns[i].Masked = maskSet[columns[i].Key]
	}
}

func rebuildRowsFromOrderedValues(columns []console.ResultColumn, rowValues [][]any) []map[string]any {
	if len(columns) == 0 || len(rowValues) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(rowValues))
	for _, values := range rowValues {
		row := make(map[string]any, len(columns))
		for idx, column := range columns {
			if idx >= len(values) {
				row[column.Key] = nil
				continue
			}
			row[column.Key] = values[idx]
		}
		rows = append(rows, row)
	}
	return rows
}

func sqlMaskingMetadataIncomplete(columns []console.ResultColumn) bool {
	if len(columns) == 0 {
		return true
	}
	for _, column := range columns {
		if column.ConservativeMask || len(column.Origins) > 0 {
			return false
		}
	}
	return true
}

func rebuildOrderedValuesFromRows(columns []console.ResultColumn, rows []map[string]any) [][]any {
	if len(columns) == 0 || len(rows) == 0 {
		return nil
	}
	rowValues := make([][]any, 0, len(rows))
	for _, row := range rows {
		values := make([]any, len(columns))
		for idx, column := range columns {
			values[idx] = row[column.Key]
		}
		rowValues = append(rowValues, values)
	}
	return rowValues
}

func inferEntityHintFromColumnMeta(columns []console.ResultColumn) string {
	if len(columns) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(columns))
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		for _, origin := range column.Origins {
			table := strings.TrimSpace(origin.Table)
			if table == "" {
				continue
			}
			name := table
			if schema := strings.TrimSpace(origin.Schema); schema != "" {
				name = schema + "." + table
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func maskQueryResultAcrossAllEntities(mp *MaskingProcessor, datasourceID string, columns []string, rows []map[string]any) []string {
	if mp == nil || len(rows) == 0 {
		return nil
	}
	if len(columns) == 0 {
		columns = inferColumnsFromRows(rows)
		if len(columns) == 0 {
			return nil
		}
	}
	dc, ok := mp.store.GetDatasource(datasourceID)
	if !ok {
		return nil
	}
	normalizedCols := normalizeColumns(columns)
	shouldMask := buildMaskSetAllEntities(dc, mp.store.GetLevelConfig(), normalizedCols)
	if len(shouldMask) == 0 {
		return nil
	}
	masked := make([]string, 0, len(columns))
	maskedKeys := make(map[string][]byte, len(columns))
	for i, col := range columns {
		if shouldMask[normalizedCols[i]] {
			masked = append(masked, col)
			maskedKeys[col] = mp.maskingKey(datasourceID, normalizedCols[i])
		}
	}
	for _, row := range rows {
		for _, col := range masked {
			key := maskedKeys[col]
			if val, exists := row[col]; exists {
				if val != nil {
					row[col] = hashValue(val, key)
				}
				continue
			}
			if strings.Contains(col, ".") {
				maskNestedField(row, col, key)
			}
		}
	}
	return masked
}
