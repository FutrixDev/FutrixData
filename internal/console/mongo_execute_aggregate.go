package console

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"futrixdata/platform/internal/console/paging"
	"futrixdata/platform/internal/console/window"
	"futrixdata/platform/internal/datasource"
)

func (m *MongoAdapter) executeAggregatePaged(ctx context.Context, collection *mongo.Collection, ds datasource.DataSource, statement string, stmt mongoStatement, execOpts ExecuteOptions, start time.Time) (QueryResult, error) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	if execOpts.PagingToken != "" {
		return m.executeAggregateToken(ctx, collection, ds, statement, stmt, execOpts, policy, start)
	}

	pageSize := clampPageSize(execOpts.PageSize, policy)
	limit, limitFound := mongoPipelineLimit(stmt.Pipeline)
	totalLimit := resolveTotalLimit(limitFound, limit, policy)

	sortKeys, sortIndex := mongoSortFromPipeline(stmt.Pipeline)
	if len(sortKeys) == 0 {
		sortKeys = mongoSortKeys(nil)
	}

	fetchLimit := mongoFetchLimit(pageSize, totalLimit, 0)
	if fetchLimit == 0 {
		result := QueryResult{Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}
	pipeline := mongoApplyAggregatePaging(stmt.Pipeline, sortKeys, sortIndex, bson.D{}, DirectionNext, fetchLimit)
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return QueryResult{}, err
	}
	defer cursor.Close(ctx)
	win := window.NewRowWindow(pageSize)
	for cursor.Next(ctx) {
		var doc map[string]any
		if err := cursor.Decode(&doc); err != nil {
			return QueryResult{}, err
		}
		if !win.Push(doc) {
			break
		}
	}
	if err := cursor.Err(); err != nil {
		return QueryResult{}, err
	}
	data := win.Rows()
	hasMore := win.HasMore()
	nextToken := ""
	if len(data) > 0 {
		startCursor, err := mongoCursorValues(data[0], sortKeys)
		if err != nil {
			return QueryResult{}, err
		}
		endCursor, err := mongoCursorValues(data[len(data)-1], sortKeys)
		if err != nil {
			return QueryResult{}, err
		}
		nextOffset := pagingNextOffset(0, len(data))
		allowNext := hasMore
		if totalLimit > 0 && nextOffset >= totalLimit {
			allowNext = false
		}
		if allowNext {
			nextToken, err = mongoBuildPagingToken(ds.ID, statement, pageSize, sortKeys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
			if err != nil {
				return QueryResult{}, err
			}
		}
	}
	result := QueryResult{
		Rows:      data,
		RowCount:  int64(len(data)),
		HasMore:   hasMore,
		NextToken: nextToken,
		ElapsedMs: time.Since(start).Milliseconds(),
	}
	effective, source := pageWindowLimitMetadata(pageSize, totalLimit, 0, sqlPageSizeLimitSource(execOpts.PageSize))
	setQueryLimitMetadata(&result, effective, source)
	return result, nil
}

func (m *MongoAdapter) executeAggregateToken(ctx context.Context, collection *mongo.Collection, ds datasource.DataSource, statement string, stmt mongoStatement, execOpts ExecuteOptions, policy window.LimitPolicy, start time.Time) (QueryResult, error) {
	token, err := paging.Decode(execOpts.PagingToken)
	if err != nil {
		return QueryResult{}, err
	}
	if token.DatasourceID != "" && token.DatasourceID != ds.ID {
		return QueryResult{}, errors.New("paging token datasource mismatch")
	}
	if token.QueryHash != "" && token.QueryHash != pagingQueryHash(statement) {
		return QueryResult{}, errors.New("paging token query mismatch")
	}
	sortKeys := mongoSortKeysFromToken(token.Sort)
	if len(sortKeys) == 0 {
		return QueryResult{}, errors.New("paging token missing sort keys")
	}
	pageSize := clampPageSize(token.PageSize, policy)
	totalLimit := token.Limit
	offset := token.Offset
	cursorValues := token.EndCursor
	if token.Direction == DirectionPrev {
		cursorValues = token.StartCursor
	}
	decodedCursor, err := mongoDecodeCursorValues(cursorValues)
	if err != nil {
		return QueryResult{}, err
	}
	keyset, err := mongoKeysetFilter(sortKeys, decodedCursor, token.Direction)
	if err != nil {
		return QueryResult{}, err
	}
	fetchLimit := mongoFetchLimit(pageSize, totalLimit, offset)
	if fetchLimit == 0 {
		result := QueryResult{Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}
	_, sortIndex := mongoSortFromPipeline(stmt.Pipeline)
	pipeline := mongoApplyAggregatePaging(stmt.Pipeline, sortKeys, sortIndex, keyset, token.Direction, fetchLimit)
	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return QueryResult{}, err
	}
	defer cursor.Close(ctx)
	win := window.NewRowWindow(pageSize)
	for cursor.Next(ctx) {
		var doc map[string]any
		if err := cursor.Decode(&doc); err != nil {
			return QueryResult{}, err
		}
		if !win.Push(doc) {
			break
		}
	}
	if err := cursor.Err(); err != nil {
		return QueryResult{}, err
	}
	data := win.Rows()
	hasMore := win.HasMore()
	if token.Direction == DirectionPrev {
		reverseRows(data)
	}
	nextToken := ""
	prevToken := ""
	if len(data) > 0 {
		startCursor, err := mongoCursorValues(data[0], sortKeys)
		if err != nil {
			return QueryResult{}, err
		}
		endCursor, err := mongoCursorValues(data[len(data)-1], sortKeys)
		if err != nil {
			return QueryResult{}, err
		}
		nextOffset := pagingNextOffset(offset, len(data))
		prevOffset := pagingPrevOffset(offset, pageSize)
		hasForward := totalLimit == 0 || nextOffset < totalLimit
		if token.Direction == DirectionPrev {
			if hasMore && offset > 0 {
				prevToken, err = mongoBuildPagingToken(ds.ID, statement, pageSize, sortKeys, startCursor, endCursor, DirectionPrev, totalLimit, prevOffset)
				if err != nil {
					return QueryResult{}, err
				}
			}
			if hasForward {
				nextToken, err = mongoBuildPagingToken(ds.ID, statement, pageSize, sortKeys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
				if err != nil {
					return QueryResult{}, err
				}
			}
		} else {
			if hasMore && hasForward {
				nextToken, err = mongoBuildPagingToken(ds.ID, statement, pageSize, sortKeys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
				if err != nil {
					return QueryResult{}, err
				}
			}
			if offset > 0 {
				prevToken, err = mongoBuildPagingToken(ds.ID, statement, pageSize, sortKeys, startCursor, endCursor, DirectionPrev, totalLimit, prevOffset)
				if err != nil {
					return QueryResult{}, err
				}
			}
		}
	}
	result := QueryResult{
		Rows:      data,
		RowCount:  int64(len(data)),
		HasMore:   hasMore,
		NextToken: nextToken,
		PrevToken: prevToken,
		ElapsedMs: time.Since(start).Milliseconds(),
	}
	effective, source := pageWindowLimitMetadata(pageSize, totalLimit, offset, EffectiveLimitPagingToken)
	setQueryLimitMetadata(&result, effective, source)
	return result, nil
}

func mongoApplyAggregatePaging(pipeline []any, sortKeys []mongoSortKey, sortIndex int, keyset bson.D, direction Direction, fetchLimit int) []any {
	out := make([]any, 0, len(pipeline)+3)
	out = append(out, pipeline...)

	if len(keyset) > 0 {
		insertAt := sortIndex
		if insertAt < 0 {
			insertAt = len(out)
		}
		out = append(out[:insertAt], append([]any{bson.D{{Key: "$match", Value: keyset}}}, out[insertAt:]...)...)
		if sortIndex >= 0 {
			sortIndex++
		}
	}

	sortStage := bson.D{{Key: "$sort", Value: mongoSortDocument(sortKeys, direction)}}
	if sortIndex >= 0 && sortIndex < len(out) {
		out[sortIndex] = sortStage
	} else {
		out = append(out, sortStage)
	}

	if fetchLimit > 0 {
		out = append(out, bson.D{{Key: "$limit", Value: fetchLimit}})
	}
	return out
}
