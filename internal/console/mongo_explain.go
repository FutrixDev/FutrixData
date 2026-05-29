package console

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"

	"futrixdata/platform/internal/datasource"
)

func (m *MongoAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	client, err := m.clientFor(ds)
	if err != nil {
		return ExplainResult{}, err
	}
	stmt, err := parseMongoStatement(statement)
	if err != nil {
		return ExplainResult{}, err
	}
	dbName, err := mongoStatementDatabase(ds, stmt)
	if err != nil {
		return ExplainResult{}, err
	}
	stmt.Filter = convertMongoOIDMap(stmt.Filter)
	stmt.Pipeline = convertMongoOIDSlice(stmt.Pipeline)
	stmt.Update = convertMongoOIDValue(stmt.Update)
	stmt.Options = convertMongoOIDMap(stmt.Options)

	var inner bson.D
	switch stmt.Action {
	case "find":
		findCmd := bson.D{{Key: "find", Value: stmt.Collection}, {Key: "filter", Value: stmt.Filter}}
		if stmt.Options != nil {
			if sort, ok := stmt.Options["sort"]; ok {
				findCmd = append(findCmd, bson.E{Key: "sort", Value: sort})
			}
			if projection, ok := stmt.Options["projection"]; ok {
				findCmd = append(findCmd, bson.E{Key: "projection", Value: projection})
			}
			if skip, ok := int64From(stmt.Options["skip"]); ok {
				findCmd = append(findCmd, bson.E{Key: "skip", Value: skip})
			}
			if limit, ok := int64From(stmt.Options["limit"]); ok {
				findCmd = append(findCmd, bson.E{Key: "limit", Value: limit})
			} else if stmt.Limit > 0 {
				findCmd = append(findCmd, bson.E{Key: "limit", Value: stmt.Limit})
			}
		} else if stmt.Limit > 0 {
			findCmd = append(findCmd, bson.E{Key: "limit", Value: stmt.Limit})
		}
		inner = findCmd
	case "aggregate":
		inner = bson.D{{Key: "aggregate", Value: stmt.Collection}, {Key: "pipeline", Value: stmt.Pipeline}, {Key: "cursor", Value: bson.M{}}}
	case "updateone", "updatemany", "replaceone":
		if stmt.Update == nil {
			return ExplainResult{}, errors.New("update is required")
		}
		updateDoc := bson.D{{Key: "q", Value: stmt.Filter}, {Key: "u", Value: stmt.Update}, {Key: "multi", Value: stmt.Action == "updatemany"}}
		if stmt.Options != nil {
			if upsert, ok := stmt.Options["upsert"].(bool); ok {
				updateDoc = append(updateDoc, bson.E{Key: "upsert", Value: upsert})
			}
			if hint, ok := stmt.Options["hint"]; ok {
				updateDoc = append(updateDoc, bson.E{Key: "hint", Value: hint})
			}
		}
		inner = bson.D{{Key: "update", Value: stmt.Collection}, {Key: "updates", Value: bson.A{updateDoc}}}
	case "deleteone", "deletemany":
		delDoc := bson.D{{Key: "q", Value: stmt.Filter}}
		if stmt.Action == "deleteone" {
			delDoc = append(delDoc, bson.E{Key: "limit", Value: int32(1)})
		} else {
			delDoc = append(delDoc, bson.E{Key: "limit", Value: int32(0)})
		}
		if stmt.Options != nil {
			if hint, ok := stmt.Options["hint"]; ok {
				delDoc = append(delDoc, bson.E{Key: "hint", Value: hint})
			}
		}
		inner = bson.D{{Key: "delete", Value: stmt.Collection}, {Key: "deletes", Value: bson.A{delDoc}}}
	case "findoneandupdate", "findoneandreplace":
		if stmt.Update == nil {
			return ExplainResult{}, errors.New("update is required")
		}
		cmd := bson.D{{Key: "findAndModify", Value: stmt.Collection}, {Key: "query", Value: stmt.Filter}, {Key: "update", Value: stmt.Update}}
		if stmt.Options != nil {
			if sort, ok := stmt.Options["sort"]; ok {
				cmd = append(cmd, bson.E{Key: "sort", Value: sort})
			}
			if upsert, ok := stmt.Options["upsert"].(bool); ok {
				cmd = append(cmd, bson.E{Key: "upsert", Value: upsert})
			}
			if hint, ok := stmt.Options["hint"]; ok {
				cmd = append(cmd, bson.E{Key: "hint", Value: hint})
			}
		}
		inner = cmd
	case "findoneanddelete":
		cmd := bson.D{{Key: "findAndModify", Value: stmt.Collection}, {Key: "query", Value: stmt.Filter}, {Key: "remove", Value: true}}
		if stmt.Options != nil {
			if sort, ok := stmt.Options["sort"]; ok {
				cmd = append(cmd, bson.E{Key: "sort", Value: sort})
			}
			if hint, ok := stmt.Options["hint"]; ok {
				cmd = append(cmd, bson.E{Key: "hint", Value: hint})
			}
		}
		inner = cmd
	default:
		return ExplainResult{}, ErrUnsupported
	}

	var result bson.M
	cmd := bson.D{{Key: "explain", Value: inner}, {Key: "verbosity", Value: "executionStats"}}
	if err := client.Database(dbName).RunCommand(ctx, cmd).Decode(&result); err != nil {
		return ExplainResult{}, err
	}

	indexes := uniqueStrings(findExplainStringValues(result, "indexName"))
	totalKeys := maxInt64(findExplainIntValues(result, "totalKeysExamined"))
	totalDocs := maxInt64(findExplainIntValues(result, "totalDocsExamined"))
	usesIndex := len(indexes) > 0 || totalKeys > 0
	summary := MongoExplainPlanSummaryForResult(ExplainResult{
		Stages: uniqueStrings(findExplainStringValues(result, "stage")),
		Detail: result,
	})

	return ExplainResult{
		UsesIndex:         usesIndex,
		Indexes:           indexes,
		Stages:            summary.Stages,
		TotalKeysExamined: totalKeys,
		TotalDocsExamined: totalDocs,
		Detail:            result,
	}, nil
}
