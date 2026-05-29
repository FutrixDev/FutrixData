package console

import (
	"context"
	"errors"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestExecuteOptionsDefaults(t *testing.T) {
	opt := ExecuteOptions{}
	if opt.PageSize != 0 {
		t.Fatalf("expected default 0 pageSize, got %d", opt.PageSize)
	}
}

type stubManagerAdapter struct {
	result QueryResult
}

func (a stubManagerAdapter) TestConnection(context.Context, datasource.DataSource) error {
	return nil
}

func (a stubManagerAdapter) ListEntities(context.Context, datasource.DataSource, ListOptions) ([]string, error) {
	return nil, nil
}

func (a stubManagerAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (DescribeResult, error) {
	return DescribeResult{}, nil
}

func (a stubManagerAdapter) Execute(context.Context, datasource.DataSource, string, ExecuteOptions) (QueryResult, error) {
	return a.result, nil
}

func (a stubManagerAdapter) ExecuteRedisCommand(context.Context, datasource.DataSource, []string, ExecuteOptions) (QueryResult, error) {
	return a.result, nil
}

func (a stubManagerAdapter) Explain(context.Context, datasource.DataSource, string) (ExplainResult, error) {
	return ExplainResult{}, nil
}

type countingExecuteInterceptor struct {
	calls int
}

func (i *countingExecuteInterceptor) BeforeExecute(context.Context, datasource.DataSource, string, ExecuteOptions) error {
	i.calls++
	return nil
}

type rejectingExecuteCapper struct {
	err         error
	beforeCalls int
}

func (i *rejectingExecuteCapper) ApplyExecuteOptionsCaps(context.Context, datasource.DataSource, string, *ExecuteOptions) error {
	return i.err
}

func (i *rejectingExecuteCapper) BeforeExecute(context.Context, datasource.DataSource, string, ExecuteOptions) error {
	i.beforeCalls++
	return nil
}

type resolverFunc func(context.Context, datasource.DataSource) (datasource.DataSource, error)

func (f resolverFunc) ResolveDatasource(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	return f(ctx, ds)
}

type passwordCheckingAdapter struct {
	wantPassword string
	called       bool
}

func (a *passwordCheckingAdapter) TestConnection(_ context.Context, ds datasource.DataSource) error {
	a.called = true
	if ds.Password != a.wantPassword {
		return errors.New("password was not resolved before adapter call")
	}
	return nil
}

func (a *passwordCheckingAdapter) ListEntities(context.Context, datasource.DataSource, ListOptions) ([]string, error) {
	return nil, nil
}

func (a *passwordCheckingAdapter) DescribeEntity(context.Context, datasource.DataSource, string) (DescribeResult, error) {
	return DescribeResult{}, nil
}

func (a *passwordCheckingAdapter) Execute(context.Context, datasource.DataSource, string, ExecuteOptions) (QueryResult, error) {
	return QueryResult{}, nil
}

func (a *passwordCheckingAdapter) Explain(context.Context, datasource.DataSource, string) (ExplainResult, error) {
	return ExplainResult{}, nil
}

func TestManagerResolvesDatasourceSecretsBeforeAdapterCall(t *testing.T) {
	manager := NewManager()
	adapter := &passwordCheckingAdapter{wantPassword: "resolved-secret"}
	manager.Register(datasource.TypePostgreSQL, adapter)
	manager.SetDatasourceSecretResolver(resolverFunc(func(_ context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
		ds.Password = "resolved-secret"
		return ds, nil
	}))

	if err := manager.TestConnection(context.Background(), datasource.DataSource{Type: datasource.TypePostgreSQL}); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !adapter.called {
		t.Fatalf("expected adapter to be called")
	}
}

type passwordCheckingBatchAdapter struct {
	passwordCheckingAdapter
}

func (a *passwordCheckingBatchAdapter) ExecuteBatch(_ context.Context, ds datasource.DataSource, batchID string, _ []RedisBatchOperation) (RedisBatchResult, error) {
	a.called = true
	if ds.Password != a.wantPassword {
		return RedisBatchResult{}, errors.New("password was not resolved before batch adapter call")
	}
	return RedisBatchResult{BatchID: batchID}, nil
}

func TestManagerExecuteRedisBatchResolvesSecretsBeforeAdapterCall(t *testing.T) {
	manager := NewManager()
	adapter := &passwordCheckingBatchAdapter{passwordCheckingAdapter{wantPassword: "resolved-secret"}}
	manager.Register(datasource.TypeRedis, adapter)
	manager.SetDatasourceSecretResolver(resolverFunc(func(_ context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
		ds.Password = "resolved-secret"
		return ds, nil
	}))

	ops := []RedisBatchOperation{{Command: "GET", Args: []string{"k"}}}
	if _, err := manager.ExecuteRedisBatch(context.Background(), datasource.DataSource{Type: datasource.TypeRedis}, "batch-1", ops); err != nil {
		t.Fatalf("ExecuteRedisBatch: %v", err)
	}
	if !adapter.called {
		t.Fatalf("expected batch adapter to be called")
	}
}

func TestManagerResolveDatasourceExposesResolverForDirectFetches(t *testing.T) {
	manager := NewManager()
	manager.SetDatasourceSecretResolver(resolverFunc(func(_ context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
		ds.Password = "resolved-secret"
		return ds, nil
	}))

	got, err := manager.ResolveDatasource(context.Background(), datasource.DataSource{Type: datasource.TypeRedis})
	if err != nil {
		t.Fatalf("ResolveDatasource: %v", err)
	}
	if got.Password != "resolved-secret" {
		t.Fatalf("password = %q; want resolved-secret (Redis docs path must resolve secrets)", got.Password)
	}

	// Without a resolver the datasource passes through unchanged.
	bare := NewManager()
	passthrough, err := bare.ResolveDatasource(context.Background(), datasource.DataSource{Type: datasource.TypeRedis, Password: "literal"})
	if err != nil {
		t.Fatalf("ResolveDatasource (no resolver): %v", err)
	}
	if passthrough.Password != "literal" {
		t.Fatalf("password = %q; want literal", passthrough.Password)
	}
}

func TestManagerExecute_InterceptsRequestsByDefault(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeMySQL, stubManagerAdapter{result: QueryResult{RowCount: 1}})
	interceptor := &countingExecuteInterceptor{}
	manager.SetInterceptor(interceptor)
	ds := datasource.DataSource{Type: datasource.TypeMySQL}

	if _, err := manager.Execute(context.Background(), ds, "SELECT 1", ExecuteOptions{}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if interceptor.calls != 1 {
		t.Fatalf("expected execute to hit interceptor once, got %d calls", interceptor.calls)
	}
}

func TestManagerExecuteRedisCommand_PropagatesCapperError(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeRedis, stubManagerAdapter{result: QueryResult{RowCount: 1}})
	capperErr := errors.New("execution limits rejected")
	interceptor := &rejectingExecuteCapper{err: capperErr}
	manager.SetInterceptor(interceptor)
	ds := datasource.DataSource{Type: datasource.TypeRedis}

	_, err := manager.ExecuteRedisCommand(context.Background(), ds, []string{"GET", "cache:key"}, ExecuteOptions{})
	if !errors.Is(err, capperErr) {
		t.Fatalf("expected capper error, got %v", err)
	}
	if interceptor.beforeCalls != 0 {
		t.Fatalf("expected redis capper error to stop before BeforeExecute, got %d calls", interceptor.beforeCalls)
	}
}

func TestManagerExecuteInternal_BypassesInterceptor(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeMySQL, stubManagerAdapter{result: QueryResult{RowCount: 1}})
	interceptor := &countingExecuteInterceptor{}
	manager.SetInterceptor(interceptor)
	ds := datasource.DataSource{Type: datasource.TypeMySQL}

	if _, err := manager.ExecuteInternal(context.Background(), ds, "SELECT 1", ExecuteOptions{}); err != nil {
		t.Fatalf("internal execute failed: %v", err)
	}
	if interceptor.calls != 0 {
		t.Fatalf("expected internal execute to bypass interceptor, got %d calls", interceptor.calls)
	}
}

func TestManagerExecuteAnnotatesQueryContext(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeMySQL, stubManagerAdapter{result: QueryResult{Columns: []string{"id"}, RowCount: 1}})
	ds := datasource.DataSource{
		ID:      "ds_test",
		Type:    datasource.TypeMySQL,
		Options: map[string]any{datasource.EnvironmentOptionKey: "devint"},
	}

	result, err := manager.Execute(context.Background(), ds, "SELECT * FROM users", ExecuteOptions{PageSize: 25})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.RequestedPageSize != 25 {
		t.Fatalf("requestedPageSize = %d; want 25", result.RequestedPageSize)
	}
	if result.EffectivePageSize != 25 {
		t.Fatalf("effectivePageSize = %d; want 25", result.EffectivePageSize)
	}
	if result.EffectiveLimitSource != EffectiveLimitPageSize {
		t.Fatalf("effectiveLimitSource = %q; want %q", result.EffectiveLimitSource, EffectiveLimitPageSize)
	}
	if result.Dialect != "mysql" {
		t.Fatalf("dialect = %q; want mysql", result.Dialect)
	}
	if result.Environment != "devint" {
		t.Fatalf("environment = %q; want devint", result.Environment)
	}
}

func TestManagerExecuteDoesNotInjectLimitMetadataIntoNonQueryResult(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeMySQL, stubManagerAdapter{result: QueryResult{RowCount: 3}})
	ds := datasource.DataSource{
		Type: datasource.TypeMySQL,
	}

	result, err := manager.Execute(context.Background(), ds, "UPDATE users SET archived = 1", ExecuteOptions{})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.EffectivePageSize != 0 {
		t.Fatalf("effectivePageSize = %d; want 0 for non-query result", result.EffectivePageSize)
	}
	if result.EffectiveLimitSource != "" {
		t.Fatalf("effectiveLimitSource = %q; want empty for non-query result", result.EffectiveLimitSource)
	}
	if result.Dialect != "mysql" {
		t.Fatalf("dialect = %q; want mysql", result.Dialect)
	}
}

func TestManagerDescribeAnnotatesQueryContext(t *testing.T) {
	manager := NewManager()
	manager.Register(datasource.TypeDynamoDB, stubManagerAdapter{})
	ds := datasource.DataSource{
		Type:    datasource.TypeDynamoDB,
		Options: map[string]any{datasource.EnvironmentOptionKey: "dev"},
	}

	result, err := manager.DescribeEntity(context.Background(), ds, "orders")
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if result.Dialect != "partiql" {
		t.Fatalf("dialect = %q; want partiql", result.Dialect)
	}
	if result.Environment != "dev" {
		t.Fatalf("environment = %q; want dev", result.Environment)
	}
}
