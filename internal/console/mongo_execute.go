package console

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"futrixdata/platform/internal/datasource"
)

func (m *MongoAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	client, err := m.clientFor(ds)
	if err != nil {
		return QueryResult{}, err
	}
	stmt, err := parseMongoStatement(statement)
	if err != nil {
		return QueryResult{}, err
	}
	dbName, err := mongoStatementDatabase(ds, stmt)
	if err != nil {
		return QueryResult{}, err
	}
	stmt.Filter = convertMongoOIDMap(stmt.Filter)
	stmt.Pipeline = convertMongoOIDSlice(stmt.Pipeline)
	stmt.Document = convertMongoOIDValue(stmt.Document)
	stmt.Update = convertMongoOIDValue(stmt.Update)
	stmt.Options = convertMongoOIDMap(stmt.Options)

	collection := client.Database(dbName).Collection(stmt.Collection)
	start := time.Now()

	var result QueryResult
	var execErr error

	switch stmt.Action {
	case "find":
		result, execErr = m.executeFindPaged(ctx, collection, ds, statement, stmt, opts, start)
	case "aggregate":
		result, execErr = m.executeAggregatePaged(ctx, collection, ds, statement, stmt, opts, start)
	case "insertone":
		if stmt.Document == nil {
			return QueryResult{}, errors.New("document is required")
		}
		if _, err := collection.InsertOne(ctx, stmt.Document); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "insertmany":
		documents, ok := stmt.Document.([]any)
		if !ok || len(documents) == 0 {
			return QueryResult{}, errors.New("documents array is required")
		}
		if _, err := collection.InsertMany(ctx, documents); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: int64(len(documents)), ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "updateone":
		if stmt.Update == nil {
			return QueryResult{}, errors.New("update is required")
		}
		res, err := collection.UpdateOne(ctx, stmt.Filter, stmt.Update)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: res.ModifiedCount, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "updatemany":
		if stmt.Update == nil {
			return QueryResult{}, errors.New("update is required")
		}
		res, err := collection.UpdateMany(ctx, stmt.Filter, stmt.Update)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: res.ModifiedCount, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "deleteone":
		res, err := collection.DeleteOne(ctx, stmt.Filter)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: res.DeletedCount, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "deletemany":
		res, err := collection.DeleteMany(ctx, stmt.Filter)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: res.DeletedCount, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "createcollection":
		if err := client.Database(dbName).CreateCollection(ctx, stmt.Collection); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "drop":
		if stmt.Collection == "" {
			return QueryResult{}, errors.New("collection is required")
		}
		if err := collection.Drop(ctx); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "createuser":
		doc, ok := stmt.Document.(map[string]any)
		if !ok {
			return QueryResult{}, errors.New("user document is required")
		}
		username, ok := doc["user"].(string)
		if !ok || strings.TrimSpace(username) == "" {
			return QueryResult{}, errors.New("user is required")
		}
		command := bson.D{{Key: "createUser", Value: username}}
		keys := make([]string, 0, len(doc))
		for key := range doc {
			if key == "user" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			command = append(command, bson.E{Key: key, Value: doc[key]})
		}
		if err := client.Database(dbName).RunCommand(ctx, command).Err(); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "getusers":
		commandValue := any(int32(1))
		if len(stmt.Options) > 0 {
			commandValue = stmt.Options
		}
		var result bson.M
		command := bson.D{{Key: "usersInfo", Value: commandValue}}
		if err := client.Database(dbName).RunCommand(ctx, command).Decode(&result); err != nil {
			return QueryResult{}, err
		}
		rows := make([]map[string]any, 0)
		if rawUsers, ok := result["users"]; ok {
			switch users := rawUsers.(type) {
			case bson.A:
				for _, item := range users {
					if doc, ok := item.(bson.M); ok {
						rows = append(rows, map[string]any(doc))
					} else if doc, ok := item.(map[string]any); ok {
						rows = append(rows, doc)
					}
				}
			case []any:
				for _, item := range users {
					if doc, ok := item.(bson.M); ok {
						rows = append(rows, map[string]any(doc))
					} else if doc, ok := item.(map[string]any); ok {
						rows = append(rows, doc)
					}
				}
			}
		}
		return QueryResult{Rows: rows, RowCount: int64(len(rows)), ElapsedMs: time.Since(start).Milliseconds()}, nil
	case "serverstatus":
		var status bson.M
		if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&status); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{
			Rows:      []map[string]any{map[string]any(status)},
			RowCount:  1,
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	case "createindex":
		if len(stmt.Keys) == 0 {
			return QueryResult{}, errors.New("keys is required for createIndex")
		}
		keys := mongoKeysToBsonD(stmt.Keys)
		opts := options.Index()
		applyIndexOptions(opts, stmt.Options)
		name, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: opts})
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{
			Columns:   []string{"name"},
			Rows:      []map[string]any{{"name": name}},
			RowCount:  1,
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	case "dropindex":
		name, _ := stmt.Options["name"].(string)
		if name == "" {
			return QueryResult{}, errors.New("index name is required")
		}
		if _, err := collection.Indexes().DropOne(ctx, name); err != nil {
			return QueryResult{}, err
		}
		return QueryResult{RowCount: 1, ElapsedMs: time.Since(start).Milliseconds()}, nil
	default:
		return QueryResult{}, ErrUnsupported
	}

	if execErr != nil {
		return result, execErr
	}
	// Only set SourceEntity when we can be confident the result comes from
	// a single collection. Aggregates with $lookup/$unionWith/$merge pull
	// from multiple collections, so we leave SourceEntity empty and let the
	// masking processor skip or use a conservative fallback.
	if stmt.Action == "find" || stmt.Action == "count" {
		result.SourceEntity = stmt.Collection
	} else if stmt.Action == "aggregate" && !mongoAggregateJoinsOtherCollections(stmt.Pipeline) {
		result.SourceEntity = stmt.Collection
	}
	return result, nil
}

// mongoAggregateJoinsOtherCollections returns true if the pipeline contains
// stages that pull data from other collections ($lookup, $unionWith, $merge,
// $out), including when nested inside $facet or other compound stages.
func mongoAggregateJoinsOtherCollections(pipeline []any) bool {
	for _, stage := range pipeline {
		if mongoValueContainsJoinStage(stage) {
			return true
		}
	}
	return false
}

var mongoJoinStageKeys = map[string]bool{
	"$lookup": true, "$unionWith": true, "$merge": true, "$out": true,
}

// mongoValueContainsJoinStage recursively checks if a value contains any
// join-related stage key, handling nested pipelines inside $facet etc.
func mongoValueContainsJoinStage(v any) bool {
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			if mongoJoinStageKeys[key] {
				return true
			}
			if mongoValueContainsJoinStage(child) {
				return true
			}
		}
	case []any:
		for _, item := range val {
			if mongoValueContainsJoinStage(item) {
				return true
			}
		}
	}
	return false
}
