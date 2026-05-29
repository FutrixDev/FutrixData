package console

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var sqlEqualityFieldPatternFallback = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*=`)

type SQLRiskFacts struct {
	Verb               string
	HasWhere           bool
	IsReadQuery        bool
	TargetEntity       string
	TargetEntities     []string
	EqualityFields     []string
	HasUnsafeWhereBool bool
	HasJoin            bool
	JoinCount          int
	HasSubquery        bool
	ExplainEligible    bool
	StatementCount     int
	ParseFailed        bool
}

func SQLRiskFactsForStatement(statement, dialect string) (SQLRiskFacts, error) {
	statementCount := countTopLevelSQLStatements(statement, dialect)
	a, err := analyzeSQLForDialect(statement, dialect)
	if err == nil {
		facts := sqlRiskFactsFromAnalysis(a)
		fallback := fallbackSQLRiskFacts(statement, dialect)
		facts = mergeFallbackSQLRiskFacts(facts, fallback)
		facts.StatementCount = maxSQLStatementCount(a.StatementCount, statementCount)
		if facts.Verb != "" {
			return facts, nil
		}
	}

	facts := fallbackSQLRiskFacts(statement, dialect)
	facts.StatementCount = statementCount
	facts.ParseFailed = true
	if facts.Verb != "" {
		return facts, nil
	}
	return facts, fmt.Errorf("sql risk facts unavailable: %w", err)
}

func sqlRiskFactsFromAnalysis(a *SQLAnalysis) SQLRiskFacts {
	if a == nil {
		return SQLRiskFacts{}
	}

	facts := SQLRiskFacts{
		Verb:               strings.ToLower(strings.TrimSpace(sqlVerbFromAnalysis(a))),
		IsReadQuery:        a.IsQuery,
		HasUnsafeWhereBool: a.HasUnsafeWhereBool,
		HasJoin:            a.HasJoin,
		JoinCount:          a.JoinCount,
		HasSubquery:        a.HasSubquery,
		StatementCount:     a.StatementCount,
	}
	facts.HasWhere = sqlHasWhereFromAnalysis(a, facts.Verb)
	facts.IsReadQuery = facts.IsReadQuery && sqlVerbIsReadOnly(facts.Verb)
	facts.ExplainEligible = facts.Verb != "" && facts.Verb != "show" && facts.Verb != "describe" && facts.Verb != "pragma"
	facts.EqualityFields = uniqueSortedSQLFields(a.WhereEqualityColumns)
	referencedTables := uniqueSQLTableNames(a.Tables)
	primaryEntity := normalizeSQLRiskEntity(a.PrimaryTable)
	referencedTables = filterCTEEntityNames(referencedTables, a.CTENames)
	if facts.Verb == "select" {
		primaryEntity = choosePrimarySQLReadEntity(primaryEntity, referencedTables, a.CTENames)
	}
	referencedTables = prependPrimaryEntity(primaryEntity, referencedTables)

	switch facts.Verb {
	case "insert", "update", "delete":
		facts.TargetEntities = collectSQLWriteEntities(primaryEntity, facts.Verb, a.StatementType, a.CTEWriteTargets)
		facts.TargetEntity = choosePrimarySQLWriteEntity(primaryEntity, referencedTables, facts.Verb, a.StatementType, a.CTEWriteTargets, facts.TargetEntities)
	case "select", "show", "describe", "explain":
		facts.TargetEntity = primaryEntity
	default:
		facts.TargetEntity = primaryEntity
	}
	if len(facts.TargetEntities) == 0 && facts.TargetEntity != "" {
		facts.TargetEntities = []string{facts.TargetEntity}
	}

	return facts
}

func mergeFallbackSQLRiskFacts(primary, fallback SQLRiskFacts) SQLRiskFacts {
	if primary.Verb == "" {
		primary.Verb = fallback.Verb
	}
	if !primary.HasWhere && fallback.HasWhere {
		primary.HasWhere = true
	}
	if (primary.Verb == "" || primary.Verb == fallback.Verb) && !primary.IsReadQuery && fallback.IsReadQuery {
		primary.IsReadQuery = true
	}
	if primary.TargetEntity == "" {
		primary.TargetEntity = fallback.TargetEntity
	}
	if len(primary.TargetEntities) == 0 && len(fallback.TargetEntities) > 0 {
		primary.TargetEntities = append([]string(nil), fallback.TargetEntities...)
	}
	if len(primary.EqualityFields) == 0 {
		primary.EqualityFields = append([]string(nil), fallback.EqualityFields...)
	}
	if !primary.HasUnsafeWhereBool && fallback.HasUnsafeWhereBool {
		primary.HasUnsafeWhereBool = true
	}
	if !primary.ExplainEligible {
		primary.ExplainEligible = fallback.ExplainEligible
	}
	return primary
}

func fallbackSQLRiskFacts(statement, dialect string) SQLRiskFacts {
	verb := leadingSQLKeyword(statement)
	facts := SQLRiskFacts{
		Verb:               verb,
		HasWhere:           hasTopLevelWhere(statement, dialect),
		IsReadQuery:        sqlVerbIsReadOnly(verb),
		HasUnsafeWhereBool: sqlWhereHasUnsafeBoolFallback(statement),
		ExplainEligible:    verb != "" && verb != "show" && verb != "describe" && verb != "pragma",
		StatementCount:     countTopLevelSQLStatements(statement, dialect),
	}
	target := fallbackSQLTargetEntity(statement, verb)
	facts.TargetEntity = target
	if target != "" {
		facts.TargetEntities = []string{target}
	}
	facts.EqualityFields = fallbackSQLEqualityFields(statement)
	return facts
}

func maxSQLStatementCount(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sqlVerbFromAnalysis(a *SQLAnalysis) string {
	if a == nil {
		return ""
	}
	switch a.StatementType {
	case "DELETE":
		return "delete"
	case "UPDATE":
		return "update"
	case "INSERT":
		return "insert"
	}
	switch {
	case a.HasCTEDeleteWithoutWhere || a.HasCTEDelete:
		return "delete"
	case a.HasCTEUpdateWithoutWhere || a.HasCTEUpdate:
		return "update"
	case a.HasCTEInsert:
		return "insert"
	case a.StatementType == "SELECT":
		return "select"
	case a.StatementType == "EXPLAIN":
		return "explain"
	default:
		return ""
	}
}

func sqlHasWhereFromAnalysis(a *SQLAnalysis, verb string) bool {
	if a == nil {
		return false
	}
	switch verb {
	case "delete":
		if a.StatementType == "DELETE" {
			return a.HasWhere
		}
		if a.HasCTEDelete {
			return !a.HasCTEDeleteWithoutWhere
		}
	case "update":
		if a.StatementType == "UPDATE" {
			return a.HasWhere
		}
		if a.HasCTEUpdate {
			return !a.HasCTEUpdateWithoutWhere
		}
	}
	return a.HasWhere
}

func sqlVerbIsReadOnly(verb string) bool {
	switch verb {
	case "select", "show", "describe", "explain", "pragma":
		return true
	default:
		return false
	}
}

func uniqueSQLTableNames(refs []TableRef) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := normalizeSQLRiskEntity(tableRefName(ref))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uniqueSortedSQLFields(cols []ColumnRef) []string {
	if len(cols) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cols))
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		name := strings.ToLower(strings.TrimSpace(col.Column))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func prependPrimaryEntity(primary string, referenced []string) []string {
	if primary == "" {
		return referenced
	}
	out := make([]string, 0, len(referenced)+1)
	out = append(out, primary)
	for _, item := range referenced {
		if item == primary {
			continue
		}
		out = append(out, item)
	}
	return out
}

func filterCTEEntityNames(referenced []string, cteNames []string) []string {
	if len(referenced) == 0 || len(cteNames) == 0 {
		return referenced
	}
	out := make([]string, 0, len(referenced))
	for _, item := range referenced {
		if isCTEName(item, cteNames) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeSQLRiskEntity(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ".")
	for i := range parts {
		parts[i] = strings.ToLower(strings.Trim(parts[i], "`\""))
	}
	return strings.Join(parts, ".")
}

func choosePrimarySQLReadEntity(primary string, referenced []string, cteNames []string) string {
	if primary == "" {
		if len(referenced) == 0 {
			return ""
		}
		return referenced[0]
	}
	if !isCTEName(primary, cteNames) {
		return primary
	}
	for _, table := range referenced {
		if !isCTEName(table, cteNames) {
			return table
		}
	}
	return primary
}

func collectSQLWriteEntities(primary, verb, statementType string, cteWriteTargets []SQLWriteTargetRef) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cteWriteTargets)+1)
	if isTopLevelSQLWriteStatement(statementType, verb) && primary != "" {
		seen[primary] = struct{}{}
		out = append(out, primary)
	}
	for _, ref := range cteWriteTargets {
		name := normalizeSQLRiskEntity(tableRefName(ref.Table))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func choosePrimarySQLWriteEntity(primary string, referenced []string, verb, statementType string, cteWriteTargets []SQLWriteTargetRef, targetEntities []string) string {
	if isTopLevelSQLWriteStatement(statementType, verb) && primary != "" {
		return primary
	}
	for i := len(cteWriteTargets) - 1; i >= 0; i-- {
		ref := cteWriteTargets[i]
		if ref.Verb != verb {
			continue
		}
		name := normalizeSQLRiskEntity(tableRefName(ref.Table))
		if name != "" {
			return name
		}
	}
	for _, name := range targetEntities {
		if name != "" {
			return name
		}
	}
	for _, table := range referenced {
		if table != "" {
			return table
		}
	}
	return ""
}

func isTopLevelSQLWriteStatement(statementType, verb string) bool {
	if verb == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(statementType), verb)
}

func isCTEName(name string, cteNames []string) bool {
	if name == "" || len(cteNames) == 0 {
		return false
	}
	base := name
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[idx+1:]
	}
	for _, cte := range cteNames {
		if normalizeSQLRiskEntity(cte) == base {
			return true
		}
	}
	return false
}

func fallbackSQLTargetEntity(statement, verb string) string {
	tokens := scanTopLevelTokens(statement)
	for i := 0; i < len(tokens); i++ {
		match := false
		switch verb {
		case "select", "delete":
			match = tokens[i].value == "from"
		case "insert", "replace":
			match = tokens[i].value == "into"
		case "update":
			match = tokens[i].value == "update"
		case "drop", "truncate", "alter", "create":
			match = tokens[i].value == "table"
		case "explain":
			if i+1 < len(tokens) {
				return fallbackSQLTargetEntity(statement[tokens[i].end:], leadingSQLKeyword(statement[tokens[i].end:]))
			}
		}
		if !match {
			continue
		}
		name, _ := fallbackSQLTargetIdentifier(statement, tokens[i].end, verb)
		if name != "" {
			return normalizeSQLRiskEntity(name)
		}
	}
	return ""
}

func fallbackSQLTargetIdentifier(statement string, index int, verb string) (string, int) {
	next := index
	for {
		name, end := parseIdentifierAfter(statement, next)
		if name == "" {
			return "", end
		}
		if !shouldSkipDDLTargetToken(verb, name) {
			return name, end
		}
		next = end
	}
}

func shouldSkipDDLTargetToken(verb, token string) bool {
	switch verb {
	case "drop", "truncate", "alter", "create":
		switch strings.ToLower(strings.TrimSpace(token)) {
		case "if", "exists", "not", "only":
			return true
		}
	}
	return false
}

func fallbackSQLEqualityFields(statement string) []string {
	whereClause := whereClauseFallback(statement)
	if whereClause == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	for _, groups := range sqlEqualityFieldPatternFallback.FindAllStringSubmatch(whereClause, -1) {
		for _, raw := range groups[1:] {
			field := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "`\""))
			if field == "" {
				continue
			}
			if idx := strings.LastIndex(field, "."); idx >= 0 {
				field = field[idx+1:]
			}
			if _, ok := seen[field]; ok {
				continue
			}
			seen[field] = struct{}{}
			out = append(out, field)
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}

func sqlWhereHasUnsafeBoolFallback(statement string) bool {
	where := strings.ToLower(strings.TrimSpace(whereClauseFallback(statement)))
	return strings.Contains(where, " or ") || strings.Contains(where, " not ")
}

func whereClauseFallback(statement string) string {
	tokens := scanTopLevelTokens(statement)
	whereIndex := -1
	end := len(statement)
	for _, token := range tokens {
		switch token.value {
		case "where":
			if whereIndex == -1 {
				whereIndex = token.end
			}
		case "order", "limit", "group", "returning", "having":
			if whereIndex != -1 && token.start < end {
				end = token.start
			}
		}
	}
	if whereIndex == -1 || whereIndex >= end {
		return ""
	}
	return statement[whereIndex:end]
}
