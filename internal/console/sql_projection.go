package console

import (
	"context"
	"fmt"
	"strings"

	"futrixdata/platform/internal/datasource"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
)

type sqlDescribeEntityFunc func(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error)

var parseSQLStatements = parser.Parse

type sqlProjectionResolver struct {
	ctx        context.Context
	ds         datasource.DataSource
	describe   sqlDescribeEntityFunc
	tables     []TableRef
	aliasToRef map[string]TableRef
	columnSets map[string]map[string]struct{}
}

func buildSQLResultColumns(ctx context.Context, ds datasource.DataSource, dialect, statement string, actualColumns []string, describe sqlDescribeEntityFunc) (result []ResultColumn, err error) {
	if len(actualColumns) == 0 {
		return nil, nil
	}
	if describe == nil {
		return conservativeSQLResultColumns(actualColumns), nil
	}
	defer func() {
		if r := recover(); r != nil {
			result = conservativeSQLResultColumns(actualColumns)
			err = nil
		}
	}()

	normalized := normalizeForParse(statement, dialect)
	stmts, err := parseSQLStatements(normalized)
	if err != nil || len(stmts) == 0 {
		if err != nil {
			return nil, err
		}
		return conservativeSQLResultColumns(actualColumns), nil
	}

	sc := topLevelSelectClause(stmts[0].AST)
	if sc == nil {
		return conservativeSQLResultColumns(actualColumns), nil
	}

	tables, unresolvedTopLevel := topLevelTableRefs(sc.From.Tables)
	resolver := newSQLProjectionResolver(ctx, ds, describe, tables)
	planned := make([]ResultColumn, 0, len(actualColumns))

	for _, selectExpr := range sc.Exprs {
		switch expr := selectExpr.Expr.(type) {
		case tree.UnqualifiedStar:
			if unresolvedTopLevel {
				return conservativeSQLResultColumns(actualColumns), nil
			}
			expanded, ok, err := resolver.expandAllTables()
			if err != nil {
				return nil, err
			}
			if !ok {
				return conservativeSQLResultColumns(actualColumns), nil
			}
			planned = append(planned, expanded...)
		case *tree.AllColumnsSelector:
			expanded, ok, err := resolver.expandTableSelector(expr)
			if err != nil {
				return nil, err
			}
			if !ok {
				return conservativeSQLResultColumns(actualColumns), nil
			}
			planned = append(planned, expanded...)
		default:
			planned = append(planned, resolver.buildExprColumn(selectExpr))
		}
	}

	if len(planned) != len(actualColumns) {
		return conservativeSQLResultColumns(actualColumns), nil
	}

	keys := makeSQLColumnKeys(actualColumns)
	result = make([]ResultColumn, len(actualColumns))
	for i := range actualColumns {
		col := planned[i]
		name := strings.TrimSpace(actualColumns[i])
		if name == "" {
			name = strings.TrimSpace(col.Name)
		}
		if name == "" {
			name = keys[i]
		}
		col.Key = keys[i]
		col.Name = name
		col.Position = i
		result[i] = col
	}
	return result, nil
}

func topLevelSelectClause(stmt tree.Statement) *tree.SelectClause {
	switch s := stmt.(type) {
	case *tree.Select:
		return unwrapTopLevelSelectClause(s)
	default:
		return nil
	}
}

func unwrapTopLevelSelectClause(sel *tree.Select) *tree.SelectClause {
	if sel == nil {
		return nil
	}
	switch sc := sel.Select.(type) {
	case *tree.SelectClause:
		return sc
	case *tree.ParenSelect:
		if sc.Select != nil {
			return unwrapTopLevelSelectClause(sc.Select)
		}
	}
	return nil
}

func topLevelTableRefs(exprs tree.TableExprs) ([]TableRef, bool) {
	var refs []TableRef
	unresolved := false
	var visit func(tree.TableExpr)
	visit = func(expr tree.TableExpr) {
		switch t := expr.(type) {
		case *tree.AliasedTableExpr:
			switch source := t.Expr.(type) {
			case *tree.TableName:
				refs = append(refs, TableRef{
					Schema: string(source.SchemaName),
					Table:  string(source.TableName),
					Alias:  string(t.As.Alias),
				})
			default:
				unresolved = true
			}
		case *tree.JoinTableExpr:
			visit(t.Left)
			visit(t.Right)
		case *tree.ParenTableExpr:
			visit(t.Expr)
		default:
			unresolved = true
		}
	}
	for _, expr := range exprs {
		visit(expr)
	}
	return refs, unresolved
}

func newSQLProjectionResolver(ctx context.Context, ds datasource.DataSource, describe sqlDescribeEntityFunc, tables []TableRef) *sqlProjectionResolver {
	aliasToRef := make(map[string]TableRef, len(tables)*3)
	for _, table := range tables {
		if alias := strings.ToLower(strings.TrimSpace(table.Alias)); alias != "" {
			aliasToRef[alias] = table
		}
		if full := strings.ToLower(strings.TrimSpace(tableRefName(table))); full != "" {
			aliasToRef[full] = table
		}
		if base := strings.ToLower(strings.TrimSpace(table.Table)); base != "" {
			if _, exists := aliasToRef[base]; !exists {
				aliasToRef[base] = table
			}
		}
	}
	return &sqlProjectionResolver{
		ctx:        ctx,
		ds:         ds,
		describe:   describe,
		tables:     tables,
		aliasToRef: aliasToRef,
		columnSets: make(map[string]map[string]struct{}, len(tables)),
	}
}

func (r *sqlProjectionResolver) expandAllTables() ([]ResultColumn, bool, error) {
	if len(r.tables) == 0 {
		return nil, false, nil
	}
	var out []ResultColumn
	for _, table := range r.tables {
		expanded, ok, err := r.expandTable(table)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		out = append(out, expanded...)
	}
	return out, true, nil
}

func (r *sqlProjectionResolver) expandTableSelector(selector *tree.AllColumnsSelector) ([]ResultColumn, bool, error) {
	if selector == nil || selector.TableName == nil {
		return nil, false, nil
	}
	table, ok := r.resolveTable(selector.TableName.String())
	if !ok {
		return nil, false, nil
	}
	return r.expandTable(table)
}

func (r *sqlProjectionResolver) expandTable(table TableRef) ([]ResultColumn, bool, error) {
	describeName := r.describeName(table)
	if describeName == "" {
		return nil, false, nil
	}
	desc, err := r.describe(r.ctx, r.ds, describeName)
	if err != nil {
		return nil, false, err
	}
	if len(desc.Columns) == 0 {
		return nil, false, nil
	}
	if _, ok := r.columnSets[describeName]; !ok {
		set := make(map[string]struct{}, len(desc.Columns))
		for _, column := range desc.Columns {
			set[column.Name] = struct{}{}
		}
		r.columnSets[describeName] = set
	}
	out := make([]ResultColumn, 0, len(desc.Columns))
	for _, column := range desc.Columns {
		name := strings.TrimSpace(column.Name)
		if name == "" {
			continue
		}
		out = append(out, ResultColumn{
			Name:       name,
			SourceKind: "star",
			Origins: []ResultColumnOrigin{{
				Schema: table.Schema,
				Alias:  table.Alias,
				Table:  table.Table,
				Column: name,
			}},
		})
	}
	return out, len(out) > 0, nil
}

func (r *sqlProjectionResolver) buildExprColumn(expr tree.SelectExpr) ResultColumn {
	name := strings.TrimSpace(string(expr.As))
	if name == "" {
		name = sqlProjectionColumnName(expr.Expr)
	}
	column := ResultColumn{Name: name}

	switch item := expr.Expr.(type) {
	case *tree.ColumnItem:
		if origin, ok := r.resolveColumnRef(ColumnRef{
			Table:  unresolvedObjectString(item.TableName),
			Column: string(item.ColumnName),
		}); ok {
			column.SourceKind = "column"
			column.Origins = []ResultColumnOrigin{origin}
			return column
		}
		column.SourceKind = "column"
		column.ConservativeMask = true
		return column
	case *tree.UnresolvedName:
		if ref := extractColumnFromUnresolved(item); ref != nil {
			if origin, ok := r.resolveColumnRef(*ref); ok {
				column.SourceKind = "column"
				column.Origins = []ResultColumnOrigin{origin}
				return column
			}
			column.SourceKind = "column"
			column.ConservativeMask = true
			return column
		}
	}

	refs := sqlExprColumnRefs(expr.Expr)
	if len(refs) == 0 {
		column.SourceKind = "expression"
		return column
	}
	column.SourceKind = "expression"
	origins := make([]ResultColumnOrigin, 0, len(refs))
	unresolved := false
	for _, ref := range refs {
		origin, ok := r.resolveColumnRef(ref)
		if !ok {
			unresolved = true
			continue
		}
		origins = append(origins, origin)
	}
	column.Origins = dedupeOrigins(origins)
	column.ConservativeMask = unresolved || len(column.Origins) == 0
	return column
}

func sqlProjectionColumnName(expr tree.Expr) string {
	switch item := expr.(type) {
	case *tree.ColumnItem:
		return string(item.ColumnName)
	case *tree.UnresolvedName:
		if ref := extractColumnFromUnresolved(item); ref != nil {
			return ref.Column
		}
	}
	return strings.TrimSpace(expr.String())
}

func sqlExprColumnRefs(expr tree.Expr) []ColumnRef {
	if expr == nil {
		return nil
	}
	refs := make([]ColumnRef, 0, 2)
	w := &walk.AstWalker{
		Fn: func(_ interface{}, node interface{}) (stop bool) {
			switch item := node.(type) {
			case *tree.ColumnItem:
				refs = append(refs, ColumnRef{
					Table:  unresolvedObjectString(item.TableName),
					Column: string(item.ColumnName),
				})
			case *tree.UnresolvedName:
				if ref := extractColumnFromUnresolved(item); ref != nil {
					refs = append(refs, *ref)
				}
			}
			return false
		},
	}
	fakeStmt := &tree.Select{
		Select: &tree.SelectClause{Exprs: tree.SelectExprs{{Expr: expr}}},
	}
	w.Walk(parser.Statements{{AST: fakeStmt}}, nil)
	return refs
}

func dedupeOrigins(origins []ResultColumnOrigin) []ResultColumnOrigin {
	if len(origins) < 2 {
		return origins
	}
	seen := make(map[string]struct{}, len(origins))
	out := make([]ResultColumnOrigin, 0, len(origins))
	for _, origin := range origins {
		key := origin.Schema + "\x00" + origin.Alias + "\x00" + origin.Table + "\x00" + origin.Column
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, origin)
	}
	return out
}

func unresolvedObjectString(name *tree.UnresolvedObjectName) string {
	if name == nil {
		return ""
	}
	return strings.TrimSpace(name.String())
}

func (r *sqlProjectionResolver) resolveTable(name string) (TableRef, bool) {
	ref, ok := r.aliasToRef[strings.ToLower(strings.TrimSpace(name))]
	return ref, ok
}

func (r *sqlProjectionResolver) resolveColumnRef(ref ColumnRef) (ResultColumnOrigin, bool) {
	column := strings.TrimSpace(ref.Column)
	if column == "" {
		return ResultColumnOrigin{}, false
	}
	if tableName := strings.TrimSpace(ref.Table); tableName != "" {
		table, ok := r.resolveTable(tableName)
		if !ok {
			return ResultColumnOrigin{}, false
		}
		return ResultColumnOrigin{
			Schema: table.Schema,
			Alias:  table.Alias,
			Table:  table.Table,
			Column: column,
		}, true
	}

	var matches []TableRef
	for _, table := range r.tables {
		hasColumn, err := r.tableHasColumn(table, column)
		if err != nil {
			return ResultColumnOrigin{}, false
		}
		if hasColumn {
			matches = append(matches, table)
		}
	}
	if len(matches) == 1 {
		return ResultColumnOrigin{
			Schema: matches[0].Schema,
			Alias:  matches[0].Alias,
			Table:  matches[0].Table,
			Column: column,
		}, true
	}
	if len(matches) == 0 && len(r.tables) == 1 {
		return ResultColumnOrigin{
			Schema: r.tables[0].Schema,
			Alias:  r.tables[0].Alias,
			Table:  r.tables[0].Table,
			Column: column,
		}, true
	}
	return ResultColumnOrigin{}, false
}

func (r *sqlProjectionResolver) describeName(table TableRef) string {
	if r.ds.Type == datasource.TypeMySQL {
		schema := strings.TrimSpace(table.Schema)
		if schema == "" {
			return strings.TrimSpace(table.Table)
		}
		if strings.EqualFold(schema, strings.TrimSpace(r.ds.Database)) {
			return strings.TrimSpace(table.Table)
		}
		return ""
	}
	return strings.TrimSpace(tableRefName(table))
}

func (r *sqlProjectionResolver) tableHasColumn(table TableRef, column string) (bool, error) {
	name := r.describeName(table)
	if name == "" {
		return false, nil
	}
	if set, ok := r.columnSets[name]; ok {
		_, exists := set[column]
		return exists, nil
	}
	desc, err := r.describe(r.ctx, r.ds, name)
	if err != nil {
		return false, fmt.Errorf("describe %s: %w", name, err)
	}
	set := make(map[string]struct{}, len(desc.Columns))
	for _, item := range desc.Columns {
		if trimmed := strings.TrimSpace(item.Name); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	r.columnSets[name] = set
	_, exists := set[column]
	return exists, nil
}

func sqlSourceEntityHint(statement, dialect string) string {
	analysis, err := CachedAnalyzeSQL(statement, dialect)
	if err != nil || analysis == nil || len(analysis.Tables) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(analysis.Tables))
	names := make([]string, 0, len(analysis.Tables))
	for _, table := range analysis.Tables {
		name := strings.TrimSpace(tableRefName(table))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return strings.Join(names, ",")
}
