package console

import (
	"sort"

	"go.mongodb.org/mongo-driver/bson"
)

type mongoSortKey struct {
	Field string
	Desc  bool
}

func mongoSortKeys(sortSpec any) []mongoSortKey {
	keys := mongoSortKeysFromSpec(sortSpec)
	if len(keys) == 0 {
		return []mongoSortKey{{Field: "_id", Desc: false}}
	}
	return keys
}

func mongoSortKeysFromSpec(sortSpec any) []mongoSortKey {
	switch typed := sortSpec.(type) {
	case bson.D:
		out := make([]mongoSortKey, 0, len(typed))
		for _, item := range typed {
			out = append(out, mongoSortKey{Field: item.Key, Desc: mongoSortValueDesc(item.Value)})
		}
		return out
	case bson.M:
		return mongoSortKeysFromMap(map[string]any(typed))
	case map[string]any:
		return mongoSortKeysFromMap(typed)
	case []any:
		out := make([]mongoSortKey, 0, len(typed))
		for _, item := range typed {
			if doc, ok := item.(bson.D); ok {
				out = append(out, mongoSortKeysFromSpec(doc)...)
			} else if doc, ok := item.(bson.M); ok {
				out = append(out, mongoSortKeysFromSpec(doc)...)
			} else if doc, ok := item.(map[string]any); ok {
				out = append(out, mongoSortKeysFromSpec(doc)...)
			}
		}
		return out
	default:
		return nil
	}
}

func mongoSortKeysFromMap(input map[string]any) []mongoSortKey {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]mongoSortKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, mongoSortKey{Field: key, Desc: mongoSortValueDesc(input[key])})
	}
	return out
}

func mongoSortValueDesc(value any) bool {
	switch v := value.(type) {
	case int:
		return v < 0
	case int32:
		return v < 0
	case int64:
		return v < 0
	case float64:
		return v < 0
	case float32:
		return v < 0
	case bson.D:
		for _, item := range v {
			if item.Key == "$meta" {
				return false
			}
		}
		return false
	default:
		return false
	}
}

func mongoSortFromPipeline(pipeline []any) ([]mongoSortKey, int) {
	last := -1
	var keys []mongoSortKey
	for i, stage := range pipeline {
		if value, ok := mongoStageValue(stage, "$sort"); ok {
			last = i
			keys = mongoSortKeysFromSpec(value)
		}
	}
	if last == -1 {
		return nil, -1
	}
	return keys, last
}

func mongoStageValue(stage any, name string) (any, bool) {
	switch typed := stage.(type) {
	case bson.D:
		for _, item := range typed {
			if item.Key == name {
				return item.Value, true
			}
		}
	case bson.M:
		value, ok := typed[name]
		return value, ok
	case map[string]any:
		value, ok := typed[name]
		return value, ok
	}
	return nil, false
}

func mongoSortDocument(keys []mongoSortKey, direction Direction) bson.D {
	doc := make(bson.D, 0, len(keys))
	for _, key := range keys {
		desc := key.Desc
		if direction == DirectionPrev {
			desc = !desc
		}
		value := 1
		if desc {
			value = -1
		}
		doc = append(doc, bson.E{Key: key.Field, Value: value})
	}
	return doc
}
