package riskengine

import (
	"testing"

	"futrixdata/platform/internal/rediscmd"
)

func TestParseStatement_SQL(t *testing.T) {
	tests := []struct {
		name         string
		dsType       string
		statement    string
		wantKeyword  string
		wantEntity   string
		wantHasWhere bool
		wantIsQuery  bool
	}{
		{"select", "mysql", "SELECT * FROM users WHERE id = 1", "select", "users", true, true},
		{"delete with where", "postgresql", "DELETE FROM orders WHERE id = 1", "delete", "orders", true, false},
		{"delete no where", "mysql", "DELETE FROM orders", "delete", "orders", false, false},
		{"update with where", "d1", "UPDATE users SET name = 'bob' WHERE id = 1", "update", "users", true, false},
		{"drop", "mysql", "DROP TABLE temp", "drop", "temp", false, false},
		{"insert", "postgresql", "INSERT INTO products (name) VALUES ('foo')", "insert", "products", false, false},
		{"show", "mysql", "SHOW TABLES", "show", "", false, true},
		{"describe", "mysql", "DESCRIBE users", "describe", "", false, true},
		{"explain", "mysql", "EXPLAIN SELECT * FROM users", "explain", "users", false, true},
		{"with select", "mysql", "WITH cte AS (SELECT * FROM users) SELECT * FROM cte", "select", "users", false, true},
		{"with delete", "mysql", "WITH old AS (SELECT id FROM users WHERE age > 99) DELETE FROM users WHERE id IN (SELECT id FROM old)", "delete", "users", true, false},
		{"with update", "postgresql", "WITH src AS (SELECT id FROM staging) UPDATE target SET active = true WHERE id IN (SELECT id FROM src)", "update", "target", true, false},
		{"with insert", "postgresql", "WITH src AS (SELECT * FROM raw) INSERT INTO target SELECT * FROM src", "insert", "target", false, false},
		{"cte update outer select", "postgresql", "WITH changed AS (UPDATE users SET active = true WHERE id = 7 RETURNING id) SELECT * FROM tmp_logs", "update", "users", true, false},
		{"multi cte writes", "postgresql", "WITH audit AS (INSERT INTO audit_logs(id) VALUES (1) RETURNING id), cleanup AS (DELETE FROM users WHERE id = 1 RETURNING id) SELECT * FROM reports", "delete", "users", true, false},
		{"schema update", "postgresql", "UPDATE schema_a.orders SET status = 'done' WHERE id = 42", "update", "schema_a.orders", true, false},
		{"delete using", "postgresql", "DELETE FROM accounts USING archived_accounts WHERE accounts.id = archived_accounts.id", "delete", "accounts", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement(tt.dsType, "ds1", tt.statement)
			if ps.FirstKeyword != tt.wantKeyword {
				t.Errorf("FirstKeyword = %q, want %q", ps.FirstKeyword, tt.wantKeyword)
			}
			if ps.TargetEntity != tt.wantEntity {
				t.Errorf("TargetEntity = %q, want %q", ps.TargetEntity, tt.wantEntity)
			}
			if ps.HasWhere != tt.wantHasWhere {
				t.Errorf("HasWhere = %v, want %v", ps.HasWhere, tt.wantHasWhere)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestParseStatement_MongoDB_JSON(t *testing.T) {
	tests := []struct {
		name        string
		statement   string
		wantAction  string
		wantEntity  string
		wantIsQuery bool
	}{
		{"find", `{"action":"find","collection":"users","filter":{}}`, "find", "users", true},
		{"aggregate", `{"action":"aggregate","collection":"orders"}`, "aggregate", "orders", true},
		{"drop", `{"action":"drop","collection":"temp"}`, "drop", "temp", false},
		{"insertOne", `{"action":"insertOne","collection":"users"}`, "insertone", "users", false},
		{"deleteMany", `{"action":"deleteMany","collection":"logs"}`, "deletemany", "logs", false},
		{"replaceOne", `{"action":"replaceOne","collection":"users"}`, "replaceone", "users", false},
		{"findOneAndReplace", `{"action":"findOneAndReplace","collection":"users"}`, "findoneandreplace", "users", false},
		{"findOneAndDelete", `{"action":"findOneAndDelete","collection":"users"}`, "findoneanddelete", "users", false},
		{"bulkWrite", `{"action":"bulkWrite","collection":"users"}`, "bulkwrite", "users", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement("mongodb", "ds1", tt.statement)
			if ps.MongoAction != tt.wantAction {
				t.Errorf("MongoAction = %q, want %q", ps.MongoAction, tt.wantAction)
			}
			if ps.TargetEntity != tt.wantEntity {
				t.Errorf("TargetEntity = %q, want %q", ps.TargetEntity, tt.wantEntity)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestParseStatement_MongoDB_Shell(t *testing.T) {
	tests := []struct {
		name        string
		statement   string
		wantEntity  string
		wantAction  string
		wantIsQuery bool
	}{
		{"find", "db.users.find({})", "users", "find", true},
		{"findOne fallback", "db.users.findOne({})", "users", "findone", true},
		{"getCollection", `db.getCollection("users").find({})`, "users", "find", true},
		{"getSiblingDB getCollection", `db.getSiblingDB("analytics").getCollection("users").find({})`, "users", "find", true},
		{"getSiblingDB direct collection", `db.getSiblingDB("analytics").users.find({})`, "users", "find", true},
		{"aggregate", "db.orders.aggregate([])", "orders", "aggregate", true},
		{"drop", "db.temp.drop()", "temp", "drop", false},
		{"replaceOne", "db.users.replaceOne({_id: 1}, {name: 'bob'})", "users", "replaceone", false},
		{"findOneAndReplace", "db.users.findOneAndReplace({_id: 1}, {name: 'bob'})", "users", "findoneandreplace", false},
		{"findOneAndDelete", "db.users.findOneAndDelete({_id: 1})", "users", "findoneanddelete", false},
		{"bulkWrite", "db.users.bulkWrite([{insertOne: {document: {name: 'bob'}}}])", "users", "bulkwrite", false},
		{"show", "show collections", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement("mongodb", "ds1", tt.statement)
			if ps.TargetEntity != tt.wantEntity {
				t.Errorf("TargetEntity = %q, want %q", ps.TargetEntity, tt.wantEntity)
			}
			if ps.MongoAction != tt.wantAction {
				t.Errorf("MongoAction = %q, want %q", ps.MongoAction, tt.wantAction)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestParseStatement_MongoDB_AggregateTargetEntities(t *testing.T) {
	ps := ParseStatement("mongodb", "ds1", `{"action":"aggregate","collection":"orders","pipeline":[{"$lookup":{"from":"users","localField":"userId","foreignField":"_id","as":"user"}},{"$unionWith":"archived_orders"}]}`)
	if ps.TargetEntity != "orders" {
		t.Fatalf("TargetEntity = %q, want orders", ps.TargetEntity)
	}
	if !ps.IsQuery {
		t.Fatalf("IsQuery = false, want true")
	}
	if !ps.HasJoin {
		t.Fatalf("HasJoin = false, want true")
	}
	if ps.JoinCount != 2 {
		t.Fatalf("JoinCount = %d, want 2", ps.JoinCount)
	}
	got := ps.ScopeEntities()
	want := []string{"orders", "users", "archived_orders"}
	if len(got) != len(want) {
		t.Fatalf("ScopeEntities len = %d, want %d (%v)", len(got), len(want), got)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("ScopeEntities[%d] = %q, want %q (all: %v)", idx, got[idx], want[idx], got)
		}
	}
}

func TestParseStatement_Elasticsearch(t *testing.T) {
	tests := []struct {
		name        string
		statement   string
		wantMethod  string
		wantPath    string
		wantEntity  string
		wantIsQuery bool
	}{
		{"get search", "GET /logs/_search\n{}", "GET", "/logs/_search", "logs", true},
		{"post search", "POST /orders/_search\n{}", "POST", "/orders/_search", "orders", true},
		{"delete index", "DELETE /my-index", "DELETE", "/my-index", "my-index", false},
		{"put doc", "PUT /users/_doc/1\n{}", "PUT", "/users/_doc/1", "users", false},
		{"get cat", "GET /_cat/indices", "GET", "/_cat/indices", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement("elasticsearch", "ds1", tt.statement)
			if ps.HTTPMethod != tt.wantMethod {
				t.Errorf("HTTPMethod = %q, want %q", ps.HTTPMethod, tt.wantMethod)
			}
			if ps.URLPath != tt.wantPath {
				t.Errorf("URLPath = %q, want %q", ps.URLPath, tt.wantPath)
			}
			if ps.TargetEntity != tt.wantEntity {
				t.Errorf("TargetEntity = %q, want %q", ps.TargetEntity, tt.wantEntity)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestParseStatement_Redis(t *testing.T) {
	tests := []struct {
		name        string
		statement   string
		wantCommand string
		wantKey     string
		wantIsQuery bool
	}{
		{"get", "GET mykey", "GET", "mykey", true},
		{"set", "SET mykey value", "SET", "mykey", false},
		{"del", "DEL mykey", "DEL", "mykey", false},
		{"flushall", "FLUSHALL", "FLUSHALL", "", false},
		{"keys", "KEYS *", "KEYS", "*", false},
		{"hget", "HGET myhash field", "HGET", "myhash", true},
		{"quoted key with whitespace", `SET "pd: secret" value`, "SET", "pd: secret", false},
		{"escaped quoted key", `GET "pd:\"secret\""`, "GET", `pd:"secret"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement("redis", "ds1", tt.statement)
			if ps.RedisCommand != tt.wantCommand {
				t.Errorf("RedisCommand = %q, want %q", ps.RedisCommand, tt.wantCommand)
			}
			if ps.KeyPattern != tt.wantKey {
				t.Errorf("KeyPattern = %q, want %q", ps.KeyPattern, tt.wantKey)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestSplitRedisArgsKeepsEmptyQuotedArgs(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      []string
	}{
		{
			name:      "empty key remains positional",
			statement: `SET "" v`,
			want:      []string{"SET", "", "v"},
		},
		{
			name:      "empty eval script keeps numkeys position",
			statement: `EVAL "" 1 key`,
			want:      []string{"EVAL", "", "1", "key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rediscmd.Parse(tt.statement)
			if err != nil {
				t.Fatalf("rediscmd.Parse(%q) error = %v", tt.statement, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("rediscmd.Parse(%q) len = %d (%#v), want %d (%#v)", tt.statement, len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("rediscmd.Parse(%q)[%d] = %q, want %q (got %#v)", tt.statement, i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestParseStatement_RedisLuaEvalExtractsScriptKeys(t *testing.T) {
	ps := ParseStatement("redis", "ds1", `EVAL "redis.call('DEL', KEYS[1])" 1 pd:1`)
	if ps.RedisCommand != "EVAL" {
		t.Fatalf("RedisCommand = %q, want EVAL", ps.RedisCommand)
	}
	if ps.KeyPattern != "pd:1" {
		t.Fatalf("KeyPattern = %q, want pd:1", ps.KeyPattern)
	}
	if !operationHasClass(ps.Operation, operationClassScript) {
		t.Fatalf("expected script operation class, got %#v", ps.Operation.Classes)
	}
	if !operationHasClass(ps.Operation, operationClassWrite) {
		t.Fatalf("expected write operation class from inner DEL, got %#v", ps.Operation.Classes)
	}
}

func TestParseStatement_RedisEvalEmptyScriptKeepsKeyPosition(t *testing.T) {
	ps := ParseStatement("redis", "ds1", `EVAL "" 1 key`)
	if ps.KeyPattern != "key" {
		t.Fatalf("KeyPattern = %q, want key", ps.KeyPattern)
	}
	if len(ps.Args) != 3 || ps.Args[0] != "" || ps.Args[1] != "1" || ps.Args[2] != "key" {
		t.Fatalf("Args = %#v, want []string{\"\", \"1\", \"key\"}", ps.Args)
	}
}

func TestParseRedisCommandArgs_LuaEvalExtractsOperationIntent(t *testing.T) {
	ps := ParseRedisCommandArgs("ds1", []string{"EVAL", "return redis.call('DEL', KEYS[1])", "1", "pd:1"})
	if ps.RedisCommand != "EVAL" {
		t.Fatalf("RedisCommand = %q, want EVAL", ps.RedisCommand)
	}
	if ps.KeyPattern != "pd:1" {
		t.Fatalf("KeyPattern = %q, want pd:1", ps.KeyPattern)
	}
	if !operationHasClass(ps.Operation, operationClassScript) {
		t.Fatalf("expected script operation class, got %#v", ps.Operation.Classes)
	}
	if !operationHasClass(ps.Operation, operationClassWrite) {
		t.Fatalf("expected write operation class from argv inner DEL, got %#v", ps.Operation.Classes)
	}
}

func TestParseStatement_DynamoDB(t *testing.T) {
	tests := []struct {
		name         string
		statement    string
		wantKeyword  string
		wantEntity   string
		wantTable    string
		wantIndex    string
		wantHasWhere bool
		wantIsQuery  bool
	}{
		{"select", "SELECT * FROM Orders WHERE pk = 'abc'", "select", "Orders", "Orders", "", true, true},
		{"select no where", "SELECT * FROM Orders", "select", "Orders", "Orders", "", false, true},
		{"select index", "SELECT * FROM Orders.GenreAndPriceIndex WHERE genre = 'rock'", "select", "Orders.GenreAndPriceIndex", "Orders.GenreAndPriceIndex", "", true, true},
		{"select quoted index", `SELECT * FROM "Orders"."GenreAndPriceIndex" WHERE genre = 'rock'`, "select", "Orders", "Orders", "GenreAndPriceIndex", true, true},
		{"insert", "INSERT INTO Orders VALUE {'id': '1'}", "insert", "Orders", "Orders", "", false, false},
		{"delete", "DELETE FROM Orders WHERE pk = 'abc'", "delete", "Orders", "Orders", "", true, false},
		{"update", "UPDATE Orders SET status = 'done' WHERE pk = 'abc'", "update", "Orders", "Orders", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseStatement("dynamodb", "ds1", tt.statement)
			if ps.FirstKeyword != tt.wantKeyword {
				t.Errorf("FirstKeyword = %q, want %q", ps.FirstKeyword, tt.wantKeyword)
			}
			if ps.TargetEntity != tt.wantEntity {
				t.Errorf("TargetEntity = %q, want %q", ps.TargetEntity, tt.wantEntity)
			}
			if ps.DynamoTable != tt.wantTable {
				t.Errorf("DynamoTable = %q, want %q", ps.DynamoTable, tt.wantTable)
			}
			if ps.DynamoIndex != tt.wantIndex {
				t.Errorf("DynamoIndex = %q, want %q", ps.DynamoIndex, tt.wantIndex)
			}
			if ps.HasWhere != tt.wantHasWhere {
				t.Errorf("HasWhere = %v, want %v", ps.HasWhere, tt.wantHasWhere)
			}
			if ps.IsQuery != tt.wantIsQuery {
				t.Errorf("IsQuery = %v, want %v", ps.IsQuery, tt.wantIsQuery)
			}
		})
	}
}

func TestParseStatement_Empty(t *testing.T) {
	ps := ParseStatement("mysql", "ds1", "")
	if ps.FirstKeyword != "" {
		t.Errorf("empty statement should have empty FirstKeyword, got %q", ps.FirstKeyword)
	}
}

func TestParseStatement_UnknownType(t *testing.T) {
	ps := ParseStatement("unknown", "ds1", "SOME COMMAND")
	if ps.DsType != "unknown" {
		t.Errorf("DsType = %q, want 'unknown'", ps.DsType)
	}
	if ps.Raw != "SOME COMMAND" {
		t.Errorf("Raw = %q, want 'SOME COMMAND'", ps.Raw)
	}
}
