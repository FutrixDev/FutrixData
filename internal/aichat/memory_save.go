package aichat

import (
	"fmt"
	"strings"
)

func memorySaveInputFromToolArgs(args map[string]any) MemorySaveInput {
	return MemorySaveInput{
		Problem:          strings.TrimSpace(stringArg(args, "problem")),
		Signals:          stringListArg(args, "signals"),
		Avoid:            stringListArg(args, "avoid"),
		Do:               stringListArg(args, "do"),
		Why:              strings.TrimSpace(stringArg(args, "why")),
		Confidence:       floatArg(args, "confidence", 0.7),
		EvidenceEventIDs: stringListArg(args, "evidenceEventIds"),
		ReplaceHints:     stringListArg(args, "replaceHints"),
	}
}

func stringListArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(anyToString(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		if trimmed := strings.TrimSpace(anyToString(typed)); trimmed != "" {
			return []string{trimmed}
		}
	}
	return nil
}

func floatArg(args map[string]any, key string, fallback float64) float64 {
	if args == nil {
		return fallback
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		value := strings.TrimSpace(anyToString(raw))
		if value == "" {
			return fallback
		}
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}
