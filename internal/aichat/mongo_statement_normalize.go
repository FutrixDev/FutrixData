package aichat

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

func normalizeMongoStatementForTool(statement string) (string, bool, error) {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return "", false, errors.New("mongo statement is required")
	}
	if !strings.HasPrefix(trimmed, "map[") {
		return statement, false, nil
	}

	parsed, err := parseGoStyleValue(trimmed)
	if err != nil {
		return "", false, err
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return "", false, errors.New("mongo statement must be a Go map format object")
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

type goStyleParser struct {
	s   string
	pos int
}

func parseGoStyleValue(value string) (any, error) {
	p := &goStyleParser{s: strings.TrimSpace(value)}
	p.skipSpaces()
	if strings.HasPrefix(p.s[p.pos:], "map[") {
		return p.parseMap()
	}
	if p.peek() == '[' {
		return p.parseArray()
	}
	return p.parseToken()
}

func (p *goStyleParser) skipSpaces() {
	for p.pos < len(p.s) {
		ch := p.s[p.pos]
		if ch != ' ' && ch != '\n' && ch != '\r' && ch != '\t' {
			return
		}
		p.pos++
	}
}

func (p *goStyleParser) peek() byte {
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

func (p *goStyleParser) consumePrefix(prefix string) bool {
	if strings.HasPrefix(p.s[p.pos:], prefix) {
		p.pos += len(prefix)
		return true
	}
	return false
}

func (p *goStyleParser) parseMap() (map[string]any, error) {
	if !p.consumePrefix("map[") {
		return nil, errors.New("expected map[")
	}
	out := make(map[string]any)
	for {
		p.skipSpaces()
		if p.pos >= len(p.s) {
			return nil, errors.New("unterminated map")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}

		key := p.readUntil(':')
		if key == "" {
			return nil, errors.New("missing map key")
		}
		if p.pos >= len(p.s) || p.peek() != ':' {
			return nil, errors.New("expected ':' after map key")
		}
		p.pos++

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[key] = val

		p.skipSpaces()
		if p.pos >= len(p.s) {
			return nil, errors.New("unterminated map")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}
	}
	return out, nil
}

func (p *goStyleParser) parseArray() ([]any, error) {
	if p.peek() != '[' {
		return nil, errors.New("expected '['")
	}
	p.pos++

	var out []any
	for {
		p.skipSpaces()
		if p.pos >= len(p.s) {
			return nil, errors.New("unterminated array")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, val)

		p.skipSpaces()
		if p.pos >= len(p.s) {
			return nil, errors.New("unterminated array")
		}
		if p.peek() == ']' {
			p.pos++
			break
		}
	}
	return out, nil
}

func (p *goStyleParser) parseValue() (any, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return nil, errors.New("missing value")
	}
	if strings.HasPrefix(p.s[p.pos:], "map[") {
		return p.parseMap()
	}
	if p.peek() == '[' {
		return p.parseArray()
	}
	return p.parseToken()
}

func (p *goStyleParser) parseToken() (any, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return nil, errors.New("missing token")
	}
	start := p.pos
	for p.pos < len(p.s) {
		ch := p.s[p.pos]
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == ']' {
			break
		}
		p.pos++
	}
	token := strings.TrimSpace(p.s[start:p.pos])
	if token == "" {
		return nil, errors.New("missing token")
	}

	switch token {
	case "<nil>", "nil", "null":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}

	if i, err := strconv.ParseInt(token, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(token, 64); err == nil {
		return f, nil
	}
	return token, nil
}

func (p *goStyleParser) readUntil(delim byte) string {
	start := p.pos
	for p.pos < len(p.s) {
		ch := p.s[p.pos]
		if ch == delim || ch == ']' || ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			break
		}
		p.pos++
	}
	return strings.TrimSpace(p.s[start:p.pos])
}
