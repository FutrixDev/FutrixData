package console

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"futrixdata/platform/internal/datasource"
)

func splitSchemaTableName(name string) (string, string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", trimmed
}

func formatSchemaTableName(schema, table string) string {
	schema = strings.TrimSpace(schema)
	table = strings.TrimSpace(table)
	if schema == "" {
		return table
	}
	if table == "" {
		return schema
	}
	return schema + "." + table
}

func (a *SQLAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, _ ListOptions) ([]string, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return nil, err
	}

	var query string
	switch a.dialect {
	case "mysql":
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type IN ('BASE TABLE','VIEW')"
	case "postgres":
		query = "SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_type IN ('BASE TABLE','VIEW') ORDER BY table_schema, table_name"
	default:
		return nil, ErrUnsupported
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		switch a.dialect {
		case "mysql":
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		case "postgres":
			var schema, table string
			if err := rows.Scan(&schema, &table); err != nil {
				return nil, err
			}
			full := formatSchemaTableName(schema, table)
			if full != "" {
				names = append(names, full)
			}
		default:
			return nil, ErrUnsupported
		}
	}
	return names, rows.Err()
}

func (a *SQLAdapter) ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts ListOptions, cursor string) (EntityPage, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return EntityPage{}, err
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	fetchLimit := limit + 1

	cursor = strings.TrimSpace(cursor)
	pattern := strings.TrimSpace(opts.Pattern)
	if pattern != "" {
		pattern = strings.ReplaceAll(pattern, "*", "%")
		if !strings.ContainsAny(pattern, "%_") {
			pattern = "%" + pattern + "%"
		}
	}

	var query string
	var args []any
	switch a.dialect {
	case "mysql":
		query = "SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type IN ('BASE TABLE','VIEW')"
		if pattern != "" {
			query += " AND table_name LIKE ?"
			args = append(args, pattern)
		}
		if cursor != "" {
			query += " AND table_name > ?"
			args = append(args, cursor)
		}
		query += " ORDER BY table_name LIMIT ?"
		args = append(args, fetchLimit)
	case "postgres":
		query = "SELECT table_schema, table_name, table_type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_type IN ('BASE TABLE','VIEW')"
		if pattern != "" {
			args = append(args, pattern)
			query += fmt.Sprintf(" AND (table_schema || '.' || table_name) ILIKE $%d", len(args))
		}
		if cursor != "" {
			cursorSchema, cursorTable := splitSchemaTableName(cursor)
			cursorTable = strings.TrimSpace(cursorTable)
			if cursorTable != "" {
				args = append(args, strings.TrimSpace(cursorSchema), cursorTable)
				query += fmt.Sprintf(" AND (table_schema, table_name) > ($%d, $%d)", len(args)-1, len(args))
			}
		}
		args = append(args, fetchLimit)
		query += fmt.Sprintf(" ORDER BY table_schema, table_name LIMIT $%d", len(args))
	default:
		return EntityPage{}, ErrUnsupported
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return EntityPage{}, err
	}
	defer rows.Close()

	type entityEntry struct {
		name string
		kind string
	}
	entries := make([]entityEntry, 0, limit+1)
	for rows.Next() {
		switch a.dialect {
		case "mysql":
			var name, tableType string
			if err := rows.Scan(&name, &tableType); err != nil {
				return EntityPage{}, err
			}
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			entries = append(entries, entityEntry{name: name, kind: sqlTableTypeToKind(tableType)})
		case "postgres":
			var schema, table, tableType string
			if err := rows.Scan(&schema, &table, &tableType); err != nil {
				return EntityPage{}, err
			}
			full := formatSchemaTableName(schema, table)
			if full == "" {
				continue
			}
			entries = append(entries, entityEntry{name: full, kind: sqlTableTypeToKind(tableType)})
		default:
			return EntityPage{}, ErrUnsupported
		}
	}
	if err := rows.Err(); err != nil {
		return EntityPage{}, err
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}
	if len(entries) == 0 {
		return EntityPage{Items: nil, Cursor: "", Done: true}, nil
	}

	names := make([]string, len(entries))
	var kinds map[string]string
	for i, e := range entries {
		names[i] = e.name
		if e.kind == "view" {
			if kinds == nil {
				kinds = make(map[string]string)
			}
			kinds[e.name] = "view"
		}
	}

	page := EntityPage{Items: names, Done: !hasMore, Kinds: kinds}
	if hasMore {
		page.Cursor = names[len(names)-1]
	}
	return page, nil
}

func sqlTableTypeToKind(tableType string) string {
	if strings.EqualFold(strings.TrimSpace(tableType), "VIEW") {
		return "view"
	}
	return "table"
}

func (a *SQLAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return DescribeResult{}, err
	}

	result := DescribeResult{}

	switch a.dialect {
	case "mysql":
		done := DatasourceTimingStage(ctx, "sql.describe_columns")
		rows, err := db.QueryContext(ctx, "SELECT column_name, column_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ?", name)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", a.dialect))
			return DescribeResult{}, err
		}
		for rows.Next() {
			var col ColumnInfo
			var def sql.NullString
			if err := rows.Scan(&col.Name, &col.DataType, &col.Nullable, &def); err != nil {
				rows.Close()
				return DescribeResult{}, err
			}
			if def.Valid {
				col.DefaultValue = def.String
			}
			result.Columns = append(result.Columns, col)
		}
		rows.Close()
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("dialect", a.dialect), DatasourceTimingKV("columns", len(result.Columns)))

		done = DatasourceTimingStage(ctx, "sql.describe_indexes")
		indexRows, err := db.QueryContext(
			ctx,
			"SELECT index_name, column_name, non_unique, seq_in_index FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? ORDER BY index_name, seq_in_index",
			name,
		)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", a.dialect))
			return DescribeResult{}, err
		}
		indexOrder := make([]string, 0, 8)
		indexMap := make(map[string]*IndexInfo, 8)
		indexColumns := make(map[string][]string, 8)
		for indexRows.Next() {
			var idxName, columnName string
			var nonUnique int
			var seq int
			if err := indexRows.Scan(&idxName, &columnName, &nonUnique, &seq); err != nil {
				indexRows.Close()
				return DescribeResult{}, err
			}
			if idxName == "" || columnName == "" {
				continue
			}
			idx, ok := indexMap[idxName]
			if !ok {
				indexOrder = append(indexOrder, idxName)
				indexMap[idxName] = &IndexInfo{Name: idxName, Unique: nonUnique == 0}
				indexColumns[idxName] = []string{columnName}
				continue
			}
			if nonUnique != 0 {
				idx.Unique = false
			}
			indexColumns[idxName] = append(indexColumns[idxName], columnName)
		}
		indexRows.Close()
		for _, idxName := range indexOrder {
			idx := indexMap[idxName]
			if idx == nil {
				continue
			}
			if columns := indexColumns[idxName]; len(columns) > 0 {
				idx.Column = strings.Join(columns, ", ")
			}
			result.Indexes = append(result.Indexes, *idx)
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("dialect", a.dialect), DatasourceTimingKV("indexes", len(result.Indexes)))
	case "postgres":
		schema, table := splitSchemaTableName(name)
		if strings.TrimSpace(table) == "" {
			return DescribeResult{}, fmt.Errorf("table name is required")
		}
		var rows *sql.Rows
		done := DatasourceTimingStage(ctx, "sql.describe_columns")
		if strings.TrimSpace(schema) == "" {
			rows, err = db.QueryContext(ctx, "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1", table)
		} else {
			rows, err = db.QueryContext(ctx, "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2", schema, table)
		}
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", a.dialect))
			return DescribeResult{}, err
		}
		for rows.Next() {
			var col ColumnInfo
			var def sql.NullString
			if err := rows.Scan(&col.Name, &col.DataType, &col.Nullable, &def); err != nil {
				rows.Close()
				return DescribeResult{}, err
			}
			if def.Valid {
				col.DefaultValue = def.String
			}
			result.Columns = append(result.Columns, col)
		}
		rows.Close()
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("dialect", a.dialect), DatasourceTimingKV("columns", len(result.Columns)))

		var indexRows *sql.Rows
		done = DatasourceTimingStage(ctx, "sql.describe_indexes")
		if strings.TrimSpace(schema) == "" {
			indexRows, err = db.QueryContext(ctx, "SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1", table)
		} else {
			indexRows, err = db.QueryContext(ctx, "SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = $1 AND tablename = $2", schema, table)
		}
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", a.dialect))
			return DescribeResult{}, err
		}
		for indexRows.Next() {
			var idxName, idxDef string
			if err := indexRows.Scan(&idxName, &idxDef); err != nil {
				indexRows.Close()
				return DescribeResult{}, err
			}
			result.Indexes = append(result.Indexes, IndexInfo{Name: idxName, Definition: idxDef, Unique: strings.Contains(strings.ToUpper(idxDef), "UNIQUE")})
		}
		indexRows.Close()
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("dialect", a.dialect), DatasourceTimingKV("indexes", len(result.Indexes)))

		var pkRows *sql.Rows
		done = DatasourceTimingStage(ctx, "sql.describe_primary_key")
		if strings.TrimSpace(schema) == "" {
			pkRows, err = db.QueryContext(ctx, `
SELECT c.conname, a.attname
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN unnest(c.conkey) WITH ORDINALITY AS cols(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = cols.attnum
WHERE c.contype = 'p' AND n.nspname = current_schema() AND t.relname = $1
ORDER BY cols.ord
`, table)
		} else {
			pkRows, err = db.QueryContext(ctx, `
SELECT c.conname, a.attname
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN unnest(c.conkey) WITH ORDINALITY AS cols(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = cols.attnum
WHERE c.contype = 'p' AND n.nspname = $1 AND t.relname = $2
ORDER BY cols.ord
`, schema, table)
		}
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("dialect", a.dialect))
			return DescribeResult{}, err
		}
		pkColumns := make([]string, 0, 4)
		pkName := ""
		for pkRows.Next() {
			var constraintName, columnName string
			if err := pkRows.Scan(&constraintName, &columnName); err != nil {
				pkRows.Close()
				return DescribeResult{}, err
			}
			if strings.TrimSpace(pkName) == "" {
				pkName = constraintName
			}
			if strings.TrimSpace(columnName) != "" {
				pkColumns = append(pkColumns, columnName)
			}
		}
		pkRows.Close()
		if len(pkColumns) > 0 {
			definition := "PRIMARY KEY"
			if strings.TrimSpace(pkName) != "" {
				definition = fmt.Sprintf("CONSTRAINT %s PRIMARY KEY", pkName)
			}
			result.Indexes = append(result.Indexes, IndexInfo{
				Name:       "PRIMARY",
				Column:     strings.Join(pkColumns, ", "),
				Unique:     true,
				Definition: definition,
			})
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("dialect", a.dialect), DatasourceTimingKV("primary_key_columns", len(pkColumns)))
	default:
		return DescribeResult{}, ErrUnsupported
	}

	return result, nil
}

func (a *SQLAdapter) ListDatabases(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return nil, err
	}

	pattern := strings.TrimSpace(opts.Pattern)
	if pattern != "" {
		pattern = strings.ReplaceAll(pattern, "*", "%")
		if !strings.ContainsAny(pattern, "%_") {
			pattern = "%" + pattern + "%"
		}
	}
	var rows *sql.Rows
	switch a.dialect {
	case "mysql":
		if pattern == "" {
			rows, err = db.QueryContext(ctx, "SHOW DATABASES")
		} else {
			rows, err = db.QueryContext(ctx, "SHOW DATABASES LIKE ?", pattern)
		}
	case "postgres":
		if pattern == "" {
			rows, err = db.QueryContext(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
		} else {
			rows, err = db.QueryContext(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false AND datname ILIKE $1 ORDER BY datname", pattern)
		}
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		names = append(names, name)
		if opts.Limit > 0 && len(names) >= opts.Limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}
