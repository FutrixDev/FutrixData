package riskengine

import (
	"context"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
)

// DefaultMaxExamined is the threshold for examined rows/keys before escalating risk.
const DefaultMaxExamined int64 = 1000

// DefaultSeqScanRowsThreshold is the community-standard threshold (10,000 rows)
// below which a sequential/full table scan is considered normal optimizer behavior.
const DefaultSeqScanRowsThreshold int64 = 10000

// DefaultCostThreshold is the community-standard Total Cost threshold (1,000)
// below which a query plan is considered low-risk regardless of access method.
const DefaultCostThreshold float64 = 1000

// Default DynamoDB risk thresholds. The console adapter no longer enforces a
// hard cap; these defaults stay in the risk engine so probe rules continue to
// flag runaway pagination unless the user opts out via rule configuration.
const (
	DefaultMaxDynamoDBPages          int = 20
	DefaultMaxDynamoDBEvaluatedItems int = 5000
)

type ProbePolicy struct {
	MaxExaminedRows           int64
	MaxJoinCount              int
	MaxFullScans              int
	MaxEstimatedJoinRows      int64
	SeqScanRowsThreshold      int64
	CostThreshold             float64
	MaxDynamoDBPages          int
	MaxDynamoDBEvaluatedItems int
	// AllowSafeSeqScan controls whether small-table sequential scans are
	// exempt from the "no index detected" warning. Default: true.
	AllowSafeSeqScan bool
}

func DefaultProbePolicy() ProbePolicy {
	return ProbePolicy{
		MaxExaminedRows:           DefaultMaxExamined,
		MaxJoinCount:              4,
		MaxFullScans:              1,
		MaxEstimatedJoinRows:      DefaultMaxExamined * 10,
		SeqScanRowsThreshold:      DefaultSeqScanRowsThreshold,
		CostThreshold:             DefaultCostThreshold,
		MaxDynamoDBPages:          DefaultMaxDynamoDBPages,
		MaxDynamoDBEvaluatedItems: DefaultMaxDynamoDBEvaluatedItems,
		AllowSafeSeqScan:          true,
	}
}

func (p ProbePolicy) normalized() ProbePolicy {
	defaults := DefaultProbePolicy()
	if p.MaxExaminedRows <= 0 {
		p.MaxExaminedRows = defaults.MaxExaminedRows
	}
	if p.MaxJoinCount <= 0 {
		p.MaxJoinCount = defaults.MaxJoinCount
	}
	if p.MaxFullScans <= 0 {
		p.MaxFullScans = defaults.MaxFullScans
	}
	if p.MaxEstimatedJoinRows <= 0 {
		p.MaxEstimatedJoinRows = defaults.MaxEstimatedJoinRows
	}
	if p.SeqScanRowsThreshold <= 0 {
		p.SeqScanRowsThreshold = defaults.SeqScanRowsThreshold
	}
	if p.CostThreshold <= 0 {
		p.CostThreshold = defaults.CostThreshold
	}
	if p.MaxDynamoDBPages <= 0 {
		p.MaxDynamoDBPages = defaults.MaxDynamoDBPages
	}
	if p.MaxDynamoDBEvaluatedItems <= 0 {
		p.MaxDynamoDBEvaluatedItems = defaults.MaxDynamoDBEvaluatedItems
	}
	return p
}

// ProbeProvider can execute EXPLAIN and DescribeEntity for dynamic risk assessment.
type ProbeProvider interface {
	Explain(ctx context.Context, ds datasource.DataSource, statement string) (console.ExplainResult, error)
	DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (console.DescribeResult, error)
}

type ViewProbeResult struct {
	ViewEntity    string
	EntityKind    string
	DefinitionSQL string
	BaseEntities  []string
	EntityNameMap map[string]string
	InnerFacts    console.SQLRiskFacts
}

// ProbeResult holds the outcome of a probe operation.
type ProbeResult struct {
	ExplainResult  *console.ExplainResult
	ExplainErr     error
	ExplainSkipped bool
	DescribeResult *console.DescribeResult
	DescribeEntity string
	DescribeErr    error
	ViewResult     *ViewProbeResult
	ViewParseErr   error
}
