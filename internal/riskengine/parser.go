package riskengine

import (
	"encoding/json"
	"strings"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/rediscmd"
)

// ParseStatement analyzes a raw statement and extracts risk-relevant fields.
func ParseStatement(dsType, datasourceID, statement string) ParsedStatement {
	typ := strings.ToLower(strings.TrimSpace(dsType))
	stmt := strings.TrimSpace(statement)

	ps := ParsedStatement{
		Raw:          stmt,
		DsType:       typ,
		DatasourceID: datasourceID,
	}
	if stmt == "" {
		return ps
	}

	switch typ {
	case "mysql", "postgresql", "d1":
		parseSQL(&ps)
	case "mongodb":
		parseMongoDB(&ps)
	case "elasticsearch":
		parseElasticsearch(&ps)
	case "redis", "redis_cluster":
		parseRedis(&ps)
	case "dynamodb":
		parseDynamoDB(&ps)
	}
	hydrateGenericOperationIntent(&ps)
	return ps
}

func hydrateGenericOperationIntent(ps *ParsedStatement) {
	if ps.Operation.Command != "" {
		return
	}
	command := strings.ToLower(strings.TrimSpace(ps.FirstKeyword))
	if command == "" {
		return
	}
	ps.Operation = OperationIntent{
		Command:           command,
		CommandCandidates: uniqueLowerNonEmpty(command),
		Args:              append([]string(nil), ps.Args...),
		Classes:           genericOperationClasses(ps.DsType, command, ps.IsQuery),
	}
}

func genericOperationClasses(dsType, command string, isQuery bool) []string {
	command = strings.ToLower(strings.TrimSpace(command))
	var classes []string
	if isGenericWriteCommand(dsType, command, isQuery) {
		classes = appendUniqueLower(classes, operationClassWrite)
	}
	if isGenericAdminCommand(dsType, command) {
		classes = appendUniqueLower(classes, operationClassAdmin)
	}
	if isQuery {
		classes = appendUniqueLower(classes, operationClassRead)
	}
	return classes
}

func isGenericWriteCommand(dsType, command string, isQuery bool) bool {
	switch strings.ToLower(strings.TrimSpace(dsType)) {
	case "mysql", "postgresql", "d1":
		switch command {
		case "insert", "replace", "update", "delete", "drop", "truncate", "alter", "create", "grant", "revoke", "call", "exec", "execute", "do":
			return true
		}
	case "dynamodb":
		switch command {
		case "insert", "update", "delete":
			return true
		}
	case "mongodb":
		switch command {
		case "insertone", "insertmany", "updateone", "updatemany", "replaceone", "findoneandupdate", "findoneandreplace",
			"deleteone", "deletemany", "findoneanddelete", "bulkwrite", "drop", "dropdatabase":
			return true
		}
	case "elasticsearch":
		switch strings.ToUpper(command) {
		case "DELETE", "PUT", "PATCH":
			return true
		case "POST":
			return !isQuery
		}
	}
	return false
}

func isGenericAdminCommand(dsType, command string) bool {
	switch strings.ToLower(strings.TrimSpace(dsType)) {
	case "mysql", "postgresql", "d1":
		switch command {
		case "drop", "truncate", "alter", "create", "grant", "revoke":
			return true
		}
	case "mongodb":
		switch command {
		case "drop", "dropdatabase", "createindex", "dropindex":
			return true
		}
	}
	return false
}

func parseSQL(ps *ParsedStatement) {
	dialect := ps.DsType
	facts, err := console.SQLRiskFactsForStatement(ps.Raw, dialect)
	ps.SQLStatementCount = facts.StatementCount
	ps.SQLParseFailed = facts.ParseFailed
	if err != nil {
		ps.SQLParseFailed = true
		ps.FirstKeyword = FirstKeyword(ps.Raw)
		ps.HasWhere = SQLStatementHasWhereClause(ps.Raw)
		ps.TargetEntity = SQLTargetTable(ps.Raw)
		switch ps.FirstKeyword {
		case "select", "show", "describe", "explain":
			ps.IsQuery = true
		}
		return
	}

	ps.FirstKeyword = facts.Verb
	ps.TargetEntity = facts.TargetEntity
	ps.TargetEntities = append([]string(nil), facts.TargetEntities...)
	ps.HasWhere = facts.HasWhere
	ps.EqualityFields = append([]string(nil), facts.EqualityFields...)
	ps.HasUnsafeWhereBool = facts.HasUnsafeWhereBool
	ps.HasJoin = facts.HasJoin
	ps.JoinCount = facts.JoinCount
	ps.HasSubquery = facts.HasSubquery
	ps.IsQuery = facts.IsReadQuery
}

func parseMongoDB(ps *ParsedStatement) {
	lower := strings.ToLower(ps.Raw)

	if facts, err := console.MongoRiskFactsForStatement(ps.Raw); err == nil {
		ps.MongoAction = NormalizeMongoAction(facts.Action)
		ps.TargetEntity = strings.TrimSpace(facts.Collection)
		ps.TargetEntities = append([]string(nil), facts.TargetCollections...)
		ps.FirstKeyword = ps.MongoAction
		ps.IsQuery = facts.IsQuery
		ps.HasJoin = facts.HasJoin
		ps.JoinCount = facts.JoinCount
		return
	}

	if strings.HasPrefix(strings.TrimSpace(lower), "{") {
		var payload struct {
			Action     string `json:"action"`
			Collection string `json:"collection"`
		}
		if err := json.Unmarshal([]byte(ps.Raw), &payload); err == nil {
			ps.MongoAction = NormalizeMongoAction(payload.Action)
			ps.TargetEntity = strings.TrimSpace(payload.Collection)
			ps.FirstKeyword = ps.MongoAction
			switch ps.MongoAction {
			case "find", "aggregate", "getusers", "count", "serverstatus":
				ps.IsQuery = true
			}
			return
		}
	}

	if strings.HasPrefix(lower, "db.") {
		parts := strings.SplitN(ps.Raw[3:], ".", 2)
		if len(parts) == 2 {
			ps.TargetEntity = strings.TrimSpace(parts[0])
			if parenIdx := strings.Index(parts[1], "("); parenIdx != -1 {
				ps.MongoAction = NormalizeMongoAction(parts[1][:parenIdx])
				ps.FirstKeyword = ps.MongoAction
			}
		}
	}

	// Shell commands
	switch {
	case strings.HasPrefix(lower, "show "):
		ps.FirstKeyword = "show"
		ps.IsQuery = true
	}

	if ps.MongoAction == "" && ps.FirstKeyword == "" {
		ps.FirstKeyword = FirstKeyword(ps.Raw)
	}

	if MongoShellStatementIsLowRisk(lower) {
		ps.IsQuery = true
	}
}

func parseElasticsearch(ps *ParsedStatement) {
	method, path, body, ok := ParseElasticsearchRequestShape(ps.Raw)
	if !ok {
		return
	}
	ps.HTTPMethod = method
	ps.URLPath = path
	ps.Body = body
	ps.FirstKeyword = strings.ToLower(method)
	ps.TargetEntity = ElasticsearchTargetIndex(path)

	switch method {
	case "GET", "HEAD":
		ps.IsQuery = true
	case "POST":
		if ElasticsearchPathIsSearch(path) {
			ps.IsQuery = true
		}
	}
}

func parseRedis(ps *ParsedStatement) {
	fields, err := rediscmd.Parse(ps.Raw)
	if err != nil {
		fields = strings.Fields(ps.Raw)
	}
	if len(fields) == 0 {
		return
	}
	applyRedisCommand(ps, fields[0], fields[1:])
}

func applyRedisCommand(ps *ParsedStatement, command string, args []string) {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return
	}
	commandArgs := append([]string(nil), args...)
	ps.RedisCommand = strings.ToUpper(command)
	ps.FirstKeyword = command
	ps.Args = commandArgs
	ps.Operation = redisOperationIntent(command, commandArgs)
	if len(ps.Operation.KeyCandidates) > 0 {
		ps.KeyPattern = ps.Operation.KeyCandidates[0]
	} else {
		ps.KeyPattern = ""
	}
	ps.IsQuery = RedisCommandIsLowRisk(ps.RedisCommand) &&
		!operationHasAnyClass(ps.Operation, operationClassWrite, operationClassAdmin, operationClassScan, operationClassScript)
}

func ParseRedisCommandArgs(datasourceID string, args []string) ParsedStatement {
	raw, _ := console.RedisCommandStatement(args)
	ps := ParsedStatement{
		Raw:          raw,
		DsType:       "redis",
		DatasourceID: datasourceID,
	}
	if len(args) == 0 {
		return ps
	}
	command := strings.TrimSpace(args[0])
	if command == "" {
		return ps
	}
	applyRedisCommand(&ps, command, args[1:])
	return ps
}

func parseDynamoDB(ps *ParsedStatement) {
	ps.FirstKeyword = FirstKeyword(ps.Raw)
	ps.HasWhere = SQLStatementHasWhereClause(ps.Raw)
	ps.DynamoTable, ps.DynamoIndex = DynamodbStatementTarget(ps.Raw)
	ps.TargetEntity = DynamodbStatementEntity(ps.Raw)
	switch ps.FirstKeyword {
	case "select":
		ps.IsQuery = true
	}
}
