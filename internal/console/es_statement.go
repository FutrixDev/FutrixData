package console

import (
	"errors"
	"strings"
)

type elasticsearchStatement struct {
	Method string
	Path   string
	Body   string
}

func parseElasticsearchStatement(statement string) (elasticsearchStatement, error) {
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
		return elasticsearchStatement{}, errors.New("statement required")
	}

	first := strings.TrimSpace(lines[firstLineIdx])
	parts := strings.Fields(first)
	if len(parts) < 2 {
		return elasticsearchStatement{}, errors.New("method and path are required")
	}
	method := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch method {
	case "GET", "POST", "PUT", "DELETE", "HEAD", "PATCH":
	default:
		return elasticsearchStatement{}, errors.New("unsupported http method")
	}

	path := strings.TrimSpace(parts[1])
	if path == "" {
		return elasticsearchStatement{}, errors.New("method and path are required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	body := ""
	if firstLineIdx+1 < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[firstLineIdx+1:], "\n"))
	}

	return elasticsearchStatement{
		Method: method,
		Path:   path,
		Body:   body,
	}, nil
}
