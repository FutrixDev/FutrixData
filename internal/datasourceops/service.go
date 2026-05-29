package datasourceops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourcesecrets"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/redisproto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/sensitivity"
)

type Config struct {
	Store                   *datasource.Store
	Manager                 *console.Manager
	RedisDocs               *console.RedisCommandDocsStore
	AuthStore               *auth.Store
	AuthBaseURL             string
	SchemaKnowledgeRoot     string
	RunCommand              func(ctx context.Context, command []string) ([]byte, error)
	HTTPClient              *http.Client
	OpenURL                 func(rawURL string) error
	SensitivityStore        *sensitivity.Store
	MaskingSecret           []byte
	RiskEngine              *riskengine.Engine
	RiskStore               *riskengine.Store
	RiskGuard               *riskengine.Guard
	RedisProtoStore         *redisproto.Store
	DatasourceSecrets       *datasourcesecrets.Manager
	InfoLog                 console.DatasourceTimingLogger
	DatasourceTimingEnabled func() bool
	ErrorLog                interface {
		Printf(format string, v ...any)
	}
}

type Service struct {
	store                   *datasource.Store
	manager                 *console.Manager
	redisDocs               *console.RedisCommandDocsStore
	authService             *auth.Service
	schemaKnowledgeRoot     string
	runCommand              func(ctx context.Context, command []string) ([]byte, error)
	httpClient              *http.Client
	openURL                 func(rawURL string) error
	sensitivityStore        *sensitivity.Store
	masking                 *sensitivity.MaskingProcessor
	riskEngine              *riskengine.Engine
	riskStore               *riskengine.Store
	riskGuard               *riskengine.Guard
	redisProtoStore         *redisproto.Store
	datasourceSecrets       *datasourcesecrets.Manager
	infoLog                 console.DatasourceTimingLogger
	datasourceTimingEnabled func() bool
	errorLog                interface {
		Printf(format string, v ...any)
	}
}

func NewService(cfg Config) *Service {
	var mp *sensitivity.MaskingProcessor
	if cfg.SensitivityStore != nil {
		legacySecretFunc := func() string {
			if cfg.AuthStore == nil {
				return ""
			}
			st := cfg.AuthStore.Current()
			if st.Session == nil {
				return ""
			}
			return st.Session.User.ID
		}
		mp = sensitivity.NewMaskingProcessorWithLegacyFallback(cfg.SensitivityStore, cfg.MaskingSecret, legacySecretFunc)
	}
	return &Service{
		store:                   cfg.Store,
		manager:                 cfg.Manager,
		redisDocs:               cfg.RedisDocs,
		authService:             auth.NewService(auth.ServiceConfig{BaseURL: cfg.AuthBaseURL, Store: cfg.AuthStore, OpenURL: cfg.OpenURL, HTTPClient: cfg.HTTPClient}),
		schemaKnowledgeRoot:     strings.TrimSpace(cfg.SchemaKnowledgeRoot),
		runCommand:              cfg.RunCommand,
		httpClient:              cfg.HTTPClient,
		openURL:                 cfg.OpenURL,
		sensitivityStore:        cfg.SensitivityStore,
		masking:                 mp,
		riskEngine:              cfg.RiskEngine,
		riskStore:               cfg.RiskStore,
		riskGuard:               cfg.RiskGuard,
		redisProtoStore:         cfg.RedisProtoStore,
		datasourceSecrets:       cfg.DatasourceSecrets,
		infoLog:                 cfg.InfoLog,
		datasourceTimingEnabled: cfg.DatasourceTimingEnabled,
		errorLog:                cfg.ErrorLog,
	}
}

func (s *Service) ListRiskRules(ctx context.Context, includeBuiltin bool) ([]riskengine.Rule, error) {
	_ = ctx
	if includeBuiltin {
		if s.riskEngine == nil {
			return nil, errors.New("risk engine is not configured")
		}
		return s.riskEngine.ListAllRules(), nil
	}
	if s.riskStore == nil {
		return nil, errors.New("risk store is not configured")
	}
	return s.riskStore.List(), nil
}

func (s *Service) CurrentAuth(ctx context.Context) (auth.State, error) {
	if s.authService == nil {
		return auth.State{}, nil
	}
	return s.authService.Current(ctx)
}

func (s *Service) EnsureAuthenticated(ctx context.Context) (auth.State, error) {
	if s.authService == nil {
		return auth.State{}, nil
	}
	return s.authService.EnsureAuthenticated(ctx)
}

func (s *Service) StartAuthLogin(ctx context.Context, input auth.StartLoginInput) (auth.LoginStart, error) {
	if s.authService == nil {
		return auth.LoginStart{}, errors.New("auth service is not configured")
	}
	return s.authService.StartLogin(ctx, input)
}

func (s *Service) PollAuthLogin(ctx context.Context) (auth.LoginPoll, error) {
	if s.authService == nil {
		return auth.LoginPoll{}, errors.New("auth service is not configured")
	}
	return s.authService.PollLogin(ctx)
}

func (s *Service) CompleteAuthLogin(ctx context.Context, code string) (auth.State, error) {
	if s.authService == nil {
		return auth.State{}, errors.New("auth service is not configured")
	}
	return s.authService.CompleteAuthLogin(ctx, code)
}

func (s *Service) LogoutAuth(ctx context.Context) (auth.State, error) {
	if s.authService == nil {
		return auth.State{}, nil
	}
	return s.authService.Logout(ctx)
}

func (s *Service) ListAuthDevices(ctx context.Context) (auth.DeviceList, error) {
	if s.authService == nil {
		return auth.DeviceList{}, errors.New("auth service is not configured")
	}
	return s.authService.ListDevices(ctx)
}

func (s *Service) RemoveAuthDevice(ctx context.Context, deviceID string) (auth.DeviceList, error) {
	if s.authService == nil {
		return auth.DeviceList{}, errors.New("auth service is not configured")
	}
	return s.authService.RemoveDevice(ctx, deviceID)
}

func (s *Service) ListDatasources(ctx context.Context) ([]datasource.DataSource, error) {
	_ = ctx
	if s.store == nil {
		return nil, errors.New("datasource store is not configured")
	}
	items := s.store.List()
	out := make([]datasource.DataSource, 0, len(items))
	for _, item := range items {
		out = append(out, RedactDatasource(item))
	}
	return out, nil
}

func (s *Service) GetDatasource(ctx context.Context, id string) (datasource.DataSource, error) {
	_ = ctx
	if s.store == nil {
		return datasource.DataSource{}, errors.New("datasource store is not configured")
	}
	item, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	return RedactDatasource(item), nil
}

func (s *Service) CreateDatasource(ctx context.Context, payload DataSourcePayload) (datasource.DataSource, error) {
	if s.store == nil {
		return datasource.DataSource{}, errors.New("datasource store is not configured")
	}
	createCheck, err := s.datasourceCreateCheck(ctx)
	if err != nil {
		return datasource.DataSource{}, err
	}
	if err := validateDataSourcePayload(payload); err != nil {
		return datasource.DataSource{}, err
	}
	ds := payload.ToDatasource("")
	if ds.Type == datasource.TypeD1 {
		if strings.TrimSpace(ds.ID) == "" {
			ds.ID = newDatasourceID()
		}
	}
	created, err := s.store.CreateChecked(ds, func(input *datasource.DataSource, count int) error {
		if createCheck != nil {
			if err := createCheck(count); err != nil {
				return err
			}
		}
		*input = s.withRedisClusterNodesDiscovered(ctx, *input)
		if input.Type != datasource.TypeD1 {
			next, err := s.externalizeDatasourceSecrets(ctx, *input)
			if err != nil {
				return err
			}
			*input = next
			return nil
		}
		next, err := s.withD1MetadataPrepared(*input)
		if err != nil {
			return err
		}
		next, err = s.externalizeDatasourceSecrets(ctx, next)
		if err != nil {
			return err
		}
		*input = next
		return nil
	})
	if err != nil {
		return datasource.DataSource{}, err
	}
	return RedactDatasource(created), nil
}

func (s *Service) datasourceCreateCheck(ctx context.Context) (func(count int) error, error) {
	if s.authService == nil {
		return nil, nil
	}

	current, err := s.authService.Current(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.authService.EnsureAuthenticated(ctx)
	if err != nil {
		if errors.Is(err, auth.ErrLoginRequired) {
			state = current
		} else if current.Session == nil {
			return nil, err
		} else {
			state = current
		}
	}
	plan := planlimits.PlanFree
	if state.Session != nil {
		// Use the effective plan so an expired Pro session is gated like Free
		// on every entry point (Wails UI, MCP, CLI). Computing the limit from
		// raw License.Plan would let an expired Pro user create unlimited
		// datasources via the daemon path while the UI already blocks them —
		// the exact split-brain TASK-20260513-091051 eliminated for the Wails
		// path.
		trialExpiresAt := int64(0)
		if state.Trial != nil {
			trialExpiresAt = state.Trial.ExpiresAt
		}
		plan = planlimits.EffectivePlanWithTrial(
			state.Session.License.Plan,
			state.Session.License.Status,
			state.Session.License.ExpiresAt,
			trialExpiresAt,
			time.Now(),
		)
	} else if state.Trial != nil {
		plan = planlimits.EffectivePlanWithTrial("", "", 0, state.Trial.ExpiresAt, time.Now())
	}
	limit := planlimits.DatasourceLimit(plan)
	if limit <= 0 {
		return nil, nil
	}
	return func(count int) error {
		if count >= limit {
			return errors.New(planlimits.DatasourceLimitError(plan))
		}
		return nil
	}, nil
}

func (s *Service) UpdateDatasource(ctx context.Context, id string, payload DataSourcePayload) (datasource.DataSource, error) {
	if s.store == nil {
		return datasource.DataSource{}, errors.New("datasource store is not configured")
	}
	existing, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	if err := validateDataSourcePayload(payload); err != nil {
		return datasource.DataSource{}, err
	}
	ds := payload.ToDatasource(strings.TrimSpace(id))
	ds = datasource.RestoreRedactedDatasource(ds, existing)
	if ds.Type == datasource.TypeD1 {
		ds.Options = d1CarryLegacyDevMetadataOnUpdate(ds.Options, existing.Options)
		next, err := s.withD1MetadataPrepared(ds)
		if err != nil {
			return datasource.DataSource{}, err
		}
		ds = next
	}
	ds = s.withRedisClusterNodesDiscovered(ctx, ds)
	ds, err := s.externalizeDatasourceSecrets(ctx, ds)
	if err != nil {
		return datasource.DataSource{}, err
	}
	updated, err := s.store.Update(strings.TrimSpace(id), ds)
	if err != nil {
		return datasource.DataSource{}, err
	}
	return RedactDatasource(updated), nil
}

func (s *Service) DeleteDatasource(ctx context.Context, id string) (bool, error) {
	_ = ctx
	if s.store == nil {
		return false, errors.New("datasource store is not configured")
	}
	trimmed := strings.TrimSpace(id)
	if err := s.store.Delete(trimmed); err != nil {
		return false, err
	}
	// Cascade: drop any redis protobuf schemas tied to this datasource so they
	// don't linger as orphans referencing a deleted id. Best-effort — the
	// datasource delete already succeeded.
	if s.redisProtoStore != nil {
		if _, err := s.redisProtoStore.DeleteByDatasource(trimmed); err != nil && s.errorLog != nil {
			s.errorLog.Printf("delete redis protobuf schemas for %s: %v", trimmed, err)
		}
	}
	return true, nil
}

func (s *Service) TestDatasource(ctx context.Context, id string) (bool, error) {
	if s.store == nil {
		return false, errors.New("datasource store is not configured")
	}
	ds, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return false, errors.New("datasource not found")
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.test_datasource", ds, "", console.ExecuteOptions{}, false)
	err := s.requiredManager().TestConnection(ctxOrBackground(ctx), ds)
	finishTiming(err)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) TestDatasourcePayload(ctx context.Context, payload DataSourcePayload) (bool, error) {
	// This Service surface is reached only by the agent/MCP/CLI test_datasource_payload
	// tool; the operator-driven GUI tests through App.TestDatasourcePayload instead. An
	// agent must not be able to drive external secret resolution toward a host it
	// controls, so reject SecretRefs here.
	if err := ValidateAgentDatasourceTestPayload(payload); err != nil {
		return false, err
	}
	if err := validateDataSourcePayload(payload); err != nil {
		return false, err
	}
	ds := payload.ToDatasource("")
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.test_datasource_payload", ds, "", console.ExecuteOptions{}, false)
	err := s.requiredManager().TestConnection(ctxOrBackground(ctx), ds)
	finishTiming(err)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) ListDatabases(ctx context.Context, id, pattern, executionMode string) ([]string, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return nil, err
	}
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.list_databases", ds, "", console.ExecuteOptions{}, false)
	result, err := s.requiredManager().ListDatabases(ctxOrBackground(ctx), ds, console.ListOptions{Pattern: pattern})
	finishTiming(err)
	return result, err
}

func (s *Service) ListEntities(ctx context.Context, id, pattern, database, executionMode string, forceRefresh bool) ([]string, error) {
	_ = forceRefresh
	ds, err := s.requireDatasource(id)
	if err != nil {
		return nil, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.list_entities", ds, "", console.ExecuteOptions{}, false)
	result, err := s.requiredManager().ListEntities(ctxOrBackground(ctx), ds, console.ListOptions{Pattern: pattern})
	finishTiming(err)
	return result, err
}

func (s *Service) DescribeEntity(ctx context.Context, id, name, database, executionMode string) (console.DescribeResult, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.DescribeResult{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.describe_entity", ds, "", console.ExecuteOptions{}, false)
	result, err := s.requiredManager().DescribeEntity(ctxOrBackground(ctx), ds, strings.TrimSpace(name))
	finishTiming(err)
	return result, err
}

// AssessStatement runs the full risk-engine assessment — including the probe
// path (EXPLAIN / DescribeEntity) — for a prospective execute_statement call.
// Approval gates in the MCP/CLI tool surfaces consult this method so they see
// the same picture the Guard sees at execution time, rather than a static
// SQL-parse-only heuristic that can disagree with the actual gate.
func (s *Service) AssessStatement(ctx context.Context, id, statement, database, executionMode string) (riskengine.RiskAssessment, error) {
	if s.riskGuard == nil {
		return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
	}
	ds, err := s.requireDatasource(id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.assess_statement", ds, statement, console.ExecuteOptions{}, false)
	defer finishTiming(nil)
	assessment, _, _ := s.riskGuard.Assess(ctxOrBackground(ctx), ds, statement)
	return assessment, nil
}

func (s *Service) AssessRedisCommand(ctx context.Context, id string, args []string, database, executionMode string) (riskengine.RiskAssessment, error) {
	if s.riskEngine == nil {
		return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
	}
	ds, err := s.requireDatasource(id)
	if err != nil {
		return riskengine.RiskAssessment{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	if ds.Type != datasource.TypeRedis && ds.Type != datasource.TypeRedisCluster {
		return riskengine.RiskAssessment{
			Level:   riskengine.RiskHigh,
			Action:  riskengine.ActionRequireApproval,
			Reasons: []string{"redis command tool requires a Redis datasource"},
		}, nil
	}
	return s.riskEngine.AssessParsed(riskengine.ParseRedisCommandArgs(ds.ID, args)), nil
}

func (s *Service) PreviewWriteStatement(ctx context.Context, id, statement, database, executionMode string) (console.WritePreview, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.WritePreview{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.preview_write_statement", ds, statement, console.ExecuteOptions{}, false)
	preview, err := s.requiredManager().PreviewWrite(ctxOrBackground(ctx), ds, statement, console.WritePreviewOptions{})
	finishTiming(err)
	return preview, err
}

func (s *Service) ExecuteStatement(ctx context.Context, id, statement, database, pagingToken string, pageSize int, executionMode string, bounds ...console.ExecuteBounds) (out console.QueryResult, err error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.QueryResult{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	mgr := s.requiredManager()
	opts := console.ExecuteOptions{
		PagingToken: pagingToken,
		PageSize:    pageSize,
	}
	if len(bounds) > 0 {
		opts.Bounds = bounds[0]
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.execute_statement", ds, statement, opts, isUserApproved(ctx))
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "datasourceops.apply_dynamodb_caps")
	if err := s.applyDynamoDBRiskExecutionCaps(ds, statement, &opts); err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return console.QueryResult{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	var (
		result  console.QueryResult
		execErr error
	)
	if isUserApproved(ctx) {
		// The caller (MCP/Skill or CLI tool path) has resolved that the user
		// authorized this execution — either via a danger-trust bypass or by
		// explicitly approving a gated statement. Skip the risk-engine guard.
		// Direct programmatic callers that do not set this context flag remain
		// subject to guard enforcement.
		result, execErr = mgr.ExecuteWithInteractiveApproval(ctxOrBackground(ctx), ds, statement, opts)
	} else {
		result, execErr = mgr.Execute(ctxOrBackground(ctx), ds, statement, opts)
	}
	if execErr != nil {
		return result, execErr
	}
	done = console.DatasourceTimingStage(ctx, "datasourceops.masking")
	sensitivity.ApplyQueryResultMasking(s.masking, ds.ID, &result)
	done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("masked_columns", len(result.MaskedColumns)))
	return result, nil
}

func (s *Service) ExecuteRedisBatch(ctx context.Context, id, batchID string, operations []console.RedisBatchOperation, executionMode string) (console.RedisBatchResult, error) {
	if err := console.ValidateRedisBatchOperations(operations); err != nil {
		return console.RedisBatchResult{}, err
	}
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.RedisBatchResult{}, err
	}
	if ds.Type != datasource.TypeRedis && ds.Type != datasource.TypeRedisCluster {
		return console.RedisBatchResult{}, errors.New("redis datasource required")
	}
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.execute_redis_batch", ds, console.RedisBatchStatement(operations), console.ExecuteOptions{}, isUserApproved(ctx))
	defer func() { finishTiming(err) }()
	if !isUserApproved(ctx) {
		done := console.DatasourceTimingStage(ctx, "datasourceops.redis_batch_risk")
		err = s.enforceRedisBatchRisk(ctxOrBackground(ctx), ds, operations)
		if err != nil {
			done(console.DatasourceTimingKV("status", "error"))
			return console.RedisBatchResult{}, err
		}
		done(console.DatasourceTimingKV("status", "ok"))
	}
	result, err := s.requiredManager().ExecuteRedisBatch(ctxOrBackground(ctx), ds, batchID, operations)
	return result, err
}

func (s *Service) enforceRedisBatchRisk(ctx context.Context, ds datasource.DataSource, operations []console.RedisBatchOperation) error {
	if s == nil || s.riskEngine == nil {
		return nil
	}
	for _, op := range operations {
		args := console.RedisBatchOperationArgs(op)
		assessment := s.riskEngine.AssessParsed(riskengine.ParseRedisCommandArgs(ds.ID, args))
		switch assessment.Action {
		case riskengine.ActionWarn, riskengine.ActionRequireApproval, riskengine.ActionBlock:
			return &riskengine.BlockedError{Assessment: assessment, TargetEntity: redisCommandTarget(args)}
		}
	}
	return nil
}

func (s *Service) ExecuteRedisCommand(ctx context.Context, id string, args []string, database, executionMode string) (console.QueryResult, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.QueryResult{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	if ds.Type != datasource.TypeRedis && ds.Type != datasource.TypeRedisCluster {
		return console.QueryResult{}, errors.New("redis datasource required")
	}
	statement, err := console.RedisCommandStatement(args)
	if err != nil {
		return console.QueryResult{}, err
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.execute_redis_command", ds, statement, console.ExecuteOptions{}, isUserApproved(ctx))
	defer func() { finishTiming(err) }()
	mgr := s.requiredManager()
	opts := console.ExecuteOptions{}
	var (
		result  console.QueryResult
		execErr error
	)
	if isUserApproved(ctx) {
		result, execErr = mgr.ExecuteRedisCommandWithInteractiveApproval(ctxOrBackground(ctx), ds, args, opts)
	} else {
		if s.riskEngine == nil {
			result, execErr = mgr.ExecuteRedisCommand(ctxOrBackground(ctx), ds, args, opts)
		} else {
			done := console.DatasourceTimingStage(ctx, "datasourceops.redis_command_risk")
			assessment := s.riskEngine.AssessParsed(riskengine.ParseRedisCommandArgs(ds.ID, args))
			switch assessment.Action {
			case riskengine.ActionWarn, riskengine.ActionBlock, riskengine.ActionRequireApproval:
				done(console.DatasourceTimingKV("status", "error"))
				err = &riskengine.BlockedError{Assessment: assessment, TargetEntity: redisCommandTarget(args)}
				return console.QueryResult{}, err
			}
			done(console.DatasourceTimingKV("status", "ok"))
			result, execErr = mgr.ExecuteRedisCommandInternal(ctxOrBackground(ctx), ds, args, statement, opts)
		}
	}
	if execErr != nil {
		err = execErr
		return result, execErr
	}
	done := console.DatasourceTimingStage(ctx, "datasourceops.masking")
	sensitivity.ApplyQueryResultMasking(s.masking, ds.ID, &result)
	done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("masked_columns", len(result.MaskedColumns)))
	return result, nil
}

func (s *Service) applyDynamoDBRiskExecutionCaps(ds datasource.DataSource, statement string, opts *console.ExecuteOptions) error {
	if s == nil || s.riskEngine == nil || opts == nil || ds.Type != datasource.TypeDynamoDB || !opts.Bounds.Enabled() {
		return nil
	}
	policy := s.riskEngine.ProbePolicyForParsed(riskengine.ParseStatement(string(ds.Type), ds.ID, statement))
	return riskengine.ApplyDynamoDBExecutionPolicyCaps(ds, opts, policy)
}

func redisCommandTarget(args []string) string {
	if len(args) < 2 {
		return ""
	}
	return args[1]
}

func (s *Service) ExplainStatement(ctx context.Context, id, statement string, analyze bool, database, executionMode string) (console.ExplainResult, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.ExplainResult{}, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	ds = datasourceWithD1ExecutionModeOverride(ds, executionMode)
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.explain_statement", ds, statement, console.ExecuteOptions{}, false)
	statement = console.PrepareExplainStatement(statement, analyze, ds.Type)
	result, err := s.requiredManager().Explain(ctxOrBackground(ctx), ds, statement)
	finishTiming(err)
	return result, err
}

func (s *Service) ScanRedisKeys(ctx context.Context, id, pattern, cursor string) (RedisKeyPage, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return RedisKeyPage{}, err
	}
	if ds.Type != datasource.TypeRedis {
		return RedisKeyPage{}, errors.New("redis datasource required")
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.scan_redis_keys", ds, "", console.ExecuteOptions{PagingToken: cursor}, false)
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "datasourceops.redis_scan.adapter_lookup")
	adapter, err := s.requiredManager().AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	scanner, ok := adapter.(console.KeyScanner)
	if !ok {
		err = console.ErrUnsupported
		return RedisKeyPage{}, err
	}
	// Resolve SecretRef-backed credentials; this direct adapter call bypasses the
	// manager dispatch path that normally resolves secrets.
	done = console.DatasourceTimingStage(ctx, "datasourceops.redis_scan.resolve_datasource")
	ds, err = s.requiredManager().ResolveDatasource(ctxOrBackground(ctx), ds)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	done = console.DatasourceTimingStage(ctx, "datasourceops.redis_scan.decode_cursor")
	start, err := console.DecodeRedisCursor(cursor)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	done = console.DatasourceTimingStage(ctx, "redis.scan_keys")
	keys, next, scanDone, err := scanner.ScanKeys(ctxOrBackground(ctx), ds, pattern, start)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("keys", len(keys)), console.DatasourceTimingKV("done", scanDone))
	stageDone := console.DatasourceTimingStage(ctx, "datasourceops.redis_scan.encode_cursor")
	encoded, err := console.EncodeRedisCursor(next)
	if err != nil {
		stageDone(console.DatasourceTimingKV("status", "error"))
		return RedisKeyPage{}, err
	}
	stageDone(console.DatasourceTimingKV("status", "ok"))
	return RedisKeyPage{Keys: keys, Cursor: encoded, Done: scanDone}, nil
}

func (s *Service) GetRedisCommandDocs(ctx context.Context, id, command string) (console.RedisCommandDocsEntry, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return console.RedisCommandDocsEntry{}, err
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.get_redis_command_docs", ds, command, console.ExecuteOptions{}, false)
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "datasourceops.redis_docs.adapter_lookup")
	adapter, err := s.requiredManager().AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return console.RedisCommandDocsEntry{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	redisAdapter, ok := adapter.(*console.RedisAdapter)
	if !ok {
		err = errors.New("redis adapter not available")
		return console.RedisCommandDocsEntry{}, err
	}
	if s.redisDocs == nil {
		done = console.DatasourceTimingStage(ctx, "datasourceops.redis_docs.resolve_datasource")
		resolved, err := s.requiredManager().ResolveDatasource(ctxOrBackground(ctx), ds)
		if err != nil {
			done(console.DatasourceTimingKV("status", "error"))
			return console.RedisCommandDocsEntry{}, err
		}
		done(console.DatasourceTimingKV("status", "ok"))
		done = console.DatasourceTimingStage(ctx, "redis.fetch_command_docs")
		payload, err := console.FetchRedisCommandDocs(ctxOrBackground(ctx), redisAdapter, resolved)
		if err != nil {
			done(console.DatasourceTimingKV("status", "error"))
			return console.RedisCommandDocsEntry{}, err
		}
		done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("commands", len(payload)))
		if trimmed := strings.ToUpper(strings.TrimSpace(command)); trimmed != "" {
			return console.RedisCommandDocsEntry{Commands: map[string]any{trimmed: payload[trimmed]}}, nil
		}
		return console.RedisCommandDocsEntry{Commands: payload}, nil
	}
	done = console.DatasourceTimingStage(ctx, "redis.command_docs_store_get")
	entry, err := s.redisDocs.Get(ctxOrBackground(ctx), ds.ID, func(fetchCtx context.Context) (map[string]any, error) {
		// Resolve SecretRef-backed credentials before connecting via the adapter,
		// which bypasses the manager dispatch path that normally resolves secrets.
		resolved, err := s.requiredManager().ResolveDatasource(fetchCtx, ds)
		if err != nil {
			return nil, err
		}
		return console.FetchRedisCommandDocs(fetchCtx, redisAdapter, resolved)
	})
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return console.RedisCommandDocsEntry{}, err
	}
	done(console.DatasourceTimingKV("status", "ok"), console.DatasourceTimingKV("commands", len(entry.Commands)))
	if trimmed := strings.ToUpper(strings.TrimSpace(command)); trimmed != "" && len(entry.Commands) > 0 {
		if value, ok := entry.Commands[trimmed]; ok {
			entry.Commands = map[string]any{trimmed: value}
		} else {
			entry.Commands = map[string]any{}
		}
	}
	return entry, nil
}

func (s *Service) D1DeployMigrations(ctx context.Context, id string) (bool, error) {
	ds, err := s.requireDatasource(id)
	if err != nil {
		return false, err
	}
	if ds.Type != datasource.TypeD1 {
		return false, errors.New("d1 datasource required")
	}
	if !d1DatasourceSupportsDev(ds.Options) {
		return false, errors.New("dev mode is not supported for this datasource")
	}
	ctx, finishTiming := s.beginDatasourceTiming(ctx, "datasourceops.d1_deploy_migrations", ds, "", console.ExecuteOptions{}, false)
	defer func() { finishTiming(err) }()
	done := console.DatasourceTimingStage(ctx, "datasourceops.d1_migrations.adapter_lookup")
	adapter, err := s.requiredManager().AdapterFor(ds.Type)
	if err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return false, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	d1Adapter, ok := adapter.(*console.D1Adapter)
	if !ok {
		err = errors.New("d1 adapter not available")
		return false, err
	}
	done = console.DatasourceTimingStage(ctx, "d1.deploy_migrations")
	if err = d1Adapter.DeployMigrations(ctxOrBackground(ctx), ds); err != nil {
		done(console.DatasourceTimingKV("status", "error"))
		return false, err
	}
	done(console.DatasourceTimingKV("status", "ok"))
	return true, nil
}

func (s *Service) withRedisClusterNodesDiscovered(ctx context.Context, ds datasource.DataSource) datasource.DataSource {
	if ds.Type != datasource.TypeRedis || hasRedisOptionsNodes(ds.Options) || strings.TrimSpace(ds.Host) == "" || ds.Port <= 0 || s.manager == nil {
		return ds
	}
	scanCtx := ctx
	if scanCtx == nil {
		var cancel context.CancelFunc
		scanCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	result, err := s.manager.ExecuteInternal(scanCtx, ds, "CLUSTER NODES", console.ExecuteOptions{})
	if err != nil {
		return ds
	}
	raw, ok := queryResultText(result)
	if !ok {
		return ds
	}
	nodes := parseRedisClusterNodes(raw)
	if len(nodes) == 0 {
		return ds
	}
	next := ds
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["nodes"] = nodes
	return next
}

func (s *Service) withD1MetadataPrepared(ds datasource.DataSource) (datasource.DataSource, error) {
	if ds.Type != datasource.TypeD1 {
		return ds, nil
	}
	mode := strings.ToLower(strings.TrimSpace(optionAnyString(ds.Options, "mode")))
	databaseID := strings.TrimSpace(optionAnyString(ds.Options, "databaseId"))
	if databaseID == "" {
		return datasource.DataSource{}, errors.New("databaseId is required for d1")
	}
	databaseName := strings.TrimSpace(optionAnyString(ds.Options, "databaseName"))
	if databaseName == "" {
		databaseName = strings.TrimSpace(ds.Database)
	}
	if databaseName == "" {
		if mode == "local" || mode == "cloud" {
			databaseName = databaseID
		} else {
			return datasource.DataSource{}, errors.New("databaseName is required for d1")
		}
	}
	binding := strings.TrimSpace(optionAnyString(ds.Options, "binding"))
	if binding == "" {
		binding = d1BindingFromDatabaseName(databaseName)
	}
	if binding == "" {
		return datasource.DataSource{}, errors.New("binding is required for d1")
	}
	devProjectPath, err := d1NormalizeProjectPath(optionAnyString(ds.Options, "devProjectPath"))
	if err != nil {
		return datasource.DataSource{}, err
	}
	supportDev := optionAnyBool(ds.Options, "supportDev") && devProjectPath != ""
	legacyWranglerConfigPath := strings.TrimSpace(optionAnyString(ds.Options, "wranglerConfigPath"))
	previousDatabaseID := strings.TrimSpace(optionAnyString(ds.Options, "previousDatabaseId"))
	previousBinding := strings.TrimSpace(optionAnyString(ds.Options, "previousBinding"))
	migrationDir := filepath.ToSlash(filepath.Join("migrations", d1MigrationDirName(databaseName, databaseID)))

	next := ds
	next.Database = databaseName
	next.Options = copyDatasourceOptions(ds.Options)
	next.Options["databaseId"] = databaseID
	next.Options["databaseName"] = databaseName
	next.Options["binding"] = binding
	next.Options["supportDev"] = supportDev
	if supportDev {
		configPath, err := ensureD1WranglerConfig(devProjectPath, d1WranglerDatabaseEntry{
			Binding:       binding,
			DatabaseName:  databaseName,
			DatabaseID:    databaseID,
			MigrationsDir: migrationDir,
		}, previousDatabaseID, previousBinding)
		if err != nil {
			return datasource.DataSource{}, err
		}
		next.Options["devProjectPath"] = devProjectPath
		next.Options["wranglerConfigPath"] = configPath
		next.Options["migrationsDir"] = migrationDir
	} else if mode != "local" {
		delete(next.Options, "devProjectPath")
		if legacyWranglerConfigPath == "" {
			delete(next.Options, "wranglerConfigPath")
			delete(next.Options, "migrationsDir")
		} else {
			configPath, err := ensureD1WranglerConfig(filepath.Dir(legacyWranglerConfigPath), d1WranglerDatabaseEntry{
				Binding:       binding,
				DatabaseName:  databaseName,
				DatabaseID:    databaseID,
				MigrationsDir: migrationDir,
			}, previousDatabaseID, previousBinding)
			if err != nil {
				if errors.Is(err, errD1DevProjectPathMissing) || errors.Is(err, errD1DevProjectPathNotDir) {
					delete(next.Options, "wranglerConfigPath")
					delete(next.Options, "migrationsDir")
					return next, nil
				}
				return datasource.DataSource{}, err
			}
			next.Options["wranglerConfigPath"] = configPath
			next.Options["migrationsDir"] = migrationDir
		}
	}
	delete(next.Options, "previousDatabaseId")
	delete(next.Options, "previousBinding")
	return next, nil
}

func (s *Service) requiredManager() *console.Manager {
	if s.manager == nil {
		panic("datasourceops service manager is not configured")
	}
	return s.manager
}

func (s *Service) requireDatasource(id string) (datasource.DataSource, error) {
	if s.store == nil {
		return datasource.DataSource{}, errors.New("datasource store is not configured")
	}
	ds, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return datasource.DataSource{}, errors.New("datasource not found")
	}
	return ds, nil
}

func (s *Service) externalizeDatasourceSecrets(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	if s.datasourceSecrets == nil {
		return ds, nil
	}
	return s.datasourceSecrets.ExternalizeDatasourceSecrets(ctxOrBackground(ctx), ds)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

type userApprovedCtxKey struct{}

// WithUserApproved marks the context so that an ExecuteStatement call is
// allowed to bypass the risk-engine guard. Agent tool dispatch sets this only
// when the entry-point gate has already allowed execution, such as low-risk,
// trusted auto-run, or danger-mode bypass.
func WithUserApproved(ctx context.Context) context.Context {
	return context.WithValue(ctxOrBackground(ctx), userApprovedCtxKey{}, true)
}

// IsUserApproved reports whether the given context has been marked as
// carrying an entry-point user approval via WithUserApproved. Exposed for
// tests in the CLI/MCP adapters that need to assert the context mark is
// propagated to Call closures.
func IsUserApproved(ctx context.Context) bool {
	return isUserApproved(ctx)
}

func isUserApproved(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(userApprovedCtxKey{}).(bool)
	return v
}

func (s *Service) beginDatasourceTiming(ctx context.Context, entrypoint string, ds datasource.DataSource, statement string, opts console.ExecuteOptions, approved bool) (context.Context, func(error)) {
	ctx = ctxOrBackground(ctx)
	if s == nil || s.infoLog == nil || s.datasourceTimingEnabled == nil || !s.datasourceTimingEnabled() {
		return ctx, func(error) {}
	}
	trace := console.NewDatasourceTimingTrace(s.infoLog, console.NewDatasourceTimingMetadata(entrypoint, datasourceopsDatasourceTimingRequestID(), ds, statement, opts, approved))
	ctx = console.WithDatasourceTimingTrace(ctx, trace)
	console.DatasourceTimingEvent(ctx, "start")
	return ctx, func(err error) {
		trace.Finish(console.DatasourceTimingStatus(err), console.DatasourceTimingErrorFields(err)...)
	}
}

func datasourceopsDatasourceTimingRequestID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
