package console

import (
	"database/sql"
	"fmt"
	"strings"

	"futrixdata/platform/internal/console/window"
)

type sqlRowBatch struct {
	ColumnNames []string
	ColumnKeys  []string
	Rows        []map[string]any
	RowValues   [][]any
}

func readRows(rows *sql.Rows) ([]string, []map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			val := values[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		result = append(result, row)
	}
	return columns, result, rows.Err()
}

type rowScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func readRowsWindow(rows rowScanner, limit int) ([]string, []map[string]any, bool, error) {
	batch, hasMore, err := readSQLRowsWindow(rows, limit)
	if err != nil {
		return nil, nil, false, err
	}
	return batch.ColumnNames, batch.Rows, hasMore, nil
}

func readSQLRowsWindow(rows rowScanner, limit int) (sqlRowBatch, bool, error) {
	columns, err := rows.Columns()
	if err != nil {
		return sqlRowBatch{}, false, err
	}
	keys := makeSQLColumnKeys(columns)
	rowsWindow := window.NewRowWindow(limit)
	var rowValues [][]any
	hasMore := false
	for rows.Next() {
		currentValues := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range currentValues {
			valuePtrs[i] = &currentValues[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return sqlRowBatch{}, false, err
		}
		normalizedValues := normalizeSQLValues(currentValues)
		row := make(map[string]any, len(keys))
		for i, key := range keys {
			row[key] = normalizedValues[i]
		}
		if limit == 0 || len(rowValues) >= limit {
			hasMore = true
			break
		}
		rowValues = append(rowValues, normalizedValues)
		if !rowsWindow.Push(row) {
			hasMore = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return sqlRowBatch{}, false, err
	}
	return sqlRowBatch{
		ColumnNames: columns,
		ColumnKeys:  keys,
		Rows:        rowsWindow.Rows(),
		RowValues:   rowValues,
	}, hasMore || rowsWindow.HasMore(), nil
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func normalizeSQLValues(values []any) []any {
	out := make([]any, len(values))
	for i, value := range values {
		switch v := value.(type) {
		case []byte:
			out[i] = string(v)
		default:
			out[i] = v
		}
	}
	return out
}

func makeSQLColumnKeys(columns []string) []string {
	keys := make([]string, len(columns))
	seen := make(map[string]int, len(columns))
	used := make(map[string]struct{}, len(columns))
	for i, name := range columns {
		base := strings.TrimSpace(name)
		if base == "" {
			base = fmt.Sprintf("column_%d", i+1)
		}
		seen[base]++
		candidate := base
		if seen[base] > 1 {
			candidate = fmt.Sprintf("%s__%d", base, seen[base])
		}
		for {
			if _, ok := used[candidate]; !ok {
				break
			}
			seen[base]++
			candidate = fmt.Sprintf("%s__%d", base, seen[base])
		}
		used[candidate] = struct{}{}
		keys[i] = candidate
	}
	return keys
}
