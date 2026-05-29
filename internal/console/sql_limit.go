package console

import (
	"fmt"
	"strconv"
	"strings"

	"futrixdata/platform/internal/console/window"
)

type sqlLimitInfo struct {
	found       bool
	parsed      bool
	start       int
	end         int
	count       int64
	offset      int64
	hasOffset   bool
	offsetFirst bool
}

func prepareSQLQuery(statement string, policy window.LimitPolicy) (string, window.Decision) {
	return prepareSQLQueryWithDialect(statement, "postgres", policy)
}

func prepareSQLQueryWithDialect(statement, dialect string, policy window.LimitPolicy) (string, window.Decision) {
	if sqlStatementSupportsAutoLimit(statement, dialect) {
		return applySQLLimitPolicy(statement, policy)
	}
	return statement, policy.Decide(nil)
}

func sqlStatementSupportsAutoLimit(statement, dialect string) bool {
	a, err := AnalyzeSQL(statement, normalizeSQLDialectName(dialect))
	if err == nil {
		return a.IsQuery
	}
	switch SQLStatementVerb(statement, dialect) {
	case "select":
		return true
	default:
		return false
	}
}

func applySQLLimitPolicy(statement string, policy window.LimitPolicy) (string, window.Decision) {
	info := findTopLevelLimit(statement)
	var decision window.Decision
	if info.found && info.parsed {
		limit := info.count
		decision = policy.Decide(&limit)
		if decision.Enforced {
			return rewriteSQLLimit(statement, info, decision.Fetch), decision
		}
		return statement, decision
	}
	if info.found {
		decision = policy.Decide(nil)
		return statement, decision
	}
	decision = policy.Decide(nil)
	return appendSQLLimit(statement, decision.Fetch), decision
}

func appendSQLLimit(statement string, limit int64) string {
	trimmed := strings.TrimRight(statement, " \t\n\r")
	if strings.HasSuffix(trimmed, ";") {
		return strings.TrimRight(trimmed[:len(trimmed)-1], " \t") + fmt.Sprintf(" LIMIT %d;", limit)
	}
	return trimmed + fmt.Sprintf(" LIMIT %d", limit)
}

// findTopLevelLimit scans for a top-level LIMIT clause and parses numeric forms.
func findTopLevelLimit(statement string) sqlLimitInfo {
	var info sqlLimitInfo
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false
	last := -1
	lastEnd := -1

	for i := 0; i < len(statement); {
		ch := statement[i]
		if inSingle {
			if ch == '\\' && i+1 < len(statement) {
				i += 2
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			if ch == '\\' && i+1 < len(statement) {
				i += 2
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			i++
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			i++
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
			i++
			continue
		case '"':
			inDouble = true
			i++
			continue
		case '`':
			inBacktick = true
			i++
			continue
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}

		if depth == 0 && isAlpha(statement[i]) {
			start := i
			for i < len(statement) && (isAlphaNumeric(statement[i]) || statement[i] == '_') {
				i++
			}
			token := strings.ToLower(statement[start:i])
			if token == "limit" {
				last = start
				lastEnd = i
			}
			continue
		}
		i++
	}

	if last == -1 {
		return info
	}
	info.found = true
	info.start = last
	info.end = lastEnd
	info.parsed = parseLimitClause(statement, &info)
	return info
}

func parseLimitClause(statement string, info *sqlLimitInfo) bool {
	i := info.end
	skipSpaces := func() {
		for i < len(statement) && isSpace(statement[i]) {
			i++
		}
	}
	parseNumber := func() (int64, bool) {
		skipSpaces()
		start := i
		for i < len(statement) && isDigit(statement[i]) {
			i++
		}
		if start == i {
			return 0, false
		}
		value, err := strconv.ParseInt(statement[start:i], 10, 64)
		if err != nil {
			return 0, false
		}
		return value, true
	}

	first, ok := parseNumber()
	if !ok {
		return false
	}
	skipSpaces()
	if i < len(statement) && statement[i] == ',' {
		i++
		second, ok := parseNumber()
		if !ok {
			return false
		}
		info.offsetFirst = true
		info.offset = first
		info.count = second
		info.hasOffset = true
		info.end = i
		return true
	}

	if hasKeyword(statement[i:], "offset") {
		i += len("offset")
		offset, ok := parseNumber()
		if !ok {
			return false
		}
		info.count = first
		info.offset = offset
		info.hasOffset = true
		info.end = i
		return true
	}

	info.count = first
	info.end = i
	return true
}

func rewriteSQLLimit(statement string, info sqlLimitInfo, limit int64) string {
	clause := fmt.Sprintf("LIMIT %d", limit)
	if info.hasOffset {
		if info.offsetFirst {
			clause = fmt.Sprintf("LIMIT %d, %d", info.offset, limit)
		} else {
			clause = fmt.Sprintf("LIMIT %d OFFSET %d", limit, info.offset)
		}
	}
	return statement[:info.start] + clause + statement[info.end:]
}

func hasKeyword(input, keyword string) bool {
	trimmed := strings.TrimLeft(input, " \t\n\r")
	if len(trimmed) < len(keyword) {
		return false
	}
	return strings.EqualFold(trimmed[:len(keyword)], keyword)
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isAlphaNumeric(ch byte) bool {
	return isAlpha(ch) || isDigit(ch)
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}
