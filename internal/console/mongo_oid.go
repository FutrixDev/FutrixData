package console

import (
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func convertMongoOIDMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	value := convertMongoOIDValue(input)
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return input
}

func convertMongoOIDSlice(input []any) []any {
	if input == nil {
		return nil
	}
	value := convertMongoOIDValue(input)
	if list, ok := value.([]any); ok {
		return list
	}
	return input
}

func convertMongoOIDValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 1 {
			if raw, ok := v["$oid"]; ok {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					if oid, err := primitive.ObjectIDFromHex(s); err == nil {
						return oid
					}
				}
			}
		}
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = convertMongoOIDValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, convertMongoOIDValue(item))
		}
		return out
	default:
		return value
	}
}
