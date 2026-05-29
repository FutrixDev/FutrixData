package console

import "strings"

type sqlToken struct {
	value string
	start int
	end   int
}

func scanTopLevelTokens(statement string) []sqlToken {
	var tokens []sqlToken
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false
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
		case '-':
			if i+1 < len(statement) && statement[i+1] == '-' {
				i += 2
				for i < len(statement) && statement[i] != '\n' {
					i++
				}
				continue
			}
		case '/':
			if i+1 < len(statement) && statement[i+1] == '*' {
				i += 2
				for i+1 < len(statement) && !(statement[i] == '*' && statement[i+1] == '/') {
					i++
				}
				if i+1 < len(statement) {
					i += 2
				}
				continue
			}
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
			tokens = append(tokens, sqlToken{value: strings.ToLower(statement[start:i]), start: start, end: i})
			continue
		}
		i++
	}
	return tokens
}

func countTopLevelSQLStatements(statement, dialect string) int {
	count := 0
	hasContent := false
	depth := 0
	compoundBodyDepth := 0
	compoundBodyStatementContent := false
	compoundBody := sqlCompoundBodyStatement(statement, dialect)
	backslashEscapesString := sqlBackslashEscapesString(dialect)
	inSingle := false
	inSingleBackslashEscapes := false
	inDouble := false
	inBacktick := false
	dollarQuote := ""
	inMySQLExecutableComment := false

	for i := 0; i < len(statement); {
		if dollarQuote != "" {
			if strings.HasPrefix(statement[i:], dollarQuote) {
				i += len(dollarQuote)
				dollarQuote = ""
				continue
			}
			i++
			continue
		}
		ch := statement[i]
		if inSingle {
			if inSingleBackslashEscapes && ch == '\\' && i+1 < len(statement) {
				i += 2
				continue
			}
			if ch == '\'' {
				inSingle = false
				inSingleBackslashEscapes = false
			}
			i++
			continue
		}
		if inDouble {
			if backslashEscapesString && ch == '\\' && i+1 < len(statement) {
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
		if inMySQLExecutableComment && i+1 < len(statement) && statement[i] == '*' && statement[i+1] == '/' {
			i += 2
			inMySQLExecutableComment = false
			continue
		}

		if compoundBody && depth == 0 && isAlpha(ch) {
			hadCompoundStatementContent := compoundBodyStatementContent
			start := i
			for i < len(statement) && (isAlphaNumeric(statement[i]) || statement[i] == '_') {
				i++
			}
			word := strings.ToLower(statement[start:i])
			switch word {
			case "begin":
				compoundBodyDepth++
				compoundBodyStatementContent = false
			case "end":
				if compoundBodyDepth > 0 && !hadCompoundStatementContent && sqlEndClosesCompoundBegin(statement, i, dialect) {
					compoundBodyDepth--
				}
				compoundBodyStatementContent = true
			default:
				if compoundBodyDepth > 0 {
					compoundBodyStatementContent = true
				}
			}
			hasContent = true
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
			inSingleBackslashEscapes = backslashEscapesString || sqlPostgresEscapeStringPrefix(statement, i, dialect)
			hasContent = true
			i++
			continue
		case '"':
			inDouble = true
			hasContent = true
			i++
			continue
		case '`':
			inBacktick = true
			hasContent = true
			i++
			continue
		case '$':
			if delimiter, ok := sqlDollarQuoteDelimiter(statement[i:]); ok {
				dollarQuote = delimiter
				hasContent = true
				i += len(delimiter)
				continue
			}
		case '-':
			if sqlDashDashStartsComment(statement, i, dialect) {
				i += 2
				for i < len(statement) && statement[i] != '\n' {
					i++
				}
				continue
			}
		case '#':
			if normalizeSQLDialectName(dialect) == "mysql" {
				i++
				for i < len(statement) && statement[i] != '\n' {
					i++
				}
				continue
			}
		case '/':
			if i+1 < len(statement) && statement[i+1] == '*' {
				if sqlMySQLExecutableCommentStart(statement, i, dialect) {
					i += 2
					if i < len(statement) && statement[i] == '!' {
						i++
					}
					for i < len(statement) && statement[i] >= '0' && statement[i] <= '9' {
						i++
					}
					inMySQLExecutableComment = true
					continue
				}
				i = skipSQLBlockComment(statement, i, dialect)
				continue
			}
		case '(':
			depth++
			hasContent = true
			i++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			hasContent = true
			i++
			continue
		case ';':
			if depth == 0 {
				if compoundBodyDepth > 0 {
					compoundBodyStatementContent = false
					i++
					continue
				}
				if hasContent {
					count++
					hasContent = false
				}
				compoundBodyStatementContent = false
				i++
				continue
			}
		}

		if !isSQLWhitespace(ch) {
			hasContent = true
		}
		i++
	}
	if hasContent {
		count++
	}
	return count
}

func sqlCompoundBodyStatement(statement, dialect string) bool {
	tokens := scanTopLevelTokens(statement)
	if len(tokens) == 0 {
		return false
	}
	foundCreate := false
	dialect = strings.ToLower(strings.TrimSpace(dialect))
	for _, token := range tokens {
		switch token.value {
		case "create":
			foundCreate = true
		case "begin":
			return false
		case "procedure", "function", "trigger", "event":
			if foundCreate && dialect == "mysql" {
				return true
			}
			if foundCreate && token.value == "trigger" && (dialect == "d1" || dialect == "sqlite" || dialect == "sqlite3") {
				return true
			}
		}
	}
	return false
}

func sqlEndClosesCompoundBegin(statement string, index int, dialect string) bool {
	switch nextTopLevelSQLWord(statement, index, dialect) {
	case "if", "loop", "while", "repeat", "case":
		return false
	default:
		return true
	}
}

func sqlBackslashEscapesString(dialect string) bool {
	return normalizeSQLDialectName(dialect) == "mysql"
}

func sqlPostgresEscapeStringPrefix(statement string, quoteIndex int, dialect string) bool {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "postgres", "postgresql":
	default:
		return false
	}
	if quoteIndex == 0 {
		return false
	}
	prev := statement[quoteIndex-1]
	if prev != 'e' && prev != 'E' {
		return false
	}
	if quoteIndex == 1 {
		return true
	}
	beforePrev := statement[quoteIndex-2]
	return !isAlphaNumeric(beforePrev) && beforePrev != '_'
}

func sqlMySQLExecutableCommentStart(statement string, index int, dialect string) bool {
	return normalizeSQLDialectName(dialect) == "mysql" &&
		index+2 < len(statement) &&
		statement[index] == '/' &&
		statement[index+1] == '*' &&
		statement[index+2] == '!'
}

func skipSQLBlockComment(statement string, index int, dialect string) int {
	if !sqlNestedBlockComments(dialect) {
		index += 2
		for index+1 < len(statement) && !(statement[index] == '*' && statement[index+1] == '/') {
			index++
		}
		if index+1 < len(statement) {
			index += 2
		}
		return index
	}

	depth := 1
	index += 2
	for index+1 < len(statement) && depth > 0 {
		switch {
		case statement[index] == '/' && statement[index+1] == '*':
			depth++
			index += 2
		case statement[index] == '*' && statement[index+1] == '/':
			depth--
			index += 2
		default:
			index++
		}
	}
	return index
}

func sqlNestedBlockComments(dialect string) bool {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "postgres", "postgresql":
		return true
	default:
		return false
	}
}

func nextTopLevelSQLWord(statement string, index int, dialect string) string {
	for i := index; i < len(statement); {
		ch := statement[i]
		if isSQLWhitespace(ch) {
			i++
			continue
		}
		if sqlDashDashStartsComment(statement, i, dialect) {
			i += 2
			for i < len(statement) && statement[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '#' && normalizeSQLDialectName(dialect) == "mysql" {
			i++
			for i < len(statement) && statement[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(statement) && statement[i+1] == '*' {
			i += 2
			for i+1 < len(statement) && !(statement[i] == '*' && statement[i+1] == '/') {
				i++
			}
			if i+1 < len(statement) {
				i += 2
			}
			continue
		}
		if !isAlpha(ch) {
			return ""
		}
		start := i
		for i < len(statement) && (isAlphaNumeric(statement[i]) || statement[i] == '_') {
			i++
		}
		return strings.ToLower(statement[start:i])
	}
	return ""
}

func sqlDashDashStartsComment(statement string, index int, dialect string) bool {
	if index+1 >= len(statement) || statement[index+1] != '-' {
		return false
	}
	if normalizeSQLDialectName(dialect) != "mysql" {
		return true
	}
	next := index + 2
	if next >= len(statement) {
		return true
	}
	return isSQLWhitespace(statement[next]) || statement[next] < 0x20
}

func sqlDollarQuoteDelimiter(input string) (string, bool) {
	if len(input) < 2 || input[0] != '$' {
		return "", false
	}
	end := 1
	if input[end] == '$' {
		return "$$", true
	}
	if !isSQLDollarQuoteTagStart(input[end]) {
		return "", false
	}
	for end < len(input) && isSQLDollarQuoteTagPart(input[end]) {
		end++
	}
	if end < len(input) && input[end] == '$' {
		return input[:end+1], true
	}
	return "", false
}

func isSQLDollarQuoteTagStart(ch byte) bool {
	return isAlpha(ch) || ch == '_'
}

func isSQLDollarQuoteTagPart(ch byte) bool {
	return isAlphaNumeric(ch) || ch == '_'
}

func isSQLWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func findTopLevelOrderBy(statement string) (int, int) {
	tokens := scanTopLevelTokens(statement)
	for i := 0; i < len(tokens)-1; i++ {
		if tokens[i].value == "order" && tokens[i+1].value == "by" {
			return tokens[i].start, tokens[i+1].end
		}
	}
	return -1, -1
}

func orderByClause(statement string) string {
	orderStart, clauseStart := findTopLevelOrderBy(statement)
	if orderStart == -1 {
		return ""
	}
	end := len(statement)
	limitInfo := findTopLevelLimit(statement)
	if limitInfo.found && limitInfo.start > clauseStart {
		end = limitInfo.start
	}
	clause := strings.TrimSpace(statement[clauseStart:end])
	return strings.TrimRight(clause, " \t\n\r;")
}

func parseSQLOrderByClause(clause string) []sqlSortKey {
	if clause == "" {
		return nil
	}
	parts := splitTopLevelList(clause)
	keys := make([]sqlSortKey, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		desc := false
		if strings.HasSuffix(lower, " desc") {
			desc = true
			trimmed = strings.TrimSpace(trimmed[:len(trimmed)-5])
		} else if strings.HasSuffix(lower, " asc") {
			trimmed = strings.TrimSpace(trimmed[:len(trimmed)-4])
		}
		if trimmed == "" {
			continue
		}
		keys = append(keys, sqlSortKey{Column: trimmed, Desc: desc})
	}
	return keys
}

func splitTopLevelList(input string) []string {
	var parts []string
	depth := 0
	inSingle := false
	inDouble := false
	inBacktick := false
	start := 0
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inSingle {
			if ch == '\\' && i+1 < len(input) {
				i++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && i+1 < len(input) {
				i++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inBacktick = true
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(input) {
		parts = append(parts, strings.TrimSpace(input[start:]))
	}
	return parts
}

func stripSQLPagingTail(statement string) string {
	trimmed := strings.TrimRight(statement, " \t\n\r")
	if strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSpace(trimmed[:len(trimmed)-1])
	}
	end := len(trimmed)
	limitInfo := findTopLevelLimit(trimmed)
	if limitInfo.found && limitInfo.start < end {
		end = limitInfo.start
	}
	orderStart, _ := findTopLevelOrderBy(trimmed)
	if orderStart >= 0 && orderStart < end {
		end = orderStart
	}
	return strings.TrimRight(trimmed[:end], " \t\n\r")
}

func hasTopLevelWhere(statement string, dialect string) bool {
	_ = dialect
	tokens := scanTopLevelTokens(statement)
	for _, token := range tokens {
		if token.value == "where" {
			return true
		}
	}
	return false
}

func parseSimpleFromTableFallback(statement string) (string, bool) {
	tokens := scanTopLevelTokens(statement)
	for i := 0; i < len(tokens)-1; i++ {
		if tokens[i].value != "from" {
			continue
		}
		table, next := parseIdentifierAfter(statement, tokens[i].end)
		if table == "" {
			return "", false
		}
		for j := i + 1; j < len(tokens); j++ {
			if tokens[j].start < next {
				continue
			}
			switch tokens[j].value {
			case "join", "left", "right", "inner", "outer", "cross":
				return "", false
			case "where", "order", "limit", "group", "having", "union":
				return cleanIdentifier(table), true
			}
		}
		return cleanIdentifier(table), true
	}
	return "", false
}

func parseIdentifierAfter(statement string, index int) (string, int) {
	i := index
	for i < len(statement) && isSpace(statement[i]) {
		i++
	}
	if i >= len(statement) {
		return "", i
	}
	switch statement[i] {
	case '`', '"':
		quote := statement[i]
		i++
		start := i
		for i < len(statement) && statement[i] != quote {
			i++
		}
		if i >= len(statement) {
			return "", i
		}
		return statement[start:i], i + 1
	}
	start := i
	for i < len(statement) && (isAlphaNumeric(statement[i]) || statement[i] == '_' || statement[i] == '.') {
		i++
	}
	if start == i {
		return "", i
	}
	return statement[start:i], i
}

func cleanIdentifier(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.Trim(trimmed, "`\"")
	return trimmed
}
