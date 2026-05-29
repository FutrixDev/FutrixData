package console

import (
	"encoding/json"
	"errors"
	"strings"
)

func parseMongoArgs(raw string) ([]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts, err := splitMongoArgs(raw)
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		normalized, err := normalizeMongoJSON(trimmed)
		if err != nil {
			return nil, err
		}
		var value any
		if err := json.Unmarshal([]byte(normalized), &value); err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	return args, nil
}

func splitMongoArgs(raw string) ([]string, error) {
	var args []string
	depth := 0
	var quote rune
	escaped := false
	start := 0
	for i, ch := range raw {
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
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, raw[start:i])
				start = i + 1
			}
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated string")
	}
	if start <= len(raw) {
		args = append(args, raw[start:])
	}
	return args, nil
}

func normalizeMongoJSON(input string) (string, error) {
	var builder strings.Builder
	var stack []rune
	expectingKey := false
	for i := 0; i < len(input); {
		ch := input[i]
		if ch == '"' || ch == '\'' {
			value, next, err := readStringLiteral(input, i)
			if err != nil {
				return "", err
			}
			encoded, _ := json.Marshal(value)
			builder.Write(encoded)
			i = next
			continue
		}
		if expectingKey && isIdentStart(ch) {
			ident, next := readIdent(input, i)
			encoded, _ := json.Marshal(ident)
			builder.Write(encoded)
			i = next
			continue
		}
		switch ch {
		case '{':
			stack = append(stack, '{')
			expectingKey = true
		case '[':
			stack = append(stack, '[')
			expectingKey = false
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			expectingKey = false
		case ':':
			expectingKey = false
		case ',':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				expectingKey = true
			} else {
				expectingKey = false
			}
		}
		builder.WriteByte(ch)
		i++
	}
	return builder.String(), nil
}
