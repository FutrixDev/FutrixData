package riskengine

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type Action string

const (
	ActionAllow           Action = "allow"
	ActionWarn            Action = "warn"
	ActionRequireApproval Action = "require_approval"
	ActionBlock           Action = "block"
)

type RiskAssessment struct {
	Level           RiskLevel `json:"level"`
	Action          Action    `json:"action"`
	Reasons         []string  `json:"reasons"`
	RuleID          string    `json:"ruleId,omitempty"`
	RuleCode        string    `json:"ruleCode,omitempty"`
	RuleDescription string    `json:"ruleDescription,omitempty"`
	Builtin         bool      `json:"builtin,omitempty"`
}

type ParsedStatement struct {
	Raw               string   `json:"raw"`
	DsType            string   `json:"dsType"`
	DatasourceID      string   `json:"datasourceId,omitempty"`
	FirstKeyword      string   `json:"firstKeyword"`
	TargetEntity      string   `json:"targetEntity,omitempty"`
	TargetEntities    []string `json:"targetEntities,omitempty"`
	HasWhere          bool     `json:"hasWhere,omitempty"`
	SQLStatementCount int      `json:"sqlStatementCount,omitempty"`
	SQLParseFailed    bool     `json:"sqlParseFailed,omitempty"`
	IsQuery           bool     `json:"isQuery,omitempty"`
	HTTPMethod        string   `json:"httpMethod,omitempty"`
	URLPath           string   `json:"urlPath,omitempty"`
	Body              string   `json:"body,omitempty"`
	RedisCommand      string   `json:"redisCommand,omitempty"`
	KeyPattern        string   `json:"keyPattern,omitempty"`
}

func (ps ParsedStatement) ScopeEntities() []string {
	if len(ps.TargetEntities) > 0 {
		return ps.TargetEntities
	}
	if ps.TargetEntity == "" {
		return nil
	}
	return []string{ps.TargetEntity}
}

type Rule struct {
	ID          string        `json:"id"`
	Code        string        `json:"code,omitempty"`
	Description string        `json:"description"`
	Scope       RuleScope     `json:"scope"`
	Enabled     bool          `json:"enabled"`
	Priority    int           `json:"priority"`
	Action      Action        `json:"action"`
	Reason      string        `json:"reason"`
	When        RuleCondition `json:"when"`
	Builtin     bool          `json:"builtin"`
}

type RuleScope struct {
	DsTypes       []string `json:"dsTypes,omitempty"`
	DatasourceID  string   `json:"datasourceId,omitempty"`
	Entity        string   `json:"entity,omitempty"`
	EntityPattern string   `json:"entityPattern,omitempty"`
	KeyPattern    string   `json:"keyPattern,omitempty"`
}

type RuleCondition struct {
	Command             []string        `json:"command,omitempty"`
	StatementPattern    string          `json:"statementPattern,omitempty"`
	StatementNotPattern string          `json:"statementNotPattern,omitempty"`
	HasWhere            *bool           `json:"hasWhere,omitempty"`
	SQLMultiStatement   *bool           `json:"sqlMultiStatement,omitempty"`
	SQLParseFailed      *bool           `json:"sqlParseFailed,omitempty"`
	HTTPMethod          []string        `json:"httpMethod,omitempty"`
	PathPattern         string          `json:"pathPattern,omitempty"`
	BodyPattern         string          `json:"bodyPattern,omitempty"`
	BodyNotPattern      string          `json:"bodyNotPattern,omitempty"`
	Any                 []RuleCondition `json:"any,omitempty"`
	Not                 *RuleCondition  `json:"not,omitempty"`
}

type Engine struct {
	mu           sync.RWMutex
	builtinRules []Rule
	userRules    []Rule
}

type ruleMatch struct {
	rule          Rule
	scopePriority int
}

func NewEngine() *Engine {
	return &Engine{builtinRules: BuiltinRules()}
}

func (e *Engine) LoadBuiltinRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range rules {
		rules[i].Builtin = true
	}
	e.builtinRules = rules
}

func (e *Engine) LoadUserRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userRules = append([]Rule(nil), rules...)
}

func (e *Engine) ListRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Rule, 0, len(e.userRules)+len(e.builtinRules))
	out = append(out, e.userRules...)
	out = append(out, e.builtinRules...)
	return out
}

func (e *Engine) Assess(dsType, datasourceID, statement string) RiskAssessment {
	return e.AssessParsed(ParseStatement(dsType, datasourceID, statement))
}

func (e *Engine) AssessParsed(ps ParsedStatement) RiskAssessment {
	e.mu.RLock()
	defer e.mu.RUnlock()
	matches := e.matchingRulesLocked(ps)
	if len(matches) == 0 {
		if strings.TrimSpace(ps.Raw) == "" || ps.IsQuery {
			return RiskAssessment{Level: RiskLow, Action: ActionAllow}
		}
		if ps.DsType == "mysql" || ps.DsType == "postgresql" || ps.DsType == "d1" {
			return RiskAssessment{
				Level:           RiskHigh,
				Action:          ActionRequireApproval,
				Reasons:         []string{"unsupported SQL syntax requires review"},
				RuleID:          "sql-require-approval-unsupported",
				RuleCode:        "SQL-014",
				RuleDescription: "Require approval for unsupported SQL syntax",
				Builtin:         true,
			}
		}
		return RiskAssessment{Level: RiskMedium, Action: ActionWarn, Reasons: []string{"statement requires review: no matching risk rule"}}
	}
	winner := matches[0].rule
	return RiskAssessment{
		Level:           actionToRiskLevel(winner.Action),
		Action:          winner.Action,
		Reasons:         buildReasons(winner),
		RuleID:          winner.ID,
		RuleCode:        winner.Code,
		RuleDescription: winner.Description,
		Builtin:         winner.Builtin,
	}
}

func (e *Engine) matchingRulesLocked(ps ParsedStatement) []ruleMatch {
	allRules := append(append([]Rule(nil), e.userRules...), e.builtinRules...)
	var matches []ruleMatch
	for _, rule := range allRules {
		if !rule.Enabled {
			continue
		}
		if !scopeMatches(rule.Scope, ps) {
			continue
		}
		if !conditionMatches(rule.When, ps) {
			continue
		}
		matches = append(matches, ruleMatch{rule: rule, scopePriority: scopePriority(rule.Scope)})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].scopePriority != matches[j].scopePriority {
			return matches[i].scopePriority > matches[j].scopePriority
		}
		if matches[i].rule.Priority != matches[j].rule.Priority {
			return matches[i].rule.Priority > matches[j].rule.Priority
		}
		if matches[i].rule.Builtin != matches[j].rule.Builtin {
			return !matches[i].rule.Builtin
		}
		return matches[i].rule.ID < matches[j].rule.ID
	})
	return matches
}

func ParseStatement(dsType, datasourceID, statement string) ParsedStatement {
	typ := strings.ToLower(strings.TrimSpace(dsType))
	stmt := strings.TrimSpace(statement)
	ps := ParsedStatement{Raw: stmt, DsType: typ, DatasourceID: datasourceID}
	if stmt == "" {
		return ps
	}
	switch typ {
	case "mysql", "postgresql", "d1":
		parseSQL(&ps)
	case "mongodb":
		parseMongo(&ps)
	case "elasticsearch":
		parseElasticsearch(&ps)
	case "redis", "redis_cluster":
		parseRedis(&ps)
	case "dynamodb":
		parseDynamoDB(&ps)
	default:
		ps.FirstKeyword = firstKeyword(stmt)
	}
	return ps
}

func BuiltinRules() []Rule {
	var out []Rule
	out = append(out, sqlRules()...)
	out = append(out, redisRules()...)
	out = append(out, mongoRules()...)
	out = append(out, elasticsearchRules()...)
	out = append(out, dynamoRules()...)
	for i := range out {
		out[i].Builtin = true
		if !out[i].Enabled {
			out[i].Enabled = true
		}
	}
	return out
}

func parseSQL(ps *ParsedStatement) {
	ps.SQLStatementCount = statementCount(ps.Raw)
	ps.FirstKeyword = firstKeyword(ps.Raw)
	ps.HasWhere = hasWhere(ps.Raw)
	ps.TargetEntity = targetEntity(ps.Raw)
	switch ps.FirstKeyword {
	case "select", "show", "describe", "explain":
		ps.IsQuery = true
	case "":
		ps.SQLParseFailed = true
	}
}

func parseRedis(ps *ParsedStatement) {
	fields := strings.Fields(ps.Raw)
	if len(fields) == 0 {
		return
	}
	ps.RedisCommand = strings.ToUpper(fields[0])
	ps.FirstKeyword = strings.ToLower(fields[0])
	if len(fields) > 1 {
		ps.KeyPattern = fields[1]
	}
	switch ps.RedisCommand {
	case "GET", "MGET", "EXISTS", "TTL", "PTTL", "TYPE", "SCAN", "HGET", "HGETALL", "HMGET", "LLEN", "LRANGE", "SCARD", "SMEMBERS", "ZRANGE":
		ps.IsQuery = true
	}
}

func parseMongo(ps *ParsedStatement) {
	lower := strings.ToLower(ps.Raw)
	if strings.HasPrefix(lower, "db.") {
		parts := strings.SplitN(ps.Raw[3:], ".", 2)
		if len(parts) == 2 {
			ps.TargetEntity = strings.TrimSpace(parts[0])
			if idx := strings.Index(parts[1], "("); idx >= 0 {
				ps.FirstKeyword = strings.ToLower(strings.TrimSpace(parts[1][:idx]))
			}
		}
	}
	switch ps.FirstKeyword {
	case "find", "aggregate", "count", "distinct":
		ps.IsQuery = true
	}
	if ps.FirstKeyword == "" {
		ps.FirstKeyword = firstKeyword(ps.Raw)
	}
}

func parseElasticsearch(ps *ParsedStatement) {
	lines := strings.SplitN(ps.Raw, "\n", 2)
	head := strings.Fields(lines[0])
	if len(head) < 2 {
		return
	}
	ps.HTTPMethod = strings.ToUpper(head[0])
	ps.FirstKeyword = strings.ToLower(ps.HTTPMethod)
	ps.URLPath = head[1]
	if len(lines) == 2 {
		ps.Body = lines[1]
	}
	parts := strings.Split(strings.Trim(ps.URLPath, "/"), "/")
	if len(parts) > 0 {
		ps.TargetEntity = parts[0]
	}
	if ps.HTTPMethod == "GET" || ps.HTTPMethod == "HEAD" || (ps.HTTPMethod == "POST" && strings.Contains(ps.URLPath, "_search")) {
		ps.IsQuery = true
	}
}

func parseDynamoDB(ps *ParsedStatement) {
	ps.FirstKeyword = firstKeyword(ps.Raw)
	ps.TargetEntity = targetEntity(ps.Raw)
	ps.HasWhere = hasWhere(ps.Raw)
	ps.IsQuery = ps.FirstKeyword == "select"
}

func sqlRules() []Rule {
	types := []string{"mysql", "postgresql", "d1"}
	return []Rule{
		{ID: "sql-block-multi-statement", Code: "SQL-011", Description: "Block SQL batches containing more than one statement", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 130, Action: ActionBlock, Reason: "multiple SQL statements are not allowed on agent execution paths", When: RuleCondition{SQLMultiStatement: boolPtr(true)}},
		{ID: "sql-require-approval-procedure-call", Code: "SQL-012", Description: "Require approval for stored procedure and procedural SQL calls", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 115, Action: ActionRequireApproval, Reason: "stored procedure calls require review", When: RuleCondition{Command: []string{"call", "exec", "execute", "do"}}},
		{ID: "sql-allow-read", Code: "SQL-001", Description: "Allow SELECT, SHOW, DESCRIBE, EXPLAIN", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 10, Action: ActionAllow, Reason: "read-only operation", When: RuleCondition{Command: []string{"select", "show", "describe", "explain"}}},
		{ID: "sql-block-drop-truncate", Code: "SQL-003", Description: "Block DROP and TRUNCATE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 100, Action: ActionBlock, Reason: "destructive DDL", When: RuleCondition{Command: []string{"drop", "truncate"}}},
		{ID: "sql-block-permission-change", Code: "SQL-004", Description: "Block GRANT and REVOKE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 100, Action: ActionBlock, Reason: "permission change", When: RuleCondition{Command: []string{"grant", "revoke"}}},
		{ID: "sql-block-delete-no-where", Code: "SQL-005", Description: "Block DELETE without WHERE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 90, Action: ActionBlock, Reason: "DELETE without WHERE", When: RuleCondition{Command: []string{"delete"}, HasWhere: boolPtr(false)}},
		{ID: "sql-block-update-no-where", Code: "SQL-006", Description: "Block UPDATE without WHERE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 90, Action: ActionBlock, Reason: "UPDATE without WHERE", When: RuleCondition{Command: []string{"update"}, HasWhere: boolPtr(false)}},
		{ID: "sql-warn-delete", Code: "SQL-007", Description: "Warn on DELETE with WHERE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 50, Action: ActionWarn, Reason: "DELETE", When: RuleCondition{Command: []string{"delete"}, HasWhere: boolPtr(true)}},
		{ID: "sql-warn-update", Code: "SQL-008", Description: "Warn on UPDATE with WHERE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 50, Action: ActionWarn, Reason: "UPDATE", When: RuleCondition{Command: []string{"update"}, HasWhere: boolPtr(true)}},
		{ID: "sql-warn-insert", Code: "SQL-009", Description: "Warn on INSERT/REPLACE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 40, Action: ActionWarn, Reason: "INSERT/REPLACE", When: RuleCondition{Command: []string{"insert", "replace"}}},
		{ID: "sql-warn-ddl", Code: "SQL-010", Description: "Warn on ALTER/CREATE", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 40, Action: ActionWarn, Reason: "DDL", When: RuleCondition{Command: []string{"alter", "create"}}},
	}
}

func redisRules() []Rule {
	types := []string{"redis", "redis_cluster"}
	return []Rule{
		{ID: "redis-allow-read", Code: "REDIS-001", Description: "Allow common Redis read commands", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 10, Action: ActionAllow, Reason: "read-only Redis command", When: RuleCondition{Command: []string{"get", "mget", "exists", "ttl", "pttl", "type", "scan", "hget", "hgetall", "hmget", "llen", "lrange", "scard", "smembers", "zrange"}}},
		{ID: "redis-block-dangerous", Code: "REDIS-002", Description: "Block destructive Redis commands", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 100, Action: ActionBlock, Reason: "destructive Redis command", When: RuleCondition{Command: []string{"flushall", "flushdb", "shutdown", "config", "script", "eval", "evalsha"}}},
		{ID: "redis-warn-write", Code: "REDIS-003", Description: "Warn on Redis write commands", Scope: RuleScope{DsTypes: types}, Enabled: true, Priority: 20, Action: ActionWarn, Reason: "Redis write command", When: RuleCondition{}},
	}
}

func mongoRules() []Rule {
	return []Rule{
		{ID: "mongo-allow-read", Code: "MONGO-001", Description: "Allow MongoDB read actions", Scope: RuleScope{DsTypes: []string{"mongodb"}}, Enabled: true, Priority: 10, Action: ActionAllow, Reason: "read-only MongoDB action", When: RuleCondition{Command: []string{"find", "aggregate", "count", "distinct"}}},
		{ID: "mongo-block-drop", Code: "MONGO-002", Description: "Block MongoDB drop operations", Scope: RuleScope{DsTypes: []string{"mongodb"}}, Enabled: true, Priority: 100, Action: ActionBlock, Reason: "destructive MongoDB action", When: RuleCondition{Command: []string{"drop", "dropdatabase", "dropcollection"}}},
		{ID: "mongo-warn-write", Code: "MONGO-003", Description: "Warn on MongoDB write actions", Scope: RuleScope{DsTypes: []string{"mongodb"}}, Enabled: true, Priority: 20, Action: ActionWarn, Reason: "MongoDB write action", When: RuleCondition{}},
	}
}

func elasticsearchRules() []Rule {
	return []Rule{
		{ID: "es-allow-read", Code: "ES-001", Description: "Allow Elasticsearch read and search requests", Scope: RuleScope{DsTypes: []string{"elasticsearch"}}, Enabled: true, Priority: 10, Action: ActionAllow, Reason: "read-only Elasticsearch request", When: RuleCondition{Any: []RuleCondition{{HTTPMethod: []string{"GET", "HEAD"}}, {HTTPMethod: []string{"POST"}, PathPattern: `_search`}}}},
		{ID: "es-block-delete-index", Code: "ES-002", Description: "Block Elasticsearch DELETE index requests", Scope: RuleScope{DsTypes: []string{"elasticsearch"}}, Enabled: true, Priority: 100, Action: ActionBlock, Reason: "destructive Elasticsearch request", When: RuleCondition{HTTPMethod: []string{"DELETE"}}},
		{ID: "es-warn-write", Code: "ES-003", Description: "Warn on Elasticsearch write requests", Scope: RuleScope{DsTypes: []string{"elasticsearch"}}, Enabled: true, Priority: 20, Action: ActionWarn, Reason: "Elasticsearch write request", When: RuleCondition{}},
	}
}

func dynamoRules() []Rule {
	return []Rule{
		{ID: "dynamodb-allow-read", Code: "DDB-001", Description: "Allow DynamoDB PartiQL SELECT", Scope: RuleScope{DsTypes: []string{"dynamodb"}}, Enabled: true, Priority: 10, Action: ActionAllow, Reason: "read-only DynamoDB statement", When: RuleCondition{Command: []string{"select"}}},
		{ID: "dynamodb-warn-write", Code: "DDB-002", Description: "Warn on DynamoDB write statements", Scope: RuleScope{DsTypes: []string{"dynamodb"}}, Enabled: true, Priority: 20, Action: ActionWarn, Reason: "DynamoDB write statement", When: RuleCondition{}},
	}
}

func conditionMatches(cond RuleCondition, ps ParsedStatement) bool {
	if isEmptyCondition(cond) {
		return true
	}
	if len(cond.Any) > 0 {
		ok := false
		for _, sub := range cond.Any {
			if conditionMatches(sub, ps) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
		cond.Any = nil
	}
	if cond.Not != nil {
		if conditionMatches(*cond.Not, ps) {
			return false
		}
		cond.Not = nil
	}
	if len(cond.Command) > 0 && !matchCommand(cond.Command, ps) {
		return false
	}
	if len(cond.HTTPMethod) > 0 && !matchAnyFold(cond.HTTPMethod, ps.HTTPMethod) {
		return false
	}
	if cond.HasWhere != nil && ps.HasWhere != *cond.HasWhere {
		return false
	}
	if cond.SQLMultiStatement != nil && (ps.SQLStatementCount > 1) != *cond.SQLMultiStatement {
		return false
	}
	if cond.SQLParseFailed != nil && ps.SQLParseFailed != *cond.SQLParseFailed {
		return false
	}
	if !regexMatches(cond.StatementPattern, ps.Raw, false) {
		return false
	}
	if !regexMatches(cond.StatementNotPattern, ps.Raw, true) {
		return false
	}
	if !regexMatches(cond.PathPattern, ps.URLPath, false) {
		return false
	}
	if !regexMatches(cond.BodyPattern, ps.Body, false) {
		return false
	}
	if !regexMatches(cond.BodyNotPattern, ps.Body, true) {
		return false
	}
	return true
}

func scopeMatches(scope RuleScope, ps ParsedStatement) bool {
	if len(scope.DsTypes) > 0 && !matchAnyFold(scope.DsTypes, ps.DsType) {
		return false
	}
	if scope.DatasourceID != "" && scope.DatasourceID != ps.DatasourceID {
		return false
	}
	if scope.Entity != "" && !entityMatches(scope.Entity, ps.ScopeEntities()) {
		return false
	}
	if scope.EntityPattern != "" && !patternMatchesAny(scope.EntityPattern, ps.ScopeEntities()) {
		return false
	}
	if scope.KeyPattern != "" && !wildcardMatch(scope.KeyPattern, ps.KeyPattern) {
		return false
	}
	return true
}

func firstKeyword(stmt string) string {
	fields := strings.Fields(strings.TrimSpace(stmt))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.Trim(fields[0], ";"))
}

func hasWhere(stmt string) bool {
	return regexp.MustCompile(`(?is)\bwhere\b`).MatchString(stmt)
}

func statementCount(stmt string) int {
	count := 0
	for _, part := range strings.Split(stmt, ";") {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}

func targetEntity(stmt string) string {
	re := regexp.MustCompile(`(?is)\b(?:from|into|update|table)\s+([a-zA-Z0-9_."` + "`" + `-]+)`)
	m := re.FindStringSubmatch(stmt)
	if len(m) < 2 {
		return ""
	}
	return strings.Trim(m[1], "`\"")
}

func matchCommand(commands []string, ps ParsedStatement) bool {
	target := ps.FirstKeyword
	if ps.RedisCommand != "" {
		target = strings.ToLower(ps.RedisCommand)
	}
	return matchAnyFold(commands, target)
}

func matchAnyFold(items []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func regexMatches(pattern, value string, negative bool) bool {
	if pattern == "" {
		return true
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	matched := re.MatchString(value)
	if negative {
		return !matched
	}
	return matched
}

func entityMatches(want string, entities []string) bool {
	for _, entity := range entities {
		if strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(entity)) {
			return true
		}
	}
	return false
}

func patternMatchesAny(pattern string, entities []string) bool {
	for _, entity := range entities {
		if wildcardMatch(pattern, entity) {
			return true
		}
	}
	return false
}

func wildcardMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return true
	}
	pattern = regexp.QuoteMeta(pattern)
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	re, err := regexp.Compile("(?i)^" + pattern + "$")
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func isEmptyCondition(cond RuleCondition) bool {
	return len(cond.Command) == 0 &&
		len(cond.HTTPMethod) == 0 &&
		cond.HasWhere == nil &&
		cond.SQLMultiStatement == nil &&
		cond.SQLParseFailed == nil &&
		cond.StatementPattern == "" &&
		cond.StatementNotPattern == "" &&
		cond.PathPattern == "" &&
		cond.BodyPattern == "" &&
		cond.BodyNotPattern == "" &&
		len(cond.Any) == 0 &&
		cond.Not == nil
}

func scopePriority(scope RuleScope) int {
	score := 0
	if scope.DatasourceID != "" {
		score += 100
	}
	if scope.Entity != "" {
		score += 50
	}
	if scope.EntityPattern != "" {
		score += 25
	}
	if scope.KeyPattern != "" {
		score += 10
	}
	return score
}

func actionToRiskLevel(action Action) RiskLevel {
	switch action {
	case ActionBlock, ActionRequireApproval:
		return RiskHigh
	case ActionWarn:
		return RiskMedium
	case ActionAllow:
		return RiskLow
	default:
		return RiskMedium
	}
}

func buildReasons(rule Rule) []string {
	if rule.Reason != "" {
		return []string{rule.Reason}
	}
	if rule.Description != "" {
		return []string{rule.Description}
	}
	return nil
}

func boolPtr(v bool) *bool {
	return &v
}
