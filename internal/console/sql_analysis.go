package console

import (
	"fmt"
	"strings"
	"sync"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
)

// TableRef represents a table reference extracted from SQL.
type TableRef struct {
	Schema string // e.g., "public"
	Table  string // e.g., "users"
	Alias  string // e.g., "u"
}

type SQLWriteTargetRef struct {
	Verb  string
	Table TableRef
}

// ColumnRef represents a column reference with its resolved source table.
type ColumnRef struct {
	Table  string // resolved table name or alias (empty if ambiguous)
	Column string
}

// SQLAnalysis contains the structural analysis of a SQL statement.
type SQLAnalysis struct {
	StatementType  string // "SELECT", "INSERT", "UPDATE", "DELETE", "OTHER"
	IsQuery        bool   // true for SELECT/WITH...SELECT
	StatementCount int

	Tables          []TableRef          // all table references (FROM, JOIN, subquery FROM)
	CTEWriteTargets []SQLWriteTargetRef // write targets found inside data-modifying CTEs
	Columns         []ColumnRef         // column references with best-effort source resolution

	WhereEqualityColumns []ColumnRef
	HasUnsafeWhereBool   bool

	HasWhere   bool
	HasLimit   bool
	HasOrderBy bool
	HasGroupBy bool

	LimitCount  int64
	LimitOffset int64

	OrderByKeys []sqlSortKey

	HasSubquery     bool
	HasCTE          bool
	HasUnion        bool
	HasJoin         bool
	TopLevelHasJoin bool
	JoinCount       int

	HasCTEInsert             bool
	HasCTEUpdate             bool
	HasCTEDelete             bool
	HasCTEUpdateWithoutWhere bool
	HasCTEDeleteWithoutWhere bool

	HasSelectStar bool

	PrimaryTable string // first FROM table; keeps schema when present
	CTENames     []string
}

// AnalyzeSQL parses and analyzes a SQL statement using the PostgreSQL AST parser.
// For MySQL dialect, preprocessing is applied to normalize syntax.
// Returns nil and an error if parsing fails.
func AnalyzeSQL(sql, dialect string) (result *SQLAnalysis, err error) {
	normalized := normalizeForParse(sql, dialect)

	// Guard against panics from the parser.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sql parser panic: %v", r)
			result = nil
		}
	}()

	stmts, err := parser.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("sql parse error: %w", err)
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("no statements parsed")
	}

	a := &SQLAnalysis{StatementCount: len(stmts)}
	stmt := stmts[0].AST

	switch s := stmt.(type) {
	case *tree.Select:
		a.IsQuery = true
		analyzeTopSelect(s, a)
		a.TopLevelHasJoin = topLevelSelectHasJoin(s)
	case *tree.Explain:
		a.StatementType = "EXPLAIN"
	case *tree.Insert:
		a.StatementType = "INSERT"
		if s.With != nil {
			a.HasCTE = true
			savedPrimary := a.PrimaryTable
			analyzeCTEs(s.With, a)
			a.PrimaryTable = savedPrimary
		}
		analyzeTableExpr(s.Table, a)
		if s.Rows != nil {
			rowAnalysis := &SQLAnalysis{}
			analyzeTopSelect(s.Rows, rowAnalysis)
			mergeCTEAnalysis(a, rowAnalysis)
		}
	case *tree.Update:
		a.StatementType = "UPDATE"
		if s.With != nil {
			a.HasCTE = true
			savedPrimary := a.PrimaryTable
			analyzeCTEs(s.With, a)
			a.PrimaryTable = savedPrimary
		}
		analyzeTableExpr(s.Table, a)
		analyzeFrom(s.From, a)
		if s.Where != nil {
			a.HasWhere = true
			analyzeWhereExpr(s.Where.Expr, a)
		}
	case *tree.Delete:
		a.StatementType = "DELETE"
		if s.With != nil {
			a.HasCTE = true
			savedPrimary := a.PrimaryTable
			analyzeCTEs(s.With, a)
			a.PrimaryTable = savedPrimary
		}
		analyzeTableExpr(s.Table, a)
		if s.Where != nil {
			a.HasWhere = true
			analyzeWhereExpr(s.Where.Expr, a)
		}
	default:
		a.StatementType = "OTHER"
	}

	return a, nil
}

// analyzeTopSelect analyzes a top-level tree.Select (which wraps WITH, SELECT/UNION, ORDER BY, LIMIT).
func analyzeTopSelect(sel *tree.Select, a *SQLAnalysis) {
	a.StatementType = "SELECT"

	if sel.With != nil {
		a.HasCTE = true
		// Analyze CTE bodies for table references, but preserve
		// PrimaryTable so the outer query's FROM table takes precedence.
		savedPrimary := a.PrimaryTable
		analyzeCTEs(sel.With, a)
		a.PrimaryTable = savedPrimary
	}

	if sel.Limit != nil {
		extractLimit(sel.Limit, a)
	}

	if len(sel.OrderBy) > 0 {
		a.HasOrderBy = true
		extractOrderBy(sel.OrderBy, a)
	}

	switch sc := sel.Select.(type) {
	case *tree.SelectClause:
		analyzeSelectClause(sc, a)
	case *tree.UnionClause:
		a.HasUnion = true
		analyzeUnion(sc, a)
	case *tree.ParenSelect:
		if sc.Select != nil {
			analyzeTopSelect(sc.Select, a)
		}
	case *tree.ValuesClause:
		// VALUES (...) — nothing to extract
	}
}

func analyzeSelectClause(sc *tree.SelectClause, a *SQLAnalysis) {
	if sc.Where != nil {
		a.HasWhere = true
		analyzeWhereExpr(sc.Where.Expr, a)
	}
	if len(sc.GroupBy) > 0 {
		a.HasGroupBy = true
	}

	analyzeSelectExprs(sc.Exprs, a)
	analyzeFrom(sc.From.Tables, a)

	// Detect subqueries in WHERE and SELECT expressions using walker.
	detectSubqueries(sc, a)
}

func analyzeSelectExprs(exprs tree.SelectExprs, a *SQLAnalysis) {
	for _, expr := range exprs {
		switch e := expr.Expr.(type) {
		case tree.UnqualifiedStar:
			a.HasSelectStar = true
		case *tree.AllColumnsSelector:
			a.HasSelectStar = true
		case *tree.UnresolvedName:
			col := extractColumnFromUnresolved(e)
			if col != nil {
				a.Columns = append(a.Columns, *col)
			}
			if e.Star && e.NumParts >= 1 {
				a.HasSelectStar = true
			}
		case *tree.ColumnItem:
			ref := ColumnRef{Column: string(e.ColumnName)}
			if e.TableName != nil {
				ref.Table = e.TableName.String()
			}
			a.Columns = append(a.Columns, ref)
		default:
			// Expressions like functions, casts, CASE, etc. — walk to find nested column refs.
			extractColumnsFromExpr(expr.Expr, a)
		}
	}
}

// extractColumnsFromExpr walks an expression tree to find column references
// nested inside functions, casts, CASE expressions, arithmetic, etc.
func extractColumnsFromExpr(expr tree.Expr, a *SQLAnalysis) {
	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) (stop bool) {
			switch n := node.(type) {
			case *tree.UnresolvedName:
				if col := extractColumnFromUnresolved(n); col != nil {
					a.Columns = append(a.Columns, *col)
				}
			case *tree.ColumnItem:
				ref := ColumnRef{Column: string(n.ColumnName)}
				if n.TableName != nil {
					ref.Table = n.TableName.String()
				}
				a.Columns = append(a.Columns, ref)
			}
			return false
		},
	}
	fakeStmt := &tree.Select{
		Select: &tree.SelectClause{
			Exprs: tree.SelectExprs{{Expr: expr}},
		},
	}
	fakeStmts := parser.Statements{{AST: fakeStmt}}
	w.Walk(fakeStmts, nil)
}

func extractColumnFromUnresolved(u *tree.UnresolvedName) *ColumnRef {
	if u == nil || u.NumParts == 0 {
		return nil
	}
	// Parts are in reverse order: [column, table, schema, catalog]
	col := ColumnRef{}
	switch u.NumParts {
	case 1:
		if u.Star {
			return nil // just "*"
		}
		col.Column = u.Parts[0]
	case 2:
		if u.Star {
			// table.*
			return nil
		}
		col.Column = u.Parts[0]
		col.Table = u.Parts[1]
	case 3, 4:
		col.Column = u.Parts[0]
		col.Table = u.Parts[1]
	}
	return &col
}

func analyzeFrom(tables tree.TableExprs, a *SQLAnalysis) {
	for _, te := range tables {
		analyzeTableExpr(te, a)
	}
}

func analyzeTableExpr(te tree.TableExpr, a *SQLAnalysis) {
	switch t := te.(type) {
	case *tree.TableName:
		ref := TableRef{
			Schema: string(t.SchemaName),
			Table:  string(t.TableName),
		}
		a.Tables = append(a.Tables, ref)
		if a.PrimaryTable == "" {
			a.PrimaryTable = tableRefName(ref)
		}
	case *tree.AliasedTableExpr:
		alias := string(t.As.Alias)
		switch expr := t.Expr.(type) {
		case *tree.TableName:
			ref := TableRef{
				Schema: string(expr.SchemaName),
				Table:  string(expr.TableName),
				Alias:  alias,
			}
			a.Tables = append(a.Tables, ref)
			if a.PrimaryTable == "" {
				a.PrimaryTable = tableRefName(ref)
			}
		case *tree.Subquery:
			a.HasSubquery = true
			analyzeSubquery(expr, a)
		}
	case *tree.JoinTableExpr:
		a.HasJoin = true
		a.JoinCount++
		analyzeTableExpr(t.Left, a)
		analyzeTableExpr(t.Right, a)
	case *tree.ParenTableExpr:
		analyzeTableExpr(t.Expr, a)
	}
}

func tableRefName(ref TableRef) string {
	if strings.TrimSpace(ref.Schema) == "" {
		return ref.Table
	}
	return ref.Schema + "." + ref.Table
}

func analyzeSubquery(sq *tree.Subquery, a *SQLAnalysis) {
	subAnalysis := &SQLAnalysis{}
	switch sel := sq.Select.(type) {
	case *tree.ParenSelect:
		if sel.Select != nil {
			analyzeTopSelect(sel.Select, subAnalysis)
		}
	case *tree.SelectClause:
		analyzeTopSelect(&tree.Select{Select: sel}, subAnalysis)
	case *tree.UnionClause:
		subAnalysis.HasUnion = true
		analyzeUnion(sel, subAnalysis)
	case *tree.ValuesClause:
		// nothing to extract
	}
	a.Tables = append(a.Tables, subAnalysis.Tables...)
}

func analyzeUnion(u *tree.UnionClause, a *SQLAnalysis) {
	if u.Left != nil {
		analyzeTopSelect(u.Left, a)
	}
	if u.Right != nil {
		analyzeTopSelect(u.Right, a)
	}
}

func analyzeCTEs(with *tree.With, a *SQLAnalysis) {
	for _, cte := range with.CTEList {
		if name := strings.TrimSpace(string(cte.Name.Alias)); name != "" {
			a.CTENames = append(a.CTENames, name)
		}
		switch stmt := cte.Stmt.(type) {
		case *tree.Select:
			cteAnalysis := &SQLAnalysis{}
			analyzeTopSelect(stmt, cteAnalysis)
			mergeCTEAnalysis(a, cteAnalysis)
		case *tree.Insert:
			a.HasCTEInsert = true
			cteAnalysis := &SQLAnalysis{}
			analyzeTableExpr(stmt.Table, cteAnalysis)
			appendCTEWriteTarget(a, "insert", cteAnalysis)
			if stmt.Rows != nil {
				rowAnalysis := &SQLAnalysis{}
				analyzeTopSelect(stmt.Rows, rowAnalysis)
				mergeCTEAnalysis(cteAnalysis, rowAnalysis)
			}
			mergeCTEAnalysis(a, cteAnalysis)
		case *tree.Update:
			a.HasCTEUpdate = true
			cteAnalysis := &SQLAnalysis{}
			analyzeTableExpr(stmt.Table, cteAnalysis)
			appendCTEWriteTarget(a, "update", cteAnalysis)
			analyzeFrom(stmt.From, cteAnalysis)
			if stmt.Where == nil {
				a.HasCTEUpdateWithoutWhere = true
			} else {
				cteAnalysis.HasWhere = true
				analyzeWhereExpr(stmt.Where.Expr, cteAnalysis)
			}
			mergeCTEAnalysis(a, cteAnalysis)
		case *tree.Delete:
			a.HasCTEDelete = true
			cteAnalysis := &SQLAnalysis{}
			analyzeTableExpr(stmt.Table, cteAnalysis)
			appendCTEWriteTarget(a, "delete", cteAnalysis)
			if stmt.Where == nil {
				a.HasCTEDeleteWithoutWhere = true
			} else {
				cteAnalysis.HasWhere = true
				analyzeWhereExpr(stmt.Where.Expr, cteAnalysis)
			}
			mergeCTEAnalysis(a, cteAnalysis)
		}
	}
}

func mergeCTEAnalysis(dst *SQLAnalysis, src *SQLAnalysis) {
	if src == nil {
		return
	}
	dst.Tables = append(dst.Tables, src.Tables...)
	dst.CTEWriteTargets = append(dst.CTEWriteTargets, src.CTEWriteTargets...)
	dst.Columns = append(dst.Columns, src.Columns...)
	dst.HasSubquery = dst.HasSubquery || src.HasSubquery
	dst.HasUnion = dst.HasUnion || src.HasUnion
	dst.HasJoin = dst.HasJoin || src.HasJoin
	dst.JoinCount += src.JoinCount
	dst.HasSelectStar = dst.HasSelectStar || src.HasSelectStar
	dst.HasCTE = dst.HasCTE || src.HasCTE
	dst.HasCTEInsert = dst.HasCTEInsert || src.HasCTEInsert
	dst.HasCTEUpdate = dst.HasCTEUpdate || src.HasCTEUpdate
	dst.HasCTEDelete = dst.HasCTEDelete || src.HasCTEDelete
	dst.HasCTEUpdateWithoutWhere = dst.HasCTEUpdateWithoutWhere || src.HasCTEUpdateWithoutWhere
	dst.HasCTEDeleteWithoutWhere = dst.HasCTEDeleteWithoutWhere || src.HasCTEDeleteWithoutWhere
	dst.CTENames = append(dst.CTENames, src.CTENames...)
}

func appendCTEWriteTarget(dst *SQLAnalysis, verb string, src *SQLAnalysis) {
	if dst == nil || src == nil || len(src.Tables) == 0 {
		return
	}
	dst.CTEWriteTargets = append(dst.CTEWriteTargets, SQLWriteTargetRef{
		Verb:  strings.ToLower(strings.TrimSpace(verb)),
		Table: src.Tables[0],
	})
}

func topLevelSelectHasJoin(sel *tree.Select) bool {
	if sel == nil {
		return false
	}
	switch sc := sel.Select.(type) {
	case *tree.SelectClause:
		return tableExprsHaveJoin(sc.From.Tables)
	case *tree.ParenSelect:
		if sc.Select != nil {
			return topLevelSelectHasJoin(sc.Select)
		}
	case *tree.UnionClause:
		return true
	}
	return false
}

func tableExprsHaveJoin(tables tree.TableExprs) bool {
	for _, te := range tables {
		if tableExprHasJoin(te) {
			return true
		}
	}
	return false
}

func tableExprHasJoin(te tree.TableExpr) bool {
	switch t := te.(type) {
	case *tree.JoinTableExpr:
		return true
	case *tree.ParenTableExpr:
		return tableExprHasJoin(t.Expr)
	default:
		return false
	}
}

func extractLimit(limit *tree.Limit, a *SQLAnalysis) {
	if limit.Count != nil {
		a.HasLimit = true
		if nv, ok := limit.Count.(*tree.NumVal); ok {
			if v, err := nv.AsInt64(); err == nil {
				a.LimitCount = v
			}
		}
	}
	if limit.Offset != nil {
		if nv, ok := limit.Offset.(*tree.NumVal); ok {
			if v, err := nv.AsInt64(); err == nil {
				a.LimitOffset = v
			}
		}
	}
}

func extractOrderBy(orderBy tree.OrderBy, a *SQLAnalysis) {
	for _, o := range orderBy {
		if o.OrderType != tree.OrderByColumn {
			continue
		}
		col := o.Expr.String()
		desc := o.Direction == tree.Descending
		a.OrderByKeys = append(a.OrderByKeys, sqlSortKey{Column: col, Desc: desc})
	}
}

// detectSubqueries uses the AST walker to find subqueries in WHERE and SELECT expressions.
func detectSubqueries(sc *tree.SelectClause, a *SQLAnalysis) {
	if a.HasSubquery {
		return // already detected from FROM analysis
	}
	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) (stop bool) {
			if _, ok := node.(*tree.Subquery); ok {
				a.HasSubquery = true
				return true
			}
			return false
		},
	}

	// Build a fake SELECT clause containing both SELECT exprs and WHERE
	// so that the walker traverses subqueries in either position.
	exprs := make(tree.SelectExprs, 0, len(sc.Exprs)+1)
	exprs = append(exprs, sc.Exprs...)
	if sc.Where != nil {
		exprs = append(exprs, tree.SelectExpr{Expr: sc.Where.Expr})
	}
	if len(exprs) > 0 {
		fakeStmt := &tree.Select{
			Select: &tree.SelectClause{Exprs: exprs},
		}
		fakeStmts := parser.Statements{{AST: fakeStmt}}
		w.Walk(fakeStmts, nil)
	}
}

func analyzeWhereExpr(expr tree.Expr, a *SQLAnalysis) {
	if expr == nil {
		return
	}
	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) (stop bool) {
			switch n := node.(type) {
			case *tree.OrExpr, *tree.NotExpr:
				a.HasUnsafeWhereBool = true
			case *tree.ComparisonExpr:
				if n.Operator != tree.EQ {
					return false
				}
				if col, ok := extractEqualityColumnRef(n.Left, n.Right); ok {
					a.WhereEqualityColumns = append(a.WhereEqualityColumns, col)
					return false
				}
				if col, ok := extractEqualityColumnRef(n.Right, n.Left); ok {
					a.WhereEqualityColumns = append(a.WhereEqualityColumns, col)
				}
			}
			return false
		},
	}
	fakeStmt := &tree.Select{
		Select: &tree.SelectClause{
			Exprs: tree.SelectExprs{{Expr: expr}},
		},
	}
	fakeStmts := parser.Statements{{AST: fakeStmt}}
	w.Walk(fakeStmts, nil)
}

func extractEqualityColumnRef(columnExpr tree.Expr, valueExpr tree.Expr) (ColumnRef, bool) {
	ref, ok := extractColumnRefExpr(columnExpr)
	if !ok || !looksLikeValueExpr(valueExpr) {
		return ColumnRef{}, false
	}
	return ref, true
}

func extractColumnRefExpr(expr tree.Expr) (ColumnRef, bool) {
	switch e := unwrapSQLExpr(expr).(type) {
	case *tree.ColumnItem:
		ref := ColumnRef{Column: string(e.ColumnName)}
		if e.TableName != nil {
			ref.Table = e.TableName.String()
		}
		return ref, ref.Column != ""
	case *tree.UnresolvedName:
		col := extractColumnFromUnresolved(e)
		if col == nil || col.Column == "" {
			return ColumnRef{}, false
		}
		return *col, true
	default:
		return ColumnRef{}, false
	}
}

func looksLikeValueExpr(expr tree.Expr) bool {
	expr = unwrapSQLExpr(expr)
	if _, ok := extractColumnRefExpr(expr); ok {
		return false
	}
	switch expr.(type) {
	case *tree.Subquery:
		return false
	default:
		return true
	}
}

func unwrapSQLExpr(expr tree.Expr) tree.Expr {
	for expr != nil {
		switch e := expr.(type) {
		case *tree.ParenExpr:
			expr = e.Expr
		case *tree.CastExpr:
			expr = e.Expr
		default:
			return expr
		}
	}
	return nil
}

// ResolveColumnSources maps columns to their source tables using the alias map
// built from the Tables list. This is useful for masking: knowing which table
// each column comes from.
func (a *SQLAnalysis) ResolveColumnSources() {
	if len(a.Tables) == 0 || len(a.Columns) == 0 {
		return
	}
	// Build alias → table name map.
	aliasToTable := make(map[string]string, len(a.Tables))
	for _, t := range a.Tables {
		if t.Alias != "" {
			aliasToTable[strings.ToLower(t.Alias)] = t.Table
		}
		aliasToTable[strings.ToLower(t.Table)] = t.Table
	}

	for i := range a.Columns {
		if a.Columns[i].Table != "" {
			key := strings.ToLower(a.Columns[i].Table)
			if resolved, ok := aliasToTable[key]; ok {
				a.Columns[i].Table = resolved
			}
		}
	}
}

// --- Single-entry analysis cache ---

var analysisCache struct {
	mu      sync.Mutex
	lastSQL string
	lastDia string
	lastRes *SQLAnalysis
	lastErr error
}

// CachedAnalyzeSQL returns a cached result if the SQL and dialect match,
// otherwise calls AnalyzeSQL and caches the result.
func CachedAnalyzeSQL(sql, dialect string) (*SQLAnalysis, error) {
	analysisCache.mu.Lock()
	defer analysisCache.mu.Unlock()
	if analysisCache.lastSQL == sql && analysisCache.lastDia == dialect {
		return analysisCache.lastRes, analysisCache.lastErr
	}
	res, err := AnalyzeSQL(sql, dialect)
	analysisCache.lastSQL = sql
	analysisCache.lastDia = dialect
	analysisCache.lastRes = res
	analysisCache.lastErr = err
	return res, err
}
