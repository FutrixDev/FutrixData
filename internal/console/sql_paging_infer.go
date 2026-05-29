package console

import (
	"context"
	"database/sql"
	"strings"
)

func inferSQLSortKeys(ctx context.Context, db *sql.DB, dialect, statement string) ([]sqlSortKey, error) {
	a, err := analyzeSQLForDialect(statement, dialect)
	if err == nil {
		if len(a.OrderByKeys) > 0 {
			return a.OrderByKeys, nil
		}
		table := a.PrimaryTable
		if table == "" || a.TopLevelHasJoin {
			return nil, nil
		}
		columns, err := sqlPrimaryKeyColumns(ctx, db, dialect, table)
		if err != nil {
			return nil, err
		}
		if len(columns) == 0 {
			return nil, nil
		}
		keys := make([]sqlSortKey, 0, len(columns))
		for _, column := range columns {
			keys = append(keys, sqlSortKey{Column: column, Desc: false})
		}
		return keys, nil
	}

	if keys := parseSQLOrderByClause(orderByClause(statement)); len(keys) > 0 {
		return keys, nil
	}

	table, ok := parseSimpleFromTableFallback(statement)
	if !ok || table == "" {
		return nil, nil
	}
	columns, err := sqlPrimaryKeyColumns(ctx, db, dialect, table)
	if err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, nil
	}
	keys := make([]sqlSortKey, 0, len(columns))
	for _, column := range columns {
		keys = append(keys, sqlSortKey{Column: column, Desc: false})
	}
	return keys, nil
}

func sqlPrimaryKeyColumns(ctx context.Context, db *sql.DB, dialect, table string) ([]string, error) {
	if db == nil || table == "" {
		return nil, nil
	}
	done := DatasourceTimingStage(ctx, "sql.primary_key_lookup")
	switch dialect {
	case "mysql":
		schema := ""
		name := table
		if strings.Contains(table, ".") {
			parts := strings.SplitN(table, ".", 2)
			schema = parts[0]
			name = parts[1]
		}
		query := "SELECT column_name FROM information_schema.key_column_usage WHERE constraint_name = 'PRIMARY' AND table_name = ?"
		var rows *sql.Rows
		var err error
		if schema != "" {
			query += " AND table_schema = ? ORDER BY ordinal_position"
			rows, err = db.QueryContext(ctx, query, name, schema)
		} else {
			query += " AND table_schema = DATABASE() ORDER BY ordinal_position"
			rows, err = db.QueryContext(ctx, query, name)
		}
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", dialect))
			return nil, err
		}
		defer rows.Close()
		var columns []string
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				return nil, err
			}
			columns = append(columns, column)
		}
		err = rows.Err()
		done(DatasourceTimingKV("status", timingStatus(err)), DatasourceTimingKV("dialect", dialect), DatasourceTimingKV("columns", len(columns)))
		return columns, err
	case "postgres":
		query := `
SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = $1::regclass AND i.indisprimary
ORDER BY array_position(i.indkey, a.attnum)`
		rows, err := db.QueryContext(ctx, query, table)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", dialect))
			return nil, err
		}
		defer rows.Close()
		var columns []string
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				return nil, err
			}
			columns = append(columns, column)
		}
		err = rows.Err()
		done(DatasourceTimingKV("status", timingStatus(err)), DatasourceTimingKV("dialect", dialect), DatasourceTimingKV("columns", len(columns)))
		return columns, err
	default:
		done(DatasourceTimingKV("status", "skipped"), DatasourceTimingKV("dialect", dialect))
		return nil, nil
	}
}
