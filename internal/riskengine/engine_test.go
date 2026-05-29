package riskengine

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEngine_SQL_Parity(t *testing.T) {
	e := NewEngine()
	tests := []struct {
		name       string
		dsType     string
		statement  string
		wantAction Action
		wantLevel  RiskLevel
	}{
		// Read operations -> allow
		{"select", "mysql", "SELECT * FROM users", ActionAllow, RiskLow},
		{"show", "postgresql", "SHOW TABLES", ActionAllow, RiskLow},
		{"describe", "d1", "DESCRIBE users", ActionAllow, RiskLow},
		{"explain", "mysql", "EXPLAIN SELECT * FROM users", ActionAllow, RiskLow},

		// Destructive DDL -> block
		{"drop", "mysql", "DROP TABLE users", ActionBlock, RiskHigh},
		{"truncate", "postgresql", "TRUNCATE TABLE orders", ActionBlock, RiskHigh},

		// Permission change -> block
		{"grant", "mysql", "GRANT SELECT ON db.* TO user", ActionBlock, RiskHigh},
		{"revoke", "postgresql", "REVOKE ALL ON db.* FROM user", ActionBlock, RiskHigh},

		// DELETE/UPDATE without WHERE -> block
		{"delete no where", "mysql", "DELETE FROM orders", ActionBlock, RiskHigh},
		{"update no where", "postgresql", "UPDATE users SET active = 0", ActionBlock, RiskHigh},

		// DELETE/UPDATE with WHERE -> warn
		{"delete where", "mysql", "DELETE FROM orders WHERE id = 1", ActionWarn, RiskMedium},
		{"update where", "d1", "UPDATE users SET name = 'x' WHERE id = 1", ActionWarn, RiskMedium},

		// INSERT -> warn. REPLACE is MySQL-specific and falls back to parser-failure review.
		{"insert", "mysql", "INSERT INTO users (name) VALUES ('bob')", ActionWarn, RiskMedium},
		{"replace", "mysql", "REPLACE INTO users (id, name) VALUES (1, 'bob')", ActionRequireApproval, RiskHigh},

		// DDL -> warn
		{"alter", "mysql", "ALTER TABLE users ADD COLUMN age INT", ActionWarn, RiskMedium},
		{"create", "postgresql", "CREATE TABLE temp (id INT)", ActionWarn, RiskMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Assess(tt.dsType, "ds1", tt.statement)
			if result.Action != tt.wantAction {
				t.Errorf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, tt.wantAction, result.RuleID, result.Reasons)
			}
			if result.Level != tt.wantLevel {
				t.Errorf("Level = %s, want %s", result.Level, tt.wantLevel)
			}
		})
	}
}

func TestEngine_SQLMultiStatementBlocksBeforeAllowRule(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "SELECT * FROM users; DELETE FROM users")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-multi-statement" {
		t.Fatalf("RuleID = %q, want sql-block-multi-statement", result.RuleID)
	}
	if !assessmentHasReason(result, "multiple SQL statements") {
		t.Fatalf("expected multi-statement reason, got %v", result.Reasons)
	}
}

func TestEngine_SQLMultiStatementMySQLDoesNotTreatDashDashOperatorAsComment(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "SELECT 1--1; DELETE FROM users")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-multi-statement" {
		t.Fatalf("RuleID = %q, want sql-block-multi-statement", result.RuleID)
	}
}

func TestEngine_SQLMySQLHashCommentAfterSemicolonIsNotSecondStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "SELECT 1; # trailing comment")
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("mysql hash comment should not be treated as a second statement: %+v", result)
	}
}

func TestEngine_SQLMySQLHashCommentWithQuoteDoesNotPoisonParsing(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "# don't\nSELECT 1; # trailing")
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-require-approval-parser-fallback" || result.RuleID == "sql-require-approval-unsupported" {
		t.Fatalf("mysql hash comment should not poison parser state: %+v", result)
	}
}

func TestEngine_SQLMySQLRoutineBodyIsSingleStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "CREATE PROCEDURE p() BEGIN SELECT 1; END;")
	if result.Action == ActionBlock {
		t.Fatalf("Action = block, want non-block approval/review path (rule: %s, reasons: %v)", result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("mysql routine body should not be treated as a multi-statement batch: %+v", result)
	}
}

func TestEngine_SQLMySQLRoutineBodyWithCaseExpressionIsSingleStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "CREATE PROCEDURE p() BEGIN SELECT CASE WHEN 1 = 1 THEN 1 END; END;")
	if result.Action == ActionBlock {
		t.Fatalf("Action = block, want non-block approval/review path (rule: %s, reasons: %v)", result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("mysql case expression inside routine body should not be treated as a multi-statement batch: %+v", result)
	}
}

func TestEngine_SQLD1TriggerBodyIsSingleStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("d1", "ds1", "CREATE TRIGGER trg AFTER INSERT ON users BEGIN SELECT 1; END;")
	if result.Action == ActionBlock {
		t.Fatalf("Action = block, want non-block approval/review path (rule: %s, reasons: %v)", result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("d1 trigger body should not be treated as a multi-statement batch: %+v", result)
	}
}

func TestEngine_SQLMySQLEmptyRoutineBodyPlusSecondStatementIsBlocked(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "CREATE PROCEDURE p() BEGIN END; SELECT 2")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-multi-statement" {
		t.Fatalf("RuleID = %q, want sql-block-multi-statement", result.RuleID)
	}
}

func TestEngine_SQLPostgresBackslashDoesNotEscapeQuoteAcrossStatements(t *testing.T) {
	e := NewEngine()
	result := e.Assess("postgresql", "ds1", `SELECT '\'; DELETE FROM users`)
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-multi-statement" {
		t.Fatalf("RuleID = %q, want sql-block-multi-statement", result.RuleID)
	}
}

func TestEngine_SQLPostgresEscapeStringKeepsSemicolonInsideLiteral(t *testing.T) {
	e := NewEngine()
	result := e.Assess("postgresql", "ds1", `SELECT E'abc\'; DELETE FROM users';`)
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("postgres E string semicolon should stay inside one statement: %+v", result)
	}
}

func TestEngine_SQLMySQLExecutableCommentSecondStatementIsBlocked(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "SELECT 1; /*!50000 DELETE FROM users */")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-multi-statement" {
		t.Fatalf("RuleID = %q, want sql-block-multi-statement", result.RuleID)
	}
}

func TestEngine_SQLPostgresNestedBlockCommentIsSingleStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("postgresql", "ds1", "SELECT 1; /* outer /* inner */ still comment */")
	if result.Action != ActionAllow {
		t.Fatalf("Action = %s, want allow (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("postgres nested block comment should not be treated as a second statement: %+v", result)
	}
}

func TestEngine_SQLProcedureCallRequiresApproval(t *testing.T) {
	e := NewEngine()
	result := e.Assess("postgresql", "ds1", "CALL refresh_cache()")
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want require_approval (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.Level != RiskHigh {
		t.Fatalf("Level = %s, want high", result.Level)
	}
	if result.RuleID != "sql-require-approval-procedure-call" {
		t.Fatalf("RuleID = %q, want sql-require-approval-procedure-call", result.RuleID)
	}
	if !assessmentHasReason(result, "stored procedure") {
		t.Fatalf("expected stored procedure reason, got %v", result.Reasons)
	}
}

func TestEngine_SQLDollarQuotedProcedureBodyIsSingleStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("postgresql", "ds1", "DO $$ BEGIN RAISE NOTICE 'ok'; END $$;")
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want require_approval (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID == "sql-block-multi-statement" {
		t.Fatalf("dollar-quoted procedural body should not be treated as a SQL batch: %+v", result)
	}
	if result.RuleID != "sql-require-approval-procedure-call" {
		t.Fatalf("RuleID = %q, want sql-require-approval-procedure-call", result.RuleID)
	}
}

func TestEngine_SQLParserFailureRequiresApproval(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "SELECT * FROM")
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want require_approval (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.Level != RiskHigh {
		t.Fatalf("Level = %s, want high", result.Level)
	}
	if result.RuleID != "sql-require-approval-parser-fallback" {
		t.Fatalf("RuleID = %q, want sql-require-approval-parser-fallback", result.RuleID)
	}
	if !assessmentHasReason(result, "parser") {
		t.Fatalf("expected parser reason, got %v", result.Reasons)
	}
}

func assessmentHasReason(assessment RiskAssessment, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, reason := range assessment.Reasons {
		if strings.Contains(strings.ToLower(reason), needle) {
			return true
		}
	}
	return false
}

func TestEngine_MongoDB_Parity(t *testing.T) {
	e := NewEngine()
	tests := []struct {
		name       string
		statement  string
		wantAction Action
		wantLevel  RiskLevel
	}{
		// JSON format
		{"find json", `{"action":"find","collection":"users"}`, ActionAllow, RiskLow},
		{"aggregate json", `{"action":"aggregate","collection":"orders"}`, ActionAllow, RiskLow},
		{"drop json", `{"action":"drop","collection":"temp"}`, ActionBlock, RiskHigh},
		{"create user json", `{"action":"createUser"}`, ActionBlock, RiskHigh},
		{"grant roles json", `{"action":"grantRolesToUser"}`, ActionBlock, RiskHigh},
		{"server status json", `{"action":"serverStatus"}`, ActionWarn, RiskMedium},
		{"insertOne json", `{"action":"insertOne","collection":"users"}`, ActionWarn, RiskMedium},
		{"replaceOne json", `{"action":"replaceOne","collection":"users"}`, ActionWarn, RiskMedium},
		{"findOneAndUpdate json", `{"action":"findOneAndUpdate","collection":"users"}`, ActionWarn, RiskMedium},
		{"findOneAndReplace json", `{"action":"findOneAndReplace","collection":"users"}`, ActionWarn, RiskMedium},
		{"findOneAndDelete json", `{"action":"findOneAndDelete","collection":"users"}`, ActionWarn, RiskMedium},
		{"bulkWrite json", `{"action":"bulkWrite","collection":"users"}`, ActionWarn, RiskMedium},
		{"deleteMany json", `{"action":"deleteMany","collection":"logs"}`, ActionWarn, RiskMedium},
		{"aggregate lookup json", `{"action":"aggregate","collection":"orders","pipeline":[{"$lookup":{"from":"users","localField":"userId","foreignField":"_id","as":"user"}}]}`, ActionWarn, RiskMedium},
		{"aggregate merge json", `{"action":"aggregate","collection":"orders","pipeline":[{"$merge":"order_archive"}]}`, ActionRequireApproval, RiskHigh},

		// Shell format
		{"find shell", "db.users.find({})", ActionAllow, RiskLow},
		{"getCollection shell", `db.getCollection("users").find({})`, ActionAllow, RiskLow},
		{"getSiblingDB shell", `db.getSiblingDB("analytics").users.find({})`, ActionAllow, RiskLow},
		{"aggregate shell", "db.orders.aggregate([])", ActionAllow, RiskLow},
		{"drop shell", "db.temp.drop()", ActionBlock, RiskHigh},
		{"server status shell", `db.runCommand({ serverStatus: 1 })`, ActionWarn, RiskMedium},
		{"rename collection shell", `db.users.renameCollection("users_archive")`, ActionRequireApproval, RiskHigh},
		{"collmod shell", `db.runCommand({ collMod: "users", validator: { active: true } })`, ActionRequireApproval, RiskHigh},
		{"show", "show collections", ActionAllow, RiskLow},

		// Shell write operations
		{"delete shell", "db.users.deleteMany({})", ActionWarn, RiskMedium},
		{"update shell", "db.users.updateOne({id:1}, {$set:{name:'bob'}})", ActionWarn, RiskMedium},
		{"replaceOne shell", "db.users.replaceOne({id:1}, {name:'bob'})", ActionWarn, RiskMedium},
		{"findOneAndUpdate shell", "db.users.findOneAndUpdate({id:1}, {$set:{name:'bob'}})", ActionWarn, RiskMedium},
		{"findOneAndReplace shell", "db.users.findOneAndReplace({id:1}, {name:'bob'})", ActionWarn, RiskMedium},
		{"findOneAndDelete shell", "db.users.findOneAndDelete({id:1})", ActionWarn, RiskMedium},
		{"bulkWrite shell", "db.users.bulkWrite([{insertOne:{document:{name:'bob'}}}])", ActionWarn, RiskMedium},
		{"insert shell", "db.users.insertOne({name:'bob'})", ActionWarn, RiskMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Assess("mongodb", "ds1", tt.statement)
			if result.Action != tt.wantAction {
				t.Errorf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, tt.wantAction, result.RuleID, result.Reasons)
			}
			if result.Level != tt.wantLevel {
				t.Errorf("Level = %s, want %s", result.Level, tt.wantLevel)
			}
		})
	}
}

func TestEngine_Elasticsearch_Parity(t *testing.T) {
	e := NewEngine()
	tests := []struct {
		name       string
		statement  string
		wantAction Action
	}{
		{"get search", "GET /logs/_search\n{}", ActionAllow},
		{"get doc", "GET /users/_doc/1", ActionAllow},
		{"head index", "HEAD /my-index", ActionAllow},
		{"post search", "POST /orders/_search\n{}", ActionAllow},
		{"post search with query string", "POST /orders/_search?pretty=true\n{}", ActionAllow},
		{"post count", "POST /orders/_count\n{\"query\":{\"term\":{\"status\":\"open\"}}}", ActionAllow},
		{"delete by query", "POST /orders/_delete_by_query\n{}", ActionBlock},
		{"bulk write", "POST /_bulk\n{\"index\":{}}", ActionRequireApproval},
		{"forcemerge", "POST /orders/_forcemerge", ActionRequireApproval},
		{"settings write", "PUT /orders/_settings\n{\"index\":{\"refresh_interval\":\"30s\"}}", ActionRequireApproval},
		{"delete index", "DELETE /my-index", ActionBlock},
		{"put doc", "PUT /users/_doc/1\n{}", ActionWarn},
		{"post reindex", "POST /_reindex\n{}", ActionRequireApproval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Assess("elasticsearch", "ds1", tt.statement)
			if result.Action != tt.wantAction {
				t.Errorf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, tt.wantAction, result.RuleID, result.Reasons)
			}
		})
	}
}

func TestEngine_Redis_Parity(t *testing.T) {
	e := NewEngine()
	tests := []struct {
		name       string
		statement  string
		wantAction Action
	}{
		// High risk
		{"flushall", "FLUSHALL", ActionBlock},
		{"flushdb", "FLUSHDB", ActionBlock},
		{"shutdown", "SHUTDOWN", ActionBlock},
		{"memory purge", "MEMORY PURGE", ActionBlock},
		{"slowlog reset", "SLOWLOG RESET", ActionBlock},
		{"function flush", "FUNCTION FLUSH", ActionBlock},
		{"module load", "MODULE LOAD /tmp/rejson.so", ActionBlock},
		{"cluster setslot", "CLUSTER SETSLOT 1 NODE abc", ActionBlock},

		// Write operations
		{"set", "SET key value", ActionWarn},
		{"del", "DEL key", ActionWarn},
		{"hset", "HSET hash field value", ActionWarn},
		{"lpush", "LPUSH list value", ActionWarn},
		{"sadd", "SADD set member", ActionWarn},
		{"zadd", "ZADD zset 1 member", ActionWarn},
		{"expire", "EXPIRE key 100", ActionWarn},
		{"rename", "RENAME old new", ActionWarn},

		// Scan operations
		{"keys", "KEYS *", ActionWarn},
		{"scan", "SCAN 0 MATCH *", ActionWarn},
		{"hgetall", "HGETALL hash", ActionWarn},
		{"smembers", "SMEMBERS set", ActionWarn},

		// Read operations
		{"get", "GET key", ActionAllow},
		{"type", "TYPE key", ActionAllow},
		{"ttl", "TTL key", ActionAllow},
		{"exists", "EXISTS key", ActionAllow},
		{"hget", "HGET hash field", ActionAllow},
		{"scard", "SCARD set", ActionAllow},
		{"zcard", "ZCARD zset", ActionAllow},
		{"info", "INFO", ActionAllow},
		{"memory usage", "MEMORY USAGE key", ActionAllow},
		{"ping", "PING", ActionAllow},
		{"hkeys", "HKEYS hash", ActionWarn},
		{"object encoding", "OBJECT ENCODING key", ActionWarn},
		{"wait", "WAIT 1 1000", ActionWarn},
		{"select", "SELECT 1", ActionWarn},
		{"config rewrite", "CONFIG REWRITE", ActionWarn},
		{"client pause", "CLIENT PAUSE 1000", ActionWarn},
		{"cluster nodes", "CLUSTER NODES", ActionWarn},
		{"eval unknown", `EVAL "return ARGV[1]" 0 ok`, ActionWarn},
		{"eval write", `EVAL "return redis.call('DEL', KEYS[1])" 1 key`, ActionRequireApproval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Assess("redis", "ds1", tt.statement)
			if result.Action != tt.wantAction {
				t.Errorf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, tt.wantAction, result.RuleID, result.Reasons)
			}
		})
	}
}

func TestEngine_RedisUnknownLuaScriptStillWarnsWithEvalRule(t *testing.T) {
	e := NewEngine()
	result := e.Assess("redis", "ds1", `EVAL "return ARGV[1]" 0 ok`)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionWarn, result.RuleID, result.Reasons)
	}
	if result.RuleID != "redis-warn-eval" {
		t.Fatalf("RuleID = %q, want redis-warn-eval", result.RuleID)
	}
}

func TestEngine_RedisLuaWriteRequiresApprovalWithoutUserRule(t *testing.T) {
	e := NewEngine()
	result := e.Assess("redis", "ds1", `EVAL "return redis.call('DEL', KEYS[1])" 1 pd:1`)
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionRequireApproval, result.RuleID, result.Reasons)
	}
	if result.RuleID != "redis-require-approval-lua-write" {
		t.Fatalf("RuleID = %q, want redis-require-approval-lua-write", result.RuleID)
	}
}

func TestEngine_RedisLuaWriteRequiresApprovalFromArgv(t *testing.T) {
	e := NewEngine()
	result := e.AssessParsed(ParseRedisCommandArgs("ds1", []string{"EVAL", "return redis.call('DEL', KEYS[1])", "1", "pd:1"}))
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionRequireApproval, result.RuleID, result.Reasons)
	}
	if result.RuleID != "redis-require-approval-lua-write" {
		t.Fatalf("RuleID = %q, want redis-require-approval-lua-write", result.RuleID)
	}
}

func TestEngine_RedisReadOnlyLuaVariantDoesNotEscalateInnerWrite(t *testing.T) {
	e := NewEngine()
	result := e.Assess("redis", "ds1", `EVAL_RO "return redis.call('DEL', KEYS[1])" 1 pd:1`)
	if result.Action != ActionWarn {
		t.Fatalf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionWarn, result.RuleID, result.Reasons)
	}
	if result.RuleID != "redis-warn-eval" {
		t.Fatalf("RuleID = %q, want redis-warn-eval", result.RuleID)
	}
}

func TestEngine_CustomRedisRuleMatchesCommandAndKeyPattern(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-pd-delete",
			Description: "Protect pd keys from delete",
			Scope:       RuleScope{DsTypes: []string{"redis"}, KeyPattern: "pd:*"},
			Enabled:     true,
			Priority:    200,
			Action:      ActionBlock,
			Reason:      "pd keys require review",
			When:        RuleCondition{Command: []string{"del"}},
		},
	})

	match := e.Assess("redis", "ds1", "DEL pd:1")
	if match.Action != ActionBlock {
		t.Fatalf("DEL pd:1 action = %s, want %s (rule: %s, reasons: %v)", match.Action, ActionBlock, match.RuleID, match.Reasons)
	}
	if match.RuleID != "user-redis-pd-delete" {
		t.Fatalf("DEL pd:1 rule = %q, want user-redis-pd-delete", match.RuleID)
	}
	if match.Builtin {
		t.Fatalf("expected custom Redis rule match to be marked non-builtin")
	}

	quoted := e.Assess("redis", "ds1", `DEL "pd: secret"`)
	if quoted.Action != ActionBlock {
		t.Fatalf("DEL quoted pd key action = %s, want %s (rule: %s, reasons: %v)", quoted.Action, ActionBlock, quoted.RuleID, quoted.Reasons)
	}

	other := e.Assess("redis", "ds1", "DEL other:1")
	if other.RuleID == "user-redis-pd-delete" {
		t.Fatalf("DEL other:1 matched custom pd key rule unexpectedly")
	}
}

func TestEngine_CustomRedisRuleMatchesLuaScriptWriteIntentAndKeyPattern(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-pd-delete",
			Description: "Protect pd keys from delete",
			Scope:       RuleScope{DsTypes: []string{"redis"}, KeyPattern: "pd:*"},
			Enabled:     true,
			Priority:    200,
			Action:      ActionBlock,
			Reason:      "pd keys require review",
			When:        RuleCondition{Command: []string{"del"}},
		},
	})

	result := e.Assess("redis", "ds1", `EVAL "redis.call('DEL', KEYS[1])" 1 pd:1`)
	if result.Action != ActionBlock {
		t.Fatalf("Lua DEL action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionBlock, result.RuleID, result.Reasons)
	}
	if result.RuleID != "user-redis-pd-delete" {
		t.Fatalf("Lua DEL rule = %q, want user-redis-pd-delete", result.RuleID)
	}
	if result.Builtin {
		t.Fatalf("expected custom Redis Lua rule match to be marked non-builtin")
	}

	other := e.Assess("redis", "ds1", `EVAL "redis.call('DEL', KEYS[1])" 1 other:1`)
	if other.RuleID == "user-redis-pd-delete" {
		t.Fatalf("Lua DEL other:1 matched custom pd key rule unexpectedly")
	}
	if other.Action == ActionAllow {
		t.Fatalf("unknown Lua write must not auto-allow; got rule=%s reasons=%v", other.RuleID, other.Reasons)
	}
}

func TestEngine_ReadOnlyLuaVariantDoesNotMatchInnerWriteCommandRule(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-pd-delete",
			Description: "Protect pd keys from delete",
			Scope:       RuleScope{DsTypes: []string{"redis"}, KeyPattern: "pd:*"},
			Enabled:     true,
			Priority:    200,
			Action:      ActionBlock,
			Reason:      "pd keys require review",
			When:        RuleCondition{Command: []string{"del"}},
		},
	})

	result := e.Assess("redis", "ds1", `EVAL_RO "return redis.call('DEL', KEYS[1])" 1 pd:1`)
	if result.RuleID == "user-redis-pd-delete" {
		t.Fatalf("read-only Lua variant matched inner write command rule unexpectedly")
	}
	if result.Action != ActionWarn || result.RuleID != "redis-warn-eval" {
		t.Fatalf("Action/rule = %s/%s, want %s/redis-warn-eval (reasons: %v)", result.Action, result.RuleID, ActionWarn, result.Reasons)
	}
}

func TestEngine_CustomRedisWriteClassMatchesFlushAll(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-write-approval",
			Description: "Require approval for Redis writes",
			Scope:       RuleScope{DsTypes: []string{"redis"}},
			Enabled:     true,
			Priority:    200,
			Action:      ActionRequireApproval,
			Reason:      "Redis writes require approval",
			When:        RuleCondition{OperationClass: []string{"write"}},
		},
	})

	result := e.Assess("redis", "ds1", "FLUSHALL")
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionRequireApproval, result.RuleID, result.Reasons)
	}
	if result.RuleID != "user-redis-write-approval" {
		t.Fatalf("RuleID = %q, want user-redis-write-approval", result.RuleID)
	}
}

func TestEngine_ReadOnlyLuaVariantDoesNotMatchInnerFlushAllCommandRule(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-flushall",
			Description: "Block direct FLUSHALL",
			Scope:       RuleScope{DsTypes: []string{"redis"}},
			Enabled:     true,
			Priority:    200,
			Action:      ActionBlock,
			Reason:      "FLUSHALL blocked",
			When:        RuleCondition{Command: []string{"flushall"}},
		},
	})

	result := e.Assess("redis", "ds1", `EVAL_RO "return redis.call('FLUSHALL')" 0`)
	if result.RuleID == "user-redis-flushall" {
		t.Fatalf("read-only Lua variant matched inner FLUSHALL command rule unexpectedly")
	}
	if result.Action != ActionWarn || result.RuleID != "redis-warn-eval" {
		t.Fatalf("Action/rule = %s/%s, want %s/redis-warn-eval (reasons: %v)", result.Action, result.RuleID, ActionWarn, result.Reasons)
	}
}

func TestEngine_CustomOperationClassMatchesSQLWrite(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-sql-write-approval",
			Description: "Require approval for SQL writes",
			Scope:       RuleScope{DsTypes: []string{"mysql"}},
			Enabled:     true,
			Priority:    200,
			Action:      ActionRequireApproval,
			Reason:      "SQL writes require approval",
			When:        RuleCondition{OperationClass: []string{"write"}},
		},
	})

	write := e.Assess("mysql", "ds1", "UPDATE users SET name = 'bob' WHERE id = 1")
	if write.Action != ActionRequireApproval {
		t.Fatalf("SQL write action = %s, want %s (rule: %s, reasons: %v)", write.Action, ActionRequireApproval, write.RuleID, write.Reasons)
	}
	if write.RuleID != "user-sql-write-approval" {
		t.Fatalf("SQL write rule = %q, want user-sql-write-approval", write.RuleID)
	}

	read := e.Assess("mysql", "ds1", "SELECT * FROM users WHERE id = 1")
	if read.RuleID == "user-sql-write-approval" {
		t.Fatalf("SQL read matched write operationClass rule unexpectedly")
	}
}

func TestEngine_CustomRedisRuleMatchesLuaScriptOperationClass(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:          "user-redis-prod-write",
			Description: "Protect prod keys from Redis writes",
			Scope:       RuleScope{DsTypes: []string{"redis"}, KeyPattern: "prod:*"},
			Enabled:     true,
			Priority:    200,
			Action:      ActionRequireApproval,
			Reason:      "prod writes require approval",
			When:        RuleCondition{OperationClass: []string{"write"}},
		},
	})

	result := e.Assess("redis", "ds1", `EVAL "return redis.call('SET', KEYS[1], ARGV[1])" 1 prod:session on`)
	if result.Action != ActionRequireApproval {
		t.Fatalf("Lua SET action = %s, want %s (rule: %s, reasons: %v)", result.Action, ActionRequireApproval, result.RuleID, result.Reasons)
	}
	if result.RuleID != "user-redis-prod-write" {
		t.Fatalf("Lua SET rule = %q, want user-redis-prod-write", result.RuleID)
	}
}

func TestEngine_DynamoDB_Parity(t *testing.T) {
	e := NewEngine()
	tests := []struct {
		name       string
		statement  string
		wantAction Action
	}{
		{"select", "SELECT * FROM Orders WHERE pk = 'abc'", ActionAllow},
		{"select no where", "SELECT * FROM Orders", ActionAllow},
		{"delete no where", "DELETE FROM Orders", ActionBlock},
		{"delete where", "DELETE FROM Orders WHERE pk = 'abc'", ActionWarn},
		{"update no where", "UPDATE Orders SET status = 'done'", ActionBlock},
		{"update where", "UPDATE Orders SET status = 'done' WHERE pk = 'abc'", ActionWarn},
		{"insert", "INSERT INTO Orders VALUE {'pk': '1'}", ActionWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Assess("dynamodb", "ds1", tt.statement)
			if result.Action != tt.wantAction {
				t.Errorf("Action = %s, want %s (rule: %s, reasons: %v)", result.Action, tt.wantAction, result.RuleID, result.Reasons)
			}
		})
	}
}

func TestEngine_ReloadFromStorePublishesConsistentSnapshots(t *testing.T) {
	dir := t.TempDir()
	oldStore := NewStore(filepath.Join(dir, "old"))
	if err := oldStore.Load(); err != nil {
		t.Fatalf("old store load failed: %v", err)
	}

	newStore := NewStore(filepath.Join(dir, "new"))
	if err := newStore.Load(); err != nil {
		t.Fatalf("new store load failed: %v", err)
	}
	if err := newStore.Create(Rule{
		ID:          "user-allow-select",
		Description: "allow select",
		Enabled:     true,
		Action:      ActionAllow,
		Scope:       RuleScope{DsTypes: []string{"mysql"}},
		When:        RuleCondition{Command: []string{"select"}},
	}); err != nil {
		t.Fatalf("new store create failed: %v", err)
	}
	if err := newStore.SetBuiltinEnabled("sql-allow-insert", true); err != nil {
		t.Fatalf("new store builtin enable failed: %v", err)
	}

	engine := NewEngine()
	engine.ReloadFromStore(oldStore)

	stop := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		defer close(done)
		for i := 0; i < 20000; i++ {
			select {
			case <-stop:
				return
			default:
			}
			rules := engine.ListAllRules()
			builtinEnabled := false
			userPresent := false
			for _, rule := range rules {
				if rule.ID == "sql-allow-insert" {
					builtinEnabled = rule.Enabled
				}
				if rule.ID == "user-allow-select" {
					userPresent = true
				}
			}
			if builtinEnabled != userPresent {
				done <- &snapshotMismatchError{builtinEnabled: builtinEnabled, userPresent: userPresent}
				return
			}
			runtime.Gosched()
		}
	}()

	for i := 0; i < 20000; i++ {
		engine.ReloadFromStore(newStore)
		engine.ReloadFromStore(oldStore)
		runtime.Gosched()
	}
	close(stop)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type snapshotMismatchError struct {
	builtinEnabled bool
	userPresent    bool
}

func (e *snapshotMismatchError) Error() string {
	return "observed mixed rule snapshot during reload"
}

func TestEngine_ScopePriority(t *testing.T) {
	e := &Engine{}
	e.builtinRules = []Rule{
		{
			ID:       "global-block-drop",
			Scope:    RuleScope{DsTypes: []string{"mysql"}},
			Enabled:  true,
			Priority: 100,
			Action:   ActionBlock,
			Reason:   "global block drop",
			When:     RuleCondition{Command: []string{"drop"}},
		},
	}
	e.userRules = []Rule{
		{
			ID:       "allow-tmp-drop",
			Scope:    RuleScope{DsTypes: []string{"mysql"}, EntityPattern: "tmp_*"},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			Reason:   "tmp tables can be dropped",
			When:     RuleCondition{Command: []string{"drop"}},
		},
	}

	// tmp_data should be allowed (entity pattern rule overrides global)
	result := e.Assess("mysql", "ds1", "DROP TABLE tmp_data")
	if result.Action != ActionAllow {
		t.Errorf("tmp_data DROP: Action = %s, want allow (rule: %s)", result.Action, result.RuleID)
	}

	// users should be blocked (global rule still applies)
	result = e.Assess("mysql", "ds1", "DROP TABLE users")
	if result.Action != ActionBlock {
		t.Errorf("users DROP: Action = %s, want block (rule: %s)", result.Action, result.RuleID)
	}
}

func TestEngine_UserRuleOverridesBuiltin(t *testing.T) {
	e := NewEngine()
	e.userRules = []Rule{
		{
			ID:       "allow-all-on-staging",
			Scope:    RuleScope{DatasourceID: "ds-staging"},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			Reason:   "staging datasource: all operations allowed",
			When:     RuleCondition{},
		},
	}

	// DROP on staging should be allowed due to user rule
	result := e.Assess("mysql", "ds-staging", "DROP TABLE users")
	if result.Action != ActionAllow {
		t.Errorf("staging DROP: Action = %s, want allow (rule: %s)", result.Action, result.RuleID)
	}

	// DROP on prod should still be blocked by builtin rule
	result = e.Assess("mysql", "ds-prod", "DROP TABLE users")
	if result.Action != ActionBlock {
		t.Errorf("prod DROP: Action = %s, want block (rule: %s)", result.Action, result.RuleID)
	}
}

func TestEngine_MultiCTEWriteTargetsMatchLaterEntities(t *testing.T) {
	e := NewEngine()
	e.userRules = []Rule{
		{
			ID:       "block-users-delete",
			Scope:    RuleScope{DsTypes: []string{"postgresql"}, Entity: "users"},
			Enabled:  true,
			Priority: 300,
			Action:   ActionBlock,
			Reason:   "users writes are blocked",
			When:     RuleCondition{Command: []string{"delete"}},
		},
	}

	result := e.Assess(
		"postgresql",
		"ds1",
		"WITH audit AS (INSERT INTO audit_logs(id) VALUES (1) RETURNING id), cleanup AS (DELETE FROM users WHERE id = 1 RETURNING id) SELECT * FROM reports",
	)
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "block-users-delete" {
		t.Fatalf("RuleID = %q, want block-users-delete", result.RuleID)
	}
}

func TestEngine_TopLevelDeleteWithoutWhereStaysBlockedWhenCTEDeleteHasWhere(t *testing.T) {
	e := NewEngine()
	result := e.Assess(
		"postgresql",
		"ds1",
		"WITH cleanup AS (DELETE FROM archive WHERE id = 1 RETURNING id) DELETE FROM users",
	)
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
}

func TestEngine_TopLevelUpdateWithoutWhereStaysUpdateWhenDeleteCTEExists(t *testing.T) {
	e := NewEngine()
	result := e.Assess(
		"postgresql",
		"ds1",
		"WITH cleanup AS (DELETE FROM archive WHERE id = 1 RETURNING id) UPDATE users SET active = false",
	)
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block (rule: %s, reasons: %v)", result.Action, result.RuleID, result.Reasons)
	}
	if result.RuleID != "sql-block-update-no-where" {
		t.Fatalf("RuleID = %q, want sql-block-update-no-where", result.RuleID)
	}
}

func TestEngine_ProbePolicyForParsed_UsesUserThresholdOverride(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:       "orders-tight-read-threshold",
			Scope:    RuleScope{DsTypes: []string{"mysql"}, Entity: "orders"},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			When:     RuleCondition{Command: []string{"select"}},
			Thresholds: RuleThresholds{
				MaxExaminedRows:      int64Ptr(200),
				MaxJoinCount:         intPtr(2),
				MaxEstimatedJoinRows: int64Ptr(500),
			},
		},
	})

	ps := ParseStatement("mysql", "ds1", "SELECT * FROM orders WHERE id = 1")
	policy := e.ProbePolicyForParsed(ps)

	if policy.MaxExaminedRows != 200 {
		t.Fatalf("MaxExaminedRows = %d, want 200", policy.MaxExaminedRows)
	}
	if policy.MaxJoinCount != 2 {
		t.Fatalf("MaxJoinCount = %d, want 2", policy.MaxJoinCount)
	}
	if policy.MaxEstimatedJoinRows != 500 {
		t.Fatalf("MaxEstimatedJoinRows = %d, want 500", policy.MaxEstimatedJoinRows)
	}
}

func TestEngine_ProbePolicyForParsed_UsesProbeCatalogThresholdOverride(t *testing.T) {
	e := NewEngine()
	e.LoadProbeRules([]Rule{
		probeCatalogRule(probeRuleNoIndex, []string{"mysql"}, RuleThresholds{
			SeqScanRowsThreshold: int64Ptr(2500),
			CostThreshold:        float64Ptr(250),
			AllowSafeSeqScan:     boolPtr(false),
		}),
		probeCatalogRule(probeRuleWideScan, []string{"mysql"}, RuleThresholds{
			MaxExaminedRows: int64Ptr(150),
		}),
		probeCatalogRule(probeRulePlanRisk, []string{"mysql"}, RuleThresholds{
			MaxJoinCount:         intPtr(2),
			MaxFullScans:         intPtr(1),
			MaxEstimatedJoinRows: int64Ptr(300),
		}),
	})

	ps := ParseStatement("mysql", "ds1", "SELECT * FROM orders WHERE id = 1")
	policy := e.ProbePolicyForParsed(ps)

	if policy.SeqScanRowsThreshold != 2500 {
		t.Fatalf("SeqScanRowsThreshold = %d, want 2500", policy.SeqScanRowsThreshold)
	}
	if policy.CostThreshold != 250 {
		t.Fatalf("CostThreshold = %v, want 250", policy.CostThreshold)
	}
	if policy.AllowSafeSeqScan {
		t.Fatal("AllowSafeSeqScan = true, want false")
	}
	if policy.MaxExaminedRows != 150 {
		t.Fatalf("MaxExaminedRows = %d, want 150", policy.MaxExaminedRows)
	}
	if policy.MaxJoinCount != 2 {
		t.Fatalf("MaxJoinCount = %d, want 2", policy.MaxJoinCount)
	}
	if policy.MaxEstimatedJoinRows != 300 {
		t.Fatalf("MaxEstimatedJoinRows = %d, want 300", policy.MaxEstimatedJoinRows)
	}
}

func TestEngine_ProbePolicyForParsed_UsesDynamoDBProbeCatalogThresholdOverride(t *testing.T) {
	e := NewEngine()
	e.LoadProbeRules([]Rule{
		probeCatalogRule(probeRuleAccessPath, []string{"dynamodb"}, RuleThresholds{
			MaxDynamoDBPages:          intPtr(3),
			MaxDynamoDBEvaluatedItems: intPtr(300),
		}),
	})

	ps := ParseStatement("dynamodb", "ds1", `SELECT * FROM "orders"`)
	policy := e.ProbePolicyForParsed(ps)

	if policy.MaxDynamoDBPages != 3 {
		t.Fatalf("MaxDynamoDBPages = %d, want 3", policy.MaxDynamoDBPages)
	}
	if policy.MaxDynamoDBEvaluatedItems != 300 {
		t.Fatalf("MaxDynamoDBEvaluatedItems = %d, want 300", policy.MaxDynamoDBEvaluatedItems)
	}
}

func TestEngine_ElasticsearchBodyPatternRule(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:       "block-es-match-all",
			Scope:    RuleScope{DsTypes: []string{"elasticsearch"}},
			Enabled:  true,
			Priority: 200,
			Action:   ActionBlock,
			Reason:   "match_all search is blocked on prod",
			When: RuleCondition{
				HTTPMethod:  []string{"POST"},
				PathPattern: `(?i)/_search$`,
				BodyPattern: `(?i)"match_all"\s*:`,
			},
		},
	})

	result := e.Assess("elasticsearch", "ds1", "POST /orders/_search\n{\"query\":{\"match_all\":{}}}")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block", result.Action)
	}
}

func TestEngine_DeterministicTieBreakForEqualPriorityRules(t *testing.T) {
	e := NewEngine()
	e.LoadUserRules([]Rule{
		{
			ID:       "zzz-allow-orders-select",
			Scope:    RuleScope{DsTypes: []string{"mysql"}, Entity: "orders"},
			Enabled:  true,
			Priority: 200,
			Action:   ActionAllow,
			When:     RuleCondition{Command: []string{"select"}},
		},
		{
			ID:       "aaa-block-orders-select",
			Scope:    RuleScope{DsTypes: []string{"mysql"}, Entity: "orders"},
			Enabled:  true,
			Priority: 200,
			Action:   ActionBlock,
			When:     RuleCondition{Command: []string{"select"}},
		},
	})

	result := e.Assess("mysql", "ds1", "SELECT * FROM orders")
	if result.Action != ActionBlock {
		t.Fatalf("Action = %s, want block", result.Action)
	}
	if result.RuleID != "aaa-block-orders-select" {
		t.Fatalf("RuleID = %s, want aaa-block-orders-select", result.RuleID)
	}
}

func TestEngine_EmptyStatement(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "")
	if result.Action != ActionAllow {
		t.Errorf("empty statement: Action = %s, want allow", result.Action)
	}
}

func TestEngine_UnknownDsType(t *testing.T) {
	e := NewEngine()
	result := e.Assess("unknown", "ds1", "SOME COMMAND")
	if result.Action != ActionWarn {
		t.Errorf("unknown type: Action = %s, want warn", result.Action)
	}
}

func TestEngine_UnmatchedKnownStatementDefaultsToWarn(t *testing.T) {
	e := NewEngine()
	result := e.Assess("mysql", "ds1", "VACUUM users")
	if result.Action != ActionRequireApproval {
		t.Fatalf("Action = %s, want require_approval", result.Action)
	}
	if result.Level != RiskHigh {
		t.Fatalf("Level = %s, want high", result.Level)
	}
	if result.RuleID != "sql-require-approval-unsupported" {
		t.Fatalf("RuleID = %q, want sql-require-approval-unsupported", result.RuleID)
	}
}

func TestEngine_DisabledRule(t *testing.T) {
	e := &Engine{}
	e.builtinRules = []Rule{
		{
			ID:       "disabled-rule",
			Scope:    RuleScope{DsTypes: []string{"mysql"}},
			Enabled:  false,
			Priority: 100,
			Action:   ActionBlock,
			When:     RuleCondition{Command: []string{"select"}},
		},
	}
	result := e.Assess("mysql", "ds1", "SELECT 1")
	if result.Action != ActionAllow {
		t.Errorf("disabled rule should not match: Action = %s", result.Action)
	}
}
