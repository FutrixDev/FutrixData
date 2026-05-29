package rediscmd

import (
	"errors"
	"fmt"
	"strings"
)

// Parse tokenizes Redis CLI-style command text into binary-safe arguments.
func Parse(statement string) ([]string, error) {
	args := make([]string, 0, 4)
	for i := 0; ; {
		for i < len(statement) && isRedisSpace(statement[i]) {
			i++
		}
		if i >= len(statement) {
			break
		}

		var arg strings.Builder
		for i < len(statement) && !isRedisSpace(statement[i]) {
			var err error
			switch statement[i] {
			case '"':
				i, err = parseRedisDoubleQuoted(statement, i+1, &arg)
			case '\'':
				i, err = parseRedisSingleQuoted(statement, i+1, &arg)
			default:
				arg.WriteByte(statement[i])
				i++
			}
			if err != nil {
				return nil, err
			}
		}
		args = append(args, arg.String())
	}

	if len(args) == 0 {
		return nil, errors.New("statement required")
	}
	return args, nil
}

func parseRedisDoubleQuoted(statement string, i int, arg *strings.Builder) (int, error) {
	for i < len(statement) {
		ch := statement[i]
		if ch == '"' {
			i++
			if i < len(statement) && !isRedisSpace(statement[i]) {
				return 0, fmt.Errorf("invalid redis command: closing double quote must be followed by whitespace")
			}
			return i, nil
		}
		if ch == '\\' && i+1 < len(statement) {
			next := statement[i+1]
			if next == 'x' && i+3 < len(statement) && isRedisHex(statement[i+2]) && isRedisHex(statement[i+3]) {
				arg.WriteByte(fromRedisHex(statement[i+2])<<4 | fromRedisHex(statement[i+3]))
				i += 4
				continue
			}
			i += 2
			switch next {
			case 'n':
				arg.WriteByte('\n')
			case 'r':
				arg.WriteByte('\r')
			case 't':
				arg.WriteByte('\t')
			case 'b':
				arg.WriteByte('\b')
			case 'a':
				arg.WriteByte('\a')
			default:
				arg.WriteByte(next)
			}
			continue
		}
		arg.WriteByte(ch)
		i++
	}
	return 0, fmt.Errorf("invalid redis command: unterminated double quote")
}

func parseRedisSingleQuoted(statement string, i int, arg *strings.Builder) (int, error) {
	for i < len(statement) {
		ch := statement[i]
		if ch == '\'' {
			i++
			if i < len(statement) && !isRedisSpace(statement[i]) {
				return 0, fmt.Errorf("invalid redis command: closing single quote must be followed by whitespace")
			}
			return i, nil
		}
		if ch == '\\' && i+1 < len(statement) {
			switch statement[i+1] {
			case '\'':
				arg.WriteByte('\'')
				i += 2
				continue
			default:
				// Redis sdssplitargs preserves non-apostrophe backslashes.
				// Consume only this byte so a following \' can still escape.
				arg.WriteByte('\\')
				i++
				continue
			}
		}
		arg.WriteByte(ch)
		i++
	}
	return 0, fmt.Errorf("invalid redis command: unterminated single quote")
}

func isRedisSpace(ch byte) bool {
	switch ch {
	case ' ', '\n', '\r', '\t', '\v', '\f':
		return true
	default:
		return false
	}
}

func isRedisHex(ch byte) bool {
	return ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
}

func fromRedisHex(ch byte) byte {
	switch {
	case '0' <= ch && ch <= '9':
		return ch - '0'
	case 'a' <= ch && ch <= 'f':
		return ch - 'a' + 10
	default:
		return ch - 'A' + 10
	}
}
