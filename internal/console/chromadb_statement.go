package console

import (
	"errors"
	"strings"
)

type chromaDBStatement struct {
	Method string
	Path   string
	Body   string
}

func parseChromaDBStatement(statement string) (chromaDBStatement, error) {
	parsed, err := parseRESTStatement(statement)
	if err != nil {
		return chromaDBStatement{}, err
	}
	return chromaDBStatement(parsed), nil
}

func parseRESTStatement(statement string) (chromaDBStatement, error) {
	lines := strings.Split(statement, "\n")
	firstLineIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		firstLineIdx = i
		break
	}
	if firstLineIdx == -1 {
		return chromaDBStatement{}, errors.New("statement required")
	}

	first := strings.TrimSpace(lines[firstLineIdx])
	parts := strings.Fields(first)
	if len(parts) < 2 {
		return chromaDBStatement{}, errors.New("method and path are required")
	}
	method := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch method {
	case "GET", "POST", "HEAD":
	default:
		return chromaDBStatement{}, errors.New("unsupported http method")
	}

	path := strings.TrimSpace(parts[1])
	if path == "" {
		return chromaDBStatement{}, errors.New("method and path are required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	body := ""
	if firstLineIdx+1 < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[firstLineIdx+1:], "\n"))
	}
	return chromaDBStatement{Method: method, Path: path, Body: body}, nil
}
