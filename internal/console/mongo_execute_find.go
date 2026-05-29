package console

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"futrixdata/platform/internal/console/paging"
	"futrixdata/platform/internal/console/window"
	"futrixdata/platform/internal/datasource"
)

func (m *MongoAdapter) executeFindPaged(ctx context.Context, collection *mongo.Collection, ds datasource.DataSource, statement string, stmt mongoStatement, execOpts ExecuteOptions, start time.Time) (QueryResult, error) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	if execOpts.PagingToken != "" {
		return m.executeFindToken(ctx, collection, ds, statement, stmt, execOpts, policy, start)
	}

	pageSize := clampPageSize(execOpts.PageSize, policy)
	if stmt.Options != nil {
		if skip, ok := int64From(stmt.Options["skip"]); ok && skip > 0 {
			return m.executeFindLegacy(ctx, collection, stmt, policy, start)
		}
	}

	limit, limitFound := mongoFindLimit(stmt)
	totalLimit := resolveTotalLimit(limitFound, limit, policy)
	sortKeys := mongoSortKeys(nil)
	if stmt.Options != nil {
		if sort, ok := stmt.Options["sort"]; ok {
			sortKeys = mongoSortKeys(sort)
		}
	}

	fetchLimit := mongoFetchLimit(pageSize, totalLimit, 0)
	if fetchLimit == 0 {
		result := QueryResult{Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}
	findOpts := options.Find()
	if stmt.Options != nil {
		if projection, ok := stmt.Options["projection"]; ok {
			findOpts.SetProjection(projection)
		}
	}
	findOpts.SetSort(mongoSortDocument(sortKeys, DirectionNext))
	findOpts.SetLimit(int64(fetchLimit))

	filter := mongoMergeFilters(stmt.Filter, bson.D{})
	cursor, err := collection.Find(ctx, filter, findOpts)
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

func (m *MongoAdapter) executeFindLegacy(ctx context.Context, collection *mongo.Collection, stmt mongoStatement, policy window.LimitPolicy, start time.Time) (QueryResult, error) {
	var decision window.Decision
	if limit, ok := mongoFindLimit(stmt); ok {
		decision = policy.Decide(&limit)
	} else {
		decision = policy.Decide(nil)
	}
	findOpts := options.Find()
	if decision.Fetch > 0 {
		findOpts.SetLimit(decision.Fetch)
	}
	if stmt.Options != nil {
		if projection, ok := stmt.Options["projection"]; ok {
			findOpts.SetProjection(projection)
		}
		if sort, ok := stmt.Options["sort"]; ok {
			findOpts.SetSort(sort)
		}
		if skip, ok := int64From(stmt.Options["skip"]); ok {
			findOpts.SetSkip(skip)
		}
	}
	cursor, err := collection.Find(ctx, stmt.Filter, findOpts)
	if err != nil {
		return QueryResult{}, err
	}
	defer cursor.Close(ctx)
	win := window.NewRowWindow(int(decision.Effective))
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
	result := QueryResult{Rows: win.Rows(), RowCount: int64(len(win.Rows())), HasMore: win.HasMore(), ElapsedMs: time.Since(start).Milliseconds()}
	source := EffectiveLimitDefault
	if !decision.Enforced {
		source = EffectiveLimitStatement
	}
	setQueryLimitMetadata(&result, int(decision.Effective), source)
	return result, nil
}

func (m *MongoAdapter) executeFindToken(ctx context.Context, collection *mongo.Collection, ds datasource.DataSource, statement string, stmt mongoStatement, execOpts ExecuteOptions, policy window.LimitPolicy, start time.Time) (QueryResult, error) {
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
	filter := mongoMergeFilters(stmt.Filter, keyset)
	fetchLimit := mongoFetchLimit(pageSize, totalLimit, offset)
	if fetchLimit == 0 {
		result := QueryResult{Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}

	findOpts := options.Find()
	if stmt.Options != nil {
		if projection, ok := stmt.Options["projection"]; ok {
			findOpts.SetProjection(projection)
		}
	}
	findOpts.SetSort(mongoSortDocument(sortKeys, token.Direction))
	findOpts.SetLimit(int64(fetchLimit))

	cursor, err := collection.Find(ctx, filter, findOpts)
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
