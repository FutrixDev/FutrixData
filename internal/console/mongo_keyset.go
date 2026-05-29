package console

import (
	"errors"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
)

func mongoKeysetFilter(keys []mongoSortKey, cursor []any, direction Direction) (bson.D, error) {
	if len(keys) == 0 {
		return bson.D{}, errors.New("paging requires sort keys")
	}
	if len(cursor) == 0 {
		return bson.D{}, nil
	}
	if len(cursor) != len(keys) {
		return bson.D{}, errors.New("cursor length mismatch")
	}
	ors := make(bson.A, 0, len(keys))
	for i := range keys {
		clauses := make(bson.A, 0, i+1)
		for j := 0; j < i; j++ {
			clauses = append(clauses, bson.D{{Key: keys[j].Field, Value: cursor[j]}})
		}
		op := mongoComparisonOp(keys[i], direction)
		clauses = append(clauses, bson.D{{Key: keys[i].Field, Value: bson.D{{Key: op, Value: cursor[i]}}}})
		if len(clauses) == 1 {
			ors = append(ors, clauses[0])
		} else {
			ors = append(ors, bson.D{{Key: "$and", Value: clauses}})
		}
	}
	if len(ors) == 1 {
		return ors[0].(bson.D), nil
	}
	return bson.D{{Key: "$or", Value: ors}}, nil
}

func mongoComparisonOp(key mongoSortKey, direction Direction) string {
	if direction == DirectionPrev {
		if key.Desc {
			return "$gt"
		}
		return "$lt"
	}
	if key.Desc {
		return "$lt"
	}
	return "$gt"
}

func mongoMergeFilters(base map[string]any, keyset bson.D) bson.D {
	if len(keyset) == 0 {
		return mongoMapToDoc(base)
	}
	if base == nil || len(base) == 0 {
		return keyset
	}
	return bson.D{{Key: "$and", Value: bson.A{mongoMapToDoc(base), keyset}}}
}

func mongoMapToDoc(input map[string]any) bson.D {
	if len(input) == 0 {
		return bson.D{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	doc := make(bson.D, 0, len(keys))
	for _, key := range keys {
		doc = append(doc, bson.E{Key: key, Value: input[key]})
	}
	return doc
}
