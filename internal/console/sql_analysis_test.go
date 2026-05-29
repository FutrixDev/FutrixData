package console

import "testing"

func TestAnalyzeSQL_SimpleSelect(t *testing.T) {
	a, err := AnalyzeSQL("SELECT id, name FROM users WHERE active = true", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatementType != "SELECT" {
		t.Fatalf("expected SELECT, got %s", a.StatementType)
	}
	if !a.IsQuery {
		t.Fatal("expected IsQuery=true")
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere=true")
	}
	if len(a.Tables) != 1 || a.Tables[0].Table != "users" {
		t.Fatalf("expected 1 table 'users', got %+v", a.Tables)
	}
	if a.PrimaryTable != "users" {
		t.Fatalf("expected PrimaryTable='users', got %s", a.PrimaryTable)
	}
}

func TestAnalyzeSQL_Join(t *testing.T) {
	sql := "SELECT u.id, o.total FROM users u INNER JOIN orders o ON u.id = o.user_id WHERE o.total > 100"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasJoin {
		t.Fatal("expected HasJoin=true")
	}
	if a.JoinCount != 1 {
		t.Fatalf("expected JoinCount=1, got %d", a.JoinCount)
	}
	if len(a.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d: %+v", len(a.Tables), a.Tables)
	}
	if a.Tables[0].Table != "users" || a.Tables[0].Alias != "u" {
		t.Fatalf("expected users/u, got %+v", a.Tables[0])
	}
	if a.Tables[1].Table != "orders" || a.Tables[1].Alias != "o" {
		t.Fatalf("expected orders/o, got %+v", a.Tables[1])
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere=true")
	}
}

func TestAnalyzeSQL_MultiJoin(t *testing.T) {
	sql := `SELECT u.name, o.total, p.name
		FROM users u
		JOIN orders o ON u.id = o.user_id
		JOIN products p ON o.product_id = p.id`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.JoinCount != 2 {
		t.Fatalf("expected JoinCount=2, got %d", a.JoinCount)
	}
	if len(a.Tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(a.Tables))
	}
}

func TestAnalyzeSQL_CTE(t *testing.T) {
	sql := `WITH active_users AS (
		SELECT id, name FROM users WHERE active = true
	)
	SELECT au.name FROM active_users au`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasCTE {
		t.Fatal("expected HasCTE=true")
	}
	if a.StatementType != "SELECT" {
		t.Fatalf("expected SELECT, got %s", a.StatementType)
	}
	// CTE body tables should be extracted.
	found := false
	for _, tbl := range a.Tables {
		if tbl.Table == "users" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'users' table from CTE body in Tables list")
	}
	// PrimaryTable should be from the outer query, not the CTE body.
	if a.PrimaryTable != "active_users" {
		t.Fatalf("expected PrimaryTable='active_users' (outer FROM), got %s", a.PrimaryTable)
	}
}

func TestAnalyzeSQL_CTEPrimaryTablePreserved(t *testing.T) {
	sql := `WITH stats AS (SELECT dept_id, COUNT(*) cnt FROM employees GROUP BY dept_id)
		SELECT d.name, s.cnt FROM departments d JOIN stats s ON d.id = s.dept_id`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	// PrimaryTable should be "departments" from the outer FROM, not "employees" from CTE.
	if a.PrimaryTable != "departments" {
		t.Fatalf("expected PrimaryTable='departments', got %s", a.PrimaryTable)
	}
}

func TestAnalyzeSQL_CTEDeleteTracksWriteType(t *testing.T) {
	sql := `WITH deleted AS (DELETE FROM users WHERE id = 1 RETURNING id) SELECT id FROM deleted`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasCTE {
		t.Fatal("expected HasCTE=true")
	}
	if !a.HasCTEDelete {
		t.Fatal("expected HasCTEDelete=true")
	}
	if a.HasCTEDeleteWithoutWhere {
		t.Fatal("expected HasCTEDeleteWithoutWhere=false")
	}
}

func TestAnalyzeSQL_CTEInnerOrderByDoesNotLeakToOuter(t *testing.T) {
	sql := `WITH ranked AS (SELECT id FROM users ORDER BY created_at DESC) SELECT id FROM ranked`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.HasOrderBy {
		t.Fatal("expected HasOrderBy=false when only the CTE has ORDER BY")
	}
	if len(a.OrderByKeys) != 0 {
		t.Fatalf("expected no outer OrderByKeys, got %+v", a.OrderByKeys)
	}
}

func TestAnalyzeSQL_CTEInnerJoinDoesNotSetTopLevelJoin(t *testing.T) {
	sql := `WITH joined AS (SELECT u.id FROM users u JOIN orders o ON u.id = o.user_id) SELECT id FROM public.users`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasJoin {
		t.Fatal("expected aggregate HasJoin=true because the CTE contains a join")
	}
	if a.TopLevelHasJoin {
		t.Fatal("expected TopLevelHasJoin=false when only the CTE contains a join")
	}
}

func TestAnalyzeSQL_ColumnInFunction(t *testing.T) {
	sql := "SELECT lower(u.email), count(u.id) FROM users u"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, c := range a.Columns {
		found[c.Column] = true
	}
	if !found["email"] {
		t.Fatal("expected 'email' column extracted from lower(u.email)")
	}
	if !found["id"] {
		t.Fatal("expected 'id' column extracted from count(u.id)")
	}
}

func TestAnalyzeSQL_Union(t *testing.T) {
	sql := "SELECT id, name FROM users UNION ALL SELECT id, name FROM admins"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasUnion {
		t.Fatal("expected HasUnion=true")
	}
}

func TestAnalyzeSQL_Subquery(t *testing.T) {
	sql := "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE total > 100)"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasSubquery {
		t.Fatal("expected HasSubquery=true")
	}
	if a.HasSelectStar {
		// SELECT * at the top level
	}
}

func TestAnalyzeSQL_SubqueryInSelect(t *testing.T) {
	sql := "SELECT (SELECT COUNT(*) FROM orders WHERE orders.user_id = users.id) AS order_count FROM users"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasSubquery {
		t.Fatal("expected HasSubquery=true from SELECT expression")
	}
}

func TestAnalyzeSQL_SubqueryInFrom(t *testing.T) {
	sql := "SELECT sub.id FROM (SELECT id FROM users) AS sub"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasSubquery {
		t.Fatal("expected HasSubquery=true from FROM clause")
	}
}

func TestAnalyzeSQL_LimitOffset(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id ASC LIMIT 10 OFFSET 20"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasLimit {
		t.Fatal("expected HasLimit=true")
	}
	if a.LimitCount != 10 {
		t.Fatalf("expected LimitCount=10, got %d", a.LimitCount)
	}
	if a.LimitOffset != 20 {
		t.Fatalf("expected LimitOffset=20, got %d", a.LimitOffset)
	}
	if !a.HasOrderBy {
		t.Fatal("expected HasOrderBy=true")
	}
	if len(a.OrderByKeys) != 1 {
		t.Fatalf("expected 1 order key, got %d", len(a.OrderByKeys))
	}
	if a.OrderByKeys[0].Column != "id" || a.OrderByKeys[0].Desc {
		t.Fatalf("expected id ASC, got %+v", a.OrderByKeys[0])
	}
}

func TestAnalyzeSQL_OrderByDesc(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY created_at DESC, id ASC"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.OrderByKeys) != 2 {
		t.Fatalf("expected 2 order keys, got %d", len(a.OrderByKeys))
	}
	if !a.OrderByKeys[0].Desc {
		t.Fatal("expected first key DESC")
	}
	if a.OrderByKeys[1].Desc {
		t.Fatal("expected second key ASC")
	}
}

func TestAnalyzeSQL_MySQLBackticks(t *testing.T) {
	sql := "SELECT `id`, `name` FROM `users` WHERE `active` = 1"
	a, err := AnalyzeSQL(sql, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Tables) != 1 || a.Tables[0].Table != "users" {
		t.Fatalf("expected table 'users', got %+v", a.Tables)
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere=true")
	}
}

func TestAnalyzeSQL_MySQLLimitOffsetComma(t *testing.T) {
	sql := "SELECT `id` FROM `users` LIMIT 10, 20"
	a, err := AnalyzeSQL(sql, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasLimit {
		t.Fatal("expected HasLimit=true")
	}
	if a.LimitCount != 20 {
		t.Fatalf("expected LimitCount=20, got %d", a.LimitCount)
	}
	if a.LimitOffset != 10 {
		t.Fatalf("expected LimitOffset=10, got %d", a.LimitOffset)
	}
}

func TestAnalyzeSQL_Insert(t *testing.T) {
	a, err := AnalyzeSQL("INSERT INTO users (name) VALUES ('test')", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatementType != "INSERT" {
		t.Fatalf("expected INSERT, got %s", a.StatementType)
	}
	if a.IsQuery {
		t.Fatal("expected IsQuery=false for INSERT")
	}
}

func TestAnalyzeSQL_Update(t *testing.T) {
	a, err := AnalyzeSQL("UPDATE users SET name = 'test' WHERE id = 1", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatementType != "UPDATE" {
		t.Fatalf("expected UPDATE, got %s", a.StatementType)
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere=true for UPDATE with WHERE")
	}
}

func TestAnalyzeSQL_UpdateNoWhere(t *testing.T) {
	a, err := AnalyzeSQL("UPDATE users SET name = 'test'", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.HasWhere {
		t.Fatal("expected HasWhere=false for UPDATE without WHERE")
	}
}

func TestAnalyzeSQL_Delete(t *testing.T) {
	a, err := AnalyzeSQL("DELETE FROM users WHERE id = 1", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.StatementType != "DELETE" {
		t.Fatalf("expected DELETE, got %s", a.StatementType)
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere=true for DELETE")
	}
}

func TestAnalyzeSQL_DeleteNoWhere(t *testing.T) {
	a, err := AnalyzeSQL("DELETE FROM users", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if a.HasWhere {
		t.Fatal("expected HasWhere=false")
	}
}

func TestAnalyzeSQL_SelectStar(t *testing.T) {
	a, err := AnalyzeSQL("SELECT * FROM users", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasSelectStar {
		t.Fatal("expected HasSelectStar=true")
	}
}

func TestAnalyzeSQL_ColumnResolution(t *testing.T) {
	sql := "SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	a.ResolveColumnSources()

	found := map[string]string{}
	for _, c := range a.Columns {
		found[c.Column] = c.Table
	}
	if found["id"] != "users" {
		t.Fatalf("expected id→users, got id→%s", found["id"])
	}
	if found["total"] != "orders" {
		t.Fatalf("expected total→orders, got total→%s", found["total"])
	}
}

func TestAnalyzeSQL_SchemaQualified(t *testing.T) {
	sql := "SELECT id FROM public.users"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(a.Tables))
	}
	if a.Tables[0].Schema != "public" || a.Tables[0].Table != "users" {
		t.Fatalf("expected public.users, got %+v", a.Tables[0])
	}
	if a.PrimaryTable != "public.users" {
		t.Fatalf("expected PrimaryTable='public.users', got %s", a.PrimaryTable)
	}
}

func TestAnalyzeSQL_GroupBy(t *testing.T) {
	sql := "SELECT department, COUNT(*) FROM employees GROUP BY department"
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasGroupBy {
		t.Fatal("expected HasGroupBy=true")
	}
}

func TestAnalyzeSQL_ParseError(t *testing.T) {
	_, err := AnalyzeSQL("THIS IS NOT SQL AT ALL !!!", "postgres")
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}

func TestAnalyzeSQL_ComplexJoinCTEUnion(t *testing.T) {
	sql := `WITH recent_orders AS (
		SELECT user_id, SUM(total) as total FROM orders
		WHERE created_at > '2024-01-01' GROUP BY user_id
	)
	SELECT u.name, ro.total
	FROM users u
	JOIN recent_orders ro ON u.id = ro.user_id
	WHERE ro.total > 1000
	ORDER BY ro.total DESC
	LIMIT 50`
	a, err := AnalyzeSQL(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasCTE {
		t.Fatal("expected HasCTE")
	}
	if !a.HasJoin {
		t.Fatal("expected HasJoin")
	}
	if !a.HasWhere {
		t.Fatal("expected HasWhere")
	}
	if !a.HasOrderBy {
		t.Fatal("expected HasOrderBy")
	}
	if !a.HasLimit {
		t.Fatal("expected HasLimit")
	}
	if a.LimitCount != 50 {
		t.Fatalf("expected LimitCount=50, got %d", a.LimitCount)
	}
}

func TestCachedAnalyzeSQL(t *testing.T) {
	sql := "SELECT id FROM users"
	a1, err1 := CachedAnalyzeSQL(sql, "postgres")
	if err1 != nil {
		t.Fatal(err1)
	}
	a2, err2 := CachedAnalyzeSQL(sql, "postgres")
	if err2 != nil {
		t.Fatal(err2)
	}
	if a1 != a2 {
		t.Fatal("expected same pointer from cache")
	}

	// Different SQL should not return cached result.
	a3, _ := CachedAnalyzeSQL("SELECT name FROM products", "postgres")
	if a3 == a1 {
		t.Fatal("expected different result for different SQL")
	}
}
