package console

import (
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func mongoKeysToBsonD(values map[string]any) bson.D {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	doc := make(bson.D, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		switch v := value.(type) {
		case float64:
			if v == float64(int64(v)) {
				value = int32(v)
			}
		}
		doc = append(doc, bson.E{Key: key, Value: value})
	}
	return doc
}

func applyIndexOptions(opts *options.IndexOptions, values map[string]any) {
	if values == nil {
		return
	}
	if unique, ok := values["unique"].(bool); ok {
		opts.SetUnique(unique)
	}
	if name, ok := values["name"].(string); ok && name != "" {
		opts.SetName(name)
	}
	if sparse, ok := values["sparse"].(bool); ok {
		opts.SetSparse(sparse)
	}
}
