package riskengine

import (
	"testing"
)

func TestFirstKeyword(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT * FROM users", "select"},
		{"  DELETE FROM orders", "delete"},
		{"-- comment\nSELECT 1", "select"},
		{"/* block */SELECT 1", "select"},
		{"/* unclosed", ""},
		{"", ""},
		{"   ", ""},
		{"DROP TABLE foo", "drop"},
		{"FLUSHALL", "flushall"},
	}
	for _, tt := range tests {
		got := FirstKeyword(tt.input)
		if got != tt.want {
			t.Errorf("FirstKeyword(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSQLStatementHasWhereClause(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"SELECT * FROM users WHERE id = 1", true},
		{"SELECT * FROM users", false},
		{"DELETE FROM users", false},
		{"DELETE FROM users WHERE id = 1", true},
		{"UPDATE users SET name = 'bob' WHERE id = 1", true},
		{"UPDATE users SET name = 'bob'", false},
	}
	for _, tt := range tests {
		got := SQLStatementHasWhereClause(tt.input)
		if got != tt.want {
			t.Errorf("SQLStatementHasWhereClause(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStripSQLStringLiteralsAndComments(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT 'hello' FROM t", "SELECT         FROM t"},
		{"-- line comment\nSELECT 1", "                \nSELECT 1"},
		{"/* block */SELECT 1", "           SELECT 1"},
		{"SELECT 'it''s'", "SELECT        "},
	}
	for _, tt := range tests {
		got := StripSQLStringLiteralsAndComments(tt.input)
		if len(got) != len(tt.input) {
			t.Errorf("StripSQLStringLiteralsAndComments(%q) length = %d, want %d (same as input)", tt.input, len(got), len(tt.input))
		}
	}
}

func TestNormalizeSQLIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"`users`", "users"},
		{`"Users"`, "users"},
		{"schema.table", "table"},
		{"  TABLE  ", "table"},
	}
	for _, tt := range tests {
		got := NormalizeSQLIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeSQLIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeMongoAction(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"findOne", "findone"},
		{"insert_one", "insertone"},
		{"delete-many", "deletemany"},
		{"  Find  ", "find"},
	}
	for _, tt := range tests {
		got := NormalizeMongoAction(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeMongoAction(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRedisCommandIsLowRisk(t *testing.T) {
	low := []string{"GET", "TYPE", "TTL", "PTTL", "EXISTS", "STRLEN", "HGET", "HEXISTS", "HLEN", "LINDEX", "LLEN", "SCARD", "SISMEMBER", "ZCARD", "ZSCORE"}
	for _, cmd := range low {
		if !RedisCommandIsLowRisk(cmd) {
			t.Errorf("RedisCommandIsLowRisk(%q) = false, want true", cmd)
		}
	}
	notLow := []string{"SET", "DEL", "FLUSHALL", "KEYS", "WAIT", "SELECT", "UNKNOWN"}
	for _, cmd := range notLow {
		if RedisCommandIsLowRisk(cmd) {
			t.Errorf("RedisCommandIsLowRisk(%q) = true, want false", cmd)
		}
	}
}

func TestMongoShellStatementIsLowRisk(t *testing.T) {
	low := []string{
		"show collections",
		"show dbs",
		"show databases",
		"db.users.find({})",
		"db.orders.findOne({id: 1})",
		"db.orders.aggregate([])",
	}
	for _, stmt := range low {
		if !MongoShellStatementIsLowRisk(stmt) {
			t.Errorf("MongoShellStatementIsLowRisk(%q) = false, want true", stmt)
		}
	}
	notLow := []string{
		"db.users.drop()",
		"db.users.deleteMany({})",
		"db.users.insertOne({})",
		"",
	}
	for _, stmt := range notLow {
		if MongoShellStatementIsLowRisk(stmt) {
			t.Errorf("MongoShellStatementIsLowRisk(%q) = true, want false", stmt)
		}
	}
}

func TestParseElasticsearchRequestShape(t *testing.T) {
	tests := []struct {
		input      string
		wantMethod string
		wantPath   string
		wantOK     bool
	}{
		{"GET /logs/_search\n{}", "GET", "/logs/_search", true},
		{"POST /_search\n{}", "POST", "/_search", true},
		{"DELETE /my-index", "DELETE", "/my-index", true},
		{"INVALID /path", "", "", false},
		{"GET", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		method, path, _, ok := ParseElasticsearchRequestShape(tt.input)
		if ok != tt.wantOK || method != tt.wantMethod || path != tt.wantPath {
			t.Errorf("ParseElasticsearchRequestShape(%q) = (%q, %q, _, %v), want (%q, %q, _, %v)",
				tt.input, method, path, ok, tt.wantMethod, tt.wantPath, tt.wantOK)
		}
	}
}

func TestElasticsearchPathIsSearch(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/_search", true},
		{"/logs/_search", true},
		{"/logs/_search?pretty=true", true},
		{"/logs/_doc/1", false},
		{"/logs", false},
	}
	for _, tt := range tests {
		got := ElasticsearchPathIsSearch(tt.input)
		if got != tt.want {
			t.Errorf("ElasticsearchPathIsSearch(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDynamodbStatementTableName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT * FROM Orders", "Orders"},
		{"SELECT * FROM tenant.orders", "tenant.orders"},
		{"SELECT * FROM \"My Table\"", "My Table"},
		{"INSERT INTO Orders VALUE {'id': '1'}", ""}, // FROM-only pattern
		{"DELETE FROM Orders WHERE id = '1'", "Orders"},
		{"", ""},
	}
	for _, tt := range tests {
		got := DynamodbStatementTableName(tt.input)
		if got != tt.want {
			t.Errorf("DynamodbStatementTableName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSQLTargetTable(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT * FROM users WHERE id = 1", "users"},
		{"DELETE FROM orders WHERE id = 1", "orders"},
		{"INSERT INTO products (name) VALUES ('foo')", "products"},
		{"UPDATE accounts SET balance = 0", "accounts"},
		{"DROP TABLE temp", "temp"},
		{"SELECT 1", ""},
	}
	for _, tt := range tests {
		got := SQLTargetTable(tt.input)
		if got != tt.want {
			t.Errorf("SQLTargetTable(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestElasticsearchTargetIndex(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/logs/_search", "logs"},
		{"/logs/_search?pretty=true", "logs"},
		{"/logs-2024/_doc/1", "logs-2024"},
		{"/_search", ""},
		{"/_cat/indices", ""},
		{"/production-data/_count", "production-data"},
	}
	for _, tt := range tests {
		got := ElasticsearchTargetIndex(tt.input)
		if got != tt.want {
			t.Errorf("ElasticsearchTargetIndex(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSQLEqualityFields(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"SELECT * FROM t WHERE id = 1", []string{"id"}},
		{"SELECT * FROM t WHERE id = 1 AND name = 'bob'", []string{"id", "name"}},
		{"SELECT * FROM t", nil},
	}
	for _, tt := range tests {
		got := SQLEqualityFields(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("SQLEqualityFields(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		for _, f := range tt.want {
			if _, ok := got[f]; !ok {
				t.Errorf("SQLEqualityFields(%q) missing field %q", tt.input, f)
			}
		}
	}
}

func TestElasticsearchBodyRisks(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantEmpty bool     // true = no risks
		wantAny   []string // any of these substrings should appear in reasons
	}{
		{"empty body", "", true, nil},
		{"safe bounded search", `{"query":{"term":{"status":"ok"}},"size":10}`, true, nil},
		{"no query field", `{"size":10}`, false, []string{"no query field"}},
		{"match_all without filter", `{"query":{"match_all":{}},"size":10}`, false, []string{"broad query"}},
		{"wildcard query", `{"query":{"wildcard":{"name":"test*"}},"size":10}`, false, []string{"broad query"}},
		{"regexp query", `{"query":{"regexp":{"name":"test.*"}},"size":10}`, false, []string{"broad query"}},
		{"fuzzy query", `{"query":{"fuzzy":{"name":{"value":"test"}}},"size":10}`, false, []string{"broad query"}},
		{"script query", `{"query":{"script_score":{"query":{"match_all":{}},"script":{"source":"1"}}},"size":10}`, false, []string{"script"}},
		{"size > 10000", `{"query":{"term":{"x":"y"}},"size":50000}`, false, []string{"exceeds 10000"}},
		{"deep pagination", `{"query":{"term":{"x":"y"}},"from":15000,"size":10}`, false, []string{"deep pagination"}},
		{"aggs without query", `{"size":0,"aggs":{"x":{"terms":{"field":"f","size":10}}}}`, false, []string{"aggregation without query"}},
		{"terms agg no size", `{"query":{"term":{"x":"y"}},"size":0,"aggs":{"x":{"terms":{"field":"f"}}}}`, false, []string{"terms aggregation without size"}},
		{"deep nested aggs", `{"query":{"term":{"x":"y"}},"size":0,"aggs":{"l1":{"terms":{"field":"a","size":5},"aggs":{"l2":{"terms":{"field":"b","size":5},"aggs":{"l3":{"terms":{"field":"c","size":5},"aggs":{"l4":{"terms":{"field":"d","size":5}}}}}}}}}}`, false, []string{"nested aggregation depth"}},
		{"safe aggs", `{"query":{"term":{"x":"y"}},"size":0,"aggs":{"x":{"terms":{"field":"f","size":10}}}}`, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risks := ElasticsearchBodyRisks(tt.body)
			if tt.wantEmpty && len(risks) > 0 {
				t.Errorf("expected no risks, got %v", risks)
			}
			if !tt.wantEmpty && len(risks) == 0 {
				t.Errorf("expected risks containing %v, got none", tt.wantAny)
			}
			if len(tt.wantAny) > 0 && len(risks) > 0 {
				joined := ""
				for _, r := range risks {
					joined += r + " "
				}
				for _, want := range tt.wantAny {
					found := false
					for _, r := range risks {
						if contains(r, want) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("risks %v missing expected substring %q", risks, want)
					}
				}
			}
		})
	}
}

func TestElasticsearchPathQueryRisks(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantEmpty bool
		wantAny   []string
	}{
		{"no query", "/logs/_search", true, nil},
		{"safe query string", "/logs/_search?q=status:ok&size=10", true, nil},
		{"broad query string", "/logs/_search?q=*:*&size=10", false, []string{"broad query"}},
		{"large size", "/logs/_search?q=status:ok&size=20000", false, []string{"exceeds 10000"}},
		{"deep pagination", "/logs/_search?q=status:ok&from=9990&size=20", false, []string{"deep pagination"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risks := ElasticsearchPathQueryRisks(tt.path)
			if tt.wantEmpty && len(risks) > 0 {
				t.Fatalf("expected no risks, got %v", risks)
			}
			if !tt.wantEmpty && len(risks) == 0 {
				t.Fatalf("expected risks containing %v, got none", tt.wantAny)
			}
			for _, want := range tt.wantAny {
				found := false
				for _, risk := range risks {
					if contains(risk, want) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("risks %v missing expected substring %q", risks, want)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSqlActualKeywordAfterCTE(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", "select"},
		{"WITH cte AS (SELECT id FROM users) DELETE FROM users WHERE id IN (SELECT id FROM cte)", "delete"},
		{"WITH src AS (SELECT * FROM raw) INSERT INTO target SELECT * FROM src", "insert"},
		{"WITH src AS (SELECT id FROM staging) UPDATE target SET x = 1 WHERE id IN (SELECT id FROM src)", "update"},
		{"SELECT * FROM users", ""},
		{"WITH cte AS (SELECT 1", ""}, // malformed, no closing paren
	}
	for _, tt := range tests {
		got := sqlActualKeywordAfterCTE(tt.input)
		if got != tt.want {
			t.Errorf("sqlActualKeywordAfterCTE(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDynamodbStatementIsLowRisk(t *testing.T) {
	describe := map[string]any{
		"details": []map[string]any{
			{"label": "Partition Key", "value": "pk"},
			{"label": "Sort Key", "value": "sk"},
		},
		"indexes": []map[string]any{
			{
				"name":       "GenreAndPriceIndex",
				"definition": "genre=HASH | price=RANGE | projection=ALL",
			},
		},
	}
	tests := []struct {
		name string
		stmt string
		want bool
	}{
		{"table partition key equality", "SELECT * FROM Orders WHERE pk = 'abc'", true},
		{"composite key with between", "SELECT * FROM Orders WHERE pk = 'abc' AND sk BETWEEN 100 AND 200", true},
		{"composite key with begins_with", "SELECT * FROM Orders WHERE pk = 'abc' AND begins_with(sk, '2026-')", true},
		{"partition key IN", "SELECT * FROM Orders WHERE pk IN ['a', 'b']", true},
		{"secondary index partition key equality", `SELECT * FROM "Orders"."GenreAndPriceIndex" WHERE genre = 'rock'`, true},
		{"secondary index sort key range", `SELECT * FROM "Orders"."GenreAndPriceIndex" WHERE genre = 'country' AND price < 0.50`, true},
		{"sort key without partition key", "SELECT * FROM Orders WHERE sk BETWEEN 100 AND 200", false},
		{"secondary index sort key without partition key", "SELECT * FROM Orders.GenreAndPriceIndex WHERE price < 0.50", false},
		{"non-key filter only", "SELECT * FROM Orders WHERE other = 'abc'", false},
		{"full table scan", "SELECT * FROM Orders", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DynamodbStatementIsLowRisk(tt.stmt, describe)
			if got != tt.want {
				t.Errorf("DynamodbStatementIsLowRisk(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
		})
	}
}

func TestDynamodbStatementTarget(t *testing.T) {
	tests := []struct {
		name      string
		stmt      string
		wantTable string
		wantIndex string
	}{
		{"table only", "SELECT * FROM Orders WHERE pk = 'abc'", "Orders", ""},
		{"dotted table preserved", "SELECT * FROM Orders.GenreAndPriceIndex WHERE genre = 'rock'", "Orders.GenreAndPriceIndex", ""},
		{"quoted table and index", `SELECT * FROM "Orders"."GenreAndPriceIndex" WHERE genre = 'rock'`, "Orders", "GenreAndPriceIndex"},
		{"tenant dotted table preserved", "SELECT * FROM tenant.orders WHERE pk = 'abc'", "tenant.orders", ""},
		{"insert only has table", "INSERT INTO Orders VALUE {'pk': 'abc'}", "Orders", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tableName, indexName := DynamodbStatementTarget(tt.stmt)
			if tableName != tt.wantTable || indexName != tt.wantIndex {
				t.Fatalf("DynamodbStatementTarget(%q) = (%q, %q), want (%q, %q)", tt.stmt, tableName, indexName, tt.wantTable, tt.wantIndex)
			}
		})
	}
}
