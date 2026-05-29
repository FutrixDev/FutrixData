package riskengine

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	dynamodbSelectTargetPattern  = regexp.MustCompile(`(?is)\bfrom\s+("([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_.-]*))(?:\s*\.\s*("([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_.-]*)))?`)
	dynamodbWriteTargetPattern   = regexp.MustCompile(`(?is)(?:\binto\b|\bupdate\b)\s+("([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_.-]*))`)
	sqlWhereClausePattern        = regexp.MustCompile(`(?is)\bwhere\b(.*?)(?:\border\s+by\b|\blimit\b|\bgroup\s+by\b|\breturning\b|$)`)
	sqlEqualityFieldPattern      = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*=`)
	sqlUnsafeBoolPattern         = regexp.MustCompile(`(?i)\b(or|not)\b`)
	sqlOrPattern                 = regexp.MustCompile(`(?i)\bor\b`)
	sqlNotPattern                = regexp.MustCompile(`(?i)\bnot\b`)
	dynamoInFieldPattern         = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s+in\s*\[`)
	dynamoBetweenFieldPattern    = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s+between\b`)
	dynamoBeginsWithFieldPattern = regexp.MustCompile(`(?is)\bbegins_with\s*\(\s*(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*,`)
	dynamoComparisonFieldPattern = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*(<=|>=|<|>)`)
	dynamoIndexKeyPartPattern    = regexp.MustCompile(`(?i)^\s*([^=]+?)\s*=\s*(HASH|RANGE)\s*$`)

	sqlTargetTablePattern = regexp.MustCompile(`(?is)(?:\bfrom\b|\binto\b|\bupdate\b|\bjoin\b|\btable\b)\s+("([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_.-]*))`)

	// sqlCTEActualKeyword finds the actual DML keyword after a CTE body.
	// Matches: ) SELECT/INSERT/UPDATE/DELETE at the end of the CTE definition.
	sqlCTEActualKeyword = regexp.MustCompile(`(?is)\)\s*(select|insert|update|delete)\b`)
)

type DynamodbIndexMetadata struct {
	Name         string
	PartitionKey string
	SortKey      string
}

// DynamodbRiskMetadata holds table and index key metadata for DynamoDB risk assessment.
type DynamodbRiskMetadata struct {
	TablePartitionKey string
	TableSortKey      string
	PartitionKeys     map[string]struct{}
	Indexes           []DynamodbIndexMetadata
}

type dynamodbConditionSummary struct {
	WhereClause      string
	EqualityFields   map[string]struct{}
	InFields         map[string]struct{}
	BetweenFields    map[string]struct{}
	BeginsWithFields map[string]struct{}
	ComparisonFields map[string]struct{}
	HasOr            bool
	HasNot           bool
}

type DynamodbAccessAssessment struct {
	Safe   bool
	Reason string
}

// sqlActualKeywordAfterCTE extracts the real operation keyword from a WITH (CTE) statement.
// For "WITH cte AS (...) DELETE FROM t", returns "delete".
// Returns empty string if the keyword cannot be determined.
func sqlActualKeywordAfterCTE(statement string) string {
	m := sqlCTEActualKeyword.FindAllStringSubmatch(statement, -1)
	if len(m) == 0 {
		return ""
	}
	// Take the last match — the actual DML is at the end, after all CTE definitions.
	return strings.ToLower(m[len(m)-1][1])
}

// FirstKeyword extracts the first meaningful keyword from a statement,
// skipping SQL comments (-- and /* */).
func FirstKeyword(statement string) string {
	trimmed := strings.TrimSpace(statement)
	for {
		switch {
		case strings.HasPrefix(trimmed, "--"):
			if idx := strings.Index(trimmed, "\n"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[idx+1:])
				continue
			}
			return ""
		case strings.HasPrefix(trimmed, "/*"):
			if idx := strings.Index(trimmed, "*/"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[idx+2:])
				continue
			}
			return ""
		default:
			fields := strings.Fields(trimmed)
			if len(fields) == 0 {
				return ""
			}
			return strings.ToLower(strings.TrimSpace(fields[0]))
		}
	}
}

// NormalizeMongoAction normalizes a MongoDB action string.
func NormalizeMongoAction(action string) string {
	value := strings.ToLower(strings.TrimSpace(action))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

// SQLStatementHasWhereClause returns true if the SQL statement contains a WHERE clause.
func SQLStatementHasWhereClause(statement string) bool {
	return SQLWhereClause(statement) != ""
}

// SQLWhereClause extracts the WHERE clause from a SQL statement.
func SQLWhereClause(statement string) string {
	matches := sqlWhereClausePattern.FindStringSubmatch(StripSQLStringLiteralsAndComments(statement))
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// StripSQLStringLiteralsAndComments replaces string literals and comments with spaces,
// preserving the character positions for regex matching.
func StripSQLStringLiteralsAndComments(statement string) string {
	if statement == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(statement))

	const (
		scanNormal = iota
		scanSingleQuote
		scanLineComment
		scanBlockComment
	)

	state := scanNormal
	for i := 0; i < len(statement); i++ {
		ch := statement[i]
		switch state {
		case scanNormal:
			switch {
			case ch == '\'':
				state = scanSingleQuote
				b.WriteByte(' ')
			case ch == '-' && i+1 < len(statement) && statement[i+1] == '-':
				state = scanLineComment
				b.WriteString("  ")
				i++
			case ch == '/' && i+1 < len(statement) && statement[i+1] == '*':
				state = scanBlockComment
				b.WriteString("  ")
				i++
			default:
				b.WriteByte(ch)
			}
		case scanSingleQuote:
			if ch == '\'' {
				if i+1 < len(statement) && statement[i+1] == '\'' {
					b.WriteString("  ")
					i++
					continue
				}
				state = scanNormal
			}
			if ch == '\n' || ch == '\r' {
				b.WriteByte(ch)
			} else {
				b.WriteByte(' ')
			}
		case scanLineComment:
			if ch == '\n' || ch == '\r' {
				state = scanNormal
				b.WriteByte(ch)
			} else {
				b.WriteByte(' ')
			}
		case scanBlockComment:
			if ch == '*' && i+1 < len(statement) && statement[i+1] == '/' {
				b.WriteString("  ")
				i++
				state = scanNormal
				continue
			}
			if ch == '\n' || ch == '\r' {
				b.WriteByte(ch)
			} else {
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// NormalizeSQLIdentifier normalizes a SQL identifier by removing quotes and extracting
// the final segment after dots.
func NormalizeSQLIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "\"`")
	if idx := strings.LastIndex(trimmed, "."); idx != -1 {
		trimmed = trimmed[idx+1:]
	}
	trimmed = strings.Trim(trimmed, "\"`")
	return strings.ToLower(strings.TrimSpace(trimmed))
}

// SQLEqualityFields extracts field names used in equality conditions in the WHERE clause.
func SQLEqualityFields(statement string) map[string]struct{} {
	whereClause := SQLWhereClause(statement)
	if whereClause == "" {
		return nil
	}
	fields := make(map[string]struct{})
	for _, groups := range sqlEqualityFieldPattern.FindAllStringSubmatch(whereClause, -1) {
		for _, raw := range groups[1:] {
			normalized := NormalizeSQLIdentifier(raw)
			if normalized == "" {
				continue
			}
			fields[normalized] = struct{}{}
			break
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// SQLWhereClauseHasUnsafeBooleanOperators checks if the WHERE clause contains OR or NOT.
func SQLWhereClauseHasUnsafeBooleanOperators(statement string) bool {
	whereClause := SQLWhereClause(statement)
	if whereClause == "" {
		return false
	}
	return sqlUnsafeBoolPattern.MatchString(whereClause)
}

// ParseElasticsearchRequestShape parses an ES statement into method, path, and body.
func ParseElasticsearchRequestShape(statement string) (method string, path string, body string, ok bool) {
	lines := strings.Split(statement, "\n")
	firstLine := ""
	firstLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		firstLine = strings.TrimSpace(line)
		firstLineIdx = i
		break
	}
	if firstLine == "" {
		return "", "", "", false
	}
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		return "", "", "", false
	}
	method = strings.ToUpper(strings.TrimSpace(parts[0]))
	switch method {
	case "GET", "POST", "PUT", "DELETE", "HEAD", "PATCH":
	default:
		return "", "", "", false
	}
	path = strings.TrimSpace(parts[1])
	if path == "" {
		return "", "", "", false
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if firstLineIdx+1 < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[firstLineIdx+1:], "\n"))
	}
	return method, path, body, true
}

// ElasticsearchPathIsSearch returns true if the path targets a _search endpoint.
func ElasticsearchPathIsSearch(path string) bool {
	trimmed := strings.TrimSpace(path)
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return trimmed == "/_search" || strings.HasSuffix(trimmed, "/_search")
}

// ElasticsearchStatementIsLowRisk returns true if the ES statement is a safe, bounded search.
func ElasticsearchStatementIsLowRisk(statement string) bool {
	method, path, body, ok := ParseElasticsearchRequestShape(statement)
	if !ok {
		return false
	}
	if (method == "GET" || method == "HEAD") && strings.Contains(path, "/_doc/") {
		return true
	}
	if !ElasticsearchPathIsSearch(path) {
		return false
	}
	if strings.TrimSpace(body) == "" {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return false
	}
	if payload == nil {
		return false
	}
	if _, ok := payload["aggs"]; ok {
		return false
	}
	if _, ok := payload["aggregations"]; ok {
		return false
	}
	query, _ := payload["query"].(map[string]any)
	if len(query) == 0 {
		return false
	}
	if ElasticsearchQueryContainsBroadClause(query) {
		return false
	}
	size := intArg(payload, "size", 0)
	if size <= 0 || size > 100 {
		return false
	}
	return true
}

// ElasticsearchQueryContainsBroadClause checks if the query contains match_all, wildcard, regexp, or fuzzy.
func ElasticsearchQueryContainsBroadClause(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "match_all", "wildcard", "regexp", "fuzzy":
				return true
			}
			if ElasticsearchQueryContainsBroadClause(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ElasticsearchQueryContainsBroadClause(child) {
				return true
			}
		}
	}
	return false
}

// ElasticsearchBodyRisks analyzes an ES request body and returns a list of risk reasons.
// Returns nil if the body has no detectable risks.
func ElasticsearchBodyRisks(body string) []string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}
	if payload == nil {
		return nil
	}

	var risks []string

	// 1. Check for missing query field (unbounded scan)
	query, hasQuery := payload["query"].(map[string]any)
	hasAggs := false
	if _, ok := payload["aggs"]; ok {
		hasAggs = true
	}
	if _, ok := payload["aggregations"]; ok {
		hasAggs = true
	}

	if !hasQuery || len(query) == 0 {
		if hasAggs {
			risks = append(risks, "aggregation without query filter")
		} else {
			risks = append(risks, "no query field")
		}
	}

	// 2. Check for broad clauses (match_all, wildcard, regexp, fuzzy)
	if hasQuery && ElasticsearchQueryContainsBroadClause(query) {
		risks = append(risks, "broad query clause (match_all/wildcard/regexp/fuzzy)")
	}

	// 3. Check for script / script_score
	if esBodyContainsKey(payload, "script") || esBodyContainsKey(payload, "script_score") {
		risks = append(risks, "script query detected")
	}

	// 4. Check size limits
	size := intArg(payload, "size", -1)
	if size < 0 {
		// No size specified — default is 10 in ES, but explicit is safer
		// Only warn if there's also a risky query pattern
		if !hasQuery || len(query) == 0 {
			risks = append(risks, "no size limit")
		}
	} else if size > 10000 {
		risks = append(risks, fmt.Sprintf("size %d exceeds 10000", size))
	}

	// 5. Deep pagination: from + size > 10000
	from := intArg(payload, "from", 0)
	effectiveSize := size
	if effectiveSize < 0 {
		effectiveSize = 10 // ES default
	}
	if from+effectiveSize > 10000 {
		risks = append(risks, fmt.Sprintf("deep pagination (from=%d + size=%d > 10000)", from, effectiveSize))
	}

	// 6. Aggregation analysis
	if hasAggs {
		aggs := esGetAggs(payload)
		if aggs != nil {
			depth := esAggsDepth(aggs, 0)
			if depth > 3 {
				risks = append(risks, fmt.Sprintf("nested aggregation depth %d exceeds 3", depth))
			}
			if esTermsAggMissingSize(aggs) {
				risks = append(risks, "terms aggregation without size limit")
			}
		}
	}

	return risks
}

func ElasticsearchPathQueryRisks(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil
	}
	query := u.Query()
	if len(query) == 0 {
		return nil
	}

	var risks []string

	q := strings.TrimSpace(query.Get("q"))
	if q != "" {
		lowerQ := strings.ToLower(q)
		if strings.Contains(lowerQ, "*:*") || strings.Contains(lowerQ, "*") {
			risks = append(risks, "broad query clause (query string)")
		}
	}

	size := 10
	if raw := strings.TrimSpace(query.Get("size")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			size = parsed
			if parsed > 10000 {
				risks = append(risks, fmt.Sprintf("size %d exceeds 10000", parsed))
			}
		}
	}

	from := 0
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			from = parsed
		}
	}
	if from+size > 10000 {
		risks = append(risks, fmt.Sprintf("deep pagination (from=%d + size=%d > 10000)", from, size))
	}

	if len(risks) == 0 {
		return nil
	}
	return risks
}

// esBodyContainsKey recursively checks if a JSON structure contains a given key.
func esBodyContainsKey(value any, target string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.ToLower(strings.TrimSpace(key)) == target {
				return true
			}
			if esBodyContainsKey(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if esBodyContainsKey(child, target) {
				return true
			}
		}
	}
	return false
}

// esGetAggs extracts the aggregations map from a payload.
func esGetAggs(payload map[string]any) map[string]any {
	if aggs, ok := payload["aggs"].(map[string]any); ok {
		return aggs
	}
	if aggs, ok := payload["aggregations"].(map[string]any); ok {
		return aggs
	}
	return nil
}

// esAggsDepth computes the maximum nesting depth of aggregations.
func esAggsDepth(aggs map[string]any, current int) int {
	if aggs == nil {
		return current
	}
	maxDepth := current + 1
	for _, v := range aggs {
		aggDef, ok := v.(map[string]any)
		if !ok {
			continue
		}
		// Check for nested aggs inside this aggregation definition
		if nested := esGetAggs(aggDef); nested != nil {
			d := esAggsDepth(nested, current+1)
			if d > maxDepth {
				maxDepth = d
			}
		}
	}
	return maxDepth
}

// esTermsAggMissingSize checks if any terms aggregation lacks a size parameter.
func esTermsAggMissingSize(aggs map[string]any) bool {
	for _, v := range aggs {
		aggDef, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if terms, ok := aggDef["terms"].(map[string]any); ok {
			if _, hasSize := terms["size"]; !hasSize {
				return true
			}
		}
		// Recurse into nested aggs
		if nested := esGetAggs(aggDef); nested != nil {
			if esTermsAggMissingSize(nested) {
				return true
			}
		}
	}
	return false
}

// RedisCommandIsLowRisk returns true if the Redis command is a read-only O(1) operation.
func RedisCommandIsLowRisk(cmd string) bool {
	switch strings.ToUpper(strings.TrimSpace(cmd)) {
	case "GET", "TYPE", "TTL", "PTTL", "EXISTS", "STRLEN", "GETRANGE", "HGET", "HEXISTS", "HLEN", "LINDEX", "LLEN", "SCARD", "SISMEMBER", "ZCARD", "ZSCORE":
		return true
	default:
		return false
	}
}

// MongoShellStatementIsLowRisk returns true if the mongo shell statement is a read-only operation.
func MongoShellStatementIsLowRisk(lowerStatement string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(lowerStatement))
	if trimmed == "" {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "show collections"),
		strings.HasPrefix(trimmed, "show dbs"),
		strings.HasPrefix(trimmed, "show databases"),
		strings.Contains(trimmed, ".find("),
		strings.Contains(trimmed, ".findone("),
		strings.Contains(trimmed, ".aggregate("):
		return true
	default:
		return false
	}
}

// DynamodbStatementTableName extracts the table name from a DynamoDB PartiQL SELECT/DELETE
// statement (FROM clause only). Preserves the original behavior for risk policy checks.
func DynamodbStatementTableName(statement string) string {
	matches := dynamodbSelectTargetPattern.FindStringSubmatch(strings.TrimSpace(statement))
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(matches[2], matches[3], matches[4]))
}

// DynamodbStatementEntity extracts the target table from any DynamoDB PartiQL statement
// (SELECT/DELETE FROM, INSERT INTO, UPDATE). Used for entity extraction in ParsedStatement.
func DynamodbStatementEntity(statement string) string {
	trimmed := strings.TrimSpace(statement)
	if matches := dynamodbSelectTargetPattern.FindStringSubmatch(trimmed); len(matches) > 0 {
		return strings.TrimSpace(firstNonEmpty(matches[2], matches[3], matches[4]))
	}
	if matches := dynamodbWriteTargetPattern.FindStringSubmatch(trimmed); len(matches) > 0 {
		return strings.TrimSpace(firstNonEmpty(matches[2], matches[3], matches[4]))
	}
	return ""
}

// DynamodbStatementTarget extracts the table name and optional index name from a
// DynamoDB PartiQL statement when the syntax separates them unambiguously. If the
// raw target is an unquoted dotted identifier, it is preserved as a table name so
// dotted table names do not get rewritten into fake index lookups.
func DynamodbStatementTarget(statement string) (tableName string, indexName string) {
	trimmed := strings.TrimSpace(statement)

	if matches := dynamodbSelectTargetPattern.FindStringSubmatch(trimmed); len(matches) > 0 {
		tableName = firstNonEmpty(matches[2], matches[3], matches[4])
		indexName = firstNonEmpty(matches[6], matches[7], matches[8])
		return strings.TrimSpace(tableName), strings.TrimSpace(indexName)
	}

	if matches := dynamodbWriteTargetPattern.FindStringSubmatch(trimmed); len(matches) > 0 {
		tableName = firstNonEmpty(matches[2], matches[3], matches[4])
		return strings.TrimSpace(tableName), ""
	}

	return "", ""
}

// DynamodbRiskMetadataFromDescribe extracts partition key metadata from a DescribeEntity result.
func DynamodbRiskMetadataFromDescribe(raw any) (DynamodbRiskMetadata, bool) {
	if raw == nil {
		return DynamodbRiskMetadata{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return DynamodbRiskMetadata{}, false
	}
	var payload struct {
		Details []struct {
			Label string `json:"label"`
			Value any    `json:"value"`
		} `json:"details"`
		Indexes []struct {
			Name       string `json:"name"`
			Definition string `json:"definition"`
		} `json:"indexes"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return DynamodbRiskMetadata{}, false
	}
	meta := DynamodbRiskMetadata{PartitionKeys: make(map[string]struct{})}
	for _, detail := range payload.Details {
		switch strings.ToLower(strings.TrimSpace(detail.Label)) {
		case "partition key":
			key := NormalizeSQLIdentifier(fmt.Sprint(detail.Value))
			if key != "" {
				meta.TablePartitionKey = key
				meta.PartitionKeys[key] = struct{}{}
			}
		case "sort key":
			key := NormalizeSQLIdentifier(fmt.Sprint(detail.Value))
			if key != "" {
				meta.TableSortKey = key
			}
		}
	}
	for _, idx := range payload.Indexes {
		indexMeta := DynamodbIndexMetadata{Name: strings.TrimSpace(idx.Name)}
		for _, part := range strings.Split(idx.Definition, "|") {
			match := dynamoIndexKeyPartPattern.FindStringSubmatch(strings.TrimSpace(part))
			if len(match) != 3 {
				continue
			}
			key := NormalizeSQLIdentifier(match[1])
			if key != "" {
				switch {
				case strings.EqualFold(match[2], "HASH"):
					indexMeta.PartitionKey = key
					meta.PartitionKeys[key] = struct{}{}
				case strings.EqualFold(match[2], "RANGE"):
					indexMeta.SortKey = key
				}
			}
		}
		if indexMeta.PartitionKey != "" {
			meta.Indexes = append(meta.Indexes, indexMeta)
		}
	}
	return meta, meta.TablePartitionKey != "" || len(meta.Indexes) > 0
}

func splitDynamodbQualifiedTarget(value string) (tableName string, indexName string) {
	trimmed := strings.TrimSpace(value)
	dot := strings.Index(trimmed, ".")
	if dot <= 0 || dot == len(trimmed)-1 {
		return trimmed, ""
	}
	return strings.TrimSpace(trimmed[:dot]), strings.TrimSpace(trimmed[dot+1:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeIdentifierSetFromPattern(pattern *regexp.Regexp, whereClause string) map[string]struct{} {
	if strings.TrimSpace(whereClause) == "" {
		return nil
	}
	fields := make(map[string]struct{})
	for _, groups := range pattern.FindAllStringSubmatch(whereClause, -1) {
		for i := 1; i < len(groups); i++ {
			normalized := NormalizeSQLIdentifier(groups[i])
			if normalized == "" {
				continue
			}
			fields[normalized] = struct{}{}
			break
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func dynamodbConditionSummaryForStatement(statement string) dynamodbConditionSummary {
	whereClause := SQLWhereClause(statement)
	return dynamodbConditionSummary{
		WhereClause:      whereClause,
		EqualityFields:   SQLEqualityFields(statement),
		InFields:         normalizeIdentifierSetFromPattern(dynamoInFieldPattern, whereClause),
		BetweenFields:    normalizeIdentifierSetFromPattern(dynamoBetweenFieldPattern, whereClause),
		BeginsWithFields: normalizeIdentifierSetFromPattern(dynamoBeginsWithFieldPattern, whereClause),
		ComparisonFields: normalizeIdentifierSetFromPattern(dynamoComparisonFieldPattern, whereClause),
		HasOr:            sqlOrPattern.MatchString(whereClause),
		HasNot:           sqlNotPattern.MatchString(whereClause),
	}
}

func dynamodbFieldSetContains(fields map[string]struct{}, field string) bool {
	if len(fields) == 0 {
		return false
	}
	_, ok := fields[NormalizeSQLIdentifier(field)]
	return ok
}

func dynamodbOnlyUsesPartitionEquality(summary dynamodbConditionSummary, partitionKey string) bool {
	partitionKey = NormalizeSQLIdentifier(partitionKey)
	if partitionKey == "" {
		return false
	}
	if len(summary.BetweenFields) > 0 || len(summary.BeginsWithFields) > 0 || len(summary.ComparisonFields) > 0 {
		return false
	}
	if len(summary.InFields) > 0 {
		return len(summary.InFields) == 1 && dynamodbFieldSetContains(summary.InFields, partitionKey) && len(summary.EqualityFields) == 0
	}
	return len(summary.EqualityFields) == 1 && dynamodbFieldSetContains(summary.EqualityFields, partitionKey)
}

func dynamodbHasPartitionEqualityOrIn(summary dynamodbConditionSummary, partitionKey string) bool {
	return dynamodbFieldSetContains(summary.EqualityFields, partitionKey) || dynamodbFieldSetContains(summary.InFields, partitionKey)
}

func dynamodbHasSortKeyCondition(summary dynamodbConditionSummary, sortKey string) bool {
	return dynamodbFieldSetContains(summary.EqualityFields, sortKey) ||
		dynamodbFieldSetContains(summary.BetweenFields, sortKey) ||
		dynamodbFieldSetContains(summary.BeginsWithFields, sortKey) ||
		dynamodbFieldSetContains(summary.ComparisonFields, sortKey)
}

func findDynamodbIndex(meta DynamodbRiskMetadata, indexName string) (DynamodbIndexMetadata, bool) {
	for _, idx := range meta.Indexes {
		if strings.EqualFold(strings.TrimSpace(idx.Name), strings.TrimSpace(indexName)) {
			return idx, true
		}
	}
	return DynamodbIndexMetadata{}, false
}

func resolveDynamodbIndexTarget(statement, describedEntity string, meta DynamodbRiskMetadata) (DynamodbIndexMetadata, bool) {
	_, explicitIndex := DynamodbStatementTarget(statement)
	if strings.TrimSpace(explicitIndex) != "" {
		return findDynamodbIndex(meta, explicitIndex)
	}
	return DynamodbIndexMetadata{}, false
}

// AnalyzeDynamodbStatementAccess classifies whether a DynamoDB PartiQL statement
// follows a key-based access path or looks scan-like.
func AnalyzeDynamodbStatementAccess(statement string, describe any, describedEntity ...string) DynamodbAccessAssessment {
	meta, ok := DynamodbRiskMetadataFromDescribe(describe)
	if !ok {
		return DynamodbAccessAssessment{Reason: "DynamoDB table metadata not available for access-path verification"}
	}
	summary := dynamodbConditionSummaryForStatement(statement)
	if strings.TrimSpace(summary.WhereClause) == "" {
		return DynamodbAccessAssessment{Reason: "missing partition key equality or IN looks scan-like"}
	}
	if summary.HasNot {
		return DynamodbAccessAssessment{Reason: "NOT predicate prevents verified key-based access"}
	}

	partitionKey := meta.TablePartitionKey
	sortKey := meta.TableSortKey
	if idx, found := resolveDynamodbIndexTarget(statement, firstNonEmpty(describedEntity...), meta); found {
		partitionKey = idx.PartitionKey
		sortKey = idx.SortKey
	} else if _, explicitIndex := DynamodbStatementTarget(statement); strings.TrimSpace(explicitIndex) != "" {
		return DynamodbAccessAssessment{Reason: dynamodbUnknownExplicitIndexReason(explicitIndex, meta)}
	}

	if summary.HasOr && !dynamodbOnlyUsesPartitionEquality(summary, partitionKey) {
		return DynamodbAccessAssessment{Reason: "OR predicate prevents verified key-based access"}
	}
	if dynamodbHasPartitionEqualityOrIn(summary, partitionKey) {
		return DynamodbAccessAssessment{Safe: true}
	}
	if sortKey != "" && dynamodbHasSortKeyCondition(summary, sortKey) {
		return DynamodbAccessAssessment{Reason: "sort key filter without partition key equality looks scan-like"}
	}
	if _, explicitIndex := DynamodbStatementTarget(statement); strings.TrimSpace(explicitIndex) != "" {
		return DynamodbAccessAssessment{Reason: "missing index partition key equality or IN looks scan-like"}
	}
	return DynamodbAccessAssessment{Reason: "missing partition key equality or IN looks scan-like"}
}

func dynamodbUnknownExplicitIndexReason(explicitIndex string, meta DynamodbRiskMetadata) string {
	reason := fmt.Sprintf("target index metadata not available: requested index %s", strings.TrimSpace(explicitIndex))
	if len(meta.Indexes) == 0 {
		return reason + "; no known indexes in table metadata"
	}
	names := make([]string, 0, len(meta.Indexes))
	for _, idx := range meta.Indexes {
		if strings.TrimSpace(idx.Name) != "" {
			names = append(names, strings.TrimSpace(idx.Name))
		}
	}
	if len(names) == 0 {
		return reason + "; no known indexes in table metadata"
	}
	return reason + "; known indexes: " + strings.Join(names, ", ")
}

// DynamodbStatementIsLowRisk checks if a DynamoDB statement follows a key-based access path.
func DynamodbStatementIsLowRisk(statement string, describe any) bool {
	return AnalyzeDynamodbStatementAccess(statement, describe).Safe
}

// SQLTargetTable extracts the target table from a SQL statement
// (FROM, INTO, UPDATE, JOIN, TABLE clauses).
func SQLTargetTable(statement string) string {
	stripped := StripSQLStringLiteralsAndComments(statement)
	matches := sqlTargetTablePattern.FindStringSubmatch(stripped)
	if len(matches) == 0 {
		return ""
	}
	for _, value := range matches[2:] {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return NormalizeSQLIdentifier(trimmed)
		}
	}
	return ""
}

// ElasticsearchTargetIndex extracts the index name from an ES URL path.
// Returns the first path segment that doesn't start with '_'.
// For API-only paths like /_cat/indices, returns empty.
func ElasticsearchTargetIndex(path string) string {
	trimmed := strings.TrimSpace(path)
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	segments := strings.Split(trimmed, "/")
	if len(segments) == 0 {
		return ""
	}
	// If the first segment starts with '_', the entire path is an API path
	// (e.g., /_cat/indices, /_search, /_cluster/health).
	// Only return an index if the first segment is a user-defined index name.
	first := strings.TrimSpace(segments[0])
	if first == "" || strings.HasPrefix(first, "_") {
		return ""
	}
	return first
}

func intArg(args map[string]any, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &parsed); err == nil {
			return parsed
		}
		return fallback
	}
}
