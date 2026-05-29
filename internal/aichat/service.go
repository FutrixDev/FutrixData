package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"futrixdata/platform/internal/riskengine"
)

type Model interface {
	Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error)
}

type ModelResolver interface {
	Resolve(aiConfigID string) (Model, error)
}

type Tools interface {
	ListDatasources(ctx context.Context) ([]DatasourceSummary, error)
	GetDatasource(ctx context.Context, id string) (DatasourceSummary, error)
	CreateDatasource(ctx context.Context, input DatasourceCreateInput) (DatasourceSummary, error)
	DeleteDatasource(ctx context.Context, datasourceID string) error
	ListDatabases(ctx context.Context, datasourceID, pattern string) ([]string, error)
	ListEntities(ctx context.Context, datasourceID, pattern, database string) ([]string, error)
	DescribeEntity(ctx context.Context, datasourceID, name, database string) (any, error)
	ExplainStatement(ctx context.Context, datasourceID, statement, database string) (console.ExplainResult, error)
	ExecuteStatement(ctx context.Context, datasourceID, statement, database, pagingToken string, pageSize int, approved bool) (QueryResult, error)
	GetRedisCommandDocs(ctx context.Context, datasourceID, command string) (any, error)
	GetSchemaKnowledge(ctx context.Context, datasourceID, entity, database string) (any, error)
	GetERKnowledge(ctx context.Context, datasourceID, database string) (any, error)
}

// RiskGuard evaluates statement risk before execution. Assess returns the pure
// assessment plus any probe explain result; BeforeExecute is retained for the
// console.ExecuteInterceptor path but AI Chat should prefer Assess + DecideGate.
type RiskGuard interface {
	Assess(ctx context.Context, ds datasource.DataSource, statement string) (riskengine.RiskAssessment, *console.ExplainResult, string)
	BeforeExecute(ctx context.Context, ds datasource.DataSource, statement string, opts console.ExecuteOptions) error
}

type Service struct {
	models    ModelResolver
	tools     Tools
	web       WebSearchProvider
	riskGuard RiskGuard

	approvals        *approvalStore
	einoCheckpoints  *einoCheckpointStore
	einoApprovals    *einoApprovalResumeStore
	analysis         *analysisMemoryStore
	threadStore      threadStore
	memoryStore      memoryStore
	memoryRecall     MemoryRecallProvider
	memoryCapture    memoryCapturePlanner
	workingSetConfig workingSetConfig
	baseSystemPrompt string
	promptModules    PromptModules
	knowledgeDir     string
	userKnowledgeDir string
	diag             Diagnostics
}

type uiLocale string

const (
	uiLocaleEN uiLocale = "en"
	uiLocaleZH uiLocale = "zh"
)

func uiLocaleFromString(value string) uiLocale {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return uiLocaleEN
	}
	if strings.HasPrefix(trimmed, "zh") || strings.Contains(trimmed, "hans") || strings.Contains(trimmed, "cn") {
		return uiLocaleZH
	}
	return uiLocaleEN
}

func containsCJK(text string) bool {
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
			return true
		case r >= 0x3400 && r <= 0x4DBF: // CJK Unified Ideographs Extension A
			return true
		case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
			return true
		}
	}
	return false
}

func detectUserLocale(req TurnRequest) uiLocale {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if strings.ToLower(strings.TrimSpace(msg.Role)) != "user" {
			continue
		}
		if containsCJK(msg.Content) {
			return uiLocaleZH
		}
	}
	return uiLocaleEN
}

func NewService(models ModelResolver, tools Tools) *Service {
	return &Service{
		models:           models,
		tools:            tools,
		web:              newDefaultWebSearchProvider(),
		approvals:        newApprovalStore(),
		einoCheckpoints:  newEinoCheckpointStore(),
		einoApprovals:    newEinoApprovalResumeStore(),
		analysis:         newAnalysisMemoryStore(),
		memoryRecall:     noopMemoryRecallProvider{},
		memoryCapture:    defaultMemoryCapturePlanner{},
		baseSystemPrompt: defaultBaseSystemPrompt,
		promptModules:    DefaultPromptModules(),
	}
}

func (s *Service) SetRiskGuard(guard RiskGuard) {
	if s == nil {
		return
	}
	s.riskGuard = guard
}

func (s *Service) SetBaseSystemPrompt(prompt string) {
	if strings.TrimSpace(prompt) == "" {
		return
	}
	s.baseSystemPrompt = prompt
}

func (s *Service) SetPromptModules(modules PromptModules) {
	if s == nil || modules.empty() {
		return
	}
	s.promptModules = s.promptModules.merge(modules)
}

func (s *Service) SetKnowledgeDir(dir string) {
	if s == nil {
		return
	}
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return
	}
	s.knowledgeDir = trimmed
}

func (s *Service) SetUserKnowledgeDir(dir string) {
	if s == nil {
		return
	}
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return
	}
	s.userKnowledgeDir = trimmed
}

func (s *Service) SetWebSearchProvider(provider WebSearchProvider) {
	if s == nil || provider == nil {
		return
	}
	s.web = provider
}

func (s *Service) webSearch(ctx context.Context, args map[string]any) (WebSearchResponse, error) {
	query := strings.TrimSpace(stringArg(args, "query", "q"))
	if query == "" {
		return WebSearchResponse{}, errors.New("query is required")
	}
	engine := strings.TrimSpace(stringArg(args, "engine"))
	maxResults := intArg(args, "maxResults", 5)

	if s.web == nil {
		s.web = newDefaultWebSearchProvider()
	}
	return s.web.Search(ctx, WebSearchRequest{
		Query:      query,
		Engine:     engine,
		MaxResults: maxResults,
	})
}

func (s *Service) SetDiagnostics(diag Diagnostics) {
	s.diag = diag
}

type ctxKey string

const (
	ctxKeyStreamID       ctxKey = "aichat_stream_id"
	ctxKeyConversationID ctxKey = "aichat_conversation_id"
)

func WithDiagnosticsContext(ctx context.Context, conversationID string, streamID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if trimmed := strings.TrimSpace(conversationID); trimmed != "" {
		ctx = context.WithValue(ctx, ctxKeyConversationID, trimmed)
	}
	if trimmed := strings.TrimSpace(streamID); trimmed != "" {
		ctx = context.WithValue(ctx, ctxKeyStreamID, trimmed)
	}
	return ctx
}

func (s *Service) log(ctx context.Context, event string, fields map[string]any) {
	if s == nil || s.diag == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	if conv, _ := ctx.Value(ctxKeyConversationID).(string); strings.TrimSpace(conv) != "" {
		fields["conversationId"] = conv
	}
	if stream, _ := ctx.Value(ctxKeyStreamID).(string); strings.TrimSpace(stream) != "" {
		fields["streamId"] = stream
	}
	s.diag.Log(event, fields)
}

func (s *Service) diagRawEnabled() bool {
	if s == nil || s.diag == nil {
		return false
	}
	type rawEnabled interface {
		IncludeRaw() bool
	}
	rd, ok := s.diag.(rawEnabled)
	if !ok {
		return false
	}
	return rd.IncludeRaw()
}

func looksLikeToolProtocolJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	trimmed = stripCodeFence(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, `"assistantmessage"`) ||
		strings.Contains(lower, `"assistant_message"`) ||
		strings.Contains(lower, `"toolcalls"`) ||
		strings.Contains(lower, `"tool_calls"`)
}

func toolProtocolRepairInstruction(locale uiLocale) string {
	if locale == uiLocaleZH {
		return strings.TrimSpace(`
你上一条回复的 JSON 格式不合法或被截断了。
请重新输出【完整且合法】的 tool-protocol JSON 对象，并且只输出 JSON（不要额外文本/不要 Markdown code fence）。

重要：这里的 JSON 不是 Mongo/SQL 语句 JSON，而是包含 assistantMessage/toolCalls（以及可选 intent）的响应 JSON。
你可以把 assistantMessage 设为 ""，只用 toolCalls 驱动工具调用。

示例（请按此结构输出）：
{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_xxx","database":"db","statement":"SELECT 1","pageSize":100}}],"intent":{"currentFocus":"auto","reason":"optional","confidence":0.8}}

要求：
- 只允许一个 JSON 对象
- 必须包含键：assistantMessage（string） 和 toolCalls（array，可为空）
- intent 可选；如果输出，必须是对象，并使用结构化字段而不是自然语言标签
- assistantMessage 内如果包含双引号/换行，必须正确转义
`)
	}

	return strings.TrimSpace(`
Your previous response was not valid JSON or was truncated.
Please re-output a COMPLETE, VALID tool-protocol JSON object and output ONLY JSON (no extra text / no Markdown code fences).

Important: this JSON is NOT a Mongo/SQL statement JSON; it must contain assistantMessage/toolCalls (and may include optional intent).
You may set assistantMessage to "" and rely on toolCalls only.

Example (follow this exact shape):
{"assistantMessage":"","toolCalls":[{"name":"execute_statement","arguments":{"datasourceId":"ds_xxx","database":"db","statement":"SELECT 1","pageSize":100}}],"intent":{"currentFocus":"auto","reason":"optional","confidence":0.8}}

Requirements:
- exactly one JSON object
- must include keys: assistantMessage (string) and toolCalls (array, may be empty)
- intent is optional; when present it must stay structured
- correctly escape quotes/newlines inside assistantMessage
`)
}

func executeStatementRepairInstruction(locale uiLocale) string {
	if locale == uiLocaleZH {
		return strings.TrimSpace(`
上一次 execute_statement 执行失败；错误详情已在上方的 tool_result 中提供。
请根据 error 修正 statement（例如语法/字段/引号/HTTP method/参数等），并重新输出【完整且合法】的 tool-protocol JSON（只输出 JSON，不要额外文本/不要 Markdown code fence）。

规则：
- 如果你要重试执行：assistantMessage 设为 ""，并输出新的 execute_statement toolCalls。
- arguments.statement 必须是原始语句（不要 Markdown code fence）。
- datasourceId/database 默认保持不变（除非你明确需要更改，并在 assistantMessage 里说明原因）。
- 如果无法通过修改 statement 修复（例如连接/权限/鉴权问题）：请在 assistantMessage 里说明原因和下一步，toolCalls 置空。

提示：
- Elasticsearch：当请求包含 JSON body 时，优先用 POST（避免某些网关/代理不支持 GET body）。
`)
	}

	return strings.TrimSpace(`
The previous execute_statement failed; the error details are provided in the tool_result above.
Please fix the statement based on the error (syntax/fields/quoting/HTTP method/params, etc.) and re-output a COMPLETE, VALID tool-protocol JSON object (output ONLY JSON; no extra text / no Markdown code fences).

Rules:
- If you want to retry: set assistantMessage="" and output new execute_statement toolCalls.
- arguments.statement must be raw (no Markdown fences).
- Keep datasourceId/database unchanged by default (unless you must change them and explain why in assistantMessage).
- If the failure cannot be fixed by changing the statement (e.g. connectivity/auth/permissions), explain next steps in assistantMessage and leave toolCalls empty.

Tip:
- Elasticsearch: when sending a JSON body, prefer POST to avoid gateways/proxies that reject GET bodies.
`)
}

type explainStatementArgs struct {
	DatasourceID string
	Database     string
	Statement    string
}

func explainStatementArgsFromToolArgs(req TurnRequest, args map[string]any) (explainStatementArgs, error) {
	statement := sanitizeStatementForTool(statementArg(args, "statement"))
	if statement == "" {
		return explainStatementArgs{}, errors.New("explain_statement requires statement")
	}
	datasourceID := stringArg(args, "datasourceId", "id")
	if datasourceID == "" {
		datasourceID = strings.TrimSpace(defaultDatasourceIDForTool(req))
	}
	if datasourceID == "" {
		return explainStatementArgs{}, errors.New("explain_statement requires datasourceId (or an established working context)")
	}
	database := stringArg(args, "database")
	if database == "" {
		database = strings.TrimSpace(defaultDatabaseForTool(req))
	}
	return explainStatementArgs{
		DatasourceID: datasourceID,
		Database:     database,
		Statement:    statement,
	}, nil
}

type executeStatementArgs struct {
	DatasourceID string
	Database     string
	Statement    string
	PagingToken  string
	PageSize     int
}

func shouldPreferPageContextForExecuteDefaults(req TurnRequest) bool {
	if turnIntentPrefersCurrentFocus(req) {
		return true
	}
	if turnIntentAvoidsCurrentFocus(req) {
		return false
	}
	if req.WorkingContext == nil {
		return false
	}
	if strings.TrimSpace(req.PageContext.CurrentDatasourceID) == "" && strings.TrimSpace(req.PageContext.CurrentDatabase) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(req.WorkingContext.Source), "sticky")
}

func defaultToolTargetContext(req TurnRequest) executeStatementContext {
	focus := pageExecuteStatementContext(req.PageContext)
	if shouldPreferPageContextForExecuteDefaults(req) {
		return focus
	}
	if working := establishedWorkingContext(req); working != nil {
		target := executeStatementContext{
			DatasourceID: strings.TrimSpace(working.DatasourceID),
			Database:     strings.TrimSpace(working.Database),
		}
		if target.DatasourceID == "" {
			target.DatasourceID = focus.DatasourceID
		}
		// Only inherit the page database when both contexts stay on the same datasource.
		if target.Database == "" && (target.DatasourceID == "" || strings.EqualFold(target.DatasourceID, focus.DatasourceID)) {
			target.Database = focus.Database
		}
		if target.DatasourceID != "" || target.Database != "" {
			return target
		}
	}
	if turnIntentAvoidsCurrentFocus(req) {
		return executeStatementContext{}
	}
	return focus
}

func defaultDatasourceIDForTool(req TurnRequest) string {
	return strings.TrimSpace(defaultToolTargetContext(req).DatasourceID)
}

func defaultDatabaseForTool(req TurnRequest) string {
	return strings.TrimSpace(defaultToolTargetContext(req).Database)
}

func executeStatementArgsFromToolArgs(req TurnRequest, args map[string]any) (executeStatementArgs, error) {
	statement := sanitizeStatementForTool(statementArg(args, "statement"))
	if statement == "" {
		return executeStatementArgs{}, errors.New("execute_statement requires statement")
	}
	datasourceID := stringArg(args, "datasourceId", "id")
	if datasourceID == "" {
		datasourceID = strings.TrimSpace(defaultDatasourceIDForTool(req))
	}
	if datasourceID == "" {
		return executeStatementArgs{}, errors.New("execute_statement requires datasourceId (or an established working context)")
	}
	database := stringArg(args, "database")
	if database == "" {
		database = strings.TrimSpace(defaultDatabaseForTool(req))
	}
	pageSize := normalizeExecuteStatementPageSize(intArg(args, "pageSize", 100))
	pagingToken := stringArg(args, "pagingToken", "pageToken", "nextToken")
	return executeStatementArgs{
		DatasourceID: datasourceID,
		Database:     database,
		Statement:    statement,
		PagingToken:  pagingToken,
		PageSize:     pageSize,
	}, nil
}

func intArg(args map[string]any, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &parsed); err == nil {
			return parsed
		}
		return fallback
	}
}

func executeStatementErrorPayload(ds DatasourceSummary, req executeStatementArgs, execErr error) map[string]any {
	payload := map[string]any{
		"error":        strings.TrimSpace(errString(execErr)),
		"datasourceId": strings.TrimSpace(req.DatasourceID),
		"database":     strings.TrimSpace(req.Database),
		"pageSize":     req.PageSize,
		"pagingToken":  strings.TrimSpace(req.PagingToken),
	}
	if strings.TrimSpace(ds.Name) != "" {
		payload["datasourceName"] = strings.TrimSpace(ds.Name)
	}
	if strings.TrimSpace(ds.Type) != "" {
		payload["datasourceType"] = strings.TrimSpace(ds.Type)
	}
	if strings.TrimSpace(ds.Dialect) != "" {
		payload["dialect"] = strings.TrimSpace(ds.Dialect)
	}
	if strings.TrimSpace(ds.Environment) != "" {
		payload["environment"] = strings.TrimSpace(ds.Environment)
	}
	if info, ok := console.RiskInfoFromError(execErr); ok {
		risk := map[string]any{
			"action":  info.Action,
			"level":   info.Level,
			"reasons": info.Reasons,
		}
		if strings.TrimSpace(info.RuleID) != "" {
			risk["ruleId"] = info.RuleID
		}
		if strings.TrimSpace(info.RuleCode) != "" {
			risk["ruleCode"] = info.RuleCode
		}
		if strings.TrimSpace(info.RuleDescription) != "" {
			risk["ruleDescription"] = info.RuleDescription
		}
		if len(info.SuggestedRewrites) > 0 {
			risk["suggestedRewrites"] = info.SuggestedRewrites
		}
		payload["risk"] = risk
	}

	stmt := strings.TrimSpace(req.Statement)
	const maxStatementRunes = 10_000
	if maxStatementRunes > 0 {
		var runeCount int
		for i := range stmt {
			if runeCount >= maxStatementRunes {
				stmt = stmt[:i] + "…"
				payload["statementTruncated"] = true
				break
			}
			runeCount++
		}
	}
	payload["statement"] = stmt
	return payload
}

func isConsoleRoute(pc PageContext) bool {
	name := strings.ToLower(strings.TrimSpace(pc.RouteName))
	if name == "console" {
		return true
	}
	path := strings.ToLower(strings.TrimSpace(pc.RoutePath))
	return strings.Contains(path, "/console")
}

func lastUserText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.ToLower(strings.TrimSpace(messages[i].Role)) == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func userRequestedResultAnalysis(userText string) bool {
	lower := strings.ToLower(strings.TrimSpace(userText))
	if lower == "" {
		return false
	}

	analysisNeedles := []string{
		"分析", "总结", "解读", "洞察", "insight", "analy", "summarize", "summary", "interpret",
	}
	pastNeedles := []string{
		"刚才", "上次", "刚刚", "之前", "上述", "以上", "前面", "上面", "上一条", "上一步", "上一个",
		"last", "previous", "earlier", "above",
	}

	hasAnalysis := false
	for _, needle := range analysisNeedles {
		if strings.Contains(lower, needle) {
			hasAnalysis = true
			break
		}
	}
	if !hasAnalysis {
		return false
	}

	for _, needle := range pastNeedles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func userRequestedResultVisualization(userText string) bool {
	lower := strings.ToLower(strings.TrimSpace(userText))
	if lower == "" {
		return false
	}

	visNeedles := []string{
		"可视化", "图表", "画图", "绘图", "趋势图", "柱状图", "折线图", "饼图", "散点图", "直方图", "热力图",
		"visualize", "visualisation", "visualization", "chart", "plot", "graph",
		"echarts", "e charts", "three", "threejs", "three.js",
	}
	pastNeedles := []string{
		"刚才", "上次", "刚刚", "之前", "上述", "以上", "前面", "上面", "上一条", "上一步", "上一个", "这次", "这个", "结果", "查询结果",
		"last", "previous", "earlier", "above", "result",
	}

	hasVis := false
	for _, needle := range visNeedles {
		if strings.Contains(lower, needle) {
			hasVis = true
			break
		}
	}
	if !hasVis {
		return false
	}

	for _, needle := range pastNeedles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func (s *Service) tryDirectAnalyzeResultApproval(
	ctx context.Context,
	req TurnRequest,
	conversationID string,
	locale uiLocale,
	mode string,
) (TurnResponse, bool) {
	if s == nil || s.analysis == nil || s.approvals == nil {
		return TurnResponse{}, false
	}
	if !isConsoleRoute(req.PageContext) {
		return TurnResponse{}, false
	}

	userText := lastUserText(req.Messages)
	if !userRequestedResultAnalysis(userText) {
		return TurnResponse{}, false
	}

	stored, ok := s.analysis.GetResult(conversationID)
	if !ok || len(stored.Rows) == 0 {
		if locale == uiLocaleZH {
			return TurnResponse{AssistantMessage: "没有可供分析的最近查询结果，请先执行一次查询。"}, true
		}
		return TurnResponse{AssistantMessage: "No recent query result available to analyze. Please run a query first."}, true
	}

	approvalID := newApprovalID()
	storedArgs := map[string]any{
		"aiConfigId": strings.TrimSpace(req.AIConfigID),
		"question":   strings.TrimSpace(userText),
		"lang":       string(locale),
	}
	s.approvals.put(conversationID, approvalID, pendingToolCall{
		ThreadID:  strings.TrimSpace(req.ThreadID),
		Name:      "analyze_result",
		Arguments: storedArgs,
	})
	s.log(ctx, "approval_pending", map[string]any{
		"kind":       string(ApprovalAnalyzeResult),
		"approvalId": approvalID,
		"rowCount":   stored.RowCount,
		"rows":       len(stored.Rows),
		"bytes":      stored.ApproxBytes,
		"truncated":  stored.RowsTruncated,
		"reason":     "direct_result_analysis",
		"mode":       mode,
	})
	return TurnResponse{
		AssistantMessage: defaultAnalyzeResultAssistantMessage(locale, stored),
		Approval: &Approval{
			ID:      approvalID,
			Kind:    ApprovalAnalyzeResult,
			Summary: summarizeAnalyzeResult(locale, stored),
			Payload: sanitizeAnalyzeResultPayload(stored),
		},
	}, true
}

func (s *Service) tryDirectCreateVisualizationApproval(
	ctx context.Context,
	req TurnRequest,
	conversationID string,
	locale uiLocale,
	mode string,
) (TurnResponse, bool) {
	if s == nil || s.analysis == nil || s.approvals == nil {
		return TurnResponse{}, false
	}

	userText := lastUserText(req.Messages)
	if !userRequestedResultVisualization(userText) {
		return TurnResponse{}, false
	}

	stored, ok := s.analysis.GetResult(conversationID)
	if !ok || len(stored.Rows) == 0 {
		if locale == uiLocaleZH {
			return TurnResponse{AssistantMessage: "没有可供可视化的最近查询结果，请先执行一次查询。"}, true
		}
		return TurnResponse{AssistantMessage: "No recent query result available to visualize. Please run a query first."}, true
	}

	approvalID := newApprovalID()
	storedArgs := map[string]any{
		"aiConfigId": strings.TrimSpace(req.AIConfigID),
		"question":   strings.TrimSpace(userText),
		"lang":       string(locale),
	}
	s.approvals.put(conversationID, approvalID, pendingToolCall{
		ThreadID:  strings.TrimSpace(req.ThreadID),
		Name:      "create_visualization",
		Arguments: storedArgs,
	})
	s.log(ctx, "approval_pending", map[string]any{
		"kind":       string(ApprovalCreateVisualization),
		"approvalId": approvalID,
		"rowCount":   stored.RowCount,
		"rows":       len(stored.Rows),
		"bytes":      stored.ApproxBytes,
		"truncated":  stored.RowsTruncated,
		"reason":     "direct_result_visualization",
		"mode":       mode,
	})
	return TurnResponse{
		AssistantMessage: defaultCreateVisualizationAssistantMessage(locale, stored),
		Approval: &Approval{
			ID:      approvalID,
			Kind:    ApprovalCreateVisualization,
			Summary: summarizeCreateVisualization(locale, stored),
			Payload: sanitizeCreateVisualizationPayload(stored),
		},
	}, true
}

func defaultExecuteAssistantMessage(locale uiLocale, ds DatasourceSummary, req executeStatementArgs) string {
	label := ds.Name
	if strings.TrimSpace(label) == "" {
		label = req.DatasourceID
	}
	codeLang := "text"
	statement := strings.TrimSpace(req.Statement)
	switch strings.ToLower(strings.TrimSpace(ds.Type)) {
	case "mysql", "postgresql", "d1":
		codeLang = "sql"
	case "mongodb":
		codeLang = "javascript"
		statement = formatMongoStatementForHuman(statement)
	case "redis":
		codeLang = "redis"
	}
	if locale == uiLocaleZH {
		return fmt.Sprintf("我可以在 `%s` 上执行：\n\n```%s\n%s\n```", label, codeLang, statement)
	}
	return fmt.Sprintf("I can run this on `%s`:\n\n```%s\n%s\n```", label, codeLang, statement)
}

func formatMongoStatementForHuman(statement string) string {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "map[") {
		if normalized, changed, err := normalizeMongoStatementForTool(trimmed); err == nil && changed {
			trimmed = strings.TrimSpace(normalized)
		}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed
	}

	type mongoStatementPayload struct {
		Collection string         `json:"collection"`
		Action     string         `json:"action"`
		Filter     map[string]any `json:"filter"`
		Document   any            `json:"document"`
		Update     any            `json:"update"`
		Pipeline   any            `json:"pipeline"`
		Keys       any            `json:"keys"`
		Options    map[string]any `json:"options"`
		Limit      any            `json:"limit"`
		Sort       any            `json:"sort"`
		Projection any            `json:"projection"`
		Skip       any            `json:"skip"`
		Hint       any            `json:"hint"`
	}

	var payload mongoStatementPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}

	action := normalizeMongoAction(payload.Action)
	collection := strings.TrimSpace(payload.Collection)
	if collection == "" || action == "" {
		return trimmed
	}

	filter := payload.Filter
	if filter == nil {
		filter = map[string]any{}
	}
	options := payload.Options
	if options == nil {
		options = map[string]any{}
	}
	if payload.Sort != nil && options["sort"] == nil {
		options["sort"] = payload.Sort
	}
	if payload.Projection != nil && options["projection"] == nil {
		options["projection"] = payload.Projection
	}
	if payload.Skip != nil && options["skip"] == nil {
		options["skip"] = payload.Skip
	}
	if payload.Hint != nil && options["hint"] == nil {
		options["hint"] = payload.Hint
	}

	renderJSON := func(value any) string {
		if value == nil {
			return "{}"
		}
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}

	switch action {
	case "find":
		var b strings.Builder
		b.WriteString("db.")
		b.WriteString(collection)
		b.WriteString(".find(")
		b.WriteString(renderJSON(filter))
		if proj, ok := options["projection"]; ok && proj != nil {
			b.WriteString(", ")
			b.WriteString(renderJSON(proj))
		}
		b.WriteString(")")
		if sort, ok := options["sort"]; ok && sort != nil {
			b.WriteString(".sort(")
			b.WriteString(renderJSON(sort))
			b.WriteString(")")
		}
		if hint, ok := options["hint"]; ok && hint != nil {
			b.WriteString(".hint(")
			b.WriteString(renderJSON(hint))
			b.WriteString(")")
		}
		if skip, ok := options["skip"]; ok && skip != nil {
			b.WriteString(".skip(")
			b.WriteString(fmt.Sprint(skip))
			b.WriteString(")")
		}
		limit := payload.Limit
		if limit == nil {
			limit = options["limit"]
		}
		if limit != nil {
			b.WriteString(".limit(")
			b.WriteString(fmt.Sprint(limit))
			b.WriteString(")")
		}
		return b.String()
	case "aggregate":
		return fmt.Sprintf("db.%s.aggregate(%s)", collection, renderJSON(payload.Pipeline))
	case "insertone":
		return fmt.Sprintf("db.%s.insertOne(%s)", collection, renderJSON(payload.Document))
	case "insertmany":
		return fmt.Sprintf("db.%s.insertMany(%s)", collection, renderJSON(payload.Document))
	case "updateone":
		return fmt.Sprintf("db.%s.updateOne(%s, %s, %s)", collection, renderJSON(filter), renderJSON(payload.Update), renderJSON(options))
	case "updatemany":
		return fmt.Sprintf("db.%s.updateMany(%s, %s, %s)", collection, renderJSON(filter), renderJSON(payload.Update), renderJSON(options))
	case "deleteone":
		return fmt.Sprintf("db.%s.deleteOne(%s, %s)", collection, renderJSON(filter), renderJSON(options))
	case "deletemany":
		return fmt.Sprintf("db.%s.deleteMany(%s, %s)", collection, renderJSON(filter), renderJSON(options))
	case "createindex":
		return fmt.Sprintf("db.%s.createIndex(%s, %s)", collection, renderJSON(payload.Keys), renderJSON(options))
	case "dropindex":
		name, _ := options["name"]
		if name != nil {
			return fmt.Sprintf("db.%s.dropIndex(%s)", collection, renderJSON(name))
		}
		return fmt.Sprintf("db.%s.dropIndex(<indexName>)", collection)
	case "createcollection":
		return fmt.Sprintf("db.createCollection(%s)", renderJSON(collection))
	case "drop":
		return fmt.Sprintf("db.%s.drop()", collection)
	default:
		return trimmed
	}
}

func normalizeMongoAction(action string) string {
	return riskengine.NormalizeMongoAction(action)
}
func datasourceSummaryToDataSource(ds DatasourceSummary) datasource.DataSource {
	out := datasource.DataSource{
		ID:       ds.ID,
		Name:     ds.Name,
		Type:     datasource.DataSourceType(ds.Type),
		Host:     ds.Host,
		Port:     ds.Port,
		Database: ds.Database,
	}
	if trust := strings.TrimSpace(ds.TrustLevel); trust != "" {
		out.Options = map[string]any{datasource.TrustLevelOptionKey: trust}
	}
	if env := strings.TrimSpace(ds.Environment); env != "" {
		if out.Options == nil {
			out.Options = map[string]any{}
		}
		out.Options[datasource.EnvironmentOptionKey] = env
	}
	return out
}

// assessStatement runs the riskengine Guard as a pure fact query. Returns the
// assessment plus any probe explain result. Callers combine the assessment
// with the datasource trust level via riskengine.DecideGate to decide whether
// to auto-run or request approval.
// Panics if riskGuard is nil — it must be set during bootstrap via SetRiskGuard.
func (s *Service) assessStatement(ctx context.Context, ds DatasourceSummary, statement string) (riskengine.RiskAssessment, *console.ExplainResult) {
	if s.riskGuard == nil {
		panic("aichat: riskGuard not set — call SetRiskGuard during bootstrap")
	}
	fullDS := datasourceSummaryToDataSource(ds)
	assessment, explain, _ := s.riskGuard.Assess(ctx, fullDS, statement)
	return assessment, explain
}

func appendRiskAutoExecuteDetails(locale uiLocale, message string, ds DatasourceSummary, req executeStatementArgs) string {
	if strings.TrimSpace(message) == "" {
		return message
	}
	target := strings.TrimSpace(ds.Name)
	if target == "" {
		target = req.DatasourceID
	}
	var autoLine string
	if locale == uiLocaleZH {
		autoLine = "正在自动执行..."
	} else {
		autoLine = "Auto-executing..."
	}
	return strings.TrimSpace(message) + "\n" + autoLine
}

func appendRiskApprovalDetails(locale uiLocale, message string, ds DatasourceSummary, req executeStatementArgs, assessment riskengine.RiskAssessment, explain *console.ExplainResult) string {
	b := strings.Builder{}
	if strings.TrimSpace(message) != "" {
		b.WriteString(strings.TrimSpace(message))
		b.WriteString("\n\n")
	}

	riskLabel := "Risk"
	riskNotesLabel := "Notes"
	approveLine := "⬆️ Click **Approve** above to execute."
	if locale == uiLocaleZH {
		riskLabel = "风险"
		riskNotesLabel = "说明"
		approveLine = "⬆️ 点击上方 **批准** 执行。"
	}

	level := strings.ToUpper(string(assessment.Level))
	if locale == uiLocaleZH {
		switch assessment.Level {
		case riskengine.RiskLow:
			level = "低"
		case riskengine.RiskMedium:
			level = "中"
		case riskengine.RiskHigh:
			level = "高"
		}
	}
	b.WriteString(fmt.Sprintf("- %s: %s\n", riskLabel, level))
	if len(assessment.Reasons) > 0 {
		b.WriteString(fmt.Sprintf("- %s: %s\n", riskNotesLabel, strings.Join(assessment.Reasons, "; ")))
	}
	if explain != nil {
		if explain.UsesIndex {
			if len(explain.Indexes) > 0 {
				b.WriteString(fmt.Sprintf("- EXPLAIN: uses index (%s)\n", strings.Join(explain.Indexes, ", ")))
			}
		} else {
			stageHint := ""
			if len(explain.Stages) > 0 {
				stageHint = fmt.Sprintf(" (%s)", strings.Join(explain.Stages, ", "))
			}
			b.WriteString(fmt.Sprintf("- EXPLAIN: no index%s\n", stageHint))
		}
	}
	b.WriteString("\n" + approveLine)
	return strings.TrimSpace(b.String())
}

func summarizeExecuteStatement(locale uiLocale, ds DatasourceSummary, assessment riskengine.RiskAssessment, explain *console.ExplainResult) string {
	target := strings.TrimSpace(ds.Name)
	if target == "" {
		target = ds.ID
	}
	tags := make([]string, 0, 2)
	if assessment.Level == riskengine.RiskHigh {
		tags = append(tags, "destructive")
	}
	if explain != nil && !explain.UsesIndex {
		tags = append(tags, "no index")
	}
	if len(tags) > 0 {
		if locale == uiLocaleZH {
			translated := make([]string, 0, len(tags))
			for _, tag := range tags {
				switch tag {
				case "destructive":
					translated = append(translated, "高风险")
				case "no index":
					translated = append(translated, "无索引")
				default:
					translated = append(translated, tag)
				}
			}
			return fmt.Sprintf(`在 "%s" 上执行语句（%s）`, target, strings.Join(translated, "，"))
		}
		return fmt.Sprintf(`Execute statement on "%s" (%s)`, target, strings.Join(tags, ", "))
	}
	if locale == uiLocaleZH {
		return fmt.Sprintf(`在 "%s" 上执行语句`, target)
	}
	return fmt.Sprintf(`Execute statement on "%s"`, target)
}

func sanitizeExecuteStatementPayload(ds DatasourceSummary, req executeStatementArgs, assessment riskengine.RiskAssessment, explain *console.ExplainResult) map[string]any {
	payload := map[string]any{
		"datasourceId": req.DatasourceID,
		"database":     strings.TrimSpace(req.Database),
		"pageSize":     req.PageSize,
		"pagingToken":  strings.TrimSpace(req.PagingToken),
	}
	stmt := strings.TrimSpace(req.Statement)
	if strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
		human := formatMongoStatementForHuman(stmt)
		if human != stmt {
			stmt = human
		} else {
			trimmed := strings.TrimSpace(stmt)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				var parsed any
				if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
					if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
						stmt = string(pretty)
					}
				}
			}
		}
	}

	const maxApprovalStatementRunes = 10_000
	truncated := false
	if maxApprovalStatementRunes > 0 {
		var runeCount int
		for i := range stmt {
			if runeCount >= maxApprovalStatementRunes {
				stmt = stmt[:i] + "…"
				truncated = true
				break
			}
			runeCount++
		}
	}
	payload["statement"] = stmt
	if truncated {
		payload["statementTruncated"] = true
	}
	level := string(assessment.Level)
	if strings.TrimSpace(level) == "" {
		level = string(riskengine.RiskLow)
	}
	risk := map[string]any{
		"level":   level,
		"reasons": assessment.Reasons,
	}
	if strings.TrimSpace(assessment.RuleID) != "" {
		risk["ruleId"] = assessment.RuleID
	}
	if strings.TrimSpace(assessment.RuleCode) != "" {
		risk["ruleCode"] = assessment.RuleCode
	}
	if strings.TrimSpace(assessment.RuleDescription) != "" {
		risk["ruleDescription"] = assessment.RuleDescription
	}
	if rewrites := riskengine.SuggestedRewritesForAssessment(assessment); len(rewrites) > 0 {
		risk["suggestedRewrites"] = rewrites
	}
	risk["builtin"] = assessment.Builtin
	payload["risk"] = risk
	if explain != nil {
		payload["explain"] = explain
	}
	if strings.TrimSpace(ds.Name) != "" {
		payload["datasourceName"] = ds.Name
	}
	if strings.TrimSpace(ds.Type) != "" {
		payload["datasourceType"] = ds.Type
	}
	if strings.TrimSpace(ds.Dialect) != "" {
		payload["dialect"] = ds.Dialect
	}
	if strings.TrimSpace(ds.Environment) != "" {
		payload["environment"] = ds.Environment
	}
	if trust := strings.TrimSpace(ds.TrustLevel); trust != "" {
		payload["trustLevel"] = trust
	}
	return payload
}

func validateMongoStatementForTool(statement string) error {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return errors.New("mongo statement is required")
	}
	if strings.HasPrefix(trimmed, "map[") {
		return errors.New("mongo statement must be JSON or Mongo Shell (db.xxx...), not Go map format")
	}
	if strings.HasPrefix(trimmed, "[") {
		return errors.New("mongo statement must be a JSON object with action/collection (not a bare pipeline array)")
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Action     string `json:"action"`
			Collection string `json:"collection"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return fmt.Errorf("invalid mongo statement JSON: %w", err)
		}
		if strings.TrimSpace(payload.Collection) == "" {
			return errors.New("mongo statement JSON missing collection")
		}
		if strings.TrimSpace(payload.Action) == "" {
			return errors.New("mongo statement JSON missing action")
		}
		return nil
	}
	if strings.HasPrefix(trimmed, "db") {
		return nil
	}
	return errors.New("mongo statement must start with db. or be a JSON object with action/collection")
}

func (s *Service) Approve(ctx context.Context, req ApproveRequest) (resp TurnResponse, err error) {
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return TurnResponse{}, errors.New("conversationId is required")
	}
	ctx = WithDiagnosticsContext(ctx, conversationID, "")
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID == "" {
		return TurnResponse{}, errors.New("approvalId is required")
	}
	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision == "" {
		return TurnResponse{}, errors.New("decision is required")
	}
	if decision != "approve" && decision != "reject" {
		return TurnResponse{}, errors.New("decision must be approve or reject")
	}
	req.ThreadID = s.resolveApprovalThreadID(req)
	defer func() {
		if err != nil {
			return
		}
		s.appendThreadResponseEvents(req.ThreadID, resp)
		resp.Memory = mergeMemoryEnvelopes(resp.Memory, s.buildResponseMemory(req.ThreadID))
	}()

	s.log(ctx, "approval_decision", map[string]any{
		"approvalId": approvalID,
		"decision":   decision,
	})

	if handledResp, handled, handledErr := s.approveWithEinoCheckpoint(ctx, req, decision); handled {
		resp = handledResp
		err = handledErr
		if err == nil {
			s.persistThreadSession(req.ThreadID, conversationID)
			s.appendThreadApprovalDecision(req.ThreadID, approvalID, decision)
		}
		return resp, err
	}

	call, ok := s.approvals.take(conversationID, approvalID)
	if !ok {
		return TurnResponse{}, errors.New("approval not found")
	}
	s.persistThreadSession(req.ThreadID, conversationID)
	s.appendThreadApprovalDecision(req.ThreadID, approvalID, decision)

	if decision == "reject" {
		locale := uiLocaleFromString(stringArg(call.Arguments, "lang", "locale", "language"))
		if locale == uiLocaleZH {
			return TurnResponse{AssistantMessage: "好的，已取消。"}, nil
		}
		return TurnResponse{AssistantMessage: "OK, cancelled."}, nil
	}

	switch call.Name {
	case "create_datasource":
		s.log(ctx, "approval_execute", map[string]any{
			"kind": string(ApprovalCreateDatasource),
		})
		input, err := datasourceCreateInputFromArgs(call.Arguments)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}
		created, err := s.tools.CreateDatasource(ctx, input)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}
		return TurnResponse{
			AssistantMessage: fmt.Sprintf(`Created datasource "%s" (%s).`, created.Name, created.ID),
			Effects:          Effects{DatasourcesChanged: true},
		}, nil
	case "delete_datasource":
		s.log(ctx, "approval_execute", map[string]any{
			"kind": string(ApprovalDeleteDatasource),
		})
		target, err := datasourceDeleteTargetFromArgs(call.Arguments)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}
		id := strings.TrimSpace(target.DatasourceID)
		if id == "" {
			list, err := s.tools.ListDatasources(ctx)
			if err != nil {
				return TurnResponse{}, err
			}
			for _, ds := range list {
				if strings.EqualFold(ds.Name, target.Name) {
					id = ds.ID
					break
				}
			}
			if id == "" {
				return TurnResponse{AssistantMessage: "Datasource not found."}, nil
			}
		}
		if err := s.tools.DeleteDatasource(ctx, id); err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}
		return TurnResponse{
			AssistantMessage: fmt.Sprintf("Deleted datasource %s.", id),
			Effects:          Effects{DatasourcesChanged: true},
		}, nil
	case "execute_statement":
		s.log(ctx, "approval_execute", map[string]any{
			"kind": string(ApprovalExecuteStatement),
		})
		locale := uiLocaleFromString(stringArg(call.Arguments, "lang", "locale", "language"))
		aiConfigID := strings.TrimSpace(stringArg(call.Arguments, "aiConfigId", "aiConfigID", "ai_config_id"))
		question := strings.TrimSpace(stringArg(call.Arguments, "question", "prompt", "instruction", "userText", "user_text"))
		repairAttempt := intArg(call.Arguments, "repairAttempt", 0)
		execReq, err := executeStatementArgsFromApprovalArgs(call.Arguments)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}
		ds, _ := s.tools.GetDatasource(ctx, execReq.DatasourceID)
		if strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
			if normalized, changed, err := normalizeMongoStatementForTool(execReq.Statement); err == nil {
				if changed {
					s.log(ctx, "mongo_statement_normalized", map[string]any{
						"mode":          "approve",
						"datasourceId":  execReq.DatasourceID,
						"database":      execReq.Database,
						"statementLen":  len(execReq.Statement),
						"normalizedLen": len(normalized),
					})
					execReq.Statement = normalized
				}
			} else {
				if locale == uiLocaleZH {
					return TurnResponse{AssistantMessage: fmt.Sprintf("执行失败：%s", err.Error())}, nil
				}
				return TurnResponse{AssistantMessage: fmt.Sprintf("Execute failed: %s", err.Error())}, nil
			}
			if err := validateMongoStatementForTool(execReq.Statement); err != nil {
				if locale == uiLocaleZH {
					return TurnResponse{AssistantMessage: fmt.Sprintf("执行失败：%s", err.Error())}, nil
				}
				return TurnResponse{AssistantMessage: fmt.Sprintf("Execute failed: %s", err.Error())}, nil
			}
		}
		execStartAt := time.Now()
		result, err := s.tools.ExecuteStatement(ctx, execReq.DatasourceID, execReq.Statement, execReq.Database, execReq.PagingToken, execReq.PageSize, true)
		if err != nil {
			s.log(ctx, "tool_execute_statement", map[string]any{
				"durationMs":   time.Since(execStartAt).Milliseconds(),
				"error":        err.Error(),
				"datasourceId": execReq.DatasourceID,
				"database":     execReq.Database,
				"statementLen": len(execReq.Statement),
			})
			if repairAttempt < 1 {
				if repaired, ok := s.tryRepairApprovedExecuteStatement(ctx, req.ThreadID, conversationID, aiConfigID, question, locale, ds, execReq, err, repairAttempt+1); ok {
					return repaired, nil
				}
			}
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: fmt.Sprintf("执行失败：%s", err.Error())}, nil
			}
			return TurnResponse{AssistantMessage: fmt.Sprintf("Execute failed: %s", err.Error())}, nil
		}
		s.log(ctx, "tool_execute_statement", map[string]any{
			"durationMs":   time.Since(execStartAt).Milliseconds(),
			"error":        "",
			"datasourceId": execReq.DatasourceID,
			"database":     execReq.Database,
			"statementLen": len(execReq.Statement),
			"rowCount":     result.RowCount,
			"hasMore":      result.HasMore,
			"elapsedMs":    result.ElapsedMs,
		})
		effectStatement := execReq.Statement
		if strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
			if formatted := formatMongoStatementForHuman(effectStatement); formatted != "" {
				effectStatement = formatted
			}
		}
		effect := ConsoleResultEffect{
			DatasourceID:   execReq.DatasourceID,
			DatasourceType: ds.Type,
			Database:       execReq.Database,
			Statement:      effectStatement,
			Result:         result,
		}
		s.analysis.PutResult(conversationID, effect)
		return TurnResponse{
			AssistantMessage: formatExecuteResultMarkdown(locale, ds, execReq, result),
			Effects: Effects{
				ConsoleResult: &effect,
			},
		}, nil
	case "analyze_result":
		s.log(ctx, "approval_execute", map[string]any{
			"kind": string(ApprovalAnalyzeResult),
		})
		locale := uiLocaleFromString(stringArg(call.Arguments, "lang", "locale", "language"))
		aiConfigID := strings.TrimSpace(stringArg(call.Arguments, "aiConfigId", "aiConfigID", "ai_config_id"))
		question := strings.TrimSpace(stringArg(call.Arguments, "question", "prompt", "instruction"))

		stored, ok := s.analysis.GetResult(conversationID)
		if !ok || len(stored.Rows) == 0 {
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: "没有可供分析的最近查询结果，请先执行一次查询。"}, nil
			}
			return TurnResponse{AssistantMessage: "No recent query result available to analyze. Please run a query first."}, nil
		}

		model, err := s.models.Resolve(aiConfigID)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}

		systemPrompt := analysisSystemPrompt(locale)
		analysisMessages := []Message{{Role: "user", Content: analysisUserContent(locale, stored, question)}}
		summary, err := s.runApprovedResultAnalysis(ctx, model, locale, stored, question, systemPrompt, analysisMessages)
		if err != nil {
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: fmt.Sprintf("分析失败：%s", err.Error())}, nil
			}
			return TurnResponse{AssistantMessage: fmt.Sprintf("Analysis failed: %s", err.Error())}, nil
		}

		s.analysis.PutSummary(conversationID, summary)
		return TurnResponse{AssistantMessage: summary}, nil
	case "create_visualization":
		s.log(ctx, "approval_execute", map[string]any{
			"kind": string(ApprovalCreateVisualization),
		})
		locale := uiLocaleFromString(stringArg(call.Arguments, "lang", "locale", "language"))
		aiConfigID := strings.TrimSpace(stringArg(call.Arguments, "aiConfigId", "aiConfigID", "ai_config_id"))
		question := strings.TrimSpace(stringArg(call.Arguments, "question", "prompt", "instruction"))

		stored, ok := s.analysis.GetResult(conversationID)
		if !ok || len(stored.Rows) == 0 {
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: "没有可供可视化的最近查询结果，请先执行一次查询。"}, nil
			}
			return TurnResponse{AssistantMessage: "No recent query result available to visualize. Please run a query first."}, nil
		}

		model, err := s.models.Resolve(aiConfigID)
		if err != nil {
			return TurnResponse{AssistantMessage: err.Error()}, nil
		}

		systemPrompt := visualizationSystemPrompt(locale)
		visMessages := []Message{{Role: "user", Content: visualizationUserContent(locale, stored, question)}}
		raw, err := model.Chat(ctx, systemPrompt, visMessages)
		if err != nil {
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: fmt.Sprintf("生成可视化失败：%s", err.Error())}, nil
			}
			return TurnResponse{AssistantMessage: fmt.Sprintf("Visualization failed: %s", err.Error())}, nil
		}

		parsed, err := parseVisualizationModelOutput(raw)
		if err != nil {
			if locale == uiLocaleZH {
				return TurnResponse{AssistantMessage: fmt.Sprintf("生成可视化失败：%s", err.Error())}, nil
			}
			return TurnResponse{AssistantMessage: fmt.Sprintf("Visualization failed: %s", err.Error())}, nil
		}

		effect := VisualizationEffect{
			Title:        strings.TrimSpace(parsed.Title),
			Renderer:     strings.TrimSpace(parsed.Renderer),
			Spec:         parsed.Spec,
			DatasourceID: strings.TrimSpace(stored.DatasourceID),
			Database:     strings.TrimSpace(stored.Database),
			Statement:    strings.TrimSpace(stored.Statement),
			RowCount:     stored.RowCount,
		}

		msg := "Visualization ready."
		if locale == uiLocaleZH {
			msg = "可视化已生成。"
		}
		return TurnResponse{
			AssistantMessage: msg,
			Effects: Effects{
				NavigateTo:    "/visualization",
				Visualization: &effect,
			},
		}, nil
	default:
		return TurnResponse{AssistantMessage: "Nothing to approve."}, nil
	}
}

func (s *Service) tryRepairApprovedExecuteStatement(
	ctx context.Context,
	threadID string,
	conversationID string,
	aiConfigID string,
	question string,
	locale uiLocale,
	ds DatasourceSummary,
	failedReq executeStatementArgs,
	execErr error,
	repairAttempt int,
) (TurnResponse, bool) {
	if s == nil || s.models == nil {
		return TurnResponse{}, false
	}
	model, err := s.models.Resolve(strings.TrimSpace(aiConfigID))
	if err != nil || model == nil {
		s.log(ctx, "execute_statement_repair_resolve_error", map[string]any{
			"error": errString(err),
		})
		return TurnResponse{}, false
	}

	systemPrompt := s.buildSystemPrompt(ctx, TurnRequest{
		AIConfigID:     strings.TrimSpace(aiConfigID),
		ConversationID: strings.TrimSpace(conversationID),
		PageContext: PageContext{
			CurrentDatasourceID:   strings.TrimSpace(failedReq.DatasourceID),
			CurrentDatasourceType: strings.TrimSpace(ds.Type),
			CurrentDatabase:       strings.TrimSpace(failedReq.Database),
			LastConsoleError:      strings.TrimSpace(errString(execErr)),
		},
	}, nil)

	seed := strings.TrimSpace(question)
	if seed == "" {
		if locale == uiLocaleZH {
			seed = "请根据错误信息修正语句并重试。"
		} else {
			seed = "Please fix the statement based on the error and retry."
		}
	}
	messages := []Message{
		{Role: "user", Content: seed},
		toolResultMessage("execute_statement", executeStatementErrorPayload(ds, failedReq, execErr)),
		{Role: "user", Content: executeStatementRepairInstruction(locale)},
	}

	startAt := time.Now()
	raw, err := model.Chat(ctx, systemPrompt, messages)
	elapsedMs := time.Since(startAt).Milliseconds()
	if err != nil {
		s.log(ctx, "execute_statement_repair_model_call", map[string]any{
			"durationMs": elapsedMs,
			"error":      err.Error(),
		})
		return TurnResponse{}, false
	}

	parsed, ok, parseErr := parseModelOutputDetailed(raw)
	if !ok && looksLikeToolProtocolJSON(raw) {
		s.log(ctx, "execute_statement_repair_tool_protocol_invalid", map[string]any{
			"rawLen":     len(raw),
			"parseError": errString(parseErr),
		})
		repairMessages := append(messages, Message{Role: "user", Content: toolProtocolRepairInstruction(locale)})
		if repairedRaw, err := model.Chat(ctx, systemPrompt, repairMessages); err == nil {
			if repairedParsed, repairedOK := parseModelOutput(repairedRaw); repairedOK {
				messages = repairMessages
				parsed = repairedParsed
				ok = true
			}
		}
	}
	if !ok {
		s.log(ctx, "execute_statement_repair_unparseable", map[string]any{
			"rawLen":     len(raw),
			"parseError": errString(parseErr),
		})
		return TurnResponse{}, false
	}

	if len(parsed.ToolCalls) == 0 {
		msg := strings.TrimSpace(parsed.AssistantMessage)
		if locale == uiLocaleZH {
			msg = strings.TrimSpace("执行失败：" + errString(execErr) + "\n\n" + msg)
		} else {
			msg = strings.TrimSpace("Execute failed: " + errString(execErr) + "\n\n" + msg)
		}
		if msg == "" {
			if locale == uiLocaleZH {
				msg = fmt.Sprintf("执行失败：%s", errString(execErr))
			} else {
				msg = fmt.Sprintf("Execute failed: %s", errString(execErr))
			}
		}
		return TurnResponse{AssistantMessage: msg}, true
	}

	for _, call := range parsed.ToolCalls {
		name := strings.ToLower(strings.TrimSpace(call.Name))
		if name != "execute_statement" {
			continue
		}
		args := call.args()
		toolReq := TurnRequest{
			ConversationID: strings.TrimSpace(conversationID),
			PageContext: PageContext{
				CurrentDatasourceID: strings.TrimSpace(failedReq.DatasourceID),
				CurrentDatabase:     strings.TrimSpace(failedReq.Database),
			},
		}
		retryReq, err := executeStatementArgsFromToolArgs(toolReq, args)
		if err != nil {
			continue
		}

		nextDS, dsErr := s.tools.GetDatasource(ctx, retryReq.DatasourceID)
		if dsErr == nil && strings.EqualFold(strings.TrimSpace(nextDS.Type), "mongodb") {
			if normalized, changed, err := normalizeMongoStatementForTool(retryReq.Statement); err == nil {
				if changed {
					retryReq.Statement = normalized
				}
			} else {
				continue
			}
			if err := validateMongoStatementForTool(retryReq.Statement); err != nil {
				continue
			}
		}

		var (
			repairAssessment riskengine.RiskAssessment
			repairExplain    *console.ExplainResult
		)
		if dsErr == nil {
			repairAssessment, repairExplain = s.assessStatement(ctx, nextDS, retryReq.Statement)
		}

		msg := strings.TrimSpace(parsed.AssistantMessage)
		if msg == "" {
			if locale == uiLocaleZH {
				msg = fmt.Sprintf("上一次执行失败：%s\n\n已根据错误信息生成修正语句，请再次确认执行。", errString(execErr))
			} else {
				msg = fmt.Sprintf("Previous execution failed: %s\n\nI updated the statement based on the error. Please approve to retry.", errString(execErr))
			}
		}
		if dsErr == nil {
			msg = appendRiskApprovalDetails(locale, msg, nextDS, retryReq, repairAssessment, repairExplain)
		}

		approvalID := newApprovalID()
		storedArgs := map[string]any{
			"datasourceId":  retryReq.DatasourceID,
			"database":      retryReq.Database,
			"statement":     retryReq.Statement,
			"pagingToken":   retryReq.PagingToken,
			"pageSize":      retryReq.PageSize,
			"lang":          string(locale),
			"aiConfigId":    strings.TrimSpace(aiConfigID),
			"question":      strings.TrimSpace(question),
			"repairAttempt": repairAttempt,
		}
		s.approvals.put(conversationID, approvalID, pendingToolCall{
			ThreadID:  strings.TrimSpace(threadID),
			Name:      "execute_statement",
			Arguments: storedArgs,
		})
		return TurnResponse{
			AssistantMessage: msg,
			Approval: &Approval{
				ID:      approvalID,
				Kind:    ApprovalExecuteStatement,
				Summary: summarizeExecuteStatement(locale, nextDS, repairAssessment, repairExplain),
				Payload: sanitizeExecuteStatementPayload(nextDS, retryReq, repairAssessment, repairExplain),
			},
		}, true
	}

	msg := strings.TrimSpace(parsed.AssistantMessage)
	if msg == "" {
		msg = strings.TrimSpace(raw)
	}
	if locale == uiLocaleZH {
		msg = strings.TrimSpace("执行失败：" + errString(execErr) + "\n\n" + msg)
	} else {
		msg = strings.TrimSpace("Execute failed: " + errString(execErr) + "\n\n" + msg)
	}
	return TurnResponse{AssistantMessage: msg}, true
}

func (s *Service) resolveApprovalThreadID(req ApproveRequest) string {
	conversationID := strings.TrimSpace(req.ConversationID)
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID != "" {
		if s != nil && s.einoApprovals != nil {
			if item, ok := s.einoApprovals.get(conversationID, approvalID); ok {
				return resolveThreadID(item.Request.ThreadID, conversationID)
			}
		}
		if s != nil && s.approvals != nil {
			if call, ok := s.approvals.get(conversationID, approvalID); ok {
				return resolveThreadID(call.ThreadID, conversationID)
			}
		}
	}
	return resolveThreadID(req.ThreadID, conversationID)
}

func (s *Service) runApprovedResultAnalysis(
	ctx context.Context,
	model Model,
	locale uiLocale,
	stored storedAnalysisResult,
	question string,
	systemPrompt string,
	analysisMessages []Message,
) (string, error) {
	if model == nil {
		return "", errors.New("ai model is nil")
	}

	runOnce := func(messages []Message, attempt int) (string, int64, error) {
		startAt := time.Now()
		raw, err := model.Chat(ctx, systemPrompt, messages)
		elapsedMs := time.Since(startAt).Milliseconds()
		if err != nil {
			s.log(ctx, "analysis_model_call", map[string]any{
				"attempt":     attempt,
				"durationMs":  elapsedMs,
				"error":       err.Error(),
				"rows":        len(stored.Rows),
				"bytes":       stored.ApproxBytes,
				"questionLen": len(question),
			})
			return "", elapsedMs, err
		}

		summary := extractAnalysisSummary(raw)
		if summary == "" {
			s.log(ctx, "analysis_model_call", map[string]any{
				"attempt":     attempt,
				"durationMs":  elapsedMs,
				"error":       "",
				"rows":        len(stored.Rows),
				"bytes":       stored.ApproxBytes,
				"questionLen": len(question),
				"assistantLen": func() int {
					return 0
				}(),
				"note": "empty_response",
			})
			return "", elapsedMs, nil
		}

		s.log(ctx, "analysis_model_call", map[string]any{
			"attempt":      attempt,
			"durationMs":   elapsedMs,
			"error":        "",
			"rows":         len(stored.Rows),
			"bytes":        stored.ApproxBytes,
			"questionLen":  len(question),
			"assistantLen": len(summary),
		})
		return summary, elapsedMs, nil
	}

	// First attempt.
	if summary, _, err := runOnce(analysisMessages, 1); err != nil {
		return "", err
	} else if summary != "" {
		return summary, nil
	}

	// Retry once with an explicit nudge; user has already approved sharing the same payload.
	retryInstruction := "Your previous response was empty. Please respond with a non-empty Markdown analysis."
	if locale == uiLocaleZH {
		retryInstruction = "你上一条回复是空的。请基于相同数据输出一段【非空】的 Markdown 分析（如果无法分析，请说明原因）。"
	}
	retryMessages := append(append([]Message(nil), analysisMessages...), Message{Role: "user", Content: retryInstruction})
	if summary, _, err := runOnce(retryMessages, 2); err != nil {
		return "", err
	} else if summary != "" {
		return summary, nil
	}

	// Last resort: provide a local summary instead of returning an empty response.
	return localAnalyzeResultSummary(locale, stored), nil
}

func extractAnalysisSummary(raw string) string {
	summary := strings.TrimSpace(raw)
	if summary == "" {
		return ""
	}
	if parsed, ok := parseModelOutput(raw); ok && len(parsed.ToolCalls) == 0 && strings.TrimSpace(parsed.AssistantMessage) != "" {
		return strings.TrimSpace(parsed.AssistantMessage)
	}
	return summary
}

func executeStatementArgsFromApprovalArgs(args map[string]any) (executeStatementArgs, error) {
	statement := sanitizeStatementForTool(statementArg(args, "statement"))
	if statement == "" {
		return executeStatementArgs{}, errors.New("execute_statement approval missing statement")
	}
	datasourceID := stringArg(args, "datasourceId", "id")
	if datasourceID == "" {
		return executeStatementArgs{}, errors.New("execute_statement approval missing datasourceId")
	}
	database := stringArg(args, "database")
	pageSize := normalizeExecuteStatementPageSize(intArg(args, "pageSize", 100))
	pagingToken := stringArg(args, "pagingToken", "pageToken", "nextToken")
	return executeStatementArgs{
		DatasourceID: datasourceID,
		Database:     database,
		Statement:    statement,
		PagingToken:  pagingToken,
		PageSize:     pageSize,
	}, nil
}

func sanitizeStatementForTool(statement string) string {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return ""
	}
	trimmed = normalizeSmartQuotes(trimmed)
	trimmed = stripAnyCodeFence(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`") {
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "`"), "`")
	}
	return strings.TrimSpace(trimmed)
}

func normalizeExecuteStatementPageSize(pageSize int) int {
	if pageSize < 1 {
		return 100
	}
	if pageSize > 500 {
		return 500
	}
	return pageSize
}

func normalizeSmartQuotes(input string) string {
	if input == "" {
		return ""
	}
	runes := []rune(input)
	var b strings.Builder
	b.Grow(len(input))

	inSingle := false
	inDouble := false
	inBacktick := false
	singleKind := literalQuoteKindNone
	doubleKind := literalQuoteKindNone

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		switch {
		case inBacktick:
			b.WriteRune(ch)
			if ch == '`' {
				inBacktick = false
			}
			continue
		case inSingle:
			if ch == '\\' && i+1 < len(runes) {
				b.WriteRune(ch)
				i++
				b.WriteRune(runes[i])
				continue
			}
			if singleKind == literalQuoteKindASCII {
				if ch == '\'' {
					if i+1 < len(runes) && runes[i+1] == '\'' {
						b.WriteString("''")
						i++
						continue
					}
					b.WriteByte('\'')
					inSingle = false
					singleKind = literalQuoteKindNone
					continue
				}
				b.WriteRune(ch)
				continue
			}
			if isSmartSingleQuoteRune(ch) {
				if !shouldCloseSmartQuotedLiteral(runes, i, literalQuoteKindSmartSingle) {
					b.WriteRune(ch)
					continue
				}
				b.WriteByte('\'')
				inSingle = false
				singleKind = literalQuoteKindNone
				continue
			}
			b.WriteRune(ch)
			continue
		case inDouble:
			if ch == '\\' && i+1 < len(runes) {
				b.WriteRune(ch)
				i++
				b.WriteRune(runes[i])
				continue
			}
			if doubleKind == literalQuoteKindASCII {
				if ch == '"' {
					if i+1 < len(runes) && runes[i+1] == '"' {
						b.WriteString(`""`)
						i++
						continue
					}
					b.WriteByte('"')
					inDouble = false
					doubleKind = literalQuoteKindNone
					continue
				}
				b.WriteRune(ch)
				continue
			}
			if isSmartDoubleQuoteRune(ch) {
				if !shouldCloseSmartQuotedLiteral(runes, i, literalQuoteKindSmartDouble) {
					b.WriteRune(ch)
					continue
				}
				b.WriteByte('"')
				inDouble = false
				doubleKind = literalQuoteKindNone
				continue
			}
			b.WriteRune(ch)
			continue
		}

		switch {
		case ch == '`':
			inBacktick = true
			b.WriteRune(ch)
		case ch == '\'':
			inSingle = true
			singleKind = literalQuoteKindASCII
			b.WriteByte('\'')
		case isSmartSingleQuoteRune(ch):
			inSingle = true
			singleKind = literalQuoteKindSmart
			b.WriteByte('\'')
		case ch == '"':
			inDouble = true
			doubleKind = literalQuoteKindASCII
			b.WriteByte('"')
		case isSmartDoubleQuoteRune(ch):
			inDouble = true
			doubleKind = literalQuoteKindSmart
			b.WriteByte('"')
		default:
			b.WriteRune(ch)
		}
	}

	return b.String()
}

type literalQuoteKind uint8

const (
	literalQuoteKindNone literalQuoteKind = iota
	literalQuoteKindASCII
	literalQuoteKindSmart
	literalQuoteKindSmartSingle
	literalQuoteKindSmartDouble
)

func shouldCloseSmartQuotedLiteral(runes []rune, idx int, kind literalQuoteKind) bool {
	if !hasLaterSmartQuoteRune(runes, idx, kind) {
		return true
	}
	nextIdx := nextNonSpaceRuneIndex(runes, idx+1)
	if nextIdx == -1 {
		return true
	}
	next := runes[nextIdx]
	if isSmartLiteralBoundaryRune(next) {
		return true
	}
	token := readWordToken(runes, nextIdx)
	if token != "" && isLikelyStatementKeyword(token) {
		return true
	}
	return false
}

func hasLaterSmartQuoteRune(runes []rune, idx int, kind literalQuoteKind) bool {
	for i := idx + 1; i < len(runes); i++ {
		switch kind {
		case literalQuoteKindSmartSingle:
			if isSmartSingleQuoteRune(runes[i]) {
				return true
			}
		case literalQuoteKindSmartDouble:
			if isSmartDoubleQuoteRune(runes[i]) {
				return true
			}
		}
	}
	return false
}

func nextNonSpaceRuneIndex(runes []rune, start int) int {
	for i := start; i < len(runes); i++ {
		if !unicode.IsSpace(runes[i]) {
			return i
		}
	}
	return -1
}

func readWordToken(runes []rune, start int) string {
	if start < 0 || start >= len(runes) {
		return ""
	}
	if !isWordLikeRune(runes[start]) {
		return ""
	}
	var b strings.Builder
	for i := start; i < len(runes); i++ {
		if !isWordLikeRune(runes[i]) {
			break
		}
		b.WriteRune(unicode.ToUpper(runes[i]))
	}
	return b.String()
}

func isLikelyStatementKeyword(token string) bool {
	switch token {
	case "AND", "OR", "NOT", "IN", "IS", "LIKE", "ILIKE", "BETWEEN", "EXISTS",
		"FROM", "WHERE", "GROUP", "ORDER", "LIMIT", "OFFSET", "HAVING", "UNION",
		"INTERSECT", "EXCEPT", "JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "CROSS",
		"ON", "AS", "WHEN", "THEN", "ELSE", "END", "CASE", "SET", "VALUES",
		"RETURNING", "INTO", "OVER", "PARTITION", "BY", "ASC", "DESC", "NULL",
		"TRUE", "FALSE":
		return true
	default:
		return false
	}
}

func isWordLikeRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isSmartLiteralBoundaryRune(r rune) bool {
	switch r {
	case ',', ';', ':', '.', ')', ']', '}', '+', '-', '*', '/', '%', '=', '<', '>', '!', '?', '|', '&':
		return true
	default:
		return false
	}
}

func isSmartSingleQuoteRune(r rune) bool {
	switch r {
	case '‘', '’', '‚', '‛', '＇':
		return true
	default:
		return false
	}
}

func isSmartDoubleQuoteRune(r rune) bool {
	switch r {
	case '“', '”', '„', '‟', '＂':
		return true
	default:
		return false
	}
}

func stripAnyCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	trimmed = strings.TrimSpace(trimmed[3:])
	if trimmed == "" {
		return ""
	}
	end := strings.LastIndex(trimmed, "```")
	if end == -1 {
		inner := strings.TrimSpace(trimmed)
		if inner == "" {
			return ""
		}
		if idx := strings.Index(inner, "\n"); idx != -1 {
			header := strings.TrimSpace(inner[:idx])
			if isFenceInfoToken(header) {
				inner = inner[idx+1:]
			}
		}
		inner = strings.TrimRight(inner, "`")
		return strings.TrimSpace(inner)
	}
	inner := strings.TrimSpace(trimmed[:end])
	if inner == "" {
		return ""
	}
	if idx := strings.Index(inner, "\n"); idx != -1 {
		header := strings.TrimSpace(inner[:idx])
		if isFenceInfoToken(header) {
			inner = inner[idx+1:]
		}
	}
	return strings.TrimSpace(inner)
}

func isFenceInfoToken(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		isAlpha := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
		isDigit := ch >= '0' && ch <= '9'
		isAllowed := ch == '_' || ch == '-' || ch == '+'
		if !(isAlpha || isDigit || isAllowed) {
			return false
		}
	}
	return true
}

func formatExecuteResultMarkdown(locale uiLocale, ds DatasourceSummary, req executeStatementArgs, result QueryResult) string {
	var b strings.Builder
	title := "Execution result"
	datasourceLabel := "Datasource"
	databaseLabel := "Database"
	elapsedLabel := "Elapsed"
	rowsAffectedLabel := "Rows affected"
	rowsReturnedLabel := "Rows returned"
	columnsLabel := "Columns"
	hasMoreLabel := "Has more"
	nextTokenLabel := "Next page token"
	resultsNote := "_Results are shown in the Console results panel._"
	if locale == uiLocaleZH {
		title = "执行结果"
		datasourceLabel = "数据源"
		databaseLabel = "数据库"
		elapsedLabel = "耗时"
		rowsAffectedLabel = "影响行数"
		rowsReturnedLabel = "返回行数"
		columnsLabel = "列数"
		hasMoreLabel = "还有更多"
		nextTokenLabel = "下一页 token"
		resultsNote = "_结果已展示在 Console 结果面板中。_"
	}
	b.WriteString("### " + title + "\n\n")

	name := strings.TrimSpace(ds.Name)
	if name == "" {
		name = req.DatasourceID
	}
	codeLang := "text"
	switch strings.ToLower(strings.TrimSpace(ds.Type)) {
	case "mysql", "postgresql":
		codeLang = "sql"
	case "mongodb":
		codeLang = "javascript"
	case "redis":
		codeLang = "redis"
	}
	b.WriteString(fmt.Sprintf("- %s: `%s`", datasourceLabel, name))
	if strings.TrimSpace(ds.ID) != "" && ds.ID != name {
		b.WriteString(fmt.Sprintf(" (`%s`)", ds.ID))
	}
	b.WriteString("\n")
	if db := strings.TrimSpace(req.Database); db != "" {
		b.WriteString(fmt.Sprintf("- %s: `%s`\n", databaseLabel, db))
	}
	if result.ElapsedMs > 0 {
		b.WriteString(fmt.Sprintf("- %s: %dms\n", elapsedLabel, result.ElapsedMs))
	}

	if len(result.Columns) == 0 {
		stmt := strings.TrimSpace(req.Statement)
		stmtLang := codeLang
		if strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
			trimmed := strings.TrimSpace(stmt)
			if formatted := formatMongoStatementForHuman(trimmed); formatted != trimmed && !strings.HasPrefix(formatted, "{") {
				stmt = formatted
				// Keep stmtLang as javascript
			} else if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				var parsed any
				if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
					if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
						stmt = string(pretty)
						stmtLang = "json"
					}
				}
			}
		}
		b.WriteString(fmt.Sprintf("- %s: %d\n\n", rowsAffectedLabel, result.RowCount))
		b.WriteString("```" + stmtLang + "\n")
		b.WriteString(stmt)
		b.WriteString("\n```")
		b.WriteString("\n\n" + resultsNote)
		return strings.TrimSpace(b.String())
	}

	b.WriteString(fmt.Sprintf("- %s: %d\n", rowsReturnedLabel, result.RowCount))
	b.WriteString(fmt.Sprintf("- %s: %d\n", columnsLabel, len(result.Columns)))
	if result.HasMore {
		b.WriteString(fmt.Sprintf("- %s: true\n", hasMoreLabel))
	} else {
		b.WriteString(fmt.Sprintf("- %s: false\n", hasMoreLabel))
	}
	if strings.TrimSpace(result.NextToken) != "" {
		if locale == uiLocaleZH {
			b.WriteString(fmt.Sprintf("- %s: 可用\n", nextTokenLabel))
		} else {
			b.WriteString(fmt.Sprintf("- %s: available\n", nextTokenLabel))
		}
	}
	b.WriteString("\n\n" + resultsNote)
	return strings.TrimSpace(b.String())
}

type toolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
}

func (c toolCall) args() map[string]any {
	if c.Arguments != nil {
		return c.Arguments
	}
	return c.Args
}

type modelOutput struct {
	AssistantMessage string         `json:"assistantMessage"`
	ToolCalls        []toolCall     `json:"toolCalls,omitempty"`
	Agent            *AgentDecision `json:"agent,omitempty"`
	Plan             *AgentPlan     `json:"plan,omitempty"`
	Intent           *TurnIntent    `json:"intent,omitempty"`
}

type snakeCaseModelOutput struct {
	AssistantMessage string         `json:"assistant_message"`
	ToolCalls        []toolCall     `json:"tool_calls,omitempty"`
	Agent            *AgentDecision `json:"agent,omitempty"`
	Plan             *AgentPlan     `json:"plan,omitempty"`
	Intent           *TurnIntent    `json:"intent,omitempty"`
}

func parseModelOutput(raw string) (modelOutput, bool) {
	parsed, ok, _ := parseModelOutputDetailed(raw)
	return parsed, ok
}

func parseModelOutputDetailed(raw string) (modelOutput, bool, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = stripCodeFence(trimmed)
	start := strings.Index(trimmed, "{")
	if start == -1 {
		return modelOutput{}, false, errors.New("no_json_object")
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed[start:]))
	var payload json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		return modelOutput{}, false, err
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return modelOutput{}, false, err
	}

	if _, ok := obj["assistantMessage"]; ok {
		var parsed modelOutput
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return modelOutput{}, false, err
		}
		if len(parsed.ToolCalls) == 0 {
			if rawCalls, ok := obj["tool_calls"]; ok && len(rawCalls) > 0 {
				var calls []toolCall
				if err := json.Unmarshal(rawCalls, &calls); err == nil {
					parsed.ToolCalls = calls
				}
			}
		}
		parsed.Agent = pickModelOutputAgent(obj, parsed.Agent)
		parsed.Plan = pickModelOutputPlan(obj, parsed.Plan)
		parsed.Intent = pickModelOutputIntent(obj, parsed.Intent)
		return parsed, true, nil
	}
	if _, ok := obj["assistant_message"]; ok {
		var snakeCase snakeCaseModelOutput
		if err := json.Unmarshal(payload, &snakeCase); err != nil {
			return modelOutput{}, false, err
		}
		out := modelOutput{
			AssistantMessage: snakeCase.AssistantMessage,
			ToolCalls:        snakeCase.ToolCalls,
			Agent:            snakeCase.Agent,
			Plan:             snakeCase.Plan,
			Intent:           snakeCase.Intent,
		}
		if len(out.ToolCalls) == 0 {
			if rawCalls, ok := obj["toolCalls"]; ok && len(rawCalls) > 0 {
				var calls []toolCall
				if err := json.Unmarshal(rawCalls, &calls); err == nil {
					out.ToolCalls = calls
				}
			}
		}
		out.Agent = pickModelOutputAgent(obj, out.Agent)
		out.Plan = pickModelOutputPlan(obj, out.Plan)
		out.Intent = pickModelOutputIntent(obj, out.Intent)
		return out, true, nil
	}
	return modelOutput{}, false, errors.New("missing_tool_protocol_keys")
}

func pickModelOutputAgent(obj map[string]json.RawMessage, fallback *AgentDecision) *AgentDecision {
	if fallback != nil {
		return cloneAgentDecision(fallback)
	}
	for _, key := range []string{"agent", "agentDecision", "agent_decision", "agentSelection", "agent_selection"} {
		raw := obj[key]
		if len(raw) == 0 {
			continue
		}
		var value AgentDecision
		if err := json.Unmarshal(raw, &value); err == nil {
			return cloneAgentDecision(&value)
		}
	}
	return nil
}

func pickModelOutputPlan(obj map[string]json.RawMessage, fallback *AgentPlan) *AgentPlan {
	if fallback != nil {
		return cloneAgentPlan(fallback)
	}
	for _, key := range []string{"plan", "executionPlan", "execution_plan"} {
		raw := obj[key]
		if len(raw) == 0 {
			continue
		}
		var value AgentPlan
		if err := json.Unmarshal(raw, &value); err == nil {
			return cloneAgentPlan(&value)
		}
	}
	return nil
}

func pickModelOutputIntent(obj map[string]json.RawMessage, fallback *TurnIntent) *TurnIntent {
	if fallback != nil {
		return cloneTurnIntent(fallback)
	}
	for _, key := range []string{"intent", "turnIntent", "turn_intent", "focusIntent", "focus_intent"} {
		raw := obj[key]
		if len(raw) == 0 {
			continue
		}
		var value TurnIntent
		if err := json.Unmarshal(raw, &value); err == nil {
			return cloneTurnIntent(&value)
		}
	}
	return nil
}

func cloneAgentDecision(value *AgentDecision) *AgentDecision {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneAgentPlan(value *AgentPlan) *AgentPlan {
	if value == nil {
		return nil
	}
	out := *value
	if len(value.Steps) > 0 {
		out.Steps = append([]AgentPlanStep(nil), value.Steps...)
	}
	return &out
}

func summarizeToolCallsForLog(calls []toolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	const max = 6
	out := make([]string, 0, min(len(calls), max))
	for i := 0; i < len(calls) && i < max; i++ {
		name := strings.ToLower(strings.TrimSpace(calls[i].Name))
		if name == "" {
			name = "(unknown)"
		}
		out = append(out, name)
	}
	if len(calls) > max {
		out = append(out, "…")
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func previewForLog(value string) string {
	const max = 240
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

func rawForLog(value string) string {
	const max = 4000
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func summarizeMessagesForLog(msgs []Message) map[string]any {
	if len(msgs) == 0 {
		return map[string]any{"count": 0}
	}
	totalChars := 0
	maxLen := 0
	for _, m := range msgs {
		l := len(m.Content)
		totalChars += l
		if l > maxLen {
			maxLen = l
		}
	}
	last := msgs[len(msgs)-1]
	out := map[string]any{
		"count":      len(msgs),
		"totalChars": totalChars,
		"maxLen":     maxLen,
		"lastRole":   strings.TrimSpace(last.Role),
		"lastLen":    len(last.Content),
	}

	trimmed := strings.TrimSpace(last.Content)
	if strings.HasPrefix(trimmed, "[tool_result]") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "[tool_result]"))
		toolName := rest
		if idx := strings.Index(rest, "\n"); idx != -1 {
			toolName = rest[:idx]
		}
		toolName = strings.TrimSpace(toolName)
		if toolName != "" {
			out["lastToolResult"] = toolName
		} else {
			out["lastToolResult"] = true
		}
	}

	return out
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
		if end := strings.LastIndex(trimmed, "```"); end != -1 {
			trimmed = trimmed[:end]
		}
	}
	return strings.TrimSpace(trimmed)
}

func stringArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok || raw == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" || s == "<nil>" {
			continue
		}
		return s
	}
	return ""
}

func statementArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s != "" && s != "<nil>" {
				return s
			}
		case []byte:
			s := strings.TrimSpace(string(v))
			if s != "" && s != "<nil>" {
				return s
			}
		case json.RawMessage:
			s := strings.TrimSpace(string(v))
			if s != "" && s != "<nil>" {
				return s
			}
		default:
			switch v.(type) {
			case map[string]any, []any:
				if data, err := json.Marshal(v); err == nil {
					s := strings.TrimSpace(string(data))
					if s != "" && s != "<nil>" {
						return s
					}
				}
			}
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func toolResultMessage(name string, payload any) Message {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"error":"failed to encode tool result"}`)
	}
	return Message{
		Role:    "assistant",
		Content: fmt.Sprintf("[tool_result] %s\n%s", name, string(body)),
	}
}

func (s *Service) buildSystemPrompt(ctx context.Context, req TurnRequest, preloadedWorkingSet *workingSetBuildResult) string {
	trimmedBase := strings.TrimSpace(s.baseSystemPrompt)
	if trimmedBase == "" {
		trimmedBase = defaultBaseSystemPrompt
	}
	datasourcePromptSection, knowledgeSection := s.buildDatasourcePromptSections(ctx, req)
	workingSet := workingSetBuildResult{}
	if preloadedWorkingSet != nil {
		workingSet = *preloadedWorkingSet
	} else {
		workingSet = s.loadThreadWorkingSet(ctx, req)
	}
	toolsSection := buildToolsSection(rankToolCards(buildToolCardContext(req, toolNamesFromWorkingSet(workingSet))))
	contextSection := s.buildContextSection(ctx, req, workingSet)

	prompt := trimmedBase
	prompt = replaceOrInsertPromptSection(prompt, "{{DATASOURCE_PROMPT}}", datasourcePromptSection)
	prompt = replaceOrInsertPromptSection(prompt, "{{KNOWLEDGE}}", knowledgeSection)
	if strings.Contains(prompt, "{{TOOLS}}") {
		prompt = strings.ReplaceAll(prompt, "{{TOOLS}}", toolsSection)
	} else if toolsSection != "" {
		prompt = prompt + "\n\n" + toolsSection
	}
	if strings.Contains(prompt, "{{CONTEXT}}") {
		prompt = strings.ReplaceAll(prompt, "{{CONTEXT}}", contextSection)
	} else if contextSection != "" {
		prompt = prompt + "\n\n" + contextSection
	}

	return strings.TrimSpace(prompt)
}

func replaceOrInsertPromptSection(prompt string, placeholder string, section string) string {
	trimmedSection := strings.TrimSpace(section)
	if strings.Contains(prompt, placeholder) {
		return strings.ReplaceAll(prompt, placeholder, trimmedSection)
	}
	if trimmedSection == "" {
		return prompt
	}

	insertAt := strings.Index(prompt, "{{TOOLS}}")
	if insertAt == -1 {
		insertAt = strings.Index(prompt, "{{CONTEXT}}")
	}
	if insertAt == -1 {
		return strings.TrimSpace(prompt) + "\n\n" + trimmedSection
	}
	head := strings.TrimRight(prompt[:insertAt], "\n")
	tail := prompt[insertAt:]
	return head + "\n\n" + trimmedSection + "\n\n" + tail
}

func (s *Service) buildDatasourcePromptSections(ctx context.Context, req TurnRequest) (string, string) {
	if s == nil || s.promptModules.empty() {
		return "", ""
	}

	datasourceID := strings.TrimSpace(req.PageContext.CurrentDatasourceID)
	datasourceType := strings.TrimSpace(req.PageContext.CurrentDatasourceType)
	if req.WorkingContext != nil {
		datasourceID = strings.TrimSpace(firstNonEmpty(req.WorkingContext.DatasourceID, datasourceID))
		datasourceType = strings.TrimSpace(firstNonEmpty(req.WorkingContext.DatasourceType, datasourceType))
	}
	if datasourceType == "" && datasourceID != "" {
		if ds, err := s.getDatasourceCached(ctx, datasourceID); err == nil {
			datasourceType = strings.TrimSpace(ds.Type)
		}
	}
	datasourceType = normalizeDatasourceType(datasourceType)

	promptParts := make([]string, 0, 2)

	if datasourceID != "" {
		if p := strings.TrimSpace(s.promptModules.DatasourcePrompts[datasourceID]); p != "" {
			promptParts = append(promptParts, p)
		}
	}

	if datasourceType != "" {
		if p := strings.TrimSpace(s.promptModules.TypePrompts[datasourceType]); p != "" {
			promptParts = append(promptParts, p)
		}
	} else {
		// When no active datasource is available, keep modules minimal and only
		// enable hints when the user explicitly mentions a datasource.
		inferred := inferDatasourceTypesFromText(lastUserText(req.Messages))
		for _, t := range inferred {
			if p := strings.TrimSpace(s.promptModules.TypePrompts[t]); p != "" {
				promptParts = append(promptParts, p)
			}
		}
	}

	return strings.TrimSpace(strings.Join(promptParts, "\n\n")), ""
}

func normalizeDatasourceType(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return ""
	}
	switch lower {
	case "redis-cluster", "redis cluster":
		return "redis_cluster"
	default:
		return lower
	}
}

var mongoShellDbCallRegexp = regexp.MustCompile(`\bdb\.(?:[a-z0-9_]+\.(?:find|findone|findoneandupdate|findoneandreplace|findoneanddelete|aggregate|insertone|insertmany|updateone|updatemany|deleteone|deletemany|replaceone|bulkwrite|createindex|createindexes|drop|count|countdocuments|estimateddocumentcount|distinct)\s*\(|getcollection\s*\(|getsiblingdb\s*\()`)

func inferDatasourceTypesFromText(text string) []string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return nil
	}

	add := func(set map[string]struct{}, key string) {
		if strings.TrimSpace(key) != "" {
			set[key] = struct{}{}
		}
	}

	set := map[string]struct{}{}
	if strings.Contains(lower, "mongodb") || strings.Contains(lower, "mongo") || mongoShellDbCallRegexp.MatchString(lower) {
		add(set, "mongodb")
	}
	if strings.Contains(lower, "redis") {
		add(set, "redis")
	}
	if strings.Contains(lower, "elasticsearch") || strings.Contains(lower, "/_cat/") || strings.Contains(lower, "_search") {
		add(set, "elasticsearch")
	}
	if strings.Contains(lower, "dynamodb") || strings.Contains(lower, "partiql") || strings.Contains(lower, "nexttoken") {
		add(set, "dynamodb")
	}
	if strings.Contains(lower, "postgresql") || strings.Contains(lower, "postgres") {
		add(set, "postgresql")
	}
	if strings.Contains(lower, "mysql") {
		add(set, "mysql")
	}

	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (s *Service) buildContextSection(ctx context.Context, req TurnRequest, workingSet workingSetBuildResult) string {
	base := buildBaseContextSection(req)
	thread := buildThreadWorkingSetSection(workingSet)
	snapshot := s.buildPageSnapshotSection(ctx, req)
	analysis := s.buildAnalysisContextSection(req)

	var parts []string
	if base != "" {
		parts = append(parts, base)
	}
	if thread != "" {
		parts = append(parts, thread)
	}
	if analysis != "" {
		parts = append(parts, analysis)
	}
	if snapshot != "" {
		parts = append(parts, snapshot)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (s *Service) buildAnalysisContextSection(req TurnRequest) string {
	if s == nil || s.analysis == nil {
		return ""
	}
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return ""
	}

	result, ok := s.analysis.GetResult(conversationID)
	summary, summaryOK := s.analysis.GetSummary(conversationID)
	if !ok && !summaryOK {
		return ""
	}

	var b strings.Builder
	b.WriteString("Recent AI console result (rows are NOT included in prompts):\n")
	if ok {
		b.WriteString(fmt.Sprintf("- datasourceId: %s\n", result.DatasourceID))
		if strings.TrimSpace(result.Database) != "" {
			b.WriteString(fmt.Sprintf("- database: %s\n", result.Database))
		}
		b.WriteString(fmt.Sprintf("- rowCount: %d\n", result.RowCount))
		b.WriteString(fmt.Sprintf("- payloadRows: %d\n", len(result.Rows)))
		if result.ApproxBytes > 0 {
			b.WriteString(fmt.Sprintf("- approxBytes: %d\n", result.ApproxBytes))
		}
		if result.RowsTruncated {
			b.WriteString("- truncated: true\n")
		} else {
			b.WriteString("- truncated: false\n")
		}
		if !result.CapturedAt.IsZero() {
			b.WriteString(fmt.Sprintf("- capturedAt: %s\n", result.CapturedAt.Format(time.RFC3339)))
		}
	} else {
		b.WriteString("- available: false\n")
	}
	b.WriteString("If the user wants you to analyze the result rows, call analyze_result (requires approval).\n")

	if summaryOK && strings.TrimSpace(summary) != "" {
		b.WriteString("\nLatest approved result analysis summary:\n")
		b.WriteString(summary)
	}

	return strings.TrimSpace(b.String())
}

func buildBaseContextSection(req TurnRequest) string {
	var b strings.Builder

	routeName := strings.TrimSpace(req.PageContext.RouteName)
	routePath := strings.TrimSpace(req.PageContext.RoutePath)
	if routeName != "" || routePath != "" {
		b.WriteString("Page context:\n")
		if routeName != "" {
			b.WriteString("routeName: " + routeName + "\n")
		}
		if routePath != "" {
			b.WriteString("routePath: " + routePath + "\n")
		}
	}
	if ds := strings.TrimSpace(req.PageContext.CurrentDatasourceID); ds != "" {
		b.WriteString("currentDatasourceId: " + ds + "\n")
	}
	if typ := strings.TrimSpace(req.PageContext.CurrentDatasourceType); typ != "" {
		b.WriteString("currentDatasourceType: " + typ + "\n")
	}
	if db := strings.TrimSpace(req.PageContext.CurrentDatabase); db != "" {
		b.WriteString("currentDatabase: " + db + "\n")
	}
	if ent := strings.TrimSpace(req.PageContext.CurrentEntity); ent != "" {
		b.WriteString("currentEntity: " + ent + "\n")
	}
	if stmt := strings.TrimSpace(req.PageContext.CurrentStatement); stmt != "" {
		b.WriteString("currentStatement: " + stmt + "\n")
	}
	if stmt := strings.TrimSpace(req.ImplicitStatement); stmt != "" {
		b.WriteString("implicitStatement: " + stmt + "\n")
	}
	if errText := strings.TrimSpace(req.PageContext.LastConsoleError); errText != "" {
		b.WriteString("lastConsoleError: " + errText + "\n")
	}
	if req.WorkingContext != nil {
		if ds := strings.TrimSpace(req.WorkingContext.DatasourceID); ds != "" {
			b.WriteString("workingDatasourceId: " + ds + "\n")
		}
		if typ := strings.TrimSpace(req.WorkingContext.DatasourceType); typ != "" {
			b.WriteString("workingDatasourceType: " + typ + "\n")
		}
		if db := strings.TrimSpace(req.WorkingContext.Database); db != "" {
			b.WriteString("workingDatabase: " + db + "\n")
		}
		if ent := strings.TrimSpace(req.WorkingContext.Entity); ent != "" {
			b.WriteString("workingEntity: " + ent + "\n")
		}
		if src := strings.TrimSpace(req.WorkingContext.Source); src != "" {
			b.WriteString("workingContextSource: " + src + "\n")
		}
	}
	if len(req.ContextChips) > 0 {
		b.WriteString("contextChips:\n")
		for _, chip := range req.ContextChips {
			if strings.TrimSpace(chip.Label) == "" {
				continue
			}
			b.WriteString("- " + chip.Label + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) buildPageSnapshotSection(ctx context.Context, req TurnRequest) string {
	routeName := strings.TrimSpace(req.PageContext.RouteName)
	routePath := strings.TrimSpace(req.PageContext.RoutePath)

	// Prefer routeName when present; fall back to path prefix checks.
	if routeName == "datasources" || routePath == "/" {
		return s.buildDatasourcesSnapshot(ctx, req)
	}
	if routeName == "console" || strings.HasPrefix(routePath, "/console/") {
		return s.buildConsoleSnapshot(ctx, req)
	}
	return ""
}

type datasourceSnapshot struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Database     string `json:"database,omitempty"`
	Environment  string `json:"environment,omitempty"`
	Dialect      string `json:"dialect,omitempty"`
	Status       string `json:"status,omitempty"`
	CheckedAt    int64  `json:"checkedAt,omitempty"`
	StatusDetail string `json:"statusDetail,omitempty"`
}

func (s *Service) buildDatasourcesSnapshot(ctx context.Context, req TurnRequest) string {
	startAt := time.Now()
	list, err := s.tools.ListDatasources(ctx)
	s.log(ctx, "tool_list_datasources", map[string]any{
		"durationMs": time.Since(startAt).Milliseconds(),
		"error":      errString(err),
		"count":      len(list),
	})
	if err != nil {
		return fmt.Sprintf("Page snapshot (datasources): error: %s", err.Error())
	}

	statusByID := make(map[string]DatasourceStatus, len(req.PageContext.DatasourceStatuses))
	for _, st := range req.PageContext.DatasourceStatuses {
		if id := strings.TrimSpace(st.ID); id != "" {
			statusByID[id] = st
		}
	}

	const maxItems = 60
	out := make([]datasourceSnapshot, 0, min(len(list), maxItems))
	for i, ds := range list {
		if i >= maxItems {
			break
		}
		status := statusByID[ds.ID]
		statusText := strings.TrimSpace(status.Status)
		if statusText == "" {
			statusText = "unknown"
		}
		out = append(out, datasourceSnapshot{
			ID:           ds.ID,
			Name:         ds.Name,
			Type:         ds.Type,
			Host:         ds.Host,
			Port:         ds.Port,
			Database:     ds.Database,
			Environment:  ds.Environment,
			Dialect:      ds.Dialect,
			Status:       statusText,
			CheckedAt:    status.CheckedAt,
			StatusDetail: strings.TrimSpace(status.Detail),
		})
	}

	payload := map[string]any{
		"datasources": out,
		"total":       len(list),
		"truncated":   len(list) > len(out),
	}
	raw, _ := json.Marshal(payload)

	var b strings.Builder
	b.WriteString("Page snapshot (datasources):\n")
	b.WriteString("```json\n")
	b.Write(raw)
	b.WriteString("\n```")
	return b.String()
}

func (s *Service) buildConsoleSnapshot(ctx context.Context, req TurnRequest) string {
	datasourceID := strings.TrimSpace(req.PageContext.CurrentDatasourceID)
	if datasourceID == "" {
		return ""
	}

	snapshot := map[string]any{
		"currentDatasourceId": datasourceID,
	}

	ds, err := s.getDatasourceCached(ctx, datasourceID)
	if err != nil {
		snapshot["currentDatasourceError"] = err.Error()
	} else {
		snapshot["currentDatasource"] = datasourceSnapshot{
			ID:       ds.ID,
			Name:     ds.Name,
			Type:     ds.Type,
			Host:     ds.Host,
			Port:     ds.Port,
			Database: ds.Database,
		}
	}

	database := strings.TrimSpace(req.PageContext.CurrentDatabase)
	if database != "" {
		snapshot["currentDatabase"] = database
	}
	currentEntity := strings.TrimSpace(req.PageContext.CurrentEntity)
	if currentEntity != "" {
		snapshot["currentEntity"] = currentEntity
	}

	raw, _ := json.Marshal(snapshot)
	var b strings.Builder
	b.WriteString("Page snapshot (console):\n")
	b.WriteString("```json\n")
	b.Write(raw)
	b.WriteString("\n```")
	return b.String()
}

type describeEntityCacheKey struct{}

type describeEntityCache struct {
	mu    sync.Mutex
	items map[string]describeEntityCacheItem
}

type describeEntityCacheItem struct {
	value any
	err   error
}

func withDescribeEntityCache(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	if ctx.Value(describeEntityCacheKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, describeEntityCacheKey{}, &describeEntityCache{items: make(map[string]describeEntityCacheItem)})
}

func (s *Service) describeEntityCached(ctx context.Context, datasourceID, name, database string) (any, error) {
	cache, _ := ctx.Value(describeEntityCacheKey{}).(*describeEntityCache)
	if cache == nil {
		return s.tools.DescribeEntity(ctx, datasourceID, name, database)
	}

	key := datasourceID + "\n" + database + "\n" + name
	cache.mu.Lock()
	item, ok := cache.items[key]
	cache.mu.Unlock()
	if ok {
		return item.value, item.err
	}

	value, err := s.tools.DescribeEntity(ctx, datasourceID, name, database)
	cache.mu.Lock()
	cache.items[key] = describeEntityCacheItem{value: value, err: err}
	cache.mu.Unlock()
	return value, err
}

type pendingToolCall struct {
	ThreadID  string
	Name      string
	Arguments map[string]any
}

type approvalStore struct {
	mu    sync.Mutex
	items map[string]map[string]pendingToolCall
}

func newApprovalStore() *approvalStore {
	return &approvalStore{items: make(map[string]map[string]pendingToolCall)}
}

func (s *approvalStore) put(conversationID, approvalID string, call pendingToolCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		byConversation = make(map[string]pendingToolCall)
		s.items[conversationID] = byConversation
	}
	byConversation[approvalID] = call
}

func (s *approvalStore) get(conversationID, approvalID string) (pendingToolCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		return pendingToolCall{}, false
	}
	call, ok := byConversation[approvalID]
	if !ok {
		return pendingToolCall{}, false
	}
	return call, true
}

func (s *approvalStore) take(conversationID, approvalID string) (pendingToolCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		return pendingToolCall{}, false
	}
	call, ok := byConversation[approvalID]
	if !ok {
		return pendingToolCall{}, false
	}
	delete(byConversation, approvalID)
	if len(byConversation) == 0 {
		delete(s.items, conversationID)
	}
	return call, true
}

func newApprovalID() string {
	return fmt.Sprintf("appr_%x", time.Now().UTC().UnixNano())
}

func summarizeCreateDatasource(input DatasourceCreateInput) string {
	var b strings.Builder
	b.WriteString(`Create datasource "`)
	b.WriteString(input.Name)
	b.WriteString(`" (` + input.Type + `)`)
	host := strings.TrimSpace(input.Host)
	if host != "" && input.Port > 0 {
		b.WriteString(" at ")
		b.WriteString(host)
		b.WriteString(fmt.Sprintf(":%d", input.Port))
	}
	db := strings.TrimSpace(input.Database)
	if db != "" {
		b.WriteString(` database "`)
		b.WriteString(db)
		b.WriteString(`"`)
	}
	return b.String()
}

func sanitizeCreateDatasourceInput(input DatasourceCreateInput) DatasourceCreateInput {
	input.Password = ""
	return input
}

type datasourceDeleteTarget struct {
	DatasourceID string `json:"datasourceId,omitempty"`
	Name         string `json:"name,omitempty"`
}

func datasourceDeleteTargetFromArgs(args map[string]any) (datasourceDeleteTarget, error) {
	rawID := stringArg(args, "datasourceId", "id")
	rawName := stringArg(args, "name", "datasourceName")
	if rawID == "" && rawName == "" {
		return datasourceDeleteTarget{}, errors.New("delete_datasource requires datasourceId or name")
	}
	return datasourceDeleteTarget{DatasourceID: rawID, Name: rawName}, nil
}

func summarizeDeleteDatasource(target datasourceDeleteTarget) string {
	if target.DatasourceID != "" && target.Name != "" {
		return fmt.Sprintf(`Delete datasource "%s" (%s)`, target.Name, target.DatasourceID)
	}
	if target.Name != "" {
		return fmt.Sprintf(`Delete datasource "%s"`, target.Name)
	}
	return fmt.Sprintf(`Delete datasource %s`, target.DatasourceID)
}

type navigationTarget struct {
	DatasourceID string `json:"datasourceId,omitempty"`
	Name         string `json:"name,omitempty"`
	Target       string `json:"target,omitempty"`
}

func navigationTargetFromArgs(args map[string]any) (navigationTarget, error) {
	rawID := stringArg(args, "datasourceId", "id")
	rawName := stringArg(args, "name", "datasourceName")
	rawTarget := strings.ToLower(strings.TrimSpace(stringArg(args, "target")))

	target := "console"
	if rawTarget != "" {
		if !isNavigationTarget(rawTarget) {
			return navigationTarget{}, errors.New("navigate_to_datasource target must be one of: console|edit|list")
		}
		target = rawTarget
	}

	// Only `list` navigation is allowed without a datasource.
	if rawID == "" && rawName == "" && target != "list" {
		return navigationTarget{}, errors.New("navigate_to_datasource requires datasourceId or name (unless target is list)")
	}

	return navigationTarget{DatasourceID: rawID, Name: rawName, Target: target}, nil
}

func isNavigationTarget(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "console", "edit", "list":
		return true
	default:
		return false
	}
}

func resolveNavigationPath(ctx context.Context, tools Tools, target navigationTarget) (string, error) {
	if strings.EqualFold(strings.TrimSpace(target.Target), "list") {
		return "/", nil
	}

	id := strings.TrimSpace(target.DatasourceID)
	if id == "" {
		list, err := tools.ListDatasources(ctx)
		if err != nil {
			return "", err
		}
		name := strings.TrimSpace(target.Name)
		matches := make([]DatasourceSummary, 0, 2)
		for _, ds := range list {
			if strings.EqualFold(ds.Name, name) {
				matches = []DatasourceSummary{ds}
				break
			}
			if strings.Contains(strings.ToLower(ds.Name), strings.ToLower(name)) {
				matches = append(matches, ds)
			}
		}
		if len(matches) == 0 {
			return "", errors.New("datasource not found")
		}
		if len(matches) > 1 {
			var b strings.Builder
			b.WriteString("Multiple datasources match. Please specify one of: ")
			for i, ds := range matches {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(ds.Name)
			}
			return "", errors.New(b.String())
		}
		id = matches[0].ID
	}

	switch strings.ToLower(strings.TrimSpace(target.Target)) {
	case "", "console":
		return "/console/" + id, nil
	case "edit":
		return "/datasources/" + id + "/edit", nil
	case "list":
		return "/", nil
	default:
		// This should not happen after strict validation, but keep a safe fallback.
		return "/console/" + id, nil
	}
}

func datasourceCreateInputFromArgs(args map[string]any) (DatasourceCreateInput, error) {
	name := stringArg(args, "name")
	typ := stringArg(args, "type")
	host := stringArg(args, "host")

	port := 0
	if raw, ok := args["port"]; ok {
		switch v := raw.(type) {
		case float64:
			port = int(v)
		case int:
			port = v
		case int64:
			port = int(v)
		default:
			var parsed int
			if _, err := fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &parsed); err == nil {
				port = parsed
			}
		}
	}

	username := stringArg(args, "username")
	password := ""
	if raw, ok := args["password"]; ok && raw != nil {
		if s, ok := raw.(string); ok {
			password = s
		} else {
			password = fmt.Sprint(raw)
		}
	}
	database := stringArg(args, "database")
	authSource := stringArg(args, "authSource")
	var options map[string]any
	if raw, ok := args["options"]; ok {
		if m, ok := raw.(map[string]any); ok {
			options = m
		}
	}

	if name == "" {
		return DatasourceCreateInput{}, errors.New("create_datasource requires name")
	}
	if typ == "" {
		return DatasourceCreateInput{}, errors.New("create_datasource requires type")
	}

	return DatasourceCreateInput{
		Name:       name,
		Type:       typ,
		Host:       host,
		Port:       port,
		Username:   username,
		Password:   password,
		Database:   database,
		AuthSource: authSource,
		Options:    options,
	}, nil
}
