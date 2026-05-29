package console

import (
	"strings"

	"futrixdata/platform/internal/datasource"
)

func PrepareExplainStatement(statement string, analyze bool, dsType datasource.DataSourceType) string {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return ""
	}

	var strippedExplain bool
	trimmed, strippedExplain = stripExplainPrefix(trimmed)
	if strippedExplain {
		if rest, ok := cutLeadingKeyword(trimmed, "analyze"); ok {
			trimmed = rest
		}
	}

	if trimmed == "" {
		return ""
	}

	if dsType != datasource.TypePostgreSQL || !analyze {
		return trimmed
	}
	return "ANALYZE " + trimmed
}

func stripExplainPrefix(statement string) (string, bool) {
	rest, ok := cutLeadingKeyword(statement, "explain")
	if !ok {
		return strings.TrimSpace(statement), false
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		if close := findClosingParen(rest); close >= 0 {
			rest = strings.TrimSpace(rest[close+1:])
		}
	}
	return rest, true
}

func cutLeadingKeyword(statement string, keyword string) (string, bool) {
	trimmed := strings.TrimSpace(statement)
	if len(trimmed) < len(keyword) {
		return trimmed, false
	}
	head := trimmed[:len(keyword)]
	if !strings.EqualFold(head, keyword) {
		return trimmed, false
	}
	if len(trimmed) == len(keyword) {
		return "", true
	}
	next := trimmed[len(keyword)]
	if !isKeywordBoundary(next) {
		return trimmed, false
	}
	return strings.TrimSpace(trimmed[len(keyword):]), true
}

func isKeywordBoundary(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '('
}

func findClosingParen(value string) int {
	if !strings.HasPrefix(value, "(") {
		return -1
	}
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
