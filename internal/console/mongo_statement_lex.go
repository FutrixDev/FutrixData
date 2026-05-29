package console

import (
	"errors"
	"strconv"
	"strings"
)

func parseMongoCall(statement string) (mongoCall, error) {
	input := strings.TrimSpace(statement)
	input = strings.TrimSuffix(input, ";")
	if !strings.HasPrefix(input, "db") {
		return mongoCall{}, errors.New("statement must start with db.")
	}
	i := len("db")
	i = skipSpaces(input, i)
	if i >= len(input) {
		return mongoCall{}, errors.New("invalid statement")
	}

	call := mongoCall{}
	for {
		i = skipSpaces(input, i)
		if i >= len(input) {
			break
		}

		switch input[i] {
		case '.':
			i++
			i = skipSpaces(input, i)
			ident, next := readIdent(input, i)
			if ident == "" {
				return mongoCall{}, errors.New("invalid db statement")
			}
			i = skipSpaces(input, next)
			if i < len(input) && input[i] == '(' {
				argsText, nextPos, err := readParenContent(input, i)
				if err != nil {
					return mongoCall{}, err
				}
				args, err := parseMongoArgs(argsText)
				if err != nil {
					return mongoCall{}, err
				}
				updated, err := applyMongoSelectorCall(call, ident, args)
				if err != nil {
					return mongoCall{}, err
				}
				call = updated
				i = nextPos
				if call.Method != "" {
					call, i, err = applyMongoCursorChain(input, i, call)
					if err != nil {
						return mongoCall{}, err
					}
					if !isTrailingOnly(input[i:]) {
						return mongoCall{}, errors.New("unexpected trailing characters")
					}
					return call, nil
				}
				continue
			}
			if call.Collection != "" || call.Method != "" {
				return mongoCall{}, errors.New("invalid collection call")
			}
			call.Collection = ident
		case '[':
			if call.Collection != "" || call.Method != "" {
				return mongoCall{}, errors.New("invalid collection call")
			}
			name, next, err := readBracketCollection(input, i)
			if err != nil {
				return mongoCall{}, err
			}
			if strings.TrimSpace(name) == "" {
				return mongoCall{}, errors.New("collection name is required")
			}
			call.Collection = name
			i = next
		default:
			return mongoCall{}, errors.New("invalid db statement")
		}
	}

	return mongoCall{}, errors.New("invalid db statement")
}

func applyMongoSelectorCall(call mongoCall, ident string, args []any) (mongoCall, error) {
	if call.Method != "" {
		return mongoCall{}, errors.New("unexpected trailing characters")
	}

	if call.Collection == "" {
		switch ident {
		case "getCollection":
			name, ok := argString(args, 0)
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				return mongoCall{}, errors.New("collection name is required")
			}
			call.Collection = name
			return call, nil
		case "getSiblingDB":
			name, ok := argString(args, 0)
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				return mongoCall{}, errors.New("database name is required")
			}
			call.Database = name
			return call, nil
		default:
			call.Method = ident
			call.Args = args
			call.DBMethod = true
			return call, nil
		}
	}

	call.Method = ident
	call.Args = args
	return call, nil
}

func applyMongoCursorChain(input string, start int, call mongoCall) (mongoCall, int, error) {
	i := start
	options := map[string]any{}

	for {
		i = skipSpaces(input, i)
		if i >= len(input) {
			break
		}
		if input[i] != '.' {
			break
		}
		i++
		i = skipSpaces(input, i)
		method, next := readIdent(input, i)
		if method == "" {
			return mongoCall{}, start, errors.New("unexpected trailing characters")
		}
		i = skipSpaces(input, next)
		if i >= len(input) || input[i] != '(' {
			return mongoCall{}, start, errors.New("unexpected trailing characters")
		}
		argsText, nextPos, err := readParenContent(input, i)
		if err != nil {
			return mongoCall{}, start, err
		}
		args, err := parseMongoArgs(argsText)
		if err != nil {
			return mongoCall{}, start, err
		}

		switch normalizeMongoAction(method) {
		case "sort":
			options["sort"] = argValue(args, 0)
		case "limit":
			options["limit"] = argValue(args, 0)
		case "skip":
			options["skip"] = argValue(args, 0)
		case "project", "projection":
			options["projection"] = argValue(args, 0)
		case "hint":
			options["hint"] = argValue(args, 0)
		case "pretty":
			// no-op, supported for compatibility with Mongo shell output.
		default:
			return mongoCall{}, start, errors.New("unexpected trailing characters")
		}

		i = nextPos
	}

	if len(options) == 0 {
		return call, start, nil
	}
	if call.DBMethod || normalizeMongoAction(call.Method) != "find" {
		return mongoCall{}, start, errors.New("unexpected trailing characters")
	}
	call.Args = mergeMongoFindCursorOptions(call.Args, options)
	return call, i, nil
}

func mergeMongoFindCursorOptions(args []any, cursorOptions map[string]any) []any {
	var options map[string]any
	if len(args) >= 2 {
		if existing, ok := args[1].(map[string]any); ok {
			options = existing
		}
	}
	if options == nil {
		options = map[string]any{}
		if len(args) == 0 {
			args = append(args, map[string]any{})
		}
		if len(args) == 1 {
			args = append(args, options)
		} else {
			args[1] = options
		}
	}
	for key, value := range cursorOptions {
		options[key] = value
	}
	return args
}

func readStringLiteral(input string, start int) (string, int, error) {
	if start >= len(input) {
		return "", start, errors.New("invalid string")
	}
	quote := input[start]
	var builder strings.Builder
	for i := start + 1; i < len(input); i++ {
		ch := input[i]
		if ch == '\\' {
			if i+1 >= len(input) {
				return "", i, errors.New("invalid escape")
			}
			next := input[i+1]
			switch next {
			case 'n':
				builder.WriteByte('\n')
			case 'r':
				builder.WriteByte('\r')
			case 't':
				builder.WriteByte('\t')
			case 'b':
				builder.WriteByte('\b')
			case 'f':
				builder.WriteByte('\f')
			case 'u':
				if i+5 >= len(input) {
					return "", i, errors.New("invalid unicode escape")
				}
				code := input[i+2 : i+6]
				r, err := strconv.ParseInt(code, 16, 32)
				if err != nil {
					return "", i, errors.New("invalid unicode escape")
				}
				builder.WriteRune(rune(r))
				i += 4
			default:
				builder.WriteByte(next)
			}
			i++
			continue
		}
		if ch == quote {
			return builder.String(), i + 1, nil
		}
		builder.WriteByte(ch)
	}
	return "", len(input), errors.New("unterminated string")
}

func readIdent(input string, start int) (string, int) {
	if start >= len(input) || !isIdentStart(input[start]) {
		return "", start
	}
	i := start + 1
	for i < len(input) && isIdentPart(input[i]) {
		i++
	}
	return input[start:i], i
}

func isIdentStart(ch byte) bool {
	return ch == '_' || ch == '$' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func skipSpaces(input string, start int) int {
	i := start
	for i < len(input) {
		if input[i] != ' ' && input[i] != '\t' && input[i] != '\n' && input[i] != '\r' {
			break
		}
		i++
	}
	return i
}

func readParenContent(input string, start int) (string, int, error) {
	if start >= len(input) || input[start] != '(' {
		return "", start, errors.New("missing '('")
	}
	depth := 0
	var quote rune
	escaped := false
	for i := start; i < len(input); i++ {
		ch := rune(input[i])
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return input[start+1 : i], i + 1, nil
			}
		}
	}
	return "", len(input), errors.New("unterminated call")
}

func readBracketCollection(input string, start int) (string, int, error) {
	if start >= len(input) || input[start] != '[' {
		return "", start, errors.New("missing '['")
	}
	i := skipSpaces(input, start+1)
	if i >= len(input) || (input[i] != '"' && input[i] != '\'') {
		return "", start, errors.New("collection name must be quoted")
	}
	value, next, err := readStringLiteral(input, i)
	if err != nil {
		return "", next, err
	}
	i = skipSpaces(input, next)
	if i >= len(input) || input[i] != ']' {
		return "", i, errors.New("missing ']'")
	}
	return value, i + 1, nil
}

func isTrailingOnly(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return true
	}
	return trimmed == ";"
}
