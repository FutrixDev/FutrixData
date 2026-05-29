package console

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"futrixdata/platform/internal/datasource"
)

type DynamoDBAdapter struct {
	client *dynamodbClient
}

func NewDynamoDBAdapter() *DynamoDBAdapter {
	return &DynamoDBAdapter{client: newDynamoDBClient()}
}

func (a *DynamoDBAdapter) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	api, err := a.client.newAPI(ctx, ds)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = api.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)})
	return err
}

func (a *DynamoDBAdapter) ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts ListOptions, cursor string) (EntityPage, error) {
	api, err := a.client.newAPI(ctx, ds)
	if err != nil {
		return EntityPage{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	pattern := strings.ToLower(strings.TrimSpace(opts.Pattern))

	if pattern == "" {
		req := &dynamodb.ListTablesInput{Limit: aws.Int32(int32(limit))}
		trimmedCursor := strings.TrimSpace(cursor)
		if trimmedCursor != "" {
			req.ExclusiveStartTableName = aws.String(trimmedCursor)
		}
		resp, err := api.ListTables(ctx, req)
		if err != nil {
			return EntityPage{}, err
		}

		items := make([]string, 0, len(resp.TableNames))
		for _, name := range resp.TableNames {
			table := strings.TrimSpace(name)
			if table == "" {
				continue
			}
			items = append(items, table)
		}
		nextCursor := strings.TrimSpace(aws.ToString(resp.LastEvaluatedTableName))
		done := nextCursor == ""
		return EntityPage{Items: items, Cursor: nextCursor, Done: done}, nil
	}

	items := make([]string, 0, limit)
	startAfter := strings.TrimSpace(cursor)
	for len(items) < limit {
		req := &dynamodb.ListTablesInput{Limit: aws.Int32(int32(limit))}
		if startAfter != "" {
			req.ExclusiveStartTableName = aws.String(startAfter)
		}
		resp, err := api.ListTables(ctx, req)
		if err != nil {
			return EntityPage{}, err
		}

		lastReturned := ""
		stopIdx := -1
		for idx, name := range resp.TableNames {
			table := strings.TrimSpace(name)
			if table == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(table), pattern) {
				continue
			}
			items = append(items, table)
			lastReturned = table
			if len(items) >= limit {
				stopIdx = idx
				break
			}
		}

		nextPageCursor := strings.TrimSpace(aws.ToString(resp.LastEvaluatedTableName))
		if stopIdx != -1 {
			remainingHasMatch := false
			for _, name := range resp.TableNames[stopIdx+1:] {
				table := strings.TrimSpace(name)
				if table == "" {
					continue
				}
				if strings.Contains(strings.ToLower(table), pattern) {
					remainingHasMatch = true
					break
				}
			}
			if remainingHasMatch {
				return EntityPage{Items: items, Cursor: lastReturned, Done: false}, nil
			}
			if nextPageCursor == "" {
				return EntityPage{Items: items, Cursor: "", Done: true}, nil
			}
			return EntityPage{Items: items, Cursor: nextPageCursor, Done: false}, nil
		}
		if nextPageCursor == "" {
			return EntityPage{Items: items, Cursor: "", Done: true}, nil
		}
		if nextPageCursor == startAfter {
			return EntityPage{}, fmt.Errorf("dynamodb ListTables cursor did not advance")
		}
		startAfter = nextPageCursor
	}
	return EntityPage{Items: items, Cursor: startAfter, Done: false}, nil
}

func (a *DynamoDBAdapter) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	api, err := a.client.newAPI(ctx, ds)
	if err != nil {
		return nil, err
	}
	pager := dynamodb.NewListTablesPaginator(api, &dynamodb.ListTablesInput{Limit: aws.Int32(100)})
	out := make([]string, 0)
	pattern := strings.ToLower(strings.TrimSpace(opts.Pattern))
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, name := range page.TableNames {
			table := strings.TrimSpace(name)
			if table == "" {
				continue
			}
			if pattern != "" && !strings.Contains(strings.ToLower(table), pattern) {
				continue
			}
			out = append(out, table)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (a *DynamoDBAdapter) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	table := strings.TrimSpace(name)
	if table == "" {
		return DescribeResult{}, errors.New("table is required")
	}
	api, err := a.client.newAPI(ctx, ds)
	if err != nil {
		return DescribeResult{}, err
	}
	resp, err := api.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
	if err != nil {
		return DescribeResult{}, err
	}
	if resp.Table == nil {
		return DescribeResult{}, errors.New("table not found")
	}

	columns := make([]ColumnInfo, 0, len(resp.Table.AttributeDefinitions))
	for _, def := range resp.Table.AttributeDefinitions {
		columns = append(columns, ColumnInfo{
			Name:         aws.ToString(def.AttributeName),
			DataType:     string(def.AttributeType),
			Nullable:     "-",
			DefaultValue: nil,
		})
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Name < columns[j].Name })

	indexes := make([]IndexInfo, 0)
	for _, idx := range resp.Table.GlobalSecondaryIndexes {
		indexes = append(indexes, indexInfoFromDynamoGSI(idx))
	}
	for _, idx := range resp.Table.LocalSecondaryIndexes {
		indexes = append(indexes, indexInfoFromDynamoLSI(idx))
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })

	partitionKey := ""
	sortKey := ""
	for _, item := range resp.Table.KeySchema {
		switch item.KeyType {
		case types.KeyTypeHash:
			partitionKey = aws.ToString(item.AttributeName)
		case types.KeyTypeRange:
			sortKey = aws.ToString(item.AttributeName)
		}
	}

	details := []DetailItem{
		{Label: "Table", Value: aws.ToString(resp.Table.TableName)},
		{Label: "Status", Value: string(resp.Table.TableStatus)},
		{Label: "Items", Value: resp.Table.ItemCount},
	}
	if strings.TrimSpace(partitionKey) != "" {
		details = append(details, DetailItem{Label: "Partition Key", Value: partitionKey})
	}
	if strings.TrimSpace(sortKey) != "" {
		details = append(details, DetailItem{Label: "Sort Key", Value: sortKey})
	}

	return DescribeResult{
		Columns: columns,
		Indexes: indexes,
		Details: details,
		Preview: nil,
	}, nil
}

func (a *DynamoDBAdapter) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	api, err := a.client.newAPI(ctx, ds)
	if err != nil {
		return QueryResult{}, err
	}

	repairedStatement, repairDetail := dynamodbRepairSingleQuotedPartiQLTarget(statement)
	execMeta := dynamodbExecutionMetadata{
		StatementRepair: repairDetail,
	}
	normalizedStatement, limit := dynamodbStripTrailingPartiqlLimit(repairedStatement)
	limitSource := EffectiveLimitStatement
	if limit == 0 {
		limitSource = EffectiveLimitServiceDefault
	}
	if opts.Bounds.Enabled() {
		return a.executeBounded(ctx, ds, api, normalizedStatement, repairedStatement, limit, opts, execMeta)
	}
	if opts.PageSize > 0 {
		pageSize, clamped := dynamodbClampPageSize(opts.PageSize)
		if clamped {
			execMeta.addClampedLimit("pageSize")
		}
		pageLimit := int32(pageSize)
		if limit == 0 || pageLimit < limit {
			limit = pageLimit
			limitSource = EffectiveLimitPageSize
		}
	}

	start := time.Now()
	input := &dynamodb.ExecuteStatementInput{Statement: aws.String(normalizedStatement)}
	if opts.PagingToken != "" {
		input.NextToken = aws.String(opts.PagingToken)
	}
	if limit > 0 {
		input.Limit = aws.Int32(limit)
	}

	resp, err := api.ExecuteStatement(ctx, input)
	if err != nil {
		return QueryResult{}, err
	}

	rows := make([]map[string]any, 0, len(resp.Items))
	for _, item := range resp.Items {
		decoded, err := dynamodbDecodeItem(item)
		if err != nil {
			return QueryResult{}, err
		}
		rows = append(rows, decoded)
	}
	next := aws.ToString(resp.NextToken)
	if len(rows) == 0 && dynamodbShouldProbeIndexTargetSuggestion(repairedStatement) {
		execMeta.IndexSuggestion = a.dynamodbIndexTargetSuggestion(ctx, ds, repairedStatement)
	}
	result := QueryResult{
		Columns:      nil,
		Rows:         rows,
		RowCount:     int64(len(rows)),
		HasMore:      next != "",
		NextToken:    next,
		PrevToken:    "",
		ElapsedMs:    time.Since(start).Milliseconds(),
		SourceEntity: dynamodbPartiQLTableName(repairedStatement),
	}
	setQueryLimitMetadata(&result, int(limit), limitSource)
	execMeta.applyNonBoundedDetail(&result)
	return result, nil
}

const (
	dynamodbDefaultBoundedPageSize          = 100
	dynamodbDefaultBoundedMaxReturnedRows   = 100
	dynamodbDefaultBoundedMaxPages          = 5
	dynamodbDefaultBoundedMaxEvaluatedItems = 500
	// dynamodbMaxPageSize matches the DynamoDB ExecuteStatement Limit ceiling.
	// This one stays — it is a service-side limit, not a product policy.
	dynamodbMaxPageSize            = 500
	dynamodbBoundedRowsPreallocCap = 1024
)

func DynamoDBMaxPageSize() int {
	return dynamodbMaxPageSize
}

func ValidateDynamoDBToolExecutionLimits(pageSize int, _ ExecuteBounds) error {
	if pageSize > dynamodbMaxPageSize {
		return fmt.Errorf("dynamodb execution limits exceed product caps: pageSize %d exceeds DynamoDB maximum %d", pageSize, dynamodbMaxPageSize)
	}
	return nil
}

type dynamodbStatementRepairDetail struct {
	Kind              string `json:"kind"`
	OriginalStatement string `json:"originalStatement"`
	RepairedStatement string `json:"repairedStatement"`
	Reason            string `json:"reason"`
}

type dynamodbIndexSuggestionDetail struct {
	Kind               string `json:"kind"`
	Table              string `json:"table"`
	Index              string `json:"index"`
	PartitionKey       string `json:"partitionKey"`
	SuggestedStatement string `json:"suggestedStatement"`
	Reason             string `json:"reason"`
}

type dynamodbExecutionMetadata struct {
	StatementRepair *dynamodbStatementRepairDetail
	IndexSuggestion *dynamodbIndexSuggestionDetail
	ClampedLimits   map[string]bool
}

func (m *dynamodbExecutionMetadata) addClampedLimit(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	if m.ClampedLimits == nil {
		m.ClampedLimits = map[string]bool{}
	}
	m.ClampedLimits[name] = true
}

func (m dynamodbExecutionMetadata) hasDetail() bool {
	return m.StatementRepair != nil || m.IndexSuggestion != nil || len(m.ClampedLimits) > 0
}

func (m dynamodbExecutionMetadata) applyNonBoundedDetail(result *QueryResult) {
	if result == nil || !m.hasDetail() {
		return
	}
	result.Detail = dynamodbNonBoundedDetail{
		Kind:              "dynamodb-execution",
		EffectivePageSize: result.EffectivePageSize,
		HasMore:           result.HasMore,
		NextTokenState:    dynamodbNextTokenState(result.NextToken),
		ClampedLimits:     m.ClampedLimits,
		StatementRepair:   m.StatementRepair,
		IndexSuggestion:   m.IndexSuggestion,
	}
}

type dynamodbNonBoundedDetail struct {
	Kind              string                         `json:"kind"`
	EffectivePageSize int                            `json:"effectivePageSize,omitempty"`
	HasMore           bool                           `json:"hasMore"`
	NextTokenState    string                         `json:"nextTokenState"`
	ClampedLimits     map[string]bool                `json:"clampedLimits,omitempty"`
	StatementRepair   *dynamodbStatementRepairDetail `json:"statementRepair,omitempty"`
	IndexSuggestion   *dynamodbIndexSuggestionDetail `json:"indexSuggestion,omitempty"`
}

type dynamodbPaginationDetail struct {
	Kind              string                         `json:"kind"`
	PageSize          int                            `json:"pageSize"`
	RequestedPageSize int                            `json:"requestedPageSize,omitempty"`
	EffectivePageSize int                            `json:"effectivePageSize"`
	MaxReturnedRows   int                            `json:"maxReturnedRows"`
	MaxPages          int                            `json:"maxPages"`
	MaxEvaluatedItems int                            `json:"maxEvaluatedItems"`
	RequestedLimits   dynamodbLimitsDetail           `json:"requestedLimits"`
	EffectiveLimits   dynamodbLimitsDetail           `json:"effectiveLimits"`
	PagesFetched      int                            `json:"pagesFetched"`
	RowsReturned      int                            `json:"rowsReturned"`
	HasMore           bool                           `json:"hasMore"`
	NextToken         string                         `json:"nextToken"`
	NextTokenState    string                         `json:"nextTokenState"`
	StopReason        string                         `json:"stopReason"`
	ClampedLimits     map[string]bool                `json:"clampedLimits,omitempty"`
	StatementRepair   *dynamodbStatementRepairDetail `json:"statementRepair,omitempty"`
	IndexSuggestion   *dynamodbIndexSuggestionDetail `json:"indexSuggestion,omitempty"`
}

type dynamodbLimitsDetail struct {
	PageSize          int `json:"pageSize,omitempty"`
	MaxReturnedRows   int `json:"maxReturnedRows,omitempty"`
	MaxPages          int `json:"maxPages,omitempty"`
	MaxEvaluatedItems int `json:"maxEvaluatedItems,omitempty"`
}

func (a *DynamoDBAdapter) executeBounded(ctx context.Context, ds datasource.DataSource, api *dynamodb.Client, normalizedStatement, originalStatement string, statementLimit int32, opts ExecuteOptions, execMeta dynamodbExecutionMetadata) (QueryResult, error) {
	start := time.Now()
	for name := range opts.ClampedLimits {
		execMeta.addClampedLimit(name)
	}
	requestedBounds := opts.RequestedExecutionBounds()
	bounds, boundClamps := normalizeDynamoDBBoundsWithCaps(opts.Bounds)
	for name := range boundClamps {
		execMeta.addClampedLimit(name)
	}
	requestedPageSize := opts.PageSize
	pageSize := requestedPageSize
	if pageSize <= 0 {
		pageSize = dynamodbDefaultBoundedPageSize
	} else if clampedPageSize, clamped := dynamodbClampPageSize(pageSize); clamped {
		pageSize = clampedPageSize
		execMeta.addClampedLimit("pageSize")
	}
	if statementLimit > 0 {
		statementCap := int(statementLimit)
		if statementCap < pageSize {
			pageSize = statementCap
		}
		if statementCap < bounds.MaxReturnedRows {
			bounds.MaxReturnedRows = statementCap
		}
		if statementCap < bounds.MaxEvaluatedItems {
			bounds.MaxEvaluatedItems = statementCap
		}
	}
	if pageSize <= 0 {
		pageSize = 1
	}
	effectiveLimit := minPositiveInt(pageSize, bounds.MaxReturnedRows, bounds.MaxEvaluatedItems)

	rows := make([]map[string]any, 0, minPositiveInt(bounds.MaxReturnedRows, dynamodbBoundedRowsPreallocCap))
	nextToken := strings.TrimSpace(opts.PagingToken)
	pagesFetched := 0
	evaluatedBudget := 0
	stopReason := ""

	for {
		if pagesFetched >= bounds.MaxPages {
			stopReason = "page_limit"
			break
		}
		if len(rows) >= bounds.MaxReturnedRows {
			stopReason = "returned_row_limit"
			break
		}
		remainingEvaluated := bounds.MaxEvaluatedItems - evaluatedBudget
		if remainingEvaluated <= 0 {
			stopReason = "evaluated_item_limit"
			break
		}
		remainingRows := bounds.MaxReturnedRows - len(rows)
		requestLimit := minPositiveInt(pageSize, remainingEvaluated, remainingRows)
		if requestLimit <= 0 {
			stopReason = "evaluated_item_limit"
			break
		}

		input := &dynamodb.ExecuteStatementInput{
			Statement: aws.String(normalizedStatement),
			Limit:     aws.Int32(int32(requestLimit)),
		}
		if nextToken != "" {
			input.NextToken = aws.String(nextToken)
		}
		resp, err := api.ExecuteStatement(ctx, input)
		if err != nil {
			return QueryResult{}, err
		}
		pagesFetched++
		// DynamoDB ExecuteStatement does not return ScannedCount/evaluated item count
		// for PartiQL pages. Spending the requested Limit keeps maxEvaluatedItems a
		// conservative hard budget instead of over-scanning sparse filters.
		evaluatedBudget += requestLimit

		for _, item := range resp.Items {
			if len(rows) >= bounds.MaxReturnedRows {
				break
			}
			decoded, err := dynamodbDecodeItem(item)
			if err != nil {
				return QueryResult{}, err
			}
			rows = append(rows, decoded)
		}
		nextToken = strings.TrimSpace(aws.ToString(resp.NextToken))
		if nextToken == "" {
			stopReason = "no_more_pages"
			break
		}
	}

	hasMore := nextToken != "" && stopReason != "no_more_pages"
	if stopReason == "" {
		stopReason = "no_more_pages"
	}
	if len(rows) == 0 && dynamodbShouldProbeIndexTargetSuggestion(originalStatement) {
		execMeta.IndexSuggestion = a.dynamodbIndexTargetSuggestion(ctx, ds, originalStatement)
	}
	result := QueryResult{
		Columns:   nil,
		Rows:      rows,
		RowCount:  int64(len(rows)),
		HasMore:   hasMore,
		NextToken: nextToken,
		PrevToken: "",
		ElapsedMs: time.Since(start).Milliseconds(),
		Detail: dynamodbPaginationDetail{
			Kind:              "dynamodb-bounded-pagination",
			PageSize:          pageSize,
			RequestedPageSize: requestedPageSize,
			EffectivePageSize: effectiveLimit,
			MaxReturnedRows:   bounds.MaxReturnedRows,
			MaxPages:          bounds.MaxPages,
			MaxEvaluatedItems: bounds.MaxEvaluatedItems,
			RequestedLimits: dynamodbLimitsDetail{
				PageSize:          requestedPageSize,
				MaxReturnedRows:   requestedBounds.MaxReturnedRows,
				MaxPages:          requestedBounds.MaxPages,
				MaxEvaluatedItems: requestedBounds.MaxEvaluatedItems,
			},
			EffectiveLimits: dynamodbLimitsDetail{
				PageSize:          pageSize,
				MaxReturnedRows:   bounds.MaxReturnedRows,
				MaxPages:          bounds.MaxPages,
				MaxEvaluatedItems: bounds.MaxEvaluatedItems,
			},
			PagesFetched:    pagesFetched,
			RowsReturned:    len(rows),
			HasMore:         hasMore,
			NextToken:       nextToken,
			NextTokenState:  dynamodbNextTokenState(nextToken),
			StopReason:      stopReason,
			ClampedLimits:   execMeta.ClampedLimits,
			StatementRepair: execMeta.StatementRepair,
			IndexSuggestion: execMeta.IndexSuggestion,
		},
		SourceEntity: dynamodbPartiQLTableName(originalStatement),
	}
	setQueryLimitMetadata(&result, effectiveLimit, EffectiveLimitBounded)
	return result, nil
}

func normalizeDynamoDBBounds(bounds ExecuteBounds) ExecuteBounds {
	normalized, _ := normalizeDynamoDBBoundsWithCaps(bounds)
	return normalized
}

func normalizeDynamoDBBoundsWithCaps(bounds ExecuteBounds) (ExecuteBounds, map[string]bool) {
	clamped := map[string]bool{}
	if bounds.MaxReturnedRows <= 0 {
		bounds.MaxReturnedRows = dynamodbDefaultBoundedMaxReturnedRows
	}
	if bounds.MaxPages <= 0 {
		bounds.MaxPages = dynamodbDefaultBoundedMaxPages
	}
	if bounds.MaxEvaluatedItems <= 0 {
		bounds.MaxEvaluatedItems = dynamodbDefaultBoundedMaxEvaluatedItems
	}
	if len(clamped) == 0 {
		clamped = nil
	}
	return bounds, clamped
}

func dynamodbClampPageSize(pageSize int) (int, bool) {
	if pageSize > dynamodbMaxPageSize {
		return dynamodbMaxPageSize, true
	}
	return pageSize, false
}

func dynamodbNextTokenState(token string) string {
	if strings.TrimSpace(token) == "" {
		return "absent"
	}
	return "present"
}

func minPositiveInt(values ...int) int {
	min := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if min == 0 || value < min {
			min = value
		}
	}
	return min
}

func (a *DynamoDBAdapter) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	target := dynamodbPartiQLTargetInfoForStatement(statement)
	if strings.TrimSpace(target.TableName) == "" {
		return ExplainResult{}, errors.New("dynamodb explain requires a SELECT statement with a FROM table")
	}
	describe, err := a.DescribeEntity(ctx, ds, target.TableName)
	if err != nil && target.UnquotedDotted && target.DottedTableCandidate != "" {
		// Unquoted FROM a.b is ambiguous because DynamoDB table names may contain dots.
		// Prefer the full table name, then fall back to table.index only when needed.
		if fallbackDescribe, fallbackErr := a.DescribeEntity(ctx, ds, target.DottedTableCandidate); fallbackErr == nil {
			describe = fallbackDescribe
			err = nil
			target.TableName = target.DottedTableCandidate
			target.IndexName = target.DottedIndexCandidate
		}
	}
	if err != nil {
		return ExplainResult{}, err
	}
	detail := dynamodbBuildAccessPathDetailForTarget(statement, describe, target.TableName, target.IndexName)
	return ExplainResult{
		UsesIndex: detail.Classification == "key_based",
		Indexes:   dynamodbExplainUsedIndexes(detail),
		Stages:    []string{detail.Classification},
		Detail:    detail,
	}, nil
}

func dynamodbExplainUsedIndexes(detail dynamodbAccessPathDetail) []string {
	if strings.TrimSpace(detail.Index) == "" || detail.Classification == "unknown" {
		return nil
	}
	return []string{detail.Index}
}

func dynamodbDecodeItem(item map[string]types.AttributeValue) (map[string]any, error) {
	var out map[string]any
	if err := attributevalue.UnmarshalMap(item, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func indexInfoFromDynamoGSI(idx types.GlobalSecondaryIndexDescription) IndexInfo {
	return IndexInfo{
		Name:       aws.ToString(idx.IndexName),
		Unique:     false,
		Definition: dynamodbIndexDefinition(idx.KeySchema, idx.Projection),
	}
}

func indexInfoFromDynamoLSI(idx types.LocalSecondaryIndexDescription) IndexInfo {
	return IndexInfo{
		Name:       aws.ToString(idx.IndexName),
		Unique:     false,
		Definition: dynamodbIndexDefinition(idx.KeySchema, idx.Projection),
	}
}

func dynamodbIndexDefinition(schema []types.KeySchemaElement, projection *types.Projection) string {
	parts := make([]string, 0, len(schema)+1)
	for _, item := range schema {
		parts = append(parts, fmt.Sprintf("%s=%s", aws.ToString(item.AttributeName), item.KeyType))
	}
	if projection != nil && projection.ProjectionType != "" {
		parts = append(parts, fmt.Sprintf("projection=%s", projection.ProjectionType))
	}
	return strings.Join(parts, " | ")
}

type dynamodbPartiQLTargetInfo struct {
	TableName            string
	IndexName            string
	UnquotedDotted       bool
	DottedTableCandidate string
	DottedIndexCandidate string
}

// dynamodbPartiQLFromPattern extracts the table/index target from a DynamoDB PartiQL FROM clause.
var dynamodbPartiQLFromPattern = regexp.MustCompile(
	`(?is)\bfrom\s+("([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z0-9_.-]+))(?:\s*\.\s*("([^"]+)"|` + "`([^`]+)`" + `))?`,
)

// dynamodbPartiQLTableName returns the table name from a PartiQL statement, or "".
func dynamodbPartiQLTableName(statement string) string {
	table, _ := dynamodbPartiQLTarget(statement)
	return table
}

func dynamodbPartiQLTarget(statement string) (tableName string, indexName string) {
	target := dynamodbPartiQLTargetInfoForStatement(statement)
	return target.TableName, target.IndexName
}

func dynamodbPartiQLTargetInfoForStatement(statement string) dynamodbPartiQLTargetInfo {
	matches := dynamodbPartiQLFromPattern.FindStringSubmatch(strings.TrimSpace(statement))
	if len(matches) == 0 {
		return dynamodbPartiQLTargetInfo{}
	}
	tableName := firstNonEmptyString(matches[2], matches[3], matches[4])
	indexName := firstNonEmptyString(matches[6], matches[7])
	target := dynamodbPartiQLTargetInfo{
		TableName: strings.TrimSpace(tableName),
		IndexName: strings.TrimSpace(indexName),
	}
	if matches[4] != "" && strings.Contains(matches[4], ".") {
		target.UnquotedDotted = true
		before, after, ok := strings.Cut(strings.TrimSpace(matches[4]), ".")
		if ok {
			target.DottedTableCandidate = strings.TrimSpace(before)
			target.DottedIndexCandidate = strings.TrimSpace(after)
		}
	}
	return target
}

var dynamodbSingleQuotedPartiQLTargetPattern = regexp.MustCompile(`(?is)(\bfrom\s+)'([A-Za-z0-9_.-]+)'(?:\s*\.\s*'([A-Za-z0-9_.-]+)')?`)

func dynamodbRepairSingleQuotedPartiQLTarget(statement string) (string, *dynamodbStatementRepairDetail) {
	trimmed := strings.TrimSpace(statement)
	if !strings.EqualFold(leadingSQLKeyword(trimmed), "select") {
		return trimmed, nil
	}
	idx := dynamodbSingleQuotedPartiQLTargetPattern.FindStringSubmatchIndex(trimmed)
	if idx == nil {
		return trimmed, nil
	}
	table := trimmed[idx[4]:idx[5]]
	replacement := trimmed[idx[2]:idx[3]] + dynamodbQuotePartiQLIdentifier(table)
	if idx[6] >= 0 && idx[7] >= 0 {
		index := trimmed[idx[6]:idx[7]]
		replacement += "." + dynamodbQuotePartiQLIdentifier(index)
	}
	repaired := trimmed[:idx[0]] + replacement + trimmed[idx[1]:]
	if repaired == trimmed {
		return trimmed, nil
	}
	return repaired, &dynamodbStatementRepairDetail{
		Kind:              "dynamodb-statement-repair",
		OriginalStatement: trimmed,
		RepairedStatement: repaired,
		Reason:            "DynamoDB PartiQL table and index identifiers must use double quotes, not single quotes.",
	}
}

func dynamodbQuotePartiQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(identifier), `"`, `""`) + `"`
}

var dynamodbPartiqlTrailingLimit = regexp.MustCompile(`(?i)\s+limit\s+(\d+)\s*;?\s*$`)

func dynamodbStripTrailingPartiqlLimit(statement string) (string, int32) {
	trimmed := strings.TrimSpace(statement)
	matches := dynamodbPartiqlTrailingLimit.FindStringSubmatchIndex(trimmed)
	if matches == nil || len(matches) < 4 {
		return trimmed, 0
	}
	limitValue := trimmed[matches[2]:matches[3]]
	parsed, err := strconv.ParseInt(limitValue, 10, 32)
	if err != nil || parsed <= 0 {
		return trimmed, 0
	}
	stripped := strings.TrimSpace(trimmed[:matches[0]])
	return stripped, int32(parsed)
}

type dynamodbAccessPathDetail struct {
	Kind                 string   `json:"kind"`
	Table                string   `json:"table"`
	Index                string   `json:"index"`
	KnownIndexes         []string `json:"knownIndexes,omitempty"`
	TablePartitionKey    string   `json:"tablePartitionKey,omitempty"`
	TableSortKey         string   `json:"tableSortKey,omitempty"`
	IndexPartitionKey    string   `json:"indexPartitionKey,omitempty"`
	IndexSortKey         string   `json:"indexSortKey,omitempty"`
	PartitionPredicate   string   `json:"partitionPredicate,omitempty"`
	SortPredicate        string   `json:"sortPredicate,omitempty"`
	FilterLikePredicates []string `json:"filterLikePredicates,omitempty"`
	Classification       string   `json:"classification"`
	Reasons              []string `json:"reasons,omitempty"`
	Recommendations      []string `json:"recommendations,omitempty"`
}

type dynamodbKeyMetadata struct {
	Name         string
	PartitionKey string
	SortKey      string
	Projection   string
}

type dynamodbPredicate struct {
	Field string
	Text  string
	Kind  string
}

func dynamodbBuildAccessPathDetail(statement string, describe DescribeResult) dynamodbAccessPathDetail {
	table, index := dynamodbPartiQLTarget(statement)
	return dynamodbBuildAccessPathDetailForTarget(statement, describe, table, index)
}

func dynamodbBuildAccessPathDetailForTarget(statement string, describe DescribeResult, table, index string) dynamodbAccessPathDetail {
	tablePK, tableSK := dynamodbTableKeysFromDescribe(describe)
	indexes := dynamodbIndexesFromDescribe(describe)
	knownIndexes := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		if idx.Name != "" {
			knownIndexes = append(knownIndexes, idx.Name)
		}
	}
	sort.Strings(knownIndexes)

	detail := dynamodbAccessPathDetail{
		Kind:              "dynamodb-access-path",
		Table:             table,
		Index:             index,
		KnownIndexes:      knownIndexes,
		TablePartitionKey: tablePK,
		TableSortKey:      tableSK,
		Classification:    "scan_like",
		Recommendations: []string{
			"This is FutrixData metadata-based access-path analysis, not an AWS server-side EXPLAIN plan.",
		},
	}

	activePK := tablePK
	activeSK := tableSK
	if index != "" {
		idx, ok := findDynamoDBIndexMetadata(indexes, index)
		if !ok {
			detail.Classification = "unknown"
			detail.Reasons = append(detail.Reasons, "target index metadata not available")
			detail.Recommendations = append(detail.Recommendations, "Use describe_entity to confirm the index name, or query a known index listed in knownIndexes.")
			return detail
		}
		detail.IndexPartitionKey = idx.PartitionKey
		detail.IndexSortKey = idx.SortKey
		activePK = idx.PartitionKey
		activeSK = idx.SortKey
	}

	whereClause := dynamodbWhereClause(statement)
	hasTopLevelOr := dynamodbWhereHasTopLevelOr(whereClause)
	orPartitionLookup := hasTopLevelOr && dynamodbTopLevelOrIsPartitionKeyLookup(whereClause, activePK)
	predicates := dynamodbPredicatesForStatement(statement)
	for _, predicate := range predicates {
		switch {
		case identifiersEqual(predicate.Field, activePK) && (predicate.Kind == "eq" || predicate.Kind == "in"):
			if detail.PartitionPredicate == "" {
				detail.PartitionPredicate = predicate.Text
			}
		case activeSK != "" && identifiersEqual(predicate.Field, activeSK):
			if detail.SortPredicate == "" {
				detail.SortPredicate = predicate.Text
			}
		default:
			detail.FilterLikePredicates = append(detail.FilterLikePredicates, predicate.Text)
		}
	}
	if hasTopLevelOr && !orPartitionLookup && whereClause != "" {
		detail.FilterLikePredicates = append(detail.FilterLikePredicates, whereClause)
	}

	switch {
	case activePK == "":
		detail.Classification = "unknown"
		detail.Reasons = append(detail.Reasons, "partition key metadata not available")
	case hasTopLevelOr && orPartitionLookup:
		detail.Classification = "key_based"
		detail.Reasons = append(detail.Reasons, "partition key equality or IN predicate found in every OR branch")
	case hasTopLevelOr:
		detail.Classification = "scan_like"
		detail.Reasons = append(detail.Reasons, "OR predicates cannot be verified as a single DynamoDB key condition")
	case detail.PartitionPredicate != "":
		detail.Classification = "key_based"
		detail.Reasons = append(detail.Reasons, "partition key equality or IN predicate found")
	case detail.SortPredicate != "":
		detail.Classification = "scan_like"
		detail.Reasons = append(detail.Reasons, "sort key predicate without partition key equality looks scan-like")
	default:
		detail.Classification = "scan_like"
		detail.Reasons = append(detail.Reasons, "missing partition key equality or IN looks scan-like")
	}
	if detail.Classification != "key_based" {
		detail.Recommendations = append(detail.Recommendations, "Add equality or IN on the table/index partition key before relying on this statement for selective access.")
	}
	return detail
}

func (a *DynamoDBAdapter) dynamodbIndexTargetSuggestion(ctx context.Context, ds datasource.DataSource, statement string) *dynamodbIndexSuggestionDetail {
	if !dynamodbShouldProbeIndexTargetSuggestion(statement) {
		return nil
	}
	target := dynamodbPartiQLTargetInfoForStatement(statement)
	describe, err := a.DescribeEntity(ctx, ds, target.TableName)
	if err != nil {
		return nil
	}
	return dynamodbBuildIndexTargetSuggestion(statement, describe, target.TableName)
}

func dynamodbShouldProbeIndexTargetSuggestion(statement string) bool {
	target := dynamodbPartiQLTargetInfoForStatement(statement)
	if strings.TrimSpace(target.TableName) == "" || strings.TrimSpace(target.IndexName) != "" {
		return false
	}
	if !strings.EqualFold(leadingSQLKeyword(statement), "select") {
		return false
	}
	whereClause := dynamodbWhereClause(statement)
	if whereClause == "" || dynamodbWhereHasTopLevelOr(whereClause) {
		return false
	}
	predicates := dynamodbPredicatesForWhereClause(whereClause)
	lookupPredicates := 0
	for _, predicate := range predicates {
		if predicate.Kind == "eq" || predicate.Kind == "in" {
			lookupPredicates++
		}
	}
	return lookupPredicates >= 2
}

func dynamodbBuildIndexTargetSuggestion(statement string, describe DescribeResult, table string) *dynamodbIndexSuggestionDetail {
	tablePK, _ := dynamodbTableKeysFromDescribe(describe)
	indexes := dynamodbIndexesFromDescribe(describe)
	predicates := dynamodbPredicatesForStatement(statement)
	candidates := make([]dynamodbKeyMetadata, 0, 1)
	for _, idx := range indexes {
		if strings.TrimSpace(idx.Name) == "" || strings.TrimSpace(idx.PartitionKey) == "" {
			continue
		}
		if !strings.EqualFold(idx.Projection, "ALL") {
			continue
		}
		if identifiersEqual(idx.PartitionKey, tablePK) {
			continue
		}
		if dynamodbPredicatesContainPartitionLookup(predicates, idx.PartitionKey) {
			candidates = append(candidates, idx)
		}
	}
	if len(candidates) != 1 {
		return nil
	}
	idx := candidates[0]
	suggested, ok := dynamodbReplacePartiQLTarget(statement, table, idx.Name)
	if !ok {
		return nil
	}
	return &dynamodbIndexSuggestionDetail{
		Kind:               "dynamodb-index-target-suggestion",
		Table:              table,
		Index:              idx.Name,
		PartitionKey:       idx.PartitionKey,
		SuggestedStatement: suggested,
		Reason:             "The base-table SELECT filters by a known GSI partition key; querying the index target is likely more selective.",
	}
}

func dynamodbPredicatesContainPartitionLookup(predicates []dynamodbPredicate, partitionKey string) bool {
	for _, predicate := range predicates {
		if identifiersEqual(predicate.Field, partitionKey) && (predicate.Kind == "eq" || predicate.Kind == "in") {
			return true
		}
	}
	return false
}

func dynamodbReplacePartiQLTarget(statement, table, index string) (string, bool) {
	idx := dynamodbPartiQLFromPattern.FindStringSubmatchIndex(strings.TrimSpace(statement))
	if idx == nil || idx[2] < 0 || idx[3] < 0 {
		return "", false
	}
	trimmed := strings.TrimSpace(statement)
	start := idx[2]
	end := idx[3]
	if idx[10] >= 0 && idx[11] >= 0 {
		end = idx[11]
	}
	target := dynamodbQuotePartiQLIdentifier(table)
	if strings.TrimSpace(index) != "" {
		target += "." + dynamodbQuotePartiQLIdentifier(index)
	}
	return trimmed[:start] + target + trimmed[end:], true
}

func dynamodbTableKeysFromDescribe(describe DescribeResult) (partitionKey string, sortKey string) {
	for _, item := range describe.Details {
		switch strings.ToLower(strings.TrimSpace(item.Label)) {
		case "partition key":
			partitionKey = strings.TrimSpace(fmt.Sprint(item.Value))
		case "sort key":
			sortKey = strings.TrimSpace(fmt.Sprint(item.Value))
		}
	}
	return partitionKey, sortKey
}

func dynamodbIndexesFromDescribe(describe DescribeResult) []dynamodbKeyMetadata {
	indexes := make([]dynamodbKeyMetadata, 0, len(describe.Indexes))
	for _, idx := range describe.Indexes {
		meta := dynamodbKeyMetadata{Name: strings.TrimSpace(idx.Name)}
		for _, part := range strings.Split(idx.Definition, "|") {
			pieces := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(pieces) != 2 {
				continue
			}
			key := strings.TrimSpace(pieces[0])
			value := strings.ToUpper(strings.TrimSpace(pieces[1]))
			switch value {
			case "HASH":
				meta.PartitionKey = key
			case "RANGE":
				meta.SortKey = key
			default:
				if strings.EqualFold(key, "projection") {
					meta.Projection = value
				}
			}
		}
		indexes = append(indexes, meta)
	}
	return indexes
}

func findDynamoDBIndexMetadata(indexes []dynamodbKeyMetadata, name string) (dynamodbKeyMetadata, bool) {
	for _, idx := range indexes {
		if strings.EqualFold(strings.TrimSpace(idx.Name), strings.TrimSpace(name)) {
			return idx, true
		}
	}
	return dynamodbKeyMetadata{}, false
}

var (
	dynamodbFieldExprPattern  = regexp.MustCompile(`(?is)(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*(=|\bin\b|\bbetween\b|<=|>=|<|>)\s*(.*)`)
	dynamodbBeginsWithPattern = regexp.MustCompile(`(?is)\bbegins_with\s*\(\s*(?:"([^"]+)"|` + "`([^`]+)`" + `|([A-Za-z_][A-Za-z0-9_$.]*))\s*,`)
)

func dynamodbPredicatesForStatement(statement string) []dynamodbPredicate {
	whereClause := dynamodbWhereClause(statement)
	return dynamodbPredicatesForWhereClause(whereClause)
}

func dynamodbPredicatesForWhereClause(whereClause string) []dynamodbPredicate {
	if whereClause == "" {
		return nil
	}
	parts := splitDynamoDBWherePredicates(whereClause)
	predicates := make([]dynamodbPredicate, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if match := dynamodbBeginsWithPattern.FindStringSubmatch(part); len(match) > 0 {
			predicates = append(predicates, dynamodbPredicate{
				Field: firstNonEmptyString(match[1], match[2], match[3]),
				Text:  part,
				Kind:  "begins_with",
			})
			continue
		}
		match := dynamodbFieldExprPattern.FindStringSubmatch(part)
		if len(match) == 0 {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(match[4]))
		switch kind {
		case "=":
			kind = "eq"
		case "in":
			kind = "in"
		case "between":
			kind = "between"
		default:
			kind = "comparison"
		}
		predicates = append(predicates, dynamodbPredicate{
			Field: firstNonEmptyString(match[1], match[2], match[3]),
			Text:  part,
			Kind:  kind,
		})
	}
	return predicates
}

func dynamodbWhereClause(statement string) string {
	trimmed := strings.TrimSpace(statement)
	whereStart := findDynamoDBTopLevelKeyword(trimmed, "where")
	if whereStart < 0 {
		return ""
	}
	tail := trimmed[whereStart+len("where"):]
	end := len(tail)
	for _, keyword := range []string{"order by", "limit", "group by"} {
		if idx := findDynamoDBTopLevelKeyword(tail, keyword); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(tail[:end])
}

func findDynamoDBTopLevelKeyword(input, keyword string) int {
	lower := strings.ToLower(input)
	needle := strings.ToLower(keyword)
	inQuote := byte(0)
	depth := 0
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = ch
			continue
		case '(', '[':
			depth++
			continue
		case ')', ']':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 && strings.HasPrefix(lower[i:], needle) && dynamodbKeywordBoundary(input, i, len(needle)) {
			return i
		}
	}
	return -1
}

func dynamodbKeywordBoundary(input string, start, length int) bool {
	before := start - 1
	after := start + length
	return (before < 0 || !isDynamoDBIdentifierByte(input[before])) &&
		(after >= len(input) || !isDynamoDBIdentifierByte(input[after]))
}

func isDynamoDBIdentifierByte(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

func splitDynamoDBWherePredicates(whereClause string) []string {
	parts := make([]string, 0)
	var b strings.Builder
	inQuote := byte(0)
	depth := 0
	lower := strings.ToLower(whereClause)
	for i := 0; i < len(whereClause); i++ {
		ch := whereClause[i]
		if inQuote != 0 {
			b.WriteByte(ch)
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = ch
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(lower[i:], "and") && dynamodbKeywordBoundary(whereClause, i, len("and")) && !previousTokenIsBetween(b.String()) {
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
			i += len("and") - 1
			continue
		}
		b.WriteByte(ch)
	}
	if tail := strings.TrimSpace(b.String()); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func dynamodbWhereHasTopLevelOr(whereClause string) bool {
	return findDynamoDBTopLevelKeyword(whereClause, "or") >= 0
}

func dynamodbTopLevelOrIsPartitionKeyLookup(whereClause, partitionKey string) bool {
	if strings.TrimSpace(partitionKey) == "" {
		return false
	}
	parts := splitDynamoDBTopLevelKeyword(whereClause, "or")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		part = stripDynamoDBOuterParens(strings.TrimSpace(part))
		if part == "" {
			return false
		}
		predicates := dynamodbPredicatesForWhereClause(part)
		matchesPartition := false
		for _, predicate := range predicates {
			if identifiersEqual(predicate.Field, partitionKey) && (predicate.Kind == "eq" || predicate.Kind == "in") {
				matchesPartition = true
				break
			}
		}
		if !matchesPartition {
			return false
		}
	}
	return true
}

func splitDynamoDBTopLevelKeyword(input, keyword string) []string {
	needle := strings.ToLower(keyword)
	parts := make([]string, 0, 2)
	start := 0
	inQuote := byte(0)
	depth := 0
	lower := strings.ToLower(input)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = ch
			continue
		case '(', '[':
			depth++
			continue
		case ')', ']':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 && strings.HasPrefix(lower[i:], needle) && dynamodbKeywordBoundary(input, i, len(needle)) {
			parts = append(parts, strings.TrimSpace(input[start:i]))
			i += len(needle) - 1
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}

func stripDynamoDBOuterParens(input string) string {
	for {
		trimmed := strings.TrimSpace(input)
		if len(trimmed) < 2 || trimmed[0] != '(' || trimmed[len(trimmed)-1] != ')' {
			return trimmed
		}
		inQuote := byte(0)
		depth := 0
		wraps := true
		for i := 0; i < len(trimmed); i++ {
			ch := trimmed[i]
			if inQuote != 0 {
				if ch == inQuote {
					inQuote = 0
				}
				continue
			}
			switch ch {
			case '\'', '"', '`':
				inQuote = ch
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
				if depth == 0 && i != len(trimmed)-1 {
					wraps = false
				}
			}
		}
		if !wraps || depth != 0 {
			return trimmed
		}
		input = trimmed[1 : len(trimmed)-1]
	}
}

func previousTokenIsBetween(current string) bool {
	fields := strings.Fields(strings.ToLower(current))
	for i := len(fields) - 1; i >= 0; i-- {
		if fields[i] == "between" {
			return true
		}
		if fields[i] == "and" {
			return false
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeDynamoDBIdentifier(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "\"`"))
}

func identifiersEqual(left, right string) bool {
	return normalizeDynamoDBIdentifier(left) == normalizeDynamoDBIdentifier(right)
}
