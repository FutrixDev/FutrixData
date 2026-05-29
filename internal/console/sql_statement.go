package console

import "strings"

type SQLStatementRiskInfo struct {
	Verb     string
	HasWhere bool
}

func normalizeSQLDialectName(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql", "d1", "sqlite", "sqlite3":
		return "postgres"
	default:
		return "postgres"
	}
}

func analyzeSQLForDialect(statement, dialect string) (*SQLAnalysis, error) {
	primary := normalizeSQLDialectName(dialect)
	a, err := AnalyzeSQL(statement, primary)
	if err == nil {
		return a, nil
	}
	switch primary {
	case "mysql":
		return AnalyzeSQL(statement, "postgres")
	default:
		return AnalyzeSQL(statement, "mysql")
	}
}

func AnalyzeSQLForDialect(statement, dialect string) (*SQLAnalysis, error) {
	return analyzeSQLForDialect(statement, dialect)
}

// SQLStatementVerb returns the effective top-level verb for SQL-family statements.
// It prefers AST-based classification and falls back to a comment-aware keyword scan
// for unsupported dialect extensions like PRAGMA.
func SQLStatementVerb(statement, dialect string) string {
	if facts, err := SQLRiskFactsForStatement(statement, dialect); err == nil && facts.Verb != "" {
		return facts.Verb
	}
	return leadingSQLKeyword(statement)
}

func SQLStatementIsReadQuery(statement, dialect string) bool {
	switch SQLStatementVerb(statement, dialect) {
	case "select", "show", "describe", "explain", "pragma":
		return true
	default:
		return false
	}
}

func SQLStatementRiskInfoForSafety(statement, dialect string) SQLStatementRiskInfo {
	if facts, err := SQLRiskFactsForStatement(statement, dialect); err == nil && facts.Verb != "" {
		return SQLStatementRiskInfo{Verb: facts.Verb, HasWhere: facts.HasWhere}
	}

	verb := SQLStatementVerb(statement, dialect)
	info := SQLStatementRiskInfo{Verb: verb}
	switch verb {
	case "delete", "update":
		info.HasWhere = SQLStatementHasWhereClause(statement, dialect)
	}
	return info
}

func SQLStatementHasWhereClause(statement, dialect string) bool {
	if facts, err := SQLRiskFactsForStatement(statement, dialect); err == nil {
		return facts.HasWhere
	}
	return hasTopLevelWhere(statement, dialect)
}

func SQLWhereEqualityFields(statement, dialect string) map[string]struct{} {
	facts, err := SQLRiskFactsForStatement(statement, dialect)
	if err != nil || len(facts.EqualityFields) == 0 {
		return nil
	}
	fields := make(map[string]struct{}, len(facts.EqualityFields))
	for _, name := range facts.EqualityFields {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		fields[name] = struct{}{}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func SQLWhereClauseHasUnsafeBooleanOperators(statement, dialect string) bool {
	facts, err := SQLRiskFactsForStatement(statement, dialect)
	if err != nil {
		return strings.TrimSpace(statement) != ""
	}
	return facts.HasUnsafeWhereBool
}

func leadingSQLKeyword(statement string) string {
	tokens := scanTopLevelTokens(statement)
	if len(tokens) == 0 {
		return ""
	}
	if tokens[0].value != "with" {
		return tokens[0].value
	}
	for i := 1; i < len(tokens); i++ {
		switch tokens[i].value {
		case "recursive":
			continue
		case "select", "insert", "update", "delete", "explain", "show", "describe", "pragma":
			return tokens[i].value
		}
	}
	return tokens[0].value
}

func sqlStatementRiskInfoFromAnalysis(a *SQLAnalysis) (SQLStatementRiskInfo, bool) {
	if a == nil {
		return SQLStatementRiskInfo{}, false
	}

	if a.HasCTEDeleteWithoutWhere || (a.StatementType == "DELETE" && !a.HasWhere) {
		return SQLStatementRiskInfo{Verb: "delete", HasWhere: false}, true
	}
	if a.HasCTEUpdateWithoutWhere || (a.StatementType == "UPDATE" && !a.HasWhere) {
		return SQLStatementRiskInfo{Verb: "update", HasWhere: false}, true
	}
	if a.HasCTEDelete || a.StatementType == "DELETE" {
		return SQLStatementRiskInfo{Verb: "delete", HasWhere: true}, true
	}
	if a.HasCTEUpdate || a.StatementType == "UPDATE" {
		return SQLStatementRiskInfo{Verb: "update", HasWhere: true}, true
	}
	if a.HasCTEInsert || a.StatementType == "INSERT" {
		return SQLStatementRiskInfo{Verb: "insert"}, true
	}

	switch a.StatementType {
	case "SELECT":
		return SQLStatementRiskInfo{Verb: "select", HasWhere: a.HasWhere}, true
	case "EXPLAIN":
		return SQLStatementRiskInfo{Verb: "explain"}, true
	default:
		return SQLStatementRiskInfo{}, false
	}
}
