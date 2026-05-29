package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourcesecrets"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/schemaprivacy"
	"futrixdata/platform/internal/sensitivity"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultAITimeout = 90 * time.Second

func (a *App) AiChatTurn(req aichat.TurnRequest) (aichat.TurnResponse, error) {
	if a.aiChat == nil {
		return aichat.TurnResponse{}, errors.New("ai chat service not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.aiChat.Turn(ctx, req)
}

func (a *App) AiChatTurnStream(req aichat.TurnRequest) (aichat.StreamStartResponse, error) {
	if a.aiChat == nil {
		return aichat.StreamStartResponse{}, errors.New("ai chat service not available")
	}
	if a.ctx == nil {
		return aichat.StreamStartResponse{}, errors.New("app context not available")
	}
	if strings.TrimSpace(req.ConversationID) == "" {
		return aichat.StreamStartResponse{}, errors.New("conversationId is required")
	}

	streamID := fmt.Sprintf("stream_%x", time.Now().UTC().UnixNano())
	emitCtx := a.ctx
	baseCtx := aichat.WithDiagnosticsContext(context.Background(), req.ConversationID, streamID)
	streamCtx, cancel := context.WithCancel(baseCtx)
	if a.aiChatStreams != nil {
		a.aiChatStreams.register(streamID, cancel)
	}

	go func() {
		defer func() {
			if a.aiChatStreams != nil {
				a.aiChatStreams.unregister(streamID)
			}
		}()

		resp, err := a.aiChat.TurnStream(streamCtx, req, func(delta string) {
			if delta == "" {
				return
			}
			runtime.EventsEmit(emitCtx, "aichat:delta", map[string]any{
				"streamId":       streamID,
				"conversationId": req.ConversationID,
				"delta":          delta,
			})
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				runtime.EventsEmit(emitCtx, "aichat:cancelled", map[string]any{
					"streamId":       streamID,
					"conversationId": req.ConversationID,
				})
				return
			}
			runtime.EventsEmit(emitCtx, "aichat:error", map[string]any{
				"streamId":       streamID,
				"conversationId": req.ConversationID,
				"error":          err.Error(),
			})
			return
		}
		runtime.EventsEmit(emitCtx, "aichat:done", map[string]any{
			"streamId":       streamID,
			"conversationId": req.ConversationID,
			"response":       resp,
		})
	}()

	return aichat.StreamStartResponse{StreamID: streamID}, nil
}

func (a *App) AiChatCancelStream(streamID string) bool {
	if a.aiChatStreams == nil {
		return false
	}
	return a.aiChatStreams.cancel(strings.TrimSpace(streamID))
}

func (a *App) AiChatApprove(req aichat.ApproveRequest) (aichat.TurnResponse, error) {
	if a.aiChat == nil {
		return aichat.TurnResponse{}, errors.New("ai chat service not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.aiChat.Approve(ctx, req)
}

type appAIChatModelResolver struct {
	cfg           Config
	aiconfigStore *aiconfig.Store
}

func newAppAIChatModelResolver(cfg Config, store *aiconfig.Store) aichat.ModelResolver {
	return &appAIChatModelResolver{cfg: cfg, aiconfigStore: store}
}

func (r *appAIChatModelResolver) Resolve(aiConfigID string) (aichat.Model, error) {
	if r.aiconfigStore != nil {
		if id := strings.TrimSpace(aiConfigID); id != "" {
			cfg, ok := r.aiconfigStore.Get(id)
			if !ok {
				return nil, errors.New("ai configuration not found")
			}
			return modelFromAIConfig(cfg)
		}
		if cfg, ok := r.aiconfigStore.GetPreferred(); ok {
			model, err := modelFromAIConfig(cfg)
			if err == nil {
				return model, nil
			}
		}
	}

	fallback := buildAIConfig(r.cfg)
	if strings.TrimSpace(fallback.APIKey) == "" {
		return nil, errors.New("ai provider not configured")
	}
	return aichat.NewOpenAIEinoExtModel(aichat.OpenAICompatibleModelConfig{
		BaseURL:  fallback.BaseURL,
		APIKey:   fallback.APIKey,
		Model:    fallback.Model,
		Timeout:  fallback.Timeout,
		Referer:  "http://localhost",
		AppTitle: "FutrixData Platform",
	})
}

func modelFromAIConfig(cfg aiconfig.AIConfig) (aichat.Model, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		if defaults, ok := aiconfig.ProviderDefaults[cfg.Provider]; ok {
			baseURL = defaults.BaseURL
		}
	}
	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" {
		if defaults, ok := aiconfig.ProviderDefaults[cfg.Provider]; ok {
			modelName = defaults.DefaultModel
		}
	}
	if strings.TrimSpace(cfg.APIKey) == "" || baseURL == "" || modelName == "" {
		return nil, errors.New("ai provider not configured")
	}

	maxTokens := aiChatMaxTokensFromOptions(cfg.Options)

	switch cfg.Provider {
	case aiconfig.ProviderAnthropic:
		return aichat.NewAnthropicModel(aichat.AnthropicModelConfig{
			BaseURL:   baseURL,
			APIKey:    cfg.APIKey,
			Model:     modelName,
			Timeout:   defaultAITimeout,
			MaxTokens: maxTokens,
		}), nil
	default:
		return aichat.NewOpenAIEinoExtModel(aichat.OpenAICompatibleModelConfig{
			BaseURL:   baseURL,
			APIKey:    cfg.APIKey,
			Model:     modelName,
			Timeout:   defaultAITimeout,
			MaxTokens: maxTokens,
			Referer:   "http://localhost",
			AppTitle:  "FutrixData Platform",
		})
	}
}

func aiChatMaxTokensFromOptions(options map[string]any) int {
	if options == nil {
		return 0
	}
	for _, key := range []string{"maxTokens", "maxCompletionTokens", "max_tokens", "max_completion_tokens"} {
		raw, ok := options[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case int:
			if v > 0 {
				return v
			}
		case int64:
			if v > 0 {
				return int(v)
			}
		case float64:
			if v > 0 {
				return int(v)
			}
		case float32:
			if v > 0 {
				return int(v)
			}
		default:
			var parsed int
			if _, err := fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

// providerSummaryFunc returns the active chat provider/model identity in a
// shape suitable for audit recording. Implementations resolve from the
// AIConfig store; nil is acceptable and means "no resolver wired" — Gate then
// records the egress without provider metadata.
type providerSummaryFunc func(aiConfigID string) (provider, model, configID string)

func providerSummaryFromResolver(store *aiconfig.Store) providerSummaryFunc {
	if store == nil {
		return nil
	}
	return func(aiConfigID string) (string, string, string) {
		id := strings.TrimSpace(aiConfigID)
		if id != "" {
			if cfg, ok := store.Get(id); ok {
				return string(cfg.Provider), cfg.Model, cfg.ID
			}
		}
		if cfg, ok := store.GetPreferred(); ok {
			return string(cfg.Provider), cfg.Model, cfg.ID
		}
		return "", "", ""
	}
}

type appAIChatTools struct {
	store             *datasource.Store
	manager           *console.Manager
	redisDocs         *console.RedisCommandDocsStore
	schemaKB          *schemaKnowledgeManager
	masking           *sensitivity.MaskingProcessor
	authStore         *auth.Store
	schemaPrivacy     *schemaprivacy.AuditStore
	datasourceSecrets *datasourcesecrets.Manager
	providerInfo      providerSummaryFunc
	datasourceTiming  appDatasourceTimingStarter
}

func newAppAIChatTools(
	store *datasource.Store,
	manager *console.Manager,
	redisDocs *console.RedisCommandDocsStore,
	schemaKB *schemaKnowledgeManager,
	masking *sensitivity.MaskingProcessor,
	authStore *auth.Store,
	schemaPrivacy *schemaprivacy.AuditStore,
	providerInfo providerSummaryFunc,
	datasourceSecrets *datasourcesecrets.Manager,
	datasourceTiming ...appDatasourceTimingStarter,
) aichat.Tools {
	var datasourceTimingStarter appDatasourceTimingStarter
	if len(datasourceTiming) > 0 {
		datasourceTimingStarter = datasourceTiming[0]
	}
	return &appAIChatTools{
		store:             store,
		manager:           manager,
		redisDocs:         redisDocs,
		schemaKB:          schemaKB,
		masking:           masking,
		authStore:         authStore,
		schemaPrivacy:     schemaPrivacy,
		datasourceSecrets: datasourceSecrets,
		providerInfo:      providerInfo,
		datasourceTiming:  datasourceTimingStarter,
	}
}

// schemaPrivacyGate is the in-process consent + audit hop used by every AI
// Chat tool that returns schema metadata to the model. It looks up the
// active provider so the audit log can answer "where did this go?", then
// delegates to schemaprivacy.Gate which enforces the consent and writes the
// log entry.
//
// The active AI config ID is read off the context — the chat runtime stamps
// it onto every tool invocation so the audit reflects the provider this turn
// is actually using, not the user's preferred default. Falling back to "" is
// safe: providerInfo treats an empty ID as "use the preferred config".
func (t *appAIChatTools) schemaPrivacyGate(ctx context.Context, ds datasource.DataSource, trigger schemaprivacy.TriggerSource, summary schemaprivacy.SendSummary) error {
	// Re-read the datasource snapshot from the store so consent changes
	// that landed since the tool entered — in particular, revocations
	// during a slow schema fetch — are honored at the moment the gate
	// decides allow/deny. SensitivityScan handles the same race the same
	// way; doing it here keeps the chat path symmetric.
	if fresh, ok := t.store.Get(strings.TrimSpace(ds.ID)); ok {
		ds = fresh
	}
	if t.providerInfo != nil {
		provider, model, configID := t.providerInfo(schemaprivacy.AIConfigIDFromContext(ctx))
		summary.ProviderType = provider
		summary.Model = model
		summary.AIConfigID = configID
	}
	return schemaprivacy.Gate(t.schemaPrivacy, ds, trigger, summary)
}

// schemaPrivacyPreflight runs the consent gate before the underlying schema
// fetch. Without this, a denied or unset datasource whose schema fetch
// errors out (missing cache, IO failure, adapter error) would surface as a
// generic backend error and skip the denied-egress audit row. The final
// schemaPrivacyGate call after a successful fetch is what records the
// allowed-egress row with real entity/field counts; this preflight only
// guarantees that refusals are enforced and audited up front.
//
// We re-read the datasource here before checking consent so a denied→allowed
// flip that lands between the caller's store.Get and this preflight does not
// trick us into entering the gate against a stale snapshot. Without the
// re-read, the inner schemaPrivacyGate would see fresh consent=allowed and
// write a phantom "allowed, 0 entities, 0 fields" row before the real
// post-fetch gate writes the audit row with proper counts.
func (t *appAIChatTools) schemaPrivacyPreflight(ctx context.Context, ds datasource.DataSource, trigger schemaprivacy.TriggerSource) error {
	if fresh, ok := t.store.Get(strings.TrimSpace(ds.ID)); ok {
		ds = fresh
	}
	if schemaprivacy.ConsentOf(ds) == schemaprivacy.ConsentAllowed {
		return nil
	}
	return t.schemaPrivacyGate(ctx, ds, trigger, schemaprivacy.SendSummary{})
}

func (t *appAIChatTools) currentPlan() (string, bool) {
	if t == nil || t.authStore == nil {
		return "", false
	}
	state := t.authStore.Current()
	if state.Session == nil {
		return planlimits.EffectivePlanWithTrial("", "", 0, trialExpiresAt(state), time.Now()), true
	}
	license := state.Session.License
	return planlimits.EffectivePlanWithTrial(license.Plan, license.Status, license.ExpiresAt, trialExpiresAt(state), time.Now()), true
}

func (t *appAIChatTools) ensureDatasourceCreateAllowed() error {
	check := t.datasourceCreateCheck()
	if check == nil || t == nil || t.store == nil {
		return nil
	}
	return check(len(t.store.List()))
}

func (t *appAIChatTools) datasourceCreateCheck() func(count int) error {
	plan, ok := t.currentPlan()
	if !ok || t == nil || t.store == nil {
		return nil
	}
	limit := planlimits.DatasourceLimit(plan)
	if limit <= 0 {
		return nil
	}
	return func(count int) error {
		if count >= limit {
			return errors.New(planlimits.DatasourceLimitError(plan))
		}
		return nil
	}
}

func (t *appAIChatTools) ListDatasources(ctx context.Context) ([]aichat.DatasourceSummary, error) {
	_ = ctx
	items := t.store.List()
	out := make([]aichat.DatasourceSummary, 0, len(items))
	for _, ds := range items {
		out = append(out, aichat.DatasourceSummary{
			ID:          ds.ID,
			Name:        ds.Name,
			Type:        string(ds.Type),
			Host:        ds.Host,
			Port:        ds.Port,
			Database:    ds.Database,
			TrustLevel:  string(ds.TrustLevel()),
			Environment: ds.Environment(),
			Dialect:     ds.QueryDialect(),
		})
	}
	return out, nil
}

func (t *appAIChatTools) GetDatasource(ctx context.Context, id string) (aichat.DatasourceSummary, error) {
	_ = ctx
	ds, ok := t.store.Get(id)
	if !ok {
		return aichat.DatasourceSummary{}, errors.New("datasource not found")
	}
	return aichat.DatasourceSummary{
		ID:          ds.ID,
		Name:        ds.Name,
		Type:        string(ds.Type),
		Host:        ds.Host,
		Port:        ds.Port,
		Database:    ds.Database,
		TrustLevel:  string(ds.TrustLevel()),
		Environment: ds.Environment(),
		Dialect:     ds.QueryDialect(),
	}, nil
}

func (t *appAIChatTools) CreateDatasource(ctx context.Context, input aichat.DatasourceCreateInput) (aichat.DatasourceSummary, error) {
	_ = ctx
	payload := DataSourcePayload{
		Name:       input.Name,
		Type:       datasource.DataSourceType(strings.TrimSpace(input.Type)),
		Host:       input.Host,
		Port:       input.Port,
		Username:   input.Username,
		Password:   input.Password,
		Database:   input.Database,
		AuthSource: input.AuthSource,
		Options:    input.Options,
	}
	if err := validateDataSourcePayload(payload); err != nil {
		return aichat.DatasourceSummary{}, err
	}
	createCheck := t.datasourceCreateCheck()
	created, err := t.store.CreateChecked(payload.toDataSource(""), func(input *datasource.DataSource, count int) error {
		if createCheck == nil {
			return t.externalizeDatasourceSecrets(ctx, input)
		}
		if err := createCheck(count); err != nil {
			return err
		}
		return t.externalizeDatasourceSecrets(ctx, input)
	})
	if err != nil {
		return aichat.DatasourceSummary{}, err
	}
	return aichat.DatasourceSummary{
		ID:          created.ID,
		Name:        created.Name,
		Type:        string(created.Type),
		Host:        created.Host,
		Port:        created.Port,
		Database:    created.Database,
		TrustLevel:  string(created.TrustLevel()),
		Environment: created.Environment(),
		Dialect:     created.QueryDialect(),
	}, nil
}

func (t *appAIChatTools) externalizeDatasourceSecrets(ctx context.Context, ds *datasource.DataSource) error {
	if t == nil || t.datasourceSecrets == nil || ds == nil {
		return nil
	}
	next, err := t.datasourceSecrets.ExternalizeDatasourceSecrets(ctx, *ds)
	if err != nil {
		return err
	}
	*ds = next
	return nil
}

func (t *appAIChatTools) DeleteDatasource(ctx context.Context, datasourceID string) error {
	_ = ctx
	return t.store.Delete(strings.TrimSpace(datasourceID))
}

func (t *appAIChatTools) ListDatabases(ctx context.Context, datasourceID, pattern string) ([]string, error) {
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	return t.manager.ListDatabases(ctx, ds, console.ListOptions{Pattern: pattern})
}

func (t *appAIChatTools) ListEntities(ctx context.Context, datasourceID, pattern, database string) ([]string, error) {
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	if gateErr := t.schemaPrivacyPreflight(ctx, ds, schemaprivacy.TriggerAIChatListEntities); gateErr != nil {
		return nil, gateErr
	}
	entities, err := t.manager.ListEntities(ctx, ds, console.ListOptions{Pattern: pattern})
	if err != nil {
		return nil, err
	}
	if gateErr := t.schemaPrivacyGate(ctx, ds, schemaprivacy.TriggerAIChatListEntities, schemaprivacy.SendSummary{
		EntityCount: len(entities),
	}); gateErr != nil {
		return nil, gateErr
	}
	return entities, nil
}

func (t *appAIChatTools) DescribeEntity(ctx context.Context, datasourceID, name, database string) (any, error) {
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	if gateErr := t.schemaPrivacyPreflight(ctx, ds, schemaprivacy.TriggerAIChatDescribeEntity); gateErr != nil {
		return nil, gateErr
	}
	result, err := t.manager.DescribeEntity(ctx, ds, name)
	if err != nil {
		return nil, err
	}
	summary := schemaprivacy.SendSummary{EntityCount: 1}
	summary.FieldCount, summary.IncludesComments = describeFieldStats(result)
	if gateErr := t.schemaPrivacyGate(ctx, ds, schemaprivacy.TriggerAIChatDescribeEntity, summary); gateErr != nil {
		return nil, gateErr
	}
	return result, nil
}

// describeFieldStats reports the number of column-like fields and whether
// the result includes any free-text "details" (which adapters use for
// comments, partition info, etc — anything beyond name + type). It accepts
// the raw `any` return so callers don't depend on the console package's
// struct shape.
func describeFieldStats(result any) (int, bool) {
	if result == nil {
		return 0, false
	}
	switch typed := result.(type) {
	case console.DescribeResult:
		return len(typed.Columns), len(typed.Details) > 0
	case map[string]any:
		fields := 0
		if cols, ok := typed["columns"].([]any); ok {
			fields = len(cols)
		}
		hasDetails := false
		if details, ok := typed["details"].([]any); ok && len(details) > 0 {
			hasDetails = true
		}
		return fields, hasDetails
	}
	return 0, false
}

func (t *appAIChatTools) ExplainStatement(ctx context.Context, datasourceID, statement, database string) (console.ExplainResult, error) {
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return console.ExplainResult{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	statement = console.PrepareExplainStatement(statement, false, ds.Type)
	return t.manager.Explain(ctx, ds, statement)
}

func (t *appAIChatTools) ExecuteStatement(ctx context.Context, datasourceID, statement, database, pagingToken string, pageSize int, approved bool) (out aichat.QueryResult, err error) {
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return aichat.QueryResult{}, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	execOpts := console.ExecuteOptions{
		PagingToken: pagingToken,
		PageSize:    pageSize,
	}
	finishTiming := func(error) {}
	if t.datasourceTiming != nil {
		ctx, finishTiming = t.datasourceTiming(ctx, "app.ai_chat.execute_statement", ds, statement, execOpts, approved)
		defer func() { finishTiming(err) }()
	}
	var result console.QueryResult
	if approved {
		result, err = t.manager.ExecuteWithInteractiveApproval(ctx, ds, statement, execOpts)
	} else {
		result, err = t.manager.Execute(ctx, ds, statement, execOpts)
	}
	if err != nil {
		return aichat.QueryResult{}, err
	}
	rawRows := cloneRows(result.Rows)
	rawColumns := append([]string(nil), result.Columns...)
	rawNextToken := result.NextToken
	rawPrevToken := result.PrevToken

	sensitivity.ApplyQueryResultMasking(t.masking, ds.ID, &result)
	maskedView := aichat.QueryResult{
		Columns:              append([]string(nil), result.Columns...),
		Rows:                 cloneRows(result.Rows),
		RowCount:             result.RowCount,
		HasMore:              result.HasMore,
		NextToken:            result.NextToken,
		PrevToken:            result.PrevToken,
		ElapsedMs:            result.ElapsedMs,
		RequestedPageSize:    result.RequestedPageSize,
		EffectivePageSize:    result.EffectivePageSize,
		EffectiveLimitSource: result.EffectiveLimitSource,
		Dialect:              result.Dialect,
		Environment:          result.Environment,
	}
	return aichat.QueryResult{
		Columns:              rawColumns,
		Rows:                 rawRows,
		RowCount:             result.RowCount,
		HasMore:              result.HasMore,
		NextToken:            rawNextToken,
		PrevToken:            rawPrevToken,
		ElapsedMs:            result.ElapsedMs,
		RequestedPageSize:    result.RequestedPageSize,
		EffectivePageSize:    result.EffectivePageSize,
		EffectiveLimitSource: result.EffectiveLimitSource,
		Dialect:              result.Dialect,
		Environment:          result.Environment,
		AgentView:            &maskedView,
	}, nil
}

func cloneRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			out = append(out, nil)
			continue
		}
		cloned := make(map[string]any, len(row))
		for key, value := range row {
			cloned[key] = cloneValue(value)
		}
		out = append(out, cloned)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, child := range typed {
			next[key] = cloneValue(child)
		}
		return next
	case []any:
		next := make([]any, len(typed))
		for idx, child := range typed {
			next[idx] = cloneValue(child)
		}
		return next
	default:
		return typed
	}
}

func (t *appAIChatTools) GetRedisCommandDocs(ctx context.Context, datasourceID, command string) (any, error) {
	_ = command
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	adapter, err := t.manager.AdapterFor(ds.Type)
	if err != nil {
		return nil, err
	}
	redisAdapter, ok := adapter.(*console.RedisAdapter)
	if !ok {
		return nil, errors.New("redis adapter not available")
	}
	return t.redisDocs.Get(ctx, ds.ID, func(ctx context.Context) (map[string]any, error) {
		// Resolve SecretRef-backed credentials; this adapter call bypasses the
		// manager dispatch path that normally resolves secrets.
		resolved, err := t.manager.ResolveDatasource(ctx, ds)
		if err != nil {
			return nil, err
		}
		return console.FetchRedisCommandDocs(ctx, redisAdapter, resolved)
	})
}

func (t *appAIChatTools) GetSchemaKnowledge(ctx context.Context, datasourceID, entity, database string) (any, error) {
	if t.schemaKB == nil {
		return nil, errors.New("schema knowledge is not available")
	}
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	if gateErr := t.schemaPrivacyPreflight(ctx, ds, schemaprivacy.TriggerAIChatGetSchemaKnowledge); gateErr != nil {
		return nil, gateErr
	}
	result, err := t.schemaKB.GetSchemaKnowledge(ds, entity)
	if err != nil {
		return nil, err
	}
	summary := schemaprivacy.SendSummary{}
	if count, ok := result["entityCount"].(int); ok {
		summary.EntityCount = count
	}
	if entities, ok := result["entities"].([]schemaKnowledgeEntity); ok {
		fields := 0
		for _, e := range entities {
			fields += len(e.Columns)
			if len(e.Details) > 0 {
				summary.IncludesComments = true
			}
		}
		summary.FieldCount = fields
	}
	if gateErr := t.schemaPrivacyGate(ctx, ds, schemaprivacy.TriggerAIChatGetSchemaKnowledge, summary); gateErr != nil {
		return nil, gateErr
	}
	return result, nil
}

func (t *appAIChatTools) GetERKnowledge(ctx context.Context, datasourceID, database string) (any, error) {
	if t.schemaKB == nil {
		return nil, errors.New("schema knowledge is not available")
	}
	ds, ok := t.store.Get(datasourceID)
	if !ok {
		return nil, errors.New("datasource not found")
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	if gateErr := t.schemaPrivacyPreflight(ctx, ds, schemaprivacy.TriggerAIChatGetERKnowledge); gateErr != nil {
		return nil, gateErr
	}
	result, err := t.schemaKB.GetERKnowledge(ds)
	if err != nil {
		return nil, err
	}
	if gateErr := t.schemaPrivacyGate(ctx, ds, schemaprivacy.TriggerAIChatGetERKnowledge, schemaprivacy.SendSummary{}); gateErr != nil {
		return nil, gateErr
	}
	return result, nil
}
