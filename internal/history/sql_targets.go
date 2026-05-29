package history

import "strings"

func ExtractSQLTargets(statement string) []string {
	tokens := tokenizeSQL(statement)
	var out []string
	seen := map[string]struct{}{}
	add := func(raw string) {
		name := normalizeSQLIdent(raw)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	fromMode := false
	expectTable := false

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		lower := strings.ToLower(tok)

		if fromMode {
			if isFromStop(lower) {
				fromMode = false
				if lower == "join" {
					if i+1 < len(tokens) {
						add(tokens[i+1])
					}
				}
				continue
			}
			if tok == "," {
				expectTable = true
				continue
			}
			if tok == "(" {
				fromMode = false
				expectTable = false
				continue
			}
			if expectTable {
				if lower == "as" {
					continue
				}
				add(tok)
				expectTable = false
			}
			continue
		}

		switch lower {
		case "from":
			fromMode = true
			expectTable = true
		case "join", "update", "into":
			if i+1 < len(tokens) {
				add(tokens[i+1])
			}
		}
	}

	return out
}

func isFromStop(token string) bool {
	switch token {
	case "where", "join", "left", "right", "inner", "outer", "full", "cross", "group", "order", "limit", "union", "having", "on", "values", "set", "returning":
		return true
	default:
		return false
	}
}

func tokenizeSQL(statement string) []string {
	var tokens []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		tokens = append(tokens, buf.String())
		buf.Reset()
	}

	for _, r := range statement {
		switch r {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
				flush()
			}
			if inSingle {
				continue
			}
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
		}

		if inSingle {
			continue
		}

		if isSQLSeparator(r) && !inDouble && !inBacktick {
			flush()
			if r == ',' {
				tokens = append(tokens, ",")
			}
			if r == '(' {
				tokens = append(tokens, "(")
			}
			continue
		}

		buf.WriteRune(r)
	}
	flush()
	return tokens
}

func isSQLSeparator(r rune) bool {
	switch r {
	case ' ', '\n', '\t', ',', ';', '(', ')', '=':
		return true
	default:
		return false
	}
}

func normalizeSQLIdent(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "`\"")
	trimmed = strings.Trim(trimmed, ",;")
	return trimmed
}
