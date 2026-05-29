package console

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"futrixdata/platform/internal/console/paging"
	"futrixdata/platform/internal/console/window"
	"futrixdata/platform/internal/datasource"
)

func (a *SQLAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	db, err := a.dbFor(ctx, ds)
	if err != nil {
		return QueryResult{}, err
	}
	start := time.Now()
	done := DatasourceTimingStage(ctx, "sql.classify_statement")
	trimmed := strings.TrimSpace(strings.ToLower(statement))

	if trimmed == "" {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, errors.New("statement required")
	}
	isQuery := isQueryStatement(statement, a.dialect)
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("query", isQuery))

	if isQuery {
		return a.executePagedQuery(ctx, db, ds, statement, opts, start)
	}

	done = DatasourceTimingStage(ctx, "sql.exec_context")
	result, err := db.ExecContext(ctx, statement)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	rowsAffected, _ := result.RowsAffected()
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows_affected", rowsAffected))
	return QueryResult{RowCount: rowsAffected, ElapsedMs: time.Since(start).Milliseconds()}, nil
}

func (a *SQLAdapter) executePagedQuery(ctx context.Context, db *sql.DB, ds datasource.DataSource, statement string, opts ExecuteOptions, start time.Time) (QueryResult, error) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	if opts.PagingToken != "" {
		return a.executePagedToken(ctx, db, ds, statement, opts, policy, start)
	}

	pageSize := clampPageSize(opts.PageSize, policy)
	limitInfo := findTopLevelLimit(statement)
	if limitInfo.found && limitInfo.hasOffset {
		query, decision := prepareSQLQueryWithDialect(statement, a.dialect, policy)
		done := DatasourceTimingStage(ctx, "sql.query_context")
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("mode", "limit_offset"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "limit_offset"), DatasourceTimingKV("effective_limit", int(decision.Effective)))
		defer rows.Close()

		done = DatasourceTimingStage(ctx, "sql.read_rows_window")
		batch, hasMore, err := readSQLRowsWindow(rows, int(decision.Effective))
		if err != nil {
			done(DatasourceTimingKV("status", "error"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows", len(batch.Rows)), DatasourceTimingKV("has_more", hasMore))
		done = DatasourceTimingStage(ctx, "sql.result_plan")
		result := a.buildSQLQueryResult(ctx, ds, statement, batch, hasMore, time.Since(start).Milliseconds())
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("columns", len(result.Columns)))
		setQueryLimitMetadata(&result, int(decision.Effective), sqlLimitSourceFromDecision(limitInfo, decision))
		return result, nil
	}
	totalLimit := resolveTotalLimit(limitInfo.found && limitInfo.parsed, limitInfo.count, policy)

	done := DatasourceTimingStage(ctx, "sql.infer_sort_keys")
	keys, err := inferSQLSortKeys(ctx, db, a.dialect, statement)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("sort_keys", len(keys)))
	if len(keys) == 0 {
		query, decision := prepareSQLQueryWithDialect(statement, a.dialect, policy)
		done = DatasourceTimingStage(ctx, "sql.query_context")
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("mode", "no_sort_keys"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "no_sort_keys"), DatasourceTimingKV("effective_limit", int(decision.Effective)))
		defer rows.Close()

		done = DatasourceTimingStage(ctx, "sql.read_rows_window")
		batch, hasMore, err := readSQLRowsWindow(rows, int(decision.Effective))
		if err != nil {
			done(DatasourceTimingKV("status", "error"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows", len(batch.Rows)), DatasourceTimingKV("has_more", hasMore))
		done = DatasourceTimingStage(ctx, "sql.result_plan")
		result := a.buildSQLQueryResult(ctx, ds, statement, batch, hasMore, time.Since(start).Milliseconds())
		done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("columns", len(result.Columns)))
		setQueryLimitMetadata(&result, int(decision.Effective), sqlLimitSourceFromDecision(limitInfo, decision))
		return result, nil
	}

	fetchLimit := sqlFetchLimit(pageSize, totalLimit, 0)
	if fetchLimit == 0 {
		result := QueryResult{Columns: nil, Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}
	done = DatasourceTimingStage(ctx, "sql.build_paging_query")
	query, err := buildSQLPagingQueryDetailed(statement, a.dialect, keys, nil, DirectionNext, fetchLimit)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "keyset"), DatasourceTimingKV("fetch_limit", fetchLimit))
	done = DatasourceTimingStage(ctx, "sql.query_context")
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("mode", "keyset"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "keyset"))
	defer rows.Close()

	done = DatasourceTimingStage(ctx, "sql.read_rows_window")
	batch, hasMore, err := readSQLRowsWindow(rows, pageSize)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows", len(batch.Rows)), DatasourceTimingKV("has_more", hasMore))
	done = DatasourceTimingStage(ctx, "sql.result_plan")
	plan := buildSQLResultPlan(ctx, ds, a.dialect, statement, batch.ColumnNames, a.DescribeEntity)
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("columns", len(plan.ColumnMeta)))
	nextToken := ""
	if len(batch.Rows) > 0 {
		done = DatasourceTimingStage(ctx, "sql.cursor_values")
		startCursor, err := sqlCursorValues(batch.Rows[0], batch.RowValues[0], plan.ColumnMeta, keys, statement, a.dialect)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cursor", "start"))
			return QueryResult{}, err
		}
		endCursor, err := sqlCursorValues(
			batch.Rows[len(batch.Rows)-1],
			batch.RowValues[len(batch.RowValues)-1],
			plan.ColumnMeta,
			keys,
			statement,
			a.dialect,
		)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cursor", "end"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"))
		nextOffset := pagingNextOffset(0, len(batch.Rows))
		allowNext := hasMore
		if totalLimit > 0 && nextOffset >= totalLimit {
			allowNext = false
		}
		if allowNext {
			done = DatasourceTimingStage(ctx, "sql.build_paging_token")
			nextToken, err = buildSQLPagingToken(ds, statement, pageSize, keys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
			if err != nil {
				done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("direction", "next"))
				return QueryResult{}, err
			}
			done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", "next"))
		}
	}
	result := buildSQLQueryResultFromPlan(batch, plan, hasMore, time.Since(start).Milliseconds())
	result.NextToken = nextToken
	effective, source := pageWindowLimitMetadata(pageSize, totalLimit, 0, sqlPageSizeLimitSource(opts.PageSize))
	setQueryLimitMetadata(&result, effective, source)
	return result, nil
}

func (a *SQLAdapter) executePagedToken(ctx context.Context, db *sql.DB, ds datasource.DataSource, statement string, opts ExecuteOptions, policy window.LimitPolicy, start time.Time) (QueryResult, error) {
	done := DatasourceTimingStage(ctx, "sql.decode_paging_token")
	token, err := paging.Decode(opts.PagingToken)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	if token.DatasourceID != "" && token.DatasourceID != ds.ID {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("reason", "datasource_mismatch"))
		return QueryResult{}, errors.New("paging token datasource mismatch")
	}
	if token.QueryHash != "" && token.QueryHash != pagingQueryHash(statement) {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("reason", "query_mismatch"))
		return QueryResult{}, errors.New("paging token query mismatch")
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", string(token.Direction)), DatasourceTimingKV("offset", token.Offset))

	keys := sqlSortKeysFromToken(token.Sort)
	if len(keys) == 0 {
		return QueryResult{}, errors.New("paging token missing sort keys")
	}
	pageSize := clampPageSize(token.PageSize, policy)
	totalLimit := token.Limit
	offset := token.Offset
	cursor := token.EndCursor
	if token.Direction == DirectionPrev {
		cursor = token.StartCursor
	}
	fetchLimit := sqlFetchLimit(pageSize, totalLimit, offset)
	if fetchLimit == 0 {
		result := QueryResult{Columns: nil, Rows: nil, RowCount: 0, HasMore: false, ElapsedMs: time.Since(start).Milliseconds()}
		setQueryLimitMetadata(&result, 0, EffectiveLimitStatement)
		return result, nil
	}
	done = DatasourceTimingStage(ctx, "sql.build_paging_query")
	query, err := buildSQLPagingQueryDetailed(statement, a.dialect, keys, cursor, token.Direction, fetchLimit)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "paging_token"), DatasourceTimingKV("fetch_limit", fetchLimit))
	done = DatasourceTimingStage(ctx, "sql.query_context")
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("mode", "paging_token"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("mode", "paging_token"))
	defer rows.Close()

	done = DatasourceTimingStage(ctx, "sql.read_rows_window")
	batch, hasMore, err := readSQLRowsWindow(rows, pageSize)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows", len(batch.Rows)), DatasourceTimingKV("has_more", hasMore))
	if token.Direction == DirectionPrev {
		reverseRows(batch.Rows)
		reverseRowValues(batch.RowValues)
	}
	done = DatasourceTimingStage(ctx, "sql.result_plan")
	plan := buildSQLResultPlan(ctx, ds, a.dialect, statement, batch.ColumnNames, a.DescribeEntity)
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("columns", len(plan.ColumnMeta)))
	nextToken := ""
	prevToken := ""
	if len(batch.Rows) > 0 {
		done = DatasourceTimingStage(ctx, "sql.cursor_values")
		startCursor, err := sqlCursorValues(batch.Rows[0], batch.RowValues[0], plan.ColumnMeta, keys, statement, a.dialect)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cursor", "start"))
			return QueryResult{}, err
		}
		endCursor, err := sqlCursorValues(
			batch.Rows[len(batch.Rows)-1],
			batch.RowValues[len(batch.RowValues)-1],
			plan.ColumnMeta,
			keys,
			statement,
			a.dialect,
		)
		if err != nil {
			done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("cursor", "end"))
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"))
		nextOffset := pagingNextOffset(offset, len(batch.Rows))
		prevOffset := pagingPrevOffset(offset, pageSize)
		hasForward := totalLimit == 0 || nextOffset < totalLimit
		if token.Direction == DirectionPrev {
			if hasMore && offset > 0 {
				done = DatasourceTimingStage(ctx, "sql.build_paging_token")
				prevToken, err = buildSQLPagingToken(ds, statement, pageSize, keys, startCursor, endCursor, DirectionPrev, totalLimit, prevOffset)
				if err != nil {
					done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("direction", "prev"))
					return QueryResult{}, err
				}
				done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", "prev"))
			}
			if hasForward {
				done = DatasourceTimingStage(ctx, "sql.build_paging_token")
				nextToken, err = buildSQLPagingToken(ds, statement, pageSize, keys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
				if err != nil {
					done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("direction", "next"))
					return QueryResult{}, err
				}
				done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", "next"))
			}
		} else {
			if hasMore && hasForward {
				done = DatasourceTimingStage(ctx, "sql.build_paging_token")
				nextToken, err = buildSQLPagingToken(ds, statement, pageSize, keys, startCursor, endCursor, DirectionNext, totalLimit, nextOffset)
				if err != nil {
					done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("direction", "next"))
					return QueryResult{}, err
				}
				done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", "next"))
			}
			if offset > 0 {
				done = DatasourceTimingStage(ctx, "sql.build_paging_token")
				prevToken, err = buildSQLPagingToken(ds, statement, pageSize, keys, startCursor, endCursor, DirectionPrev, totalLimit, prevOffset)
				if err != nil {
					done(DatasourceTimingKV("status", "error"), DatasourceTimingKV("direction", "prev"))
					return QueryResult{}, err
				}
				done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("direction", "prev"))
			}
		}
	}
	result := buildSQLQueryResultFromPlan(batch, plan, hasMore, time.Since(start).Milliseconds())
	result.NextToken = nextToken
	result.PrevToken = prevToken
	effective, source := pageWindowLimitMetadata(pageSize, totalLimit, offset, EffectiveLimitPagingToken)
	setQueryLimitMetadata(&result, effective, source)
	return result, nil
}

func clampPageSize(pageSize int, policy window.LimitPolicy) int {
	max := int(policy.Decide(nil).Effective)
	if pageSize <= 0 || pageSize > max {
		return max
	}
	return pageSize
}

func sqlLimitSourceFromDecision(info sqlLimitInfo, decision window.Decision) string {
	if info.found && info.parsed && !decision.Enforced {
		return EffectiveLimitStatement
	}
	if info.found {
		return EffectiveLimitPolicy
	}
	return EffectiveLimitDefault
}

func sqlPageSizeLimitSource(requested int) string {
	if requested > 0 {
		return EffectiveLimitPageSize
	}
	return EffectiveLimitDefault
}

func buildSQLPagingToken(ds datasource.DataSource, statement string, pageSize int, keys []sqlSortKey, startCursor []any, endCursor []any, direction Direction, totalLimit int64, offset int64) (string, error) {
	token := paging.Token{
		Version:      1,
		DatasourceID: ds.ID,
		QueryHash:    pagingQueryHash(statement),
		PageSize:     pageSize,
		Limit:        totalLimit,
		Offset:       offset,
		Sort:         pagingSortKeys(keys),
		StartCursor:  startCursor,
		EndCursor:    endCursor,
		Direction:    direction,
	}
	return paging.Encode(token)
}

func reverseRows(rows []map[string]any) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}

func reverseRowValues(rows [][]any) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}

func (a *SQLAdapter) buildSQLQueryResult(ctx context.Context, ds datasource.DataSource, statement string, batch sqlRowBatch, hasMore bool, elapsedMs int64) QueryResult {
	plan := buildSQLResultPlan(ctx, ds, a.dialect, statement, batch.ColumnNames, a.DescribeEntity)
	return buildSQLQueryResultFromPlan(batch, plan, hasMore, elapsedMs)
}

func isQueryStatement(statement string, dialect string) bool {
	return SQLStatementIsReadQuery(statement, dialect)
}
