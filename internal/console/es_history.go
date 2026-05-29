package console

import (
	"strings"
)

func ParseElasticsearchTargets(statement string) ([]string, error) {
	stmt, err := parseElasticsearchStatement(statement)
	if err != nil {
		return nil, err
	}
	return extractElasticsearchTargetsFromPath(stmt.Path), nil
}

func extractElasticsearchTargetsFromPath(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return nil
	}

	segment := trimmed
	if idx := strings.Index(segment, "/"); idx >= 0 {
		segment = segment[:idx]
	}
	segment = strings.TrimSpace(segment)
	if segment == "" || strings.HasPrefix(segment, "_") {
		return nil
	}

	parts := strings.Split(segment, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" || strings.HasPrefix(value, "_") {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
