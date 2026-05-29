package console

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func buildSQLPagingQuery(statement string, keys []sqlSortKey, cursor []any, direction Direction, limit int) string {
	query, _ := buildSQLPagingQueryDetailed(statement, "postgres", keys, cursor, direction, limit)
	return query
}

func buildSQLPagingQueryDetailed(statement string, dialect string, keys []sqlSortKey, cursor []any, direction Direction, limit int) (string, error) {
	if len(keys) == 0 {
		return "", errors.New("paging requires sort keys")
	}
	base := stripSQLPagingTail(statement)
	if base == "" {
		return "", errors.New("empty base query")
	}
	var predicate string
	if len(cursor) > 0 {
		if len(cursor) != len(keys) {
			return "", errors.New("cursor length mismatch")
		}
		filter, err := sqlKeysetPredicate(keys, cursor, direction)
		if err != nil {
			return "", err
		}
		predicate = filter
	}
	var builder strings.Builder
	builder.WriteString(base)
	if predicate != "" {
		if hasTopLevelWhere(base, dialect) {
			builder.WriteString(" AND ")
		} else {
			builder.WriteString(" WHERE ")
		}
		builder.WriteString(predicate)
	}
	builder.WriteString(" ORDER BY ")
	builder.WriteString(sqlOrderClause(keys, direction))
	if limit > 0 {
		builder.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	return builder.String(), nil
}

func sqlCursorValues(row map[string]any, rowValues []any, columns []ResultColumn, keys []sqlSortKey, statement, dialect string) ([]any, error) {
	if len(keys) == 0 {
		return nil, errors.New("missing sort keys")
	}
	values := make([]any, 0, len(keys))
	for _, key := range keys {
		if idx, ok := sqlCursorColumnIndex(columns, key.Column); ok && idx >= 0 && idx < len(rowValues) {
			values = append(values, rowValues[idx])
			continue
		}
		if qualifier, _ := splitQualifiedSQLIdentifier(key.Column); qualifier != "" {
			value, ok, ambiguous := sqlLookupQualifiedRowValue(row, columns, key.Column)
			if ambiguous {
				return nil, fmt.Errorf("ambiguous cursor value for qualified key %s", key.Column)
			}
			if !ok {
				return nil, fmt.Errorf("missing cursor value for %s", key.Column)
			}
			values = append(values, value)
			continue
		}
		value, ok := sqlLookupRowValue(row, key.Column)
		if !ok {
			return nil, fmt.Errorf("missing cursor value for %s", key.Column)
		}
		values = append(values, value)
	}
	return values, nil
}

func sqlCursorColumnIndex(columns []ResultColumn, target string) (int, bool) {
	if len(columns) == 0 {
		return 0, false
	}
	normalizedTarget := normalizeSQLIdentifier(target)
	if normalizedTarget == "" {
		return 0, false
	}
	for idx, column := range columns {
		if strings.EqualFold(normalizeSQLIdentifier(column.Key), normalizedTarget) {
			return sqlResultColumnIndex(column, idx), true
		}
	}
	qualifier, base := splitQualifiedSQLIdentifier(normalizedTarget)
	matches := make([]int, 0, len(columns))
	for idx, column := range columns {
		if qualifier == "" && strings.EqualFold(normalizeSQLIdentifier(column.Name), normalizedTarget) {
			matches = append(matches, sqlResultColumnIndex(column, idx))
			continue
		}
		if !sqlResultColumnMatches(column, normalizeSQLIdentifier(qualifier), base) {
			continue
		}
		matches = append(matches, sqlResultColumnIndex(column, idx))
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return 0, false
}

func sqlResultColumnIndex(column ResultColumn, fallback int) int {
	if column.Position >= 0 {
		return column.Position
	}
	return fallback
}

func sqlResultColumnMatches(column ResultColumn, qualifier, base string) bool {
	base = normalizeSQLIdentifier(base)
	if base == "" {
		return false
	}
	for _, origin := range column.Origins {
		if !strings.EqualFold(normalizeSQLIdentifier(origin.Column), base) {
			continue
		}
		if qualifier == "" {
			return true
		}
		alias := normalizeSQLIdentifier(origin.Alias)
		tableName := normalizeSQLIdentifier(origin.Table)
		fullName := normalizeSQLIdentifier(strings.Trim(strings.Join([]string{origin.Schema, origin.Table}, "."), "."))
		if strings.EqualFold(alias, qualifier) || strings.EqualFold(tableName, qualifier) || strings.EqualFold(fullName, qualifier) {
			return true
		}
	}
	return false
}

func splitQualifiedSQLIdentifier(input string) (string, string) {
	trimmed := normalizeSQLIdentifier(input)
	if trimmed == "" {
		return "", ""
	}
	if idx := strings.LastIndex(trimmed, "."); idx != -1 {
		return trimmed[:idx], trimmed[idx+1:]
	}
	return "", trimmed
}

func normalizeSQLIdentifier(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.Trim(trimmed, "`\"")
	return trimmed
}

func sqlKeysetPredicate(keys []sqlSortKey, cursor []any, direction Direction) (string, error) {
	parts := make([]string, 0, len(keys))
	for i := range keys {
		clauses := make([]string, 0, i+1)
		for j := 0; j < i; j++ {
			eq, err := sqlEquality(keys[j].Column, cursor[j])
			if err != nil {
				return "", err
			}
			clauses = append(clauses, eq)
		}
		cmp, err := sqlComparison(keys[i].Column, keys[i], cursor[i], direction)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, cmp)
		parts = append(parts, "("+strings.Join(clauses, " AND ")+")")
	}
	return "(" + strings.Join(parts, " OR ") + ")", nil
}

func sqlComparison(column string, key sqlSortKey, value any, direction Direction) (string, error) {
	if value == nil {
		return "", errors.New("null cursor value")
	}
	op := sqlComparisonOp(key, direction)
	lit, err := sqlLiteral(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s %s", column, op, lit), nil
}

func sqlEquality(column string, value any) (string, error) {
	if value == nil {
		return fmt.Sprintf("%s IS NULL", column), nil
	}
	lit, err := sqlLiteral(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s = %s", column, lit), nil
}

func sqlComparisonOp(key sqlSortKey, direction Direction) string {
	if direction == DirectionPrev {
		if key.Desc {
			return ">"
		}
		return "<"
	}
	if key.Desc {
		return "<"
	}
	return ">"
}

func sqlOrderClause(keys []sqlSortKey, direction Direction) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		desc := key.Desc
		if direction == DirectionPrev {
			desc = !desc
		}
		if desc {
			parts = append(parts, fmt.Sprintf("%s DESC", key.Column))
		} else {
			parts = append(parts, fmt.Sprintf("%s ASC", key.Column))
		}
	}
	return strings.Join(parts, ", ")
}

func sqlLiteral(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
	case []byte:
		return "'" + strings.ReplaceAll(string(v), "'", "''") + "'", nil
	case time.Time:
		return "'" + v.UTC().Format(time.RFC3339Nano) + "'", nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int, int8, int16, int32, int64:
		return fmt.Sprint(v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v), nil
	case float32, float64:
		return fmt.Sprint(v), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func sqlLookupRowValue(row map[string]any, column string) (any, bool) {
	if row == nil {
		return nil, false
	}
	if value, ok := row[column]; ok {
		return value, true
	}
	trimmed := strings.Trim(column, "`\"")
	if value, ok := row[trimmed]; ok {
		return value, true
	}
	base := trimmed
	if idx := strings.LastIndex(trimmed, "."); idx != -1 {
		base = trimmed[idx+1:]
		if value, ok := row[base]; ok {
			return value, true
		}
	}
	for key, value := range row {
		if strings.EqualFold(key, trimmed) || strings.EqualFold(key, base) {
			return value, true
		}
	}
	return nil, false
}

func sqlLookupQualifiedRowValue(row map[string]any, columns []ResultColumn, column string) (any, bool, bool) {
	if row == nil {
		return nil, false, false
	}
	if value, ok := row[column]; ok {
		return value, true, false
	}
	trimmed := strings.Trim(column, "`\"")
	if value, ok := row[trimmed]; ok {
		return value, true, false
	}
	_, base := splitQualifiedSQLIdentifier(trimmed)
	if base == "" {
		return nil, false, false
	}
	if value, ok, ambiguous := sqlLookupQualifiedColumnValue(row, columns, base); ok || ambiguous {
		return value, ok, ambiguous
	}
	var match any
	found := false
	for key, value := range row {
		if !sqlQualifiedRowKeyMatches(row, key, base) {
			continue
		}
		if found {
			return nil, false, true
		}
		match = value
		found = true
	}
	return match, found, false
}

func sqlLookupQualifiedColumnValue(row map[string]any, columns []ResultColumn, base string) (any, bool, bool) {
	if row == nil || len(columns) == 0 {
		return nil, false, false
	}
	normalizedBase := normalizeSQLIdentifier(base)
	if normalizedBase == "" {
		return nil, false, false
	}
	var match any
	found := false
	for _, column := range columns {
		if !strings.EqualFold(normalizeSQLIdentifier(column.Key), normalizedBase) &&
			!strings.EqualFold(normalizeSQLIdentifier(column.Name), normalizedBase) {
			continue
		}
		value, ok := sqlLookupExactRowKey(row, column.Key)
		if !ok {
			value, ok = sqlLookupExactRowKey(row, column.Name)
		}
		if !ok {
			continue
		}
		if found {
			return nil, false, true
		}
		match = value
		found = true
	}
	return match, found, false
}

func sqlQualifiedRowKeyMatches(row map[string]any, key, base string) bool {
	normalizedKey := normalizeSQLIdentifier(key)
	normalizedBase := normalizeSQLIdentifier(base)
	if normalizedKey == "" || normalizedBase == "" {
		return false
	}
	if strings.EqualFold(normalizedKey, normalizedBase) {
		return true
	}
	duplicateBase, ok := sqlSyntheticRowKeyBase(row, normalizedKey)
	return ok && strings.EqualFold(duplicateBase, normalizedBase)
}

func sqlSyntheticRowKeyBase(row map[string]any, key string) (string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(key), "`\"")
	idx := strings.LastIndex(trimmed, "__")
	if idx == -1 || idx+2 >= len(trimmed) {
		return "", false
	}
	if _, err := strconv.Atoi(trimmed[idx+2:]); err != nil {
		return "", false
	}
	base := trimmed[:idx]
	if base == "" || !sqlRowHasKey(row, base) {
		return "", false
	}
	return base, true
}

func sqlRowHasKey(row map[string]any, target string) bool {
	if row == nil {
		return false
	}
	if _, ok := row[target]; ok {
		return true
	}
	normalizedTarget := normalizeSQLIdentifier(target)
	for key := range row {
		if strings.EqualFold(normalizeSQLIdentifier(key), normalizedTarget) {
			return true
		}
	}
	return false
}

func sqlLookupExactRowKey(row map[string]any, target string) (any, bool) {
	if row == nil {
		return nil, false
	}
	if value, ok := row[target]; ok {
		return value, true
	}
	normalizedTarget := normalizeSQLIdentifier(target)
	if normalizedTarget == "" {
		return nil, false
	}
	for key, value := range row {
		if strings.EqualFold(normalizeSQLIdentifier(key), normalizedTarget) {
			return value, true
		}
	}
	return nil, false
}
