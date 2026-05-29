package console

import (
	"encoding/json"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

func mongoCursorValues(doc map[string]any, keys []mongoSortKey) ([]any, error) {
	if len(keys) == 0 {
		return nil, errors.New("missing sort keys")
	}
	values := make([]any, 0, len(keys))
	for _, key := range keys {
		value, ok := mongoLookupValue(doc, key.Field)
		if !ok {
			return nil, errors.New("missing cursor value")
		}
		values = append(values, value)
	}
	return values, nil
}

func mongoLookupValue(doc map[string]any, path string) (any, bool) {
	current := any(doc)
	parts := strings.Split(path, ".")
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = value
		case bson.M:
			value, ok := typed[part]
			if !ok {
				return nil, false
			}
			current = value
		case bson.D:
			found := false
			for _, item := range typed {
				if item.Key == part {
					current = item.Value
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return current, true
}

func mongoEncodeCursorValues(values []any) ([]any, error) {
	out := make([]any, 0, len(values))
	for _, value := range values {
		wrapper := bson.D{{Key: "v", Value: value}}
		rawWrapper, err := bson.MarshalExtJSON(wrapper, false, false)
		if err != nil {
			return nil, err
		}

		var wrapperMap map[string]json.RawMessage
		if err := json.Unmarshal(rawWrapper, &wrapperMap); err != nil {
			return nil, err
		}

		val, ok := wrapperMap["v"]
		if !ok {
			return nil, errors.New("failed to extract value from wrapper")
		}
		out = append(out, val)
	}
	return out, nil
}

func mongoDecodeCursorValues(values []any) ([]any, error) {
	out := make([]any, 0, len(values))
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		wrapped := []byte(`{"v":` + string(raw) + `}`)
		var wrapper struct {
			V any `bson:"v"`
		}
		if err := bson.UnmarshalExtJSON(wrapped, false, &wrapper); err != nil {
			return nil, err
		}
		out = append(out, wrapper.V)
	}
	return out, nil
}
