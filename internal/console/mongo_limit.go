package console

import (
	"go.mongodb.org/mongo-driver/bson"

	"futrixdata/platform/internal/console/window"
)

func mongoPipelineLimit(pipeline []any) (int64, bool) {
	var found bool
	var limit int64
	for _, stage := range pipeline {
		if value, ok := mongoLimitValue(stage); ok {
			found = true
			limit = value
		}
	}
	if !found {
		return 0, false
	}
	return limit, true
}

func mongoFindLimit(stmt mongoStatement) (int64, bool) {
	if stmt.Limit > 0 {
		return stmt.Limit, true
	}
	if stmt.Options == nil {
		return 0, false
	}
	return int64From(stmt.Options["limit"])
}

func mongoLimitValue(stage any) (int64, bool) {
	switch typed := stage.(type) {
	case map[string]any:
		if value, ok := typed["$limit"]; ok {
			return int64From(value)
		}
	case bson.M:
		if value, ok := typed["$limit"]; ok {
			return int64From(value)
		}
	case bson.D:
		for _, item := range typed {
			if item.Key == "$limit" {
				return int64From(item.Value)
			}
		}
	}
	return 0, false
}

func applyMongoAggregateLimit(pipeline []any, decision window.Decision) []any {
	if !decision.Enforced {
		return pipeline
	}
	return append(pipeline, map[string]any{"$limit": decision.Fetch})
}
