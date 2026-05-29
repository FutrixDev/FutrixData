package console

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

func findExplainStringValues(value any, key string) []string {
	switch v := value.(type) {
	case bson.M:
		var out []string
		for k, item := range v {
			if k == key {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			out = append(out, findExplainStringValues(item, key)...)
		}
		return out
	case map[string]any:
		var out []string
		for k, item := range v {
			if k == key {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			out = append(out, findExplainStringValues(item, key)...)
		}
		return out
	case bson.D:
		var out []string
		for _, e := range v {
			if e.Key == key {
				if s, ok := e.Value.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			out = append(out, findExplainStringValues(e.Value, key)...)
		}
		return out
	case bson.A:
		var out []string
		for _, item := range v {
			out = append(out, findExplainStringValues(item, key)...)
		}
		return out
	case []bson.E:
		var out []string
		for _, e := range v {
			if e.Key == key {
				if s, ok := e.Value.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			out = append(out, findExplainStringValues(e.Value, key)...)
		}
		return out
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, findExplainStringValues(item, key)...)
		}
		return out
	default:
		return nil
	}
}

func findExplainIntValues(value any, key string) []int64 {
	switch v := value.(type) {
	case bson.M:
		var out []int64
		for k, item := range v {
			if k == key {
				if n, ok := int64From(item); ok {
					out = append(out, n)
				}
			}
			out = append(out, findExplainIntValues(item, key)...)
		}
		return out
	case map[string]any:
		var out []int64
		for k, item := range v {
			if k == key {
				if n, ok := int64From(item); ok {
					out = append(out, n)
				}
			}
			out = append(out, findExplainIntValues(item, key)...)
		}
		return out
	case bson.D:
		var out []int64
		for _, e := range v {
			if e.Key == key {
				if n, ok := int64From(e.Value); ok {
					out = append(out, n)
				}
			}
			out = append(out, findExplainIntValues(e.Value, key)...)
		}
		return out
	case bson.A:
		var out []int64
		for _, item := range v {
			out = append(out, findExplainIntValues(item, key)...)
		}
		return out
	case []bson.E:
		var out []int64
		for _, e := range v {
			if e.Key == key {
				if n, ok := int64From(e.Value); ok {
					out = append(out, n)
				}
			}
			out = append(out, findExplainIntValues(e.Value, key)...)
		}
		return out
	case []any:
		var out []int64
		for _, item := range v {
			out = append(out, findExplainIntValues(item, key)...)
		}
		return out
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	var out []string
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func maxInt64(values []int64) int64 {
	var max int64
	for i, v := range values {
		if i == 0 || v > max {
			max = v
		}
	}
	return max
}
