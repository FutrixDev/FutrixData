package console

import (
	"context"
	"strings"

	"futrixdata/platform/internal/datasource"
)

type sqlResultPlan struct {
	ColumnMeta   []ResultColumn
	SourceEntity string
}

func buildSQLResultPlan(ctx context.Context, ds datasource.DataSource, dialect, statement string, actualColumns []string, describe sqlDescribeEntityFunc) sqlResultPlan {
	plan := sqlResultPlan{
		ColumnMeta:   conservativeSQLResultColumns(actualColumns),
		SourceEntity: sqlSourceEntityHint(statement, dialect),
	}
	if len(actualColumns) == 0 {
		plan.ColumnMeta = nil
		return plan
	}
	columnMeta, err := buildSQLResultColumns(ctx, ds, dialect, statement, actualColumns, describe)
	if err != nil || len(columnMeta) == 0 {
		return plan
	}
	plan.ColumnMeta = columnMeta
	return plan
}

func buildSQLQueryResultFromPlan(batch sqlRowBatch, plan sqlResultPlan, hasMore bool, elapsedMs int64) QueryResult {
	return QueryResult{
		Columns:      batch.ColumnKeys,
		Rows:         batch.Rows,
		ColumnMeta:   plan.ColumnMeta,
		RowValues:    batch.RowValues,
		SourceEntity: plan.SourceEntity,
		RowCount:     int64(len(batch.Rows)),
		HasMore:      hasMore,
		ElapsedMs:    elapsedMs,
	}
}

func conservativeSQLResultColumns(actualColumns []string) []ResultColumn {
	keys := makeSQLColumnKeys(actualColumns)
	out := make([]ResultColumn, len(actualColumns))
	for i, name := range actualColumns {
		display := strings.TrimSpace(name)
		if display == "" {
			display = keys[i]
		}
		out[i] = ResultColumn{
			Key:              keys[i],
			Name:             display,
			Position:         i,
			SourceKind:       "unknown",
			ConservativeMask: true,
		}
	}
	return out
}
