package console

import (
	"context"
	"errors"

	"futrixdata/platform/internal/datasource"
)

var (
	ErrAdapterNotFound = errors.New("adapter not found")
	ErrUnsupported     = errors.New("operation not supported")
)

type ListOptions struct {
	Pattern string
	Limit   int
}

type ExecuteOptions struct {
	PagingToken     string
	PageSize        int
	Bounds          ExecuteBounds
	RequestedBounds ExecuteBounds
	ClampedLimits   map[string]bool
	approvalMode    executeApprovalMode
}

type ExecuteBounds struct {
	MaxReturnedRows   int  `json:"maxReturnedRows,omitempty"`
	MaxPages          int  `json:"maxPages,omitempty"`
	MaxEvaluatedItems int  `json:"maxEvaluatedItems,omitempty"`
	StrictLimits      bool `json:"strictLimits,omitempty"`
}

func (b ExecuteBounds) Enabled() bool {
	return b.MaxReturnedRows > 0 || b.MaxPages > 0 || b.MaxEvaluatedItems > 0
}

func (o *ExecuteOptions) CaptureRequestedBounds() {
	if o == nil || !o.Bounds.Enabled() || o.RequestedBounds.Enabled() {
		return
	}
	o.RequestedBounds = o.Bounds
}

func (o *ExecuteOptions) AddClampedLimit(name string) {
	if o == nil || name == "" {
		return
	}
	if o.ClampedLimits == nil {
		o.ClampedLimits = map[string]bool{}
	}
	o.ClampedLimits[name] = true
}

func (o ExecuteOptions) RequestedExecutionBounds() ExecuteBounds {
	if o.RequestedBounds.Enabled() {
		return o.RequestedBounds
	}
	return o.Bounds
}

type executeApprovalMode uint8

const (
	executeApprovalModeNone executeApprovalMode = iota
	executeApprovalModeInteractive
)

func markInteractiveApproval(opts *ExecuteOptions) {
	if opts == nil {
		return
	}
	opts.approvalMode = executeApprovalModeInteractive
}

func AllowsInteractiveApprovalBypass(o ExecuteOptions) bool {
	return o.approvalMode == executeApprovalModeInteractive
}

type ExecuteRiskInfo struct {
	Action            string             `json:"action"`
	Level             string             `json:"level"`
	Reasons           []string           `json:"reasons,omitempty"`
	RuleID            string             `json:"ruleId,omitempty"`
	RuleCode          string             `json:"ruleCode,omitempty"`
	RuleDescription   string             `json:"ruleDescription,omitempty"`
	SuggestedRewrites []SuggestedRewrite `json:"suggestedRewrites,omitempty"`
	// Builtin — true when the matched rule is engine-shipped. Mirrors
	// riskengine.RiskAssessment.Builtin so downstream audit attribution can
	// disambiguate user/builtin rule-ID collisions.
	Builtin      bool           `json:"builtin,omitempty"`
	TargetEntity string         `json:"targetEntity,omitempty"`
	Explain      *ExplainResult `json:"explain,omitempty"`
}

type SuggestedRewrite struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	RewriteHint      string   `json:"rewriteHint,omitempty"`
	SuggestedTools   []string `json:"suggestedTools,omitempty"`
	RequiresApproval bool     `json:"requiresApproval,omitempty"`
}

type ExecuteRiskInfoProvider interface {
	error
	ExecuteRiskInfo() ExecuteRiskInfo
}

func RiskInfoFromError(err error) (ExecuteRiskInfo, bool) {
	if err == nil {
		return ExecuteRiskInfo{}, false
	}
	var provider ExecuteRiskInfoProvider
	if errors.As(err, &provider) {
		return provider.ExecuteRiskInfo(), true
	}
	return ExecuteRiskInfo{}, false
}

type ColumnInfo struct {
	Name         string `json:"name"`
	DataType     string `json:"dataType"`
	Nullable     string `json:"nullable"`
	DefaultValue any    `json:"defaultValue,omitempty"`
}

type IndexInfo struct {
	Name       string `json:"name"`
	Column     string `json:"column,omitempty"`
	Unique     bool   `json:"unique"`
	Definition string `json:"definition,omitempty"`
}

type DetailItem struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type DescribeResult struct {
	Columns       []ColumnInfo `json:"columns"`
	Indexes       []IndexInfo  `json:"indexes"`
	Details       []DetailItem `json:"details,omitempty"`
	EntityKind    string       `json:"entityKind,omitempty"`
	DefinitionSQL string       `json:"definitionSql,omitempty"`
	Preview       any          `json:"preview,omitempty"`
	Dialect       string       `json:"dialect,omitempty"`
	Environment   string       `json:"environment,omitempty"`
}

type ResultColumnOrigin struct {
	Schema string `json:"schema,omitempty"`
	Alias  string `json:"alias,omitempty"`
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
}

type ResultColumn struct {
	Key              string               `json:"key"`
	Name             string               `json:"name"`
	Position         int                  `json:"position"`
	SourceKind       string               `json:"sourceKind,omitempty"`
	Origins          []ResultColumnOrigin `json:"origins,omitempty"`
	ConservativeMask bool                 `json:"conservativeMask,omitempty"`
	Masked           bool                 `json:"masked,omitempty"`
}

type QueryResult struct {
	Columns              []string         `json:"columns"`
	Rows                 []map[string]any `json:"rows"`
	ColumnMeta           []ResultColumn   `json:"columnMeta,omitempty"`
	RowValues            [][]any          `json:"rowValues,omitempty"`
	RowCount             int64            `json:"rowCount"`
	HasMore              bool             `json:"hasMore"`
	NextToken            string           `json:"nextToken"`
	PrevToken            string           `json:"prevToken"`
	ElapsedMs            int64            `json:"elapsedMs"`
	Detail               any              `json:"detail,omitempty"`
	SourceEntity         string           `json:"sourceEntity,omitempty"`
	MaskedColumns        []string         `json:"maskedColumns,omitempty"`
	RiskInfo             *ExecuteRiskInfo `json:"riskInfo,omitempty"`
	RequestedPageSize    int              `json:"requestedPageSize"`
	EffectivePageSize    int              `json:"effectivePageSize"`
	EffectiveLimitSource string           `json:"effectiveLimitSource,omitempty"`
	Dialect              string           `json:"dialect,omitempty"`
	Environment          string           `json:"environment,omitempty"`
}

type ExplainResult struct {
	UsesIndex         bool     `json:"usesIndex"`
	Indexes           []string `json:"indexes,omitempty"`
	Stages            []string `json:"stages,omitempty"`
	TotalKeysExamined int64    `json:"totalKeysExamined,omitempty"`
	TotalDocsExamined int64    `json:"totalDocsExamined,omitempty"`
	MaxSeqScanRows    int64    `json:"maxSeqScanRows,omitempty"`
	TotalCost         float64  `json:"totalCost,omitempty"`
	Detail            any      `json:"detail"`
}

type Adapter interface {
	TestConnection(ctx context.Context, ds datasource.DataSource) error
	ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error)
	DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error)
	Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error)
	Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error)
}

type EntityPage struct {
	Items   []string                  `json:"items"`
	Cursor  string                    `json:"cursor"`
	Done    bool                      `json:"done"`
	Details map[string]DescribeResult `json:"details,omitempty"`
	Kinds   map[string]string         `json:"kinds,omitempty"`
}

type EntityPager interface {
	ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts ListOptions, cursor string) (EntityPage, error)
}

type DatabaseLister interface {
	ListDatabases(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error)
}

type KeyScanner interface {
	ScanKeys(ctx context.Context, ds datasource.DataSource, pattern string, cursor RedisScanCursor) ([]string, RedisScanCursor, bool, error)
}

// RedisKeyMetaProvider lets the Wails facade ask an adapter for batched
// type/ttl/size of a key set in a single pair of pipeline round-trips.
type RedisKeyMetaProvider interface {
	GetKeyMeta(ctx context.Context, ds datasource.DataSource, keys []string) (map[string]RedisKeyMetaItem, error)
}

type RedisCommandExecutor interface {
	ExecuteRedisCommand(ctx context.Context, ds datasource.DataSource, args []string, opts ExecuteOptions) (QueryResult, error)
}

type DatasourceSecretResolver interface {
	ResolveDatasource(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error)
}

// ExecuteInterceptor is called before every Execute to enforce risk control.
// If BeforeExecute returns a non-nil error, the execution is blocked.
type ExecuteInterceptor interface {
	BeforeExecute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) error
}

// ExecuteOptionsCapper can reduce caller-supplied execution budgets before
// risk enforcement and adapter execution.
type ExecuteOptionsCapper interface {
	ApplyExecuteOptionsCaps(ctx context.Context, ds datasource.DataSource, statement string, opts *ExecuteOptions) error
}

type Manager struct {
	adapters       map[datasource.DataSourceType]Adapter
	interceptor    ExecuteInterceptor
	secretResolver DatasourceSecretResolver
}

func NewManager() *Manager {
	return &Manager{adapters: make(map[datasource.DataSourceType]Adapter)}
}

// SetInterceptor sets the pre-execution interceptor. Pass nil to disable.
func (m *Manager) SetInterceptor(i ExecuteInterceptor) {
	m.interceptor = i
}

func (m *Manager) SetDatasourceSecretResolver(resolver DatasourceSecretResolver) {
	m.secretResolver = resolver
}

func (m *Manager) Register(typ datasource.DataSourceType, adapter Adapter) {
	m.adapters[typ] = adapter
}

func (m *Manager) AdapterFor(typ datasource.DataSourceType) (Adapter, error) {
	adapter, ok := m.adapters[typ]
	if !ok {
		return nil, ErrAdapterNotFound
	}
	return adapter, nil
}

func (m *Manager) TestConnection(ctx context.Context, ds datasource.DataSource) error {
	done := DatasourceTimingStage(ctx, "manager.test_connection.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.test_connection.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.test_connection.adapter_call")
	err = adapter.TestConnection(ctx, ds)
	done(DatasourceTimingStatusFields(err)...)
	return err
}

func (m *Manager) ListEntities(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	done := DatasourceTimingStage(ctx, "manager.list_entities.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.list_entities.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.list_entities.adapter_call")
	items, err := adapter.ListEntities(ctx, ds, opts)
	done(DatasourceTimingStatusFields(err, DatasourceTimingKV("items", len(items)))...)
	return items, err
}

func (m *Manager) ListEntitiesPage(ctx context.Context, ds datasource.DataSource, opts ListOptions, cursor string) (EntityPage, error) {
	done := DatasourceTimingStage(ctx, "manager.list_entities_page.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return EntityPage{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.list_entities_page.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return EntityPage{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	pager, ok := adapter.(EntityPager)
	if !ok {
		return EntityPage{}, ErrUnsupported
	}
	done = DatasourceTimingStage(ctx, "manager.list_entities_page.adapter_call")
	page, err := pager.ListEntitiesPage(ctx, ds, opts, cursor)
	done(DatasourceTimingStatusFields(err, DatasourceTimingKV("items", len(page.Items)), DatasourceTimingKV("done", page.Done))...)
	return page, err
}

func (m *Manager) DescribeEntity(ctx context.Context, ds datasource.DataSource, name string) (DescribeResult, error) {
	done := DatasourceTimingStage(ctx, "manager.describe.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return DescribeResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.describe.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return DescribeResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.describe.adapter_call")
	result, err := adapter.DescribeEntity(ctx, ds, name)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return DescribeResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("columns", len(result.Columns)), DatasourceTimingKV("indexes", len(result.Indexes)))
	return annotateDescribeContext(result, ds), nil
}

func (m *Manager) Execute(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	// Resolve external secrets only after the interceptor has had a chance to
	// deny the statement. Risk evaluation works on statement text and datasource
	// metadata, not the resolved credential, so a statically high-risk statement
	// must be blocked without reading the secret — and a blocked statement should
	// not surface a secret-resolution error when the provider is unavailable.
	// Probes the interceptor runs resolve independently via Explain/DescribeEntity.
	done := DatasourceTimingStage(ctx, "manager.apply_execute_caps")
	if err := m.ApplyExecuteOptionsCaps(ctx, ds, statement, &opts); err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	if m.interceptor != nil {
		done = DatasourceTimingStage(ctx, "manager.risk_guard")
		if err := m.interceptor.BeforeExecute(ctx, ds, statement, opts); err != nil {
			done(DatasourceTimingStatusFields(err)...)
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"))
	}
	done = DatasourceTimingStage(ctx, "manager.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	return m.executeInternalResolved(ctx, ds, statement, opts)
}

func (m *Manager) ApplyExecuteOptionsCaps(ctx context.Context, ds datasource.DataSource, statement string, opts *ExecuteOptions) error {
	if capper, ok := m.interceptor.(ExecuteOptionsCapper); ok {
		return capper.ApplyExecuteOptionsCaps(ctx, ds, statement, opts)
	}
	return nil
}

// ExecuteWithInteractiveApproval is reserved for in-app user-confirmed
// executions such as the Console confirm flow. Programmatic callers such as
// CLI, MCP, and datasource services must use Execute.
func (m *Manager) ExecuteWithInteractiveApproval(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	markInteractiveApproval(&opts)
	return m.Execute(ctx, ds, statement, opts)
}

func (m *Manager) ExecuteRedisCommand(ctx context.Context, ds datasource.DataSource, args []string, opts ExecuteOptions) (QueryResult, error) {
	statement, err := RedisCommandStatement(args)
	if err != nil {
		return QueryResult{}, err
	}
	// Mirror Execute: run option caps and the interceptor on the unresolved
	// datasource so a blocked command never triggers secret resolution (which
	// could fail or surface a provider error for a statement we already reject).
	// Risk evaluation only needs statement text and datasource metadata.
	done := DatasourceTimingStage(ctx, "manager.apply_execute_caps")
	if err := m.ApplyExecuteOptionsCaps(ctx, ds, statement, &opts); err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	if m.interceptor != nil {
		done = DatasourceTimingStage(ctx, "manager.risk_guard")
		if err := m.interceptor.BeforeExecute(ctx, ds, statement, opts); err != nil {
			done(DatasourceTimingStatusFields(err)...)
			return QueryResult{}, err
		}
		done(DatasourceTimingKV("status", "ok"))
	}
	done = DatasourceTimingStage(ctx, "manager.resolve_datasource")
	ds, err = m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	return m.executeRedisCommandInternalResolved(ctx, ds, args, statement, opts)
}

func (m *Manager) ExecuteRedisCommandWithInteractiveApproval(ctx context.Context, ds datasource.DataSource, args []string, opts ExecuteOptions) (QueryResult, error) {
	markInteractiveApproval(&opts)
	return m.ExecuteRedisCommand(ctx, ds, args, opts)
}

func (m *Manager) ExecuteInternal(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		return QueryResult{}, err
	}
	return m.executeInternalResolved(ctx, ds, statement, opts)
}

func (m *Manager) executeInternalResolved(ctx context.Context, ds datasource.DataSource, statement string, opts ExecuteOptions) (QueryResult, error) {
	done := DatasourceTimingStage(ctx, "manager.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.adapter_execute")
	result, err := adapter.Execute(ctx, ds, statement, opts)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return result, err
	}
	done(
		DatasourceTimingKV("status", "ok"),
		DatasourceTimingKV("row_count", result.RowCount),
		DatasourceTimingKV("columns", len(result.Columns)),
		DatasourceTimingKV("has_more", result.HasMore),
	)
	return annotateQueryContext(result, ds, statement, opts), nil
}

func (m *Manager) ExecuteRedisCommandInternal(ctx context.Context, ds datasource.DataSource, args []string, statement string, opts ExecuteOptions) (QueryResult, error) {
	done := DatasourceTimingStage(ctx, "manager.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	return m.executeRedisCommandInternalResolved(ctx, ds, args, statement, opts)
}

func (m *Manager) executeRedisCommandInternalResolved(ctx context.Context, ds datasource.DataSource, args []string, statement string, opts ExecuteOptions) (QueryResult, error) {
	done := DatasourceTimingStage(ctx, "manager.redis_command.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return QueryResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	executor, ok := adapter.(RedisCommandExecutor)
	if !ok {
		return QueryResult{}, ErrUnsupported
	}
	done = DatasourceTimingStage(ctx, "manager.redis_command.adapter_execute")
	result, err := executor.ExecuteRedisCommand(ctx, ds, args, opts)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return result, err
	}
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("row_count", result.RowCount), DatasourceTimingKV("columns", len(result.Columns)))
	return annotateQueryContext(result, ds, statement, opts), nil
}

func (m *Manager) Explain(ctx context.Context, ds datasource.DataSource, statement string) (ExplainResult, error) {
	done := DatasourceTimingStage(ctx, "manager.explain.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return ExplainResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.explain.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return ExplainResult{}, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.explain.adapter_call")
	result, err := adapter.Explain(ctx, ds, statement)
	if err != nil {
		done(DatasourceTimingStatusFields(err)...)
		return ExplainResult{}, err
	}
	done(
		DatasourceTimingKV("status", "ok"),
		DatasourceTimingKV("uses_index", result.UsesIndex),
		DatasourceTimingKV("stages", len(result.Stages)),
		DatasourceTimingKV("indexes", len(result.Indexes)),
	)
	return result, nil
}

func (m *Manager) ListDatabases(ctx context.Context, ds datasource.DataSource, opts ListOptions) ([]string, error) {
	done := DatasourceTimingStage(ctx, "manager.list_databases.resolve_datasource")
	ds, err := m.resolveDatasource(ctx, ds)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(DatasourceTimingKV("status", "ok"))
	done = DatasourceTimingStage(ctx, "manager.list_databases.adapter_lookup")
	adapter, err := m.AdapterFor(ds.Type)
	if err != nil {
		done(DatasourceTimingKV("status", "error"))
		return nil, err
	}
	done(DatasourceTimingKV("status", "ok"))
	lister, ok := adapter.(DatabaseLister)
	if !ok {
		return nil, ErrUnsupported
	}
	done = DatasourceTimingStage(ctx, "manager.list_databases.adapter_call")
	items, err := lister.ListDatabases(ctx, ds, opts)
	done(DatasourceTimingStatusFields(err, DatasourceTimingKV("items", len(items)))...)
	return items, err
}

func (m *Manager) resolveDatasource(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	if m == nil || m.secretResolver == nil {
		return ds, nil
	}
	return m.secretResolver.ResolveDatasource(ctx, ds)
}

// ResolveDatasource exposes secret resolution for callers that fetch outside the
// adapter dispatch path (e.g. Redis command-docs/autocomplete refresh), so a
// SecretRef-backed datasource connects with its resolved credentials instead of
// the redacted/empty values held in the stored record.
func (m *Manager) ResolveDatasource(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	return m.resolveDatasource(ctx, ds)
}
