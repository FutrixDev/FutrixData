package console

import (
	"strings"

	"futrixdata/platform/internal/console/paging"
	"futrixdata/platform/internal/console/window"
	"futrixdata/platform/internal/datasource"
)

const (
	EffectiveLimitNone           = "none"
	EffectiveLimitPageSize       = "pageSize"
	EffectiveLimitPagingToken    = "pagingToken"
	EffectiveLimitStatement      = "statementLimit"
	EffectiveLimitDefault        = "defaultLimit"
	EffectiveLimitPolicy         = "policyLimit"
	EffectiveLimitServiceDefault = "serviceDefault"
	EffectiveLimitBounded        = "boundedLimit"
)

func annotateDescribeContext(result DescribeResult, ds datasource.DataSource) DescribeResult {
	result.Dialect = ds.QueryDialect()
	result.Environment = ds.Environment()
	return result
}

func annotateQueryContext(result QueryResult, ds datasource.DataSource, statement string, opts ExecuteOptions) QueryResult {
	result.RequestedPageSize = opts.PageSize
	if result.Dialect == "" {
		result.Dialect = ds.QueryDialect()
	}
	if result.Environment == "" {
		result.Environment = ds.Environment()
	}
	if result.EffectiveLimitSource == "" && queryResultSupportsLimitMetadata(result) {
		result.EffectivePageSize, result.EffectiveLimitSource = defaultExecutionLimit(ds, statement, opts)
	}
	return result
}

func queryResultSupportsLimitMetadata(result QueryResult) bool {
	return len(result.Columns) > 0 ||
		len(result.ColumnMeta) > 0 ||
		len(result.Rows) > 0 ||
		len(result.RowValues) > 0 ||
		result.HasMore ||
		strings.TrimSpace(result.NextToken) != "" ||
		strings.TrimSpace(result.PrevToken) != "" ||
		strings.TrimSpace(result.SourceEntity) != ""
}

func setQueryLimitMetadata(result *QueryResult, effective int, source string) {
	if result == nil {
		return
	}
	result.EffectivePageSize = effective
	result.EffectiveLimitSource = strings.TrimSpace(source)
}

func pageWindowLimitMetadata(pageSize int, totalLimit int64, offset int64, pageSource string) (int, string) {
	if totalLimit <= 0 {
		return pageSize, pageSource
	}
	remaining := totalLimit - offset
	if remaining <= 0 {
		return 0, EffectiveLimitStatement
	}
	if remaining < int64(pageSize) {
		return int(remaining), EffectiveLimitStatement
	}
	return pageSize, pageSource
}

func defaultExecutionLimit(ds datasource.DataSource, statement string, opts ExecuteOptions) (int, string) {
	if opts.PagingToken != "" {
		if token, err := paging.Decode(opts.PagingToken); err == nil && token.PageSize > 0 {
			return token.PageSize, EffectiveLimitPagingToken
		}
		if opts.PageSize > 0 {
			return opts.PageSize, EffectiveLimitPageSize
		}
	}
	switch ds.Type {
	case datasource.TypeMySQL, datasource.TypePostgreSQL, datasource.TypeMongoDB:
		if opts.PageSize > 0 {
			return clampPageSize(opts.PageSize, window.LimitPolicy{Max: window.DefaultLimit}), EffectiveLimitPageSize
		}
		if info := findTopLevelLimit(statement); info.found && info.parsed {
			effective := info.count
			max := int64(window.DefaultLimit)
			if effective > max {
				return int(max), EffectiveLimitPolicy
			}
			if effective < 0 {
				return int(max), EffectiveLimitPolicy
			}
			return int(effective), EffectiveLimitStatement
		}
		return int(window.DefaultLimit), EffectiveLimitDefault
	case datasource.TypeD1:
		if opts.PageSize > 0 {
			return d1ClampPageSize(opts.PageSize), EffectiveLimitPageSize
		}
		if info := findTopLevelLimit(statement); info.found && info.parsed {
			return int(info.count), EffectiveLimitStatement
		}
		return d1ClampPageSize(0), EffectiveLimitDefault
	case datasource.TypeDynamoDB:
		_, statementLimit := dynamodbStripTrailingPartiqlLimit(statement)
		if opts.PageSize > 0 && (statementLimit == 0 || int32(opts.PageSize) < statementLimit) {
			return opts.PageSize, EffectiveLimitPageSize
		}
		if statementLimit > 0 {
			return int(statementLimit), EffectiveLimitStatement
		}
		return 0, EffectiveLimitServiceDefault
	default:
		return 0, EffectiveLimitNone
	}
}
