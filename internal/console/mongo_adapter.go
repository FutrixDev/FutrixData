package console

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"futrixdata/platform/internal/datasource"
)

type MongoAdapter struct {
	mu      sync.Mutex
	clients map[string]*mongo.Client
	byID    map[string]string
}

func NewMongoAdapter() *MongoAdapter {
	return &MongoAdapter{clients: make(map[string]*mongo.Client), byID: make(map[string]string)}
}

func (m *MongoAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	client, err := m.clientFor(ds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.Ping(ctx, readpref.Primary())
}

func (m *MongoAdapter) ListDatabases(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	client, err := m.clientFor(ds)
	if err != nil {
		return nil, err
	}
	names, err := client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	if opts.Pattern != "" {
		pattern := strings.ToLower(opts.Pattern)
		filtered := make([]string, 0, len(names))
		for _, name := range names {
			if strings.Contains(strings.ToLower(name), pattern) {
				filtered = append(filtered, name)
			}
		}
		names = filtered
	}
	if opts.Limit > 0 && len(names) > opts.Limit {
		names = names[:opts.Limit]
	}
	return names, nil
}

func (m *MongoAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, _ ListOptions) ([]string, error) {
	client, err := m.clientFor(ds)
	if err != nil {
		return nil, err
	}
	dbName, err := mongoDatabase(ds)
	if err != nil {
		return nil, err
	}
	listOpts := options.ListCollections().SetNameOnly(true).SetAuthorizedCollections(true)
	names, err := client.Database(dbName).ListCollectionNames(ctx, bson.D{}, listOpts)
	if err != nil {
		return nil, err
	}
	return mongoVisibleEntityNames(names), nil
}

func (m *MongoAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	client, err := m.clientFor(ds)
	if err != nil {
		return DescribeResult{}, err
	}
	dbName, err := mongoDatabase(ds)
	if err != nil {
		return DescribeResult{}, err
	}

	coll := client.Database(dbName).Collection(name)

	// Infer columns by sampling documents; non-fatal so we can still return indexes
	columns, _ := mongoInferColumns(ctx, coll)

	// Fetch indexes
	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		if mongoCanIgnoreDescribeEntityError(name, err) {
			return DescribeResult{Columns: columns}, nil
		}
		return DescribeResult{}, err
	}
	defer cursor.Close(ctx)

	var indexes []IndexInfo
	for cursor.Next(ctx) {
		var indexDoc bson.M
		if err := cursor.Decode(&indexDoc); err != nil {
			return DescribeResult{Columns: columns}, err
		}
		nameVal, _ := indexDoc["name"].(string)
		unique := false
		if uniqueVal, ok := indexDoc["unique"].(bool); ok {
			unique = uniqueVal
		}
		if nameVal == "_id_" {
			unique = true
		}
		var fields []string
		if keyVal, ok := indexDoc["key"]; ok {
			switch typed := keyVal.(type) {
			case bson.D:
				for _, e := range typed {
					fields = append(fields, e.Key)
				}
			case bson.M:
				for k := range typed {
					fields = append(fields, k)
				}
				sort.Strings(fields)
			case map[string]any:
				for k := range typed {
					fields = append(fields, k)
				}
				sort.Strings(fields)
			}
		}
		indexes = append(indexes, IndexInfo{Name: nameVal, Unique: unique, Column: strings.Join(fields, ",")})
	}
	if err := cursor.Err(); err != nil {
		if mongoCanIgnoreDescribeEntityError(name, err) {
			return DescribeResult{Columns: columns, Indexes: indexes}, nil
		}
		return DescribeResult{}, err
	}
	return DescribeResult{Columns: columns, Indexes: indexes}, nil
}

// mongoInferColumns samples documents from a collection to infer field names and BSON types.
// It uses zigzag sampling (first, last, first+n, last-n, ...) to cover schema changes
// across the collection's lifetime with at most 20 documents.
// It flattens nested objects using dot notation (e.g. "address.city").
func mongoInferColumns(ctx context.Context, coll *mongo.Collection) ([]ColumnInfo, error) {
	const maxDocs = 20
	const maxFields = 500

	findCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	docs := mongoZigzagSample(findCtx, coll, maxDocs)
	if len(docs) == 0 {
		return nil, nil
	}

	// Collect field → BSON type name across all sampled documents.
	// If a field has multiple types across documents, join them with "|".
	fieldTypes := make(map[string]map[string]bool)
	for _, doc := range docs {
		mongoFlattenFields("", doc, fieldTypes, maxFields)
	}

	if len(fieldTypes) == 0 {
		return nil, nil
	}

	columns := make([]ColumnInfo, 0, len(fieldTypes))
	for name, types := range fieldTypes {
		typeNames := make([]string, 0, len(types))
		for t := range types {
			typeNames = append(typeNames, t)
		}
		sort.Strings(typeNames)
		columns = append(columns, ColumnInfo{
			Name:     name,
			DataType: strings.Join(typeNames, "|"),
		})
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Name < columns[j].Name })
	return columns, nil
}

// mongoZigzagSample samples documents by alternating between the front and back
// of the collection: position 0, last, n, last-n, 2n, last-2n, ...
// This covers schema evolution across the collection's lifetime with minimal documents.
func mongoZigzagSample(ctx context.Context, coll *mongo.Collection, maxDocs int) []bson.M {
	count, err := coll.EstimatedDocumentCount(ctx)
	if err != nil || count == 0 {
		return nil
	}

	idSort := bson.D{{Key: "_id", Value: 1}}

	// Small collection — just fetch everything
	if count <= int64(maxDocs) {
		cursor, err := coll.Find(ctx, bson.D{}, options.Find().SetSort(idSort).SetLimit(int64(maxDocs)))
		if err != nil {
			return nil
		}
		defer cursor.Close(ctx)
		var docs []bson.M
		_ = cursor.All(ctx, &docs)
		return docs
	}

	// Calculate step so that maxDocs/2 rounds cover the full range
	step := count / int64(maxDocs/2)
	if step < 1 {
		step = 1
	}

	// Build zigzag skip positions: 0, count-1, step, count-1-step, ...
	var positions []int64
	seen := make(map[int64]bool)
	for i := int64(0); len(positions) < maxDocs; i++ {
		front := i * step
		back := count - 1 - i*step
		if front > back {
			break
		}
		if !seen[front] {
			positions = append(positions, front)
			seen[front] = true
		}
		if len(positions) >= maxDocs {
			break
		}
		if front != back && !seen[back] {
			positions = append(positions, back)
			seen[back] = true
		}
	}

	// Fetch one document at each position using Skip
	docs := make([]bson.M, 0, len(positions))
	for _, pos := range positions {
		var doc bson.M
		err := coll.FindOne(ctx, bson.D{},
			options.FindOne().SetSort(idSort).SetSkip(pos),
		).Decode(&doc)
		if err != nil {
			continue // skip on error, collect what we can
		}
		docs = append(docs, doc)
	}
	return docs
}

// mongoFlattenFields recursively extracts field paths and their BSON type names from a document.
// Keys are sorted before iteration so the maxFields cutoff is deterministic across runs.
func mongoFlattenFields(prefix string, doc bson.M, out map[string]map[string]bool, maxFields int) {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if len(out) >= maxFields {
			return
		}
		val := doc[key]
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		typeName := mongoBSONTypeName(val)
		if out[fullKey] == nil {
			out[fullKey] = make(map[string]bool)
		}
		out[fullKey][typeName] = true

		// Recurse into nested objects (but not arrays — arrays are treated as a single field)
		if nested, ok := val.(bson.M); ok {
			mongoFlattenFields(fullKey, nested, out, maxFields)
		}
	}
}

// mongoBSONTypeName returns a human-readable type name for a BSON value.
func mongoBSONTypeName(val any) string {
	switch val.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int32:
		return "int32"
	case int64:
		return "int64"
	case float64:
		return "double"
	case string:
		return "string"
	case bson.M:
		return "object"
	case bson.A:
		return "array"
	default:
		// Covers primitive.ObjectID → "objectId", primitive.DateTime → "date", etc.
		t := fmt.Sprintf("%T", val)
		// Strip package prefix for readability (e.g. "primitive.ObjectID" → "objectId")
		if idx := strings.LastIndex(t, "."); idx >= 0 {
			t = t[idx+1:]
		}
		return strings.ToLower(t[:1]) + t[1:]
	}
}

func mongoVisibleEntityNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || mongoIsHiddenSystemEntity(trimmed) {
			continue
		}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func mongoCanIgnoreDescribeEntityError(name string, err error) bool {
	return mongoIsListIndexesUnauthorizedError(err)
}

func mongoIsHiddenSystemEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "system.")
}

func mongoIsListIndexesUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	var commandErr mongo.CommandError
	if errors.As(err, &commandErr) {
		msg := strings.ToLower(strings.TrimSpace(commandErr.Message))
		if commandErr.Code == 13 && strings.Contains(msg, "listindexes") {
			return true
		}
		return strings.Contains(msg, "not authorized") && strings.Contains(msg, "listindexes")
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(lower, "not authorized") && strings.Contains(lower, "listindexes")
}
