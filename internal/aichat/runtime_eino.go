package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/schemaprivacy"

	einoModel "github.com/cloudwego/eino/components/model"
	einoTool "github.com/cloudwego/eino/components/tool"
	einoCompose "github.com/cloudwego/eino/compose"
	einoReact "github.com/cloudwego/eino/flow/agent/react"
	einoSchema "github.com/cloudwego/eino/schema"
)

const (
	einoEnvelopeMarker   = "futrix_turn_response_v1"
	einoMetaAgentKey     = "__futrix_agent"
	einoMetaPlanKey      = "__futrix_plan"
	einoMetaIntentKey    = "__futrix_intent"
	einoMessageAgentKey  = "futrix_agent"
	einoMessagePlanKey   = "futrix_plan"
	einoMessageIntentKey = "futrix_intent"
)

type einoTurnEnvelope struct {
	Marker   string       `json:"marker"`
	Response TurnResponse `json:"response"`
}

func (s *Service) Turn(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	req.ThreadID = resolveThreadID(req.ThreadID, req.ConversationID)
	workingSet := s.loadThreadWorkingSet(ctx, req)
	req = s.attachResolvedWorkingContext(ctx, req, &workingSet)
	recalled := memoryEnvelopeFromWorkingSet(&workingSet.WorkingSet)
	resp, err := s.turnEino(ctx, req, false, nil, &workingSet)
	if err != nil {
		return TurnResponse{}, err
	}
	s.persistThreadWorkingSet(req.ThreadID, workingSet)
	s.persistThreadSession(req.ThreadID, req.ConversationID)
	s.appendThreadUserMessage(req.ThreadID, req)
	s.appendThreadResponseEvents(req.ThreadID, resp)
	resp.Memory = mergeMemoryEnvelopes(recalled, s.buildResponseMemory(req.ThreadID))
	return resp, nil
}

func (s *Service) TurnStream(ctx context.Context, req TurnRequest, onDelta func(delta string)) (TurnResponse, error) {
	req.ThreadID = resolveThreadID(req.ThreadID, req.ConversationID)
	workingSet := s.loadThreadWorkingSet(ctx, req)
	req = s.attachResolvedWorkingContext(ctx, req, &workingSet)
	recalled := memoryEnvelopeFromWorkingSet(&workingSet.WorkingSet)
	resp, err := s.turnEino(ctx, req, true, onDelta, &workingSet)
	if err != nil {
		return TurnResponse{}, err
	}
	s.persistThreadWorkingSet(req.ThreadID, workingSet)
	s.persistThreadSession(req.ThreadID, req.ConversationID)
	s.appendThreadUserMessage(req.ThreadID, req)
	s.appendThreadResponseEvents(req.ThreadID, resp)
	resp.Memory = mergeMemoryEnvelopes(recalled, s.buildResponseMemory(req.ThreadID))
	return resp, nil
}

func (s *Service) turnEino(ctx context.Context, req TurnRequest, stream bool, onDelta func(delta string), preloadedWorkingSet *workingSetBuildResult) (TurnResponse, error) {
	return s.turnEinoWithOptions(ctx, req, stream, onDelta, "", "", nil, false, Effects{}, preloadedWorkingSet)
}

func (s *Service) turnEinoWithOptions(
	ctx context.Context,
	req TurnRequest,
	stream bool,
	onDelta func(delta string),
	checkpointID string,
	resumeInterruptID string,
	resumePayload map[string]any,
	skipDirectApprovals bool,
	initialEffects Effects,
	preloadedWorkingSet *workingSetBuildResult,
) (TurnResponse, error) {
	if s == nil || s.models == nil || s.tools == nil || s.analysis == nil || s.approvals == nil || s.einoCheckpoints == nil || s.einoApprovals == nil {
		return TurnResponse{}, errors.New("ai chat service not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(req.Messages) == 0 {
		return TurnResponse{}, errors.New("messages are required")
	}
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return TurnResponse{}, errors.New("conversationId is required")
	}
	ctx = WithDiagnosticsContext(ctx, conversationID, "")
	ctx = withDescribeEntityCache(ctx)
	ctx = withDatasourceCache(ctx)

	mode := "turn"
	if stream {
		mode = "turn_stream"
	}
	if strings.TrimSpace(resumeInterruptID) != "" {
		mode = "turn_resume"
	}
	locale := detectUserLocale(req)
	if !skipDirectApprovals {
		if resp, ok := s.tryDirectAnalyzeResultApproval(ctx, req, conversationID, locale, mode); ok {
			if stream && onDelta != nil && strings.TrimSpace(resp.AssistantMessage) != "" {
				onDelta(resp.AssistantMessage)
			}
			return resp, nil
		}
		if resp, ok := s.tryDirectCreateVisualizationApproval(ctx, req, conversationID, locale, mode); ok {
			if stream && onDelta != nil && strings.TrimSpace(resp.AssistantMessage) != "" {
				onDelta(resp.AssistantMessage)
			}
			return resp, nil
		}
	}

	model, err := s.models.Resolve(strings.TrimSpace(req.AIConfigID))
	if err != nil {
		return TurnResponse{}, err
	}

	systemPrompt := s.buildSystemPrompt(ctx, req, preloadedWorkingSet)
	_, canStream := model.(StreamingModel)
	s.log(ctx, "turn_start", map[string]any{
		"mode":              mode,
		"aiConfigId":        strings.TrimSpace(req.AIConfigID),
		"messageCount":      len(req.Messages),
		"contextChipCount":  len(req.ContextChips),
		"systemPromptLen":   len(systemPrompt),
		"routeName":         strings.TrimSpace(req.PageContext.RouteName),
		"routePath":         strings.TrimSpace(req.PageContext.RoutePath),
		"currentDatasource": strings.TrimSpace(req.PageContext.CurrentDatasourceID),
		"canStream":         canStream,
	})

	rt := &einoTurnRuntime{
		service:        s,
		req:            req,
		conversationID: conversationID,
		locale:         locale,
		userText:       lastUserText(req.Messages),
		mode:           mode,
		pendingEffects: cloneEffects(initialEffects),
	}

	runner, err := rt.newRunner(ctx, model, systemPrompt)
	if err != nil {
		return TurnResponse{}, err
	}

	input := rt.toEinoMessages(req.Messages)
	if len(input) == 0 {
		return TurnResponse{}, errors.New("messages are required")
	}
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		checkpointID = newEinoCheckpointID(conversationID)
	}

	runCtx := ctx
	if strings.TrimSpace(resumeInterruptID) != "" {
		runCtx = einoCompose.ResumeWithData(runCtx, strings.TrimSpace(resumeInterruptID), cloneMapAny(resumePayload))
	}

	opts := []einoCompose.Option{einoCompose.WithCheckPointID(checkpointID)}
	if !stream {
		msg, invokeErr := runner.Invoke(runCtx, input, opts...)
		if invokeErr != nil {
			if resp, ok := rt.responseFromInterrupt(runCtx, invokeErr, checkpointID); ok {
				return resp, nil
			}
			return TurnResponse{}, invokeErr
		}
		return rt.toTurnResponse(msg), nil
	}

	sr, streamErr := runner.Stream(runCtx, input, opts...)
	if streamErr != nil {
		if resp, ok := rt.responseFromInterrupt(runCtx, streamErr, checkpointID); ok {
			if onDelta != nil && strings.TrimSpace(resp.AssistantMessage) != "" {
				onDelta(resp.AssistantMessage)
			}
			return resp, nil
		}
		return TurnResponse{}, streamErr
	}
	defer sr.Close()

	chunks := make([]*einoSchema.Message, 0, 8)
	for {
		chunk, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			if resp, ok := rt.responseFromInterrupt(runCtx, recvErr, checkpointID); ok {
				if onDelta != nil && strings.TrimSpace(resp.AssistantMessage) != "" {
					onDelta(resp.AssistantMessage)
				}
				return resp, nil
			}
			return TurnResponse{}, recvErr
		}
		if chunk == nil {
			continue
		}
		chunks = append(chunks, chunk)
		if onDelta != nil && chunk.Role == einoSchema.Assistant && strings.TrimSpace(chunk.Content) != "" {
			onDelta(chunk.Content)
		}
	}

	if len(chunks) == 0 {
		return TurnResponse{}, errors.New("empty stream output")
	}

	finalMsg := chunks[len(chunks)-1]
	if len(chunks) > 1 {
		if merged, mergeErr := einoSchema.ConcatMessages(chunks); mergeErr == nil && merged != nil {
			finalMsg = merged
		}
	}
	return rt.toTurnResponse(finalMsg), nil
}

func (r *einoTurnRuntime) newRunner(
	ctx context.Context,
	model Model,
	systemPrompt string,
) (einoCompose.Runnable[[]*einoSchema.Message, *einoSchema.Message], error) {
	if r == nil || r.service == nil || r.service.einoCheckpoints == nil {
		return nil, errors.New("eino runtime not initialized")
	}
	cfg := &einoReact.AgentConfig{
		ToolCallingModel: &jsonProtocolToolCallingModel{
			service:      r.service,
			model:        model,
			systemPrompt: systemPrompt,
			req:          r.req,
			reqRef:       &r.req,
			locale:       r.locale,
			mode:         r.mode,
		},
		ToolsConfig: einoCompose.ToolsNodeConfig{
			Tools:               r.buildTools(),
			UnknownToolsHandler: r.unknownToolHandler,
		},
		MaxStep:               runtimeMaxSteps(r.mode),
		StreamToolCallChecker: allChunksStreamToolCallChecker,
	}
	agent, err := einoReact.NewAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}
	anyGraph, _ := agent.ExportGraph()
	graph, ok := anyGraph.(*einoCompose.Graph[[]*einoSchema.Message, *einoSchema.Message])
	if !ok || graph == nil {
		return nil, errors.New("failed to export react graph")
	}
	return graph.Compile(
		ctx,
		einoCompose.WithMaxRunSteps(cfg.MaxStep),
		einoCompose.WithNodeTriggerMode(einoCompose.AnyPredecessor),
		einoCompose.WithGraphName(einoReact.GraphName),
		einoCompose.WithCheckPointStore(r.service.einoCheckpoints),
	)
}

func runtimeMaxSteps(mode string) int {
	switch strings.TrimSpace(mode) {
	case "turn_resume":
		return 40
	default:
		return 36
	}
}

func allChunksStreamToolCallChecker(_ context.Context, sr *einoSchema.StreamReader[*einoSchema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if msg == nil {
			continue
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}

type jsonProtocolToolCallingModel struct {
	service      *Service
	model        Model
	systemPrompt string
	req          TurnRequest
	reqRef       *TurnRequest
	locale       uiLocale
	mode         string

	loopMu              sync.Mutex
	webSearchLoopStreak int
}

func (m *jsonProtocolToolCallingModel) currentReq() TurnRequest {
	if m == nil || m.reqRef == nil {
		return m.req
	}
	return *m.reqRef
}

func (m *jsonProtocolToolCallingModel) Generate(ctx context.Context, input []*einoSchema.Message, _ ...einoModel.Option) (*einoSchema.Message, error) {
	if m == nil || m.model == nil {
		return nil, errors.New("model not configured")
	}
	messages := fromEinoMessages(input)
	startAt := time.Now()
	raw, err := m.model.Chat(ctx, m.systemPrompt, messages)
	if err != nil {
		return nil, err
	}
	elapsedMs := time.Since(startAt).Milliseconds()
	m.logModelCall(ctx, elapsedMs, raw, false, 0, 0, 0, 0, -1, -1)

	parsed, ok, didRepair, didForce, finalRaw := m.parseWithRecovery(ctx, messages, raw)
	if !ok {
		return m.unparseableFallbackMessage(finalRaw, didRepair, didForce), nil
	}
	return m.toEinoMessage(parsed), nil
}

func (m *jsonProtocolToolCallingModel) Stream(ctx context.Context, input []*einoSchema.Message, _ ...einoModel.Option) (*einoSchema.StreamReader[*einoSchema.Message], error) {
	if m == nil || m.model == nil {
		return nil, errors.New("model not configured")
	}
	streamingModel, canStream := m.model.(StreamingModel)
	if !canStream {
		msg, err := m.Generate(ctx, input)
		if err != nil {
			return nil, err
		}
		return einoSchema.StreamReaderFromArray([]*einoSchema.Message{msg}), nil
	}

	messages := fromEinoMessages(input)
	sr, sw := einoSchema.Pipe[*einoSchema.Message](32)
	go func() {
		defer sw.Close()

		extractor := newAssistantMessageExtractor()
		startAt := time.Now()
		rawDeltaChunks := 0
		rawDeltaChars := 0
		assistantDeltaChunks := 0
		assistantDeltaChars := 0
		firstDeltaMs := int64(-1)
		firstAssistantDeltaMs := int64(-1)

		raw, err := streamingModel.ChatStream(ctx, m.systemPrompt, messages, func(delta string) {
			if delta != "" {
				rawDeltaChunks++
				rawDeltaChars += len(delta)
				if firstDeltaMs < 0 {
					firstDeltaMs = time.Since(startAt).Milliseconds()
					if m.service != nil {
						m.service.log(ctx, "model_stream_first_delta", map[string]any{
							"mode":         m.mode,
							"attempt":      1,
							"firstDeltaMs": firstDeltaMs,
						})
					}
				}
			}

			decoded := extractor.Feed(delta)
			if decoded == "" {
				return
			}
			assistantDeltaChunks++
			assistantDeltaChars += len(decoded)
			if firstAssistantDeltaMs < 0 {
				firstAssistantDeltaMs = time.Since(startAt).Milliseconds()
				if m.service != nil {
					m.service.log(ctx, "model_stream_first_assistant_delta", map[string]any{
						"mode":                  m.mode,
						"attempt":               1,
						"firstAssistantDeltaMs": firstAssistantDeltaMs,
					})
				}
			}
			if closed := sw.Send(einoSchema.AssistantMessage(decoded, nil), nil); closed {
				return
			}
		})
		if err != nil {
			sw.Send(nil, err)
			return
		}

		elapsedMs := time.Since(startAt).Milliseconds()
		m.logModelCall(ctx, elapsedMs, raw, true, rawDeltaChunks, rawDeltaChars, assistantDeltaChunks, assistantDeltaChars, firstDeltaMs, firstAssistantDeltaMs)

		parsed, ok, didRepair, didForce, finalRaw := m.parseWithRecovery(ctx, messages, raw)
		if !ok {
			sw.Send(m.unparseableFallbackMessage(finalRaw, didRepair, didForce), nil)
			return
		}

		final := m.toEinoMessage(parsed)
		if assistantDeltaChars > 0 {
			final.Content = ""
		}
		sw.Send(final, nil)
	}()

	return sr, nil
}

func (m *jsonProtocolToolCallingModel) parseWithRecovery(ctx context.Context, messages []Message, raw string) (modelOutput, bool, bool, bool, string) {
	didRepair := false
	didForceToolCalls := false
	parsed, ok, parseErr := parseModelOutputDetailed(raw)

	if !ok && !didRepair && looksLikeToolProtocolJSON(raw) {
		didRepair = true
		fields := map[string]any{
			"mode":       m.mode,
			"attempt":    1,
			"rawLen":     len(raw),
			"rawPreview": previewForLog(raw),
			"parseError": errString(parseErr),
		}
		if m.service != nil && m.service.diagRawEnabled() {
			fields["raw"] = rawForLog(raw)
		}
		if m.service != nil {
			m.service.log(ctx, "tool_protocol_repair_triggered", fields)
		}
		repairMessages := append(cloneMessages(messages), Message{Role: "user", Content: toolProtocolRepairInstruction(m.locale)})
		if repairedRaw, err := m.model.Chat(ctx, m.systemPrompt, repairMessages); err == nil {
			if repairedParsed, repairedOK := parseModelOutput(repairedRaw); repairedOK {
				raw = repairedRaw
				parsed = repairedParsed
				ok = true
			} else if m.service != nil {
				fields := map[string]any{
					"mode":            m.mode,
					"attempt":         1,
					"repairedRawLen":  len(repairedRaw),
					"repairedPreview": previewForLog(repairedRaw),
				}
				if m.service.diagRawEnabled() {
					fields["repairedRaw"] = rawForLog(repairedRaw)
				}
				m.service.log(ctx, "tool_protocol_repair_failed", fields)
			}
		}
	}

	if ok && m.shouldForceWebSearchFinalization(parsed, messages) {
		if m.service != nil {
			m.service.log(ctx, "web_search_loop_finalize_triggered", map[string]any{
				"mode":           m.mode,
				"attempt":        1,
				"toolCallsCount": len(parsed.ToolCalls),
			})
		}
		finalizeMessages := append(cloneMessages(messages), Message{
			Role:    "user",
			Content: toolProtocolForceWebSearchFinalizeInstruction(m.locale),
		})
		if forcedRaw, err := m.model.Chat(ctx, m.systemPrompt, finalizeMessages); err == nil {
			if forcedParsed, forcedOK := parseModelOutput(forcedRaw); forcedOK && len(forcedParsed.ToolCalls) == 0 && strings.TrimSpace(forcedParsed.AssistantMessage) != "" {
				raw = forcedRaw
				parsed = forcedParsed
			} else {
				parsed = modelOutput{
					AssistantMessage: m.webSearchFallbackAnswer(messages),
					ToolCalls:        nil,
				}
			}
		} else {
			parsed = modelOutput{
				AssistantMessage: m.webSearchFallbackAnswer(messages),
				ToolCalls:        nil,
			}
		}
		m.resetWebSearchLoopState()
	}

	if ok && m.shouldForceDiscoveryReplanBeforeFocusExecute(parsed, messages) {
		if m.service != nil {
			m.service.log(ctx, "focus_execute_replan_triggered", map[string]any{
				"mode":               m.mode,
				"attempt":            1,
				"toolCallsCount":     len(parsed.ToolCalls),
				"discoveryToolCount": discoveryToolResultCount(messages),
			})
		}
		finalizeMessages := append(cloneMessages(messages), Message{
			Role:    "user",
			Content: toolProtocolForceDiscoveryReplanInstruction(m.locale),
		})
		if forcedRaw, err := m.model.Chat(ctx, m.systemPrompt, finalizeMessages); err == nil {
			if forcedParsed, forcedOK := parseModelOutput(forcedRaw); forcedOK && !m.shouldForceDiscoveryReplanBeforeFocusExecute(forcedParsed, messages) {
				raw = forcedRaw
				parsed = forcedParsed
			} else {
				parsed = modelOutput{
					AssistantMessage: discoveryReplanFallbackAnswer(m.locale),
					ToolCalls:        nil,
				}
			}
		} else {
			parsed = modelOutput{
				AssistantMessage: discoveryReplanFallbackAnswer(m.locale),
				ToolCalls:        nil,
			}
		}
	}

	if ok && m.shouldBlockDuplicateExecuteStatement(parsed, messages) {
		if m.service != nil {
			m.service.log(ctx, "execute_statement_duplicate_block_triggered", map[string]any{
				"mode":           m.mode,
				"attempt":        1,
				"toolCallsCount": len(parsed.ToolCalls),
			})
		}
		finalizeMessages := append(cloneMessages(messages), Message{
			Role:    "user",
			Content: toolProtocolForceExecuteDedupInstruction(m.locale),
		})
		if forcedRaw, err := m.model.Chat(ctx, m.systemPrompt, finalizeMessages); err == nil {
			if forcedParsed, forcedOK := parseModelOutput(forcedRaw); forcedOK && !m.shouldBlockDuplicateExecuteStatement(forcedParsed, messages) {
				raw = forcedRaw
				parsed = forcedParsed
			} else {
				parsed = modelOutput{
					AssistantMessage: duplicateExecuteFallbackAnswer(m.locale),
					ToolCalls:        nil,
				}
			}
		} else {
			parsed = modelOutput{
				AssistantMessage: duplicateExecuteFallbackAnswer(m.locale),
				ToolCalls:        nil,
			}
		}
	}

	if !ok && m.service != nil {
		m.service.log(ctx, "model_output_unparseable", map[string]any{
			"mode":       m.mode,
			"attempt":    1,
			"didRepair":  didRepair,
			"didForce":   didForceToolCalls,
			"rawLen":     len(raw),
			"rawPreview": previewForLog(raw),
			"parseError": errString(parseErr),
		})
	}
	if ok && m.service != nil {
		m.service.log(ctx, "model_output_parsed", map[string]any{
			"mode":             m.mode,
			"attempt":          1,
			"assistantLen":     len(parsed.AssistantMessage),
			"toolCallsCount":   len(parsed.ToolCalls),
			"toolCallsPreview": summarizeToolCallsForLog(parsed.ToolCalls),
		})
	}
	return parsed, ok, didRepair, didForceToolCalls, raw
}

func (m *jsonProtocolToolCallingModel) shouldForceWebSearchFinalization(parsed modelOutput, messages []Message) bool {
	if strings.TrimSpace(parsed.AssistantMessage) != "" {
		m.resetWebSearchLoopState()
		return false
	}
	if !allToolCallsAreWebSearch(parsed.ToolCalls) || !containsWebSearchToolResult(messages) {
		m.resetWebSearchLoopState()
		return false
	}
	m.loopMu.Lock()
	defer m.loopMu.Unlock()
	m.webSearchLoopStreak++
	return m.webSearchLoopStreak >= 2
}

func (m *jsonProtocolToolCallingModel) resetWebSearchLoopState() {
	m.loopMu.Lock()
	m.webSearchLoopStreak = 0
	m.loopMu.Unlock()
}

func allToolCallsAreWebSearch(calls []toolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for i := range calls {
		if !strings.EqualFold(strings.TrimSpace(calls[i].Name), "web_search") {
			return false
		}
	}
	return true
}

func containsWebSearchToolResult(messages []Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		content := strings.TrimSpace(messages[i].Content)
		if strings.HasPrefix(content, "[tool_result] web_search") {
			return true
		}
	}
	return false
}

func toolProtocolForceWebSearchFinalizeInstruction(locale uiLocale) string {
	if locale == uiLocaleZH {
		return `你已经拿到了一个或多个 [tool_result] web_search。
请直接基于已有结果给出最终回答，不要再调用任何工具。
要求：
- 输出合法 JSON：{"assistantMessage":"...","toolCalls":[]}
- assistantMessage 中应给出精炼结论，并附上可点击链接（最多 3 条）。`
	}
	return `You already have one or more [tool_result] web_search messages.
Now provide the final answer directly from existing results, and DO NOT call tools again.
Requirements:
- Output valid JSON: {"assistantMessage":"...","toolCalls":[]}
- assistantMessage should include a concise conclusion plus clickable links (up to 3).`
}

func (m *jsonProtocolToolCallingModel) shouldBlockDuplicateExecuteStatement(parsed modelOutput, messages []Message) bool {
	if !allToolCallsAreExecuteStatement(parsed.ToolCalls) {
		return false
	}
	req := m.currentReq()
	if intent := cloneTurnIntent(parsed.Intent); intent != nil {
		req.Intent = intent
	}
	defaults := defaultExecuteStatementContext(req)
	requested := requestedExecuteStatements(parsed.ToolCalls, defaults)
	if len(requested) == 0 {
		return false
	}
	seen := executedStatementsFromMessages(messages, latestExecuteStatementContext(messages, defaults))
	if len(seen) == 0 {
		return false
	}
	for _, statement := range requested {
		if _, ok := seen[statement]; !ok {
			return false
		}
	}
	return true
}

func (m *jsonProtocolToolCallingModel) shouldForceDiscoveryReplanBeforeFocusExecute(parsed modelOutput, messages []Message) bool {
	if !allToolCallsAreExecuteStatement(parsed.ToolCalls) {
		return false
	}
	if containsExecuteStatementToolResult(messages) {
		return false
	}
	if discoveryToolResultCount(messages) < 3 {
		return false
	}
	if intent := cloneTurnIntent(parsed.Intent); intent != nil && intent.CurrentFocus == turnIntentFocusPreferCurrent {
		return false
	}
	req := m.currentReq()
	if hasEstablishedWorkingTarget(req) {
		return false
	}
	focus := pageExecuteStatementContext(req.PageContext)
	if strings.TrimSpace(focus.DatasourceID) == "" {
		return false
	}
	targets := requestedExecuteTargets(parsed.ToolCalls, focus)
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if strings.TrimSpace(target.DatasourceID) != strings.TrimSpace(focus.DatasourceID) {
			return false
		}
		if strings.TrimSpace(target.Database) != strings.TrimSpace(focus.Database) {
			return false
		}
	}
	return true
}

type executeStatementContext struct {
	DatasourceID string
	Database     string
}

func pageExecuteStatementContext(page PageContext) executeStatementContext {
	return executeStatementContext{
		DatasourceID: strings.TrimSpace(page.CurrentDatasourceID),
		Database:     strings.TrimSpace(page.CurrentDatabase),
	}
}

func defaultExecuteStatementContext(req TurnRequest) executeStatementContext {
	return executeStatementContext{
		DatasourceID: strings.TrimSpace(defaultDatasourceIDForTool(req)),
		Database:     strings.TrimSpace(defaultDatabaseForTool(req)),
	}
}

func (m *jsonProtocolToolCallingModel) webSearchFallbackAnswer(messages []Message) string {
	links := extractWebSearchLinksFromMessages(messages, 3)
	if m.locale == uiLocaleZH {
		if len(links) == 0 {
			return "我已经多次调用 web_search，但当前轮次未能稳定收敛到最终回答。请提供更具体的关键词（例如版本号、官网路径），我会继续检索并给出来源链接。"
		}
		return "我已停止重复检索，先给你当前可用来源链接：\n" + strings.Join(links, "\n")
	}
	if len(links) == 0 {
		return "I stopped repeated web_search retries because this turn did not converge. Please provide a narrower query (for example exact version or official site path) and I will continue."
	}
	return "I stopped repeated web_search retries. Current source links:\n" + strings.Join(links, "\n")
}

func allToolCallsAreExecuteStatement(calls []toolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for i := range calls {
		if !strings.EqualFold(strings.TrimSpace(calls[i].Name), "execute_statement") {
			return false
		}
	}
	return true
}

func requestedExecuteStatements(calls []toolCall, defaults executeStatementContext) []string {
	out := make([]string, 0, len(calls))
	for i := range calls {
		datasourceID := strings.TrimSpace(stringArg(calls[i].Arguments, "datasourceId", "id"))
		if datasourceID == "" {
			datasourceID = defaults.DatasourceID
		}
		database := strings.TrimSpace(stringArg(calls[i].Arguments, "database"))
		if database == "" {
			database = defaults.Database
		}
		signature := executeStatementSignature(
			datasourceID,
			database,
			statementArg(calls[i].Arguments, "statement"),
			stringArg(calls[i].Arguments, "pagingToken", "pageToken", "nextToken"),
			intArg(calls[i].Arguments, "pageSize", 100),
		)
		if signature == "" {
			return nil
		}
		out = append(out, signature)
	}
	return out
}

func requestedExecuteTargets(calls []toolCall, defaults executeStatementContext) []executeStatementContext {
	out := make([]executeStatementContext, 0, len(calls))
	for i := range calls {
		datasourceID := strings.TrimSpace(stringArg(calls[i].Arguments, "datasourceId", "id"))
		if datasourceID == "" {
			datasourceID = strings.TrimSpace(defaults.DatasourceID)
		}
		database := strings.TrimSpace(stringArg(calls[i].Arguments, "database"))
		if database == "" {
			database = strings.TrimSpace(defaults.Database)
		}
		if datasourceID == "" && database == "" {
			return nil
		}
		out = append(out, executeStatementContext{
			DatasourceID: datasourceID,
			Database:     database,
		})
	}
	return out
}

func latestExecuteStatementContext(messages []Message, fallback executeStatementContext) executeStatementContext {
	ctx := fallback
	for i := len(messages) - 1; i >= 0; i-- {
		content := strings.TrimSpace(messages[i].Content)
		if !strings.HasPrefix(content, "[tool_result] execute_statement") {
			continue
		}
		payloadJSON := content
		if idx := strings.Index(content, "\n"); idx != -1 {
			payloadJSON = content[idx+1:]
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(payloadJSON)), &payload); err != nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(payload["error"])) != "" && fmt.Sprint(payload["error"]) != "<nil>" {
			continue
		}
		consoleResult, _ := payload["consoleResult"].(map[string]any)
		if datasourceID := strings.TrimSpace(stringArg(consoleResult, "datasourceId", "id")); datasourceID != "" {
			ctx.DatasourceID = datasourceID
		} else if datasourceID := strings.TrimSpace(stringArg(payload, "datasourceId", "id")); datasourceID != "" {
			ctx.DatasourceID = datasourceID
		}
		if database := strings.TrimSpace(stringArg(consoleResult, "database")); database != "" {
			ctx.Database = database
		} else if database := strings.TrimSpace(stringArg(payload, "database")); database != "" {
			ctx.Database = database
		}
		return ctx
	}
	return ctx
}

func executedStatementsFromMessages(messages []Message, defaults executeStatementContext) map[string]struct{} {
	out := make(map[string]struct{})
	for i := range messages {
		content := strings.TrimSpace(messages[i].Content)
		if !strings.HasPrefix(content, "[tool_result] execute_statement") {
			continue
		}
		payloadJSON := content
		if idx := strings.Index(content, "\n"); idx != -1 {
			payloadJSON = content[idx+1:]
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(payloadJSON)), &payload); err != nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(payload["error"])) != "" && fmt.Sprint(payload["error"]) != "<nil>" {
			continue
		}
		consoleResult, _ := payload["consoleResult"].(map[string]any)
		datasourceID := strings.TrimSpace(stringArg(consoleResult, "datasourceId", "id"))
		if datasourceID == "" {
			datasourceID = strings.TrimSpace(stringArg(payload, "datasourceId", "id"))
		}
		if datasourceID == "" {
			datasourceID = defaults.DatasourceID
		}
		database := strings.TrimSpace(stringArg(consoleResult, "database"))
		if database == "" {
			database = strings.TrimSpace(stringArg(payload, "database"))
		}
		if database == "" {
			database = defaults.Database
		}
		signature := executeStatementSignature(
			datasourceID,
			database,
			stringArg(consoleResult, "statement"),
			stringArg(payload, "pagingToken", "pageToken", "nextToken"),
			intArg(payload, "pageSize", 100),
		)
		if signature == "" {
			signature = executeStatementSignature(
				datasourceID,
				database,
				stringArg(payload, "statement"),
				stringArg(payload, "pagingToken", "pageToken", "nextToken"),
				intArg(payload, "pageSize", 100),
			)
		}
		if signature != "" {
			out[signature] = struct{}{}
		}
	}
	return out
}

func containsExecuteStatementToolResult(messages []Message) bool {
	for i := range messages {
		if strings.HasPrefix(strings.TrimSpace(messages[i].Content), "[tool_result] execute_statement") {
			return true
		}
	}
	return false
}

func discoveryToolResultCount(messages []Message) int {
	count := 0
	for i := range messages {
		if isDiscoveryToolResultContent(strings.TrimSpace(messages[i].Content)) {
			count++
		}
	}
	return count
}

func isDiscoveryToolResultContent(content string) bool {
	if !strings.HasPrefix(content, "[tool_result] ") {
		return false
	}
	for _, toolName := range []string{
		"list_datasources",
		"get_datasource",
		"list_databases",
		"list_entities",
		"describe_entity",
		"search_knowledge",
		"get_schema_knowledge",
		"get_er_knowledge",
	} {
		if strings.HasPrefix(content, "[tool_result] "+toolName) {
			return true
		}
	}
	return false
}

func normalizeExecuteStatementSignature(statement string) string {
	normalized := sanitizeStatementForTool(statement)
	if normalized == "" {
		return ""
	}
	normalized = collapseExecuteStatementWhitespace(normalized)
	if normalized == "" {
		return ""
	}
	return canonicalizeExecuteStatementKeywords(normalized)
}

func executeStatementSignature(datasourceID, database, statement, pagingToken string, pageSize int) string {
	normalizedStatement := normalizeExecuteStatementSignature(statement)
	if normalizedStatement == "" {
		return ""
	}
	pageSize = normalizeExecuteStatementPageSize(pageSize)
	return strings.Join([]string{
		strings.TrimSpace(datasourceID),
		strings.TrimSpace(database),
		normalizedStatement,
		strings.TrimSpace(pagingToken),
		strconv.Itoa(pageSize),
	}, "\x1f")
}

func collapseExecuteStatementWhitespace(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(statement))

	inSingle := false
	inDouble := false
	inBacktick := false
	pendingSpace := false

	for i := 0; i < len(statement); i++ {
		ch := statement[i]
		if inSingle {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(statement) {
				i++
				b.WriteByte(statement[i])
				continue
			}
			if ch == '\'' {
				if i+1 < len(statement) && statement[i+1] == '\'' {
					i++
					b.WriteByte(statement[i])
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(statement) {
				i++
				b.WriteByte(statement[i])
				continue
			}
			if ch == '"' {
				if i+1 < len(statement) && statement[i+1] == '"' {
					i++
					b.WriteByte(statement[i])
					continue
				}
				inDouble = false
			}
			continue
		}
		if inBacktick {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(statement) {
				i++
				b.WriteByte(statement[i])
				continue
			}
			if ch == '`' {
				inBacktick = false
			}
			continue
		}
		if ch == '\'' {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			inSingle = true
			b.WriteByte(ch)
			continue
		}
		if ch == '"' {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			inDouble = true
			b.WriteByte(ch)
			continue
		}
		if ch == '`' {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			inBacktick = true
			b.WriteByte(ch)
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v' {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteByte(ch)
	}

	return strings.TrimSpace(b.String())
}

var executeStatementCanonicalKeywords = map[string]struct{}{
	"all": {}, "alter": {}, "analyze": {}, "and": {}, "as": {}, "asc": {}, "between": {}, "by": {}, "case": {},
	"create": {}, "cross": {}, "delete": {}, "desc": {}, "describe": {}, "distinct": {}, "drop": {}, "else": {},
	"end": {}, "exists": {}, "explain": {}, "false": {}, "from": {}, "full": {}, "group": {}, "having": {},
	"in": {}, "inner": {}, "insert": {}, "into": {}, "is": {}, "join": {}, "left": {}, "like": {}, "limit": {},
	"max": {}, "min": {}, "not": {}, "null": {}, "offset": {}, "on": {}, "or": {}, "order": {}, "outer": {},
	"replace": {}, "right": {}, "select": {}, "set": {}, "show": {}, "sum": {}, "table": {}, "then": {},
	"true": {}, "truncate": {}, "union": {}, "update": {}, "use": {}, "values": {}, "when": {}, "where": {},
}

func canonicalizeExecuteStatementKeywords(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(statement))

	inSingle := false
	inDouble := false
	inBacktick := false
	inBracket := false

	for i := 0; i < len(statement); {
		ch := statement[i]
		if inSingle {
			b.WriteByte(ch)
			i++
			if ch == '\\' && i < len(statement) {
				b.WriteByte(statement[i])
				i++
				continue
			}
			if ch == '\'' {
				if i < len(statement) && statement[i] == '\'' {
					b.WriteByte(statement[i])
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(ch)
			i++
			if ch == '\\' && i < len(statement) {
				b.WriteByte(statement[i])
				i++
				continue
			}
			if ch == '"' {
				if i < len(statement) && statement[i] == '"' {
					b.WriteByte(statement[i])
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		if inBacktick {
			b.WriteByte(ch)
			i++
			if ch == '\\' && i < len(statement) {
				b.WriteByte(statement[i])
				i++
				continue
			}
			if ch == '`' {
				inBacktick = false
			}
			continue
		}
		if inBracket {
			b.WriteByte(ch)
			i++
			if ch == ']' {
				if i < len(statement) && statement[i] == ']' {
					b.WriteByte(statement[i])
					i++
					continue
				}
				inBracket = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
			b.WriteByte(ch)
			i++
			continue
		case '"':
			inDouble = true
			b.WriteByte(ch)
			i++
			continue
		case '`':
			inBacktick = true
			b.WriteByte(ch)
			i++
			continue
		case '[':
			inBracket = true
			b.WriteByte(ch)
			i++
			continue
		}
		if isExecuteStatementKeywordIdentStart(ch) {
			start := i
			i++
			for i < len(statement) && isExecuteStatementKeywordIdentPart(statement[i]) {
				i++
			}
			token := statement[start:i]
			lower := strings.ToLower(token)
			if _, ok := executeStatementCanonicalKeywords[lower]; ok {
				b.WriteString(lower)
			} else {
				b.WriteString(token)
			}
			continue
		}
		b.WriteByte(ch)
		i++
	}

	return strings.TrimSpace(b.String())
}

func isExecuteStatementKeywordIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isExecuteStatementKeywordIdentPart(ch byte) bool {
	return isExecuteStatementKeywordIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func toolProtocolForceExecuteDedupInstruction(locale uiLocale) string {
	if locale == uiLocaleZH {
		return `你已经拿到了同一条 statement 的 [tool_result] execute_statement。
不要再次执行相同语句，除非你明确更改了 pagingToken，或者是在修复上一条执行错误。
优先选择：
- 直接基于已有结果给出结论；
- 或调用 describe_entity / get_schema_knowledge 查看键、索引和 schema。
要求：
- 输出合法 JSON。
- 不要再次输出相同的 execute_statement。`
	}
	return `You already have a [tool_result] execute_statement for the same statement.
Do not execute the same statement again unless you are changing pagingToken or fixing a previous execution error.
Prefer one of these:
- give the conclusion from the existing result;
- or call describe_entity / get_schema_knowledge to inspect keys, indexes, and schema.
Requirements:
- Output valid JSON.
- Do not emit the same execute_statement again.`
}

func duplicateExecuteFallbackAnswer(locale uiLocale) string {
	if locale == uiLocaleZH {
		return "我已停止重复执行同一条语句。下一步更合理的是先看 schema/索引信息，再基于已有结果解释原因。"
	}
	return "I stopped re-running the same statement. The next useful step is to inspect schema/index details or explain from the existing result."
}

func toolProtocolForceDiscoveryReplanInstruction(locale uiLocale) string {
	if locale == uiLocaleZH {
		return `你已经多次使用只读探索工具，但还没有建立稳定的 working context。
不要直接在当前页面的 datasource/database 上执行 execute_statement 来碰运气。
下一步只能这样做：
- 继续扩大探索范围，优先使用 list_datasources / get_datasource / search_knowledge / list_entities / describe_entity 来定位更合适的目标；
- 如果仍然有多个候选，直接向用户确认缺失信息。
要求：
- 先不要调用 execute_statement 或 explain_statement。
- 如果你已经判断“不要用当前页面 focus”，请在 intent.currentFocus 里明确输出 avoid_current。
- 输出合法 JSON。`
	}
	return `You already used multiple read-only discovery tools, but you still do not have a stable working context.
Do not execute against the current page datasource/database just to guess.
Next step:
- either expand discovery with list_datasources / get_datasource / search_knowledge / list_entities / describe_entity to locate a better target;
- or ask the user to clarify the missing target if multiple candidates remain.
Requirements:
- Do not call execute_statement or explain_statement yet.
- If you already know the current page focus should be avoided, set intent.currentFocus to avoid_current.
- Output valid JSON.`
}

func discoveryReplanFallbackAnswer(locale uiLocale) string {
	if locale == uiLocaleZH {
		return "我已经停止在当前页面数据源上盲目执行。请告诉我更明确的 datasource、database 或对象名，或者我可以继续跨数据源只读探索。"
	}
	return "I stopped guessing on the current page datasource. Tell me the datasource, database, or object name more explicitly, or I can continue read-only discovery across datasources."
}

func extractWebSearchLinksFromMessages(messages []Message, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for i := len(messages) - 1; i >= 0; i-- {
		content := strings.TrimSpace(messages[i].Content)
		if !strings.HasPrefix(content, "[tool_result] web_search") {
			continue
		}
		payload := content
		if idx := strings.Index(content, "\n"); idx >= 0 && idx+1 < len(content) {
			payload = strings.TrimSpace(content[idx+1:])
		}
		var resp WebSearchResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			continue
		}
		for _, item := range resp.Results {
			link := strings.TrimSpace(item.URL)
			if link == "" {
				continue
			}
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			out = append(out, fmt.Sprintf("%d. %s", len(out)+1, link))
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func (m *jsonProtocolToolCallingModel) unparseableFallbackMessage(raw string, didRepair bool, didForceToolCalls bool) *einoSchema.Message {
	if didForceToolCalls && !looksLikeToolProtocolJSON(raw) && strings.EqualFold(strings.TrimSpace(m.mode), "turn_stream") {
		content := strings.TrimSpace(raw)
		if content == "" {
			content = raw
		}
		return einoSchema.AssistantMessage(content, nil)
	}
	if didRepair || didForceToolCalls || looksLikeToolProtocolJSON(raw) {
		if m.locale == uiLocaleZH {
			return einoSchema.AssistantMessage("AI 返回内容格式异常或被截断，请重试。", nil)
		}
		return einoSchema.AssistantMessage("AI returned an invalid/truncated response. Please try again.", nil)
	}
	content := strings.TrimSpace(raw)
	if content == "" {
		content = raw
	}
	return einoSchema.AssistantMessage(content, nil)
}

func (m *jsonProtocolToolCallingModel) toEinoMessage(parsed modelOutput) *einoSchema.Message {
	calls := toEinoToolCalls(parsed.ToolCalls, parsed.Agent, parsed.Plan, parsed.Intent)
	msg := einoSchema.AssistantMessage(strings.TrimSpace(parsed.AssistantMessage), calls)
	if parsed.Agent != nil || parsed.Plan != nil {
		msg.Extra = map[string]any{}
		if parsed.Agent != nil {
			msg.Extra[einoMessageAgentKey] = parsed.Agent
		}
		if parsed.Plan != nil {
			msg.Extra[einoMessagePlanKey] = parsed.Plan
		}
	}
	return msg
}

func (m *jsonProtocolToolCallingModel) logModelCall(ctx context.Context, elapsedMs int64, raw string, canStream bool, rawDeltaChunks int, rawDeltaChars int, assistantDeltaChunks int, assistantDeltaChars int, firstDeltaMs int64, firstAssistantDeltaMs int64) {
	if m == nil || m.service == nil {
		return
	}
	fields := map[string]any{
		"mode":       m.mode,
		"attempt":    1,
		"durationMs": elapsedMs,
		"rawLen":     len(raw),
		"didRepair":  false,
		"canStream":  canStream,
	}
	if canStream {
		fields["rawDeltaChunks"] = rawDeltaChunks
		fields["rawDeltaChars"] = rawDeltaChars
		fields["assistantDeltaChunks"] = assistantDeltaChunks
		fields["assistantDeltaChars"] = assistantDeltaChars
		fields["firstDeltaMs"] = firstDeltaMs
		fields["firstAssistantDeltaMs"] = firstAssistantDeltaMs
	} else {
		fields["messageTail"] = summarizeMessagesForLog(m.currentReq().Messages)
	}
	m.service.log(ctx, "model_call", fields)
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}

func (m *jsonProtocolToolCallingModel) WithTools(_ []*einoSchema.ToolInfo) (einoModel.ToolCallingChatModel, error) {
	return m, nil
}

func (m *jsonProtocolToolCallingModel) BindTools(_ []*einoSchema.ToolInfo) error {
	return nil
}

func fromEinoMessages(input []*einoSchema.Message) []Message {
	out := make([]Message, 0, len(input))
	for _, msg := range input {
		if msg == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(string(msg.Role)))
		switch role {
		case "system", "assistant", "user":
		case "tool":
			role = "assistant"
		default:
			role = "user"
		}
		out = append(out, Message{Role: role, Content: msg.Content})
	}
	return out
}

func toEinoToolCalls(calls []toolCall, agent *AgentDecision, plan *AgentPlan, intent *TurnIntent) []einoSchema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]einoSchema.ToolCall, 0, len(calls))
	for i := range calls {
		name := strings.TrimSpace(calls[i].Name)
		if name == "" {
			continue
		}
		args := cloneMapAny(calls[i].args())
		if args == nil {
			args = map[string]any{}
		}
		if agent != nil {
			args[einoMetaAgentKey] = cloneAgentDecision(agent)
		}
		if plan != nil {
			args[einoMetaPlanKey] = cloneAgentPlan(plan)
		}
		if intent != nil {
			args[einoMetaIntentKey] = cloneTurnIntent(intent)
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			encoded = []byte("{}")
		}
		out = append(out, einoSchema.ToolCall{
			ID: fmt.Sprintf("tool_%d_%x", i+1, time.Now().UTC().UnixNano()),
			Function: einoSchema.FunctionCall{
				Name:      name,
				Arguments: string(encoded),
			},
		})
	}
	return out
}

func cloneMapAny(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type einoTurnRuntime struct {
	service        *Service
	req            TurnRequest
	conversationID string
	locale         uiLocale
	userText       string
	mode           string

	effectsMu            sync.Mutex
	pendingEffects       Effects
	memorySaveCount      int
	focusDiscoveryMisses int
}

func (r *einoTurnRuntime) toEinoMessages(messages []Message) []*einoSchema.Message {
	out := make([]*einoSchema.Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system":
			out = append(out, einoSchema.SystemMessage(msg.Content))
		case "assistant":
			out = append(out, einoSchema.AssistantMessage(msg.Content, nil))
		case "user":
			out = append(out, einoSchema.UserMessage(msg.Content))
		default:
			out = append(out, einoSchema.UserMessage(msg.Content))
		}
	}
	return out
}

func (r *einoTurnRuntime) toTurnResponse(msg *einoSchema.Message) TurnResponse {
	if msg == nil {
		return TurnResponse{AssistantMessage: "", Effects: r.pendingEffectsSnapshot()}
	}

	if msg.Role == einoSchema.Tool {
		if resp, ok := decodeEinoTurnEnvelope(msg.Content); ok {
			resp.Effects = mergeEffects(r.pendingEffectsSnapshot(), resp.Effects)
			return resp
		}
	}

	resp := TurnResponse{AssistantMessage: strings.TrimSpace(msg.Content)}
	if resp.AssistantMessage == "" {
		resp.AssistantMessage = msg.Content
	}
	resp.Effects = r.pendingEffectsSnapshot()
	resp.Agent, resp.Plan = extractMetadataFromMessage(msg)
	return resp
}

func (r *einoTurnRuntime) responseFromInterrupt(ctx context.Context, err error, checkpointID string) (TurnResponse, bool) {
	if r == nil || r.service == nil || r.service.einoApprovals == nil {
		return TurnResponse{}, false
	}
	info, ok := einoCompose.ExtractInterruptInfo(err)
	if !ok || info == nil || len(info.InterruptContexts) == 0 {
		return TurnResponse{}, false
	}

	var chosen *einoCompose.InterruptCtx
	for _, item := range info.InterruptContexts {
		if item == nil {
			continue
		}
		if _, _, _, _, _, _, ok := decodeApprovalInterruptInfo(item.Info); !ok {
			continue
		}
		if chosen == nil || item.IsRootCause {
			chosen = item
			if item.IsRootCause {
				break
			}
		}
	}
	if chosen == nil {
		return TurnResponse{}, false
	}

	kind, summary, payload, assistantMessage, agent, plan, ok := decodeApprovalInterruptInfo(chosen.Info)
	if !ok {
		return TurnResponse{}, false
	}
	approvalID := newApprovalID()
	r.service.einoApprovals.put(r.conversationID, approvalID, einoApprovalResumeItem{
		CheckpointID: strings.TrimSpace(checkpointID),
		InterruptID:  strings.TrimSpace(chosen.ID),
		Request:      r.req,
		Locale:       r.locale,
		Kind:         kind,
		Effects:      r.pendingEffectsSnapshot(),
		CreatedAt:    time.Now().UTC(),
	})
	r.service.log(ctx, "approval_pending", map[string]any{
		"kind":         string(kind),
		"approvalId":   approvalID,
		"checkpointId": strings.TrimSpace(checkpointID),
		"interruptId":  strings.TrimSpace(chosen.ID),
		"mode":         r.mode,
		"reason":       "eino_interrupt_checkpoint",
	})
	resp := TurnResponse{
		AssistantMessage: strings.TrimSpace(assistantMessage),
		Effects:          r.pendingEffectsSnapshot(),
		Approval: &Approval{
			ID:      approvalID,
			Kind:    kind,
			Summary: strings.TrimSpace(summary),
			Payload: payload,
		},
		Agent: agent,
		Plan:  plan,
	}
	return resp, true
}

func (r *einoTurnRuntime) rememberEffects(effects Effects) {
	r.effectsMu.Lock()
	defer r.effectsMu.Unlock()
	r.pendingEffects = mergeEffects(r.pendingEffects, effects)
}

func (r *einoTurnRuntime) pendingEffectsSnapshot() Effects {
	r.effectsMu.Lock()
	defer r.effectsMu.Unlock()
	return cloneEffects(r.pendingEffects)
}

func mergeEffects(base Effects, next Effects) Effects {
	merged := cloneEffects(base)
	if next.DatasourcesChanged {
		merged.DatasourcesChanged = true
	}
	if strings.TrimSpace(next.NavigateTo) != "" {
		merged.NavigateTo = strings.TrimSpace(next.NavigateTo)
	}
	if next.ConsoleResult != nil {
		value := *next.ConsoleResult
		merged.ConsoleResult = &value
	}
	if next.Visualization != nil {
		value := *next.Visualization
		merged.Visualization = &value
	}
	return merged
}

func cloneEffects(value Effects) Effects {
	out := value
	if value.ConsoleResult != nil {
		copyValue := *value.ConsoleResult
		out.ConsoleResult = &copyValue
	}
	if value.Visualization != nil {
		copyValue := *value.Visualization
		out.Visualization = &copyValue
	}
	return out
}

func (r *einoTurnRuntime) buildTools() []einoTool.BaseTool {
	tools := make([]einoTool.BaseTool, 0, 16)
	tools = append(tools,
		r.newTool("list_datasources", "List configured datasources.", nil, r.toolListDatasources),
		r.newTool("get_datasource", "Get datasource details by id.", paramsString(map[string]bool{"datasourceId": true, "id": false}), r.toolGetDatasource),
		r.newTool("list_databases", "List databases in datasource.", paramsString(map[string]bool{"datasourceId": true, "pattern": false}), r.toolListDatabases),
		r.newTool("list_entities", "List entities/tables/collections in datasource.", paramsString(map[string]bool{"datasourceId": true, "database": false, "pattern": false}), r.toolListEntities),
		r.newTool("describe_entity", "Describe one entity.", paramsString(map[string]bool{"datasourceId": true, "name": true, "entity": false, "database": false}), r.toolDescribeEntity),
		r.newTool("search_knowledge", "Search project knowledge and user knowledge packs.", paramsString(map[string]bool{"query": false, "q": false, "topic": false, "intent": false, "hint": false, "scope": false, "datasourceId": false, "datasourceType": false, "maxHits": false, "contextLines": false, "maxFiles": false}), r.toolSearchKnowledge),
		r.newTool("memory_save", "Save a reusable troubleshooting pattern into long-term memory.", paramsMemorySave(), r.toolMemorySave),
		r.newTool("web_search", "Search public web results via Google/DuckDuckGo/Bing.", paramsMixed(map[string]paramKind{"query": paramStringRequired, "engine": paramStringOptional, "maxResults": paramIntegerOptional}), r.toolWebSearch),
		r.newTool("explain_statement", "Explain statement and estimate scan/index usage.", paramsMixed(map[string]paramKind{"datasourceId": paramStringRequired, "statement": paramStringRequired, "database": paramStringOptional}), r.toolExplainStatement),
		r.newTool("execute_statement", "Prepare execution and request user approval when needed.", paramsMixed(map[string]paramKind{"datasourceId": paramStringOptional, "statement": paramStringRequired, "database": paramStringOptional, "pagingToken": paramStringOptional, "pageSize": paramIntegerOptional}), r.toolExecuteStatement),
		r.newTool("analyze_result", "Analyze latest AI console result rows (approval required).", paramsString(map[string]bool{"question": false}), r.toolAnalyzeResult),
		r.newTool("create_visualization", "Generate visualization from latest AI console result rows (approval required).", paramsString(map[string]bool{"question": false}), r.toolCreateVisualization),
		r.newTool("get_redis_command_docs", "Get Redis command documentation.", paramsString(map[string]bool{"datasourceId": true, "command": true}), r.toolGetRedisCommandDocs),
		r.newTool("get_schema_knowledge", "Get datasource schema knowledge from customer knowledge base.", paramsString(map[string]bool{"datasourceId": true, "entity": false, "database": false}), r.toolGetSchemaKnowledge),
		r.newTool("get_er_knowledge", "Get datasource ER knowledge generated from schema snapshot.", paramsString(map[string]bool{"datasourceId": true, "database": false}), r.toolGetERKnowledge),
		r.newTool("create_datasource", "Create datasource after approval.", paramsMixed(map[string]paramKind{"name": paramStringRequired, "type": paramStringRequired, "host": paramStringOptional, "port": paramIntegerOptional, "username": paramStringOptional, "password": paramStringOptional, "database": paramStringOptional, "authSource": paramStringOptional}), r.toolCreateDatasource),
		r.newTool("delete_datasource", "Delete datasource after approval.", paramsString(map[string]bool{"datasourceId": false, "name": false}), r.toolDeleteDatasource),
		r.newTool("navigate_to_datasource", "Navigate UI to datasource console or route.", paramsString(map[string]bool{"datasourceId": false, "name": false}), r.toolNavigateDatasource),
	)
	return tools
}

func (r *einoTurnRuntime) unknownToolHandler(_ context.Context, name, _ string) (string, error) {
	body, err := marshalToolResult(map[string]any{"error": fmt.Sprintf("unknown tool: %s", strings.TrimSpace(name))})
	if err != nil {
		return "", err
	}
	return formatToolResultForModel(name, body), nil
}

type einoInvokableTool struct {
	info *einoSchema.ToolInfo
	run  func(ctx context.Context, args map[string]any) (string, error)
}

func (t *einoInvokableTool) Info(context.Context) (*einoSchema.ToolInfo, error) {
	return t.info, nil
}

func (t *einoInvokableTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einoTool.Option) (string, error) {
	args := map[string]any{}
	if strings.TrimSpace(argumentsInJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			body, bodyErr := marshalToolResult(map[string]any{"error": fmt.Sprintf("invalid tool arguments: %v", err)})
			if bodyErr != nil {
				return "", bodyErr
			}
			return formatToolResultForModel(t.info.Name, body), nil
		}
	}
	result, err := t.run(ctx, args)
	if err != nil {
		return "", err
	}
	if _, ok := decodeEinoTurnEnvelope(result); ok {
		return result, nil
	}
	return formatToolResultForModel(t.info.Name, result), nil
}

func (r *einoTurnRuntime) newTool(name, desc string, params *einoSchema.ParamsOneOf, run func(ctx context.Context, args map[string]any) (string, error)) einoTool.BaseTool {
	return &einoInvokableTool{
		info: &einoSchema.ToolInfo{
			Name:        name,
			Desc:        desc,
			ParamsOneOf: params,
		},
		run: func(ctx context.Context, args map[string]any) (string, error) {
			r.rememberIntent(decodeTurnIntentFromAny(args[einoMetaIntentKey]))
			ctx = schemaprivacy.ContextWithAIConfigID(ctx, r.req.AIConfigID)
			return run(ctx, args)
		},
	}
}

type paramKind uint8

const (
	paramStringOptional paramKind = iota
	paramStringRequired
	paramIntegerOptional
)

func paramsString(fields map[string]bool) *einoSchema.ParamsOneOf {
	params := make(map[string]*einoSchema.ParameterInfo, len(fields))
	for name, required := range fields {
		params[name] = &einoSchema.ParameterInfo{Type: einoSchema.String, Required: required}
	}
	return einoSchema.NewParamsOneOfByParams(params)
}

func paramsMixed(fields map[string]paramKind) *einoSchema.ParamsOneOf {
	params := make(map[string]*einoSchema.ParameterInfo, len(fields))
	for name, kind := range fields {
		info := &einoSchema.ParameterInfo{}
		switch kind {
		case paramStringRequired:
			info.Type = einoSchema.String
			info.Required = true
		case paramIntegerOptional:
			info.Type = einoSchema.Integer
		default:
			info.Type = einoSchema.String
		}
		params[name] = info
	}
	return einoSchema.NewParamsOneOfByParams(params)
}

func paramsMemorySave() *einoSchema.ParamsOneOf {
	return einoSchema.NewParamsOneOfByParams(map[string]*einoSchema.ParameterInfo{
		"problem":          {Type: einoSchema.String, Required: true},
		"signals":          {Type: einoSchema.Array, ElemInfo: &einoSchema.ParameterInfo{Type: einoSchema.String}},
		"avoid":            {Type: einoSchema.Array, ElemInfo: &einoSchema.ParameterInfo{Type: einoSchema.String}},
		"do":               {Type: einoSchema.Array, ElemInfo: &einoSchema.ParameterInfo{Type: einoSchema.String}},
		"why":              {Type: einoSchema.String},
		"confidence":       {Type: einoSchema.Number},
		"evidenceEventIds": {Type: einoSchema.Array, ElemInfo: &einoSchema.ParameterInfo{Type: einoSchema.String}},
		"replaceHints":     {Type: einoSchema.Array, ElemInfo: &einoSchema.ParameterInfo{Type: einoSchema.String}},
	})
}

func (r *einoTurnRuntime) resumeArgsForTool(ctx context.Context, toolName string) (map[string]any, bool) {
	wasInterrupted, hasState, state := einoTool.GetInterruptState[map[string]any](ctx)
	if !wasInterrupted || !hasState || len(state) == 0 {
		return nil, false
	}
	if !strings.EqualFold(strings.TrimSpace(stringArg(state, "tool")), strings.TrimSpace(toolName)) {
		return nil, false
	}
	isTarget, hasData, data := einoTool.GetResumeContext[map[string]any](ctx)
	if !isTarget {
		return nil, false
	}
	if hasData {
		decision := strings.ToLower(strings.TrimSpace(stringArg(data, "decision")))
		if decision != "" && decision != "approve" {
			return nil, false
		}
	}
	rawArgs, ok := state["arguments"]
	if !ok {
		return nil, false
	}
	return mapAnyFromValue(rawArgs), true
}

func (r *einoTurnRuntime) requestApprovalInterrupt(
	ctx context.Context,
	kind ApprovalKind,
	assistantMessage string,
	summary string,
	payload any,
	agent *AgentDecision,
	plan *AgentPlan,
	toolName string,
	args map[string]any,
) (string, error) {
	info := map[string]any{
		"kind":             string(kind),
		"summary":          strings.TrimSpace(summary),
		"payload":          payload,
		"assistantMessage": strings.TrimSpace(assistantMessage),
		"agent":            cloneAgentDecision(agent),
		"plan":             cloneAgentPlan(plan),
	}
	state := map[string]any{
		"tool":      strings.TrimSpace(toolName),
		"arguments": cloneMapAny(args),
	}
	return "", einoTool.StatefulInterrupt(ctx, info, state)
}

func (r *einoTurnRuntime) runApprovedToolByLegacyApprove(ctx context.Context, toolName string, args map[string]any) (string, error) {
	if r == nil || r.service == nil || r.service.approvals == nil {
		return "", errors.New("approval runtime not initialized")
	}
	approvalID := newApprovalID()
	r.service.approvals.put(r.conversationID, approvalID, pendingToolCall{
		ThreadID:  strings.TrimSpace(r.req.ThreadID),
		Name:      strings.TrimSpace(toolName),
		Arguments: cloneMapAny(args),
	})
	resp, err := r.service.Approve(ctx, r.internalApproveRequest(approvalID))
	if err != nil {
		return "", err
	}
	return r.directReturn(ctx, resp)
}

func (r *einoTurnRuntime) internalApproveRequest(approvalID string) ApproveRequest {
	return ApproveRequest{
		ThreadID:       strings.TrimSpace(r.req.ThreadID),
		ConversationID: strings.TrimSpace(r.conversationID),
		ApprovalID:     strings.TrimSpace(approvalID),
		Decision:       "approve",
	}
}

func sameWorkingContext(a *WorkingContext, b *WorkingContext) bool {
	a = cloneWorkingContext(a)
	b = cloneWorkingContext(b)
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.DatasourceID == b.DatasourceID &&
			a.DatasourceType == b.DatasourceType &&
			a.Database == b.Database &&
			a.Entity == b.Entity
	}
}

func hasEstablishedWorkingTarget(req TurnRequest) bool {
	working := cloneWorkingContext(req.WorkingContext)
	if working == nil {
		return false
	}
	focus := workingContextFromPageContext(req.PageContext)
	if focus == nil {
		return working.DatasourceID != "" || working.Database != "" || working.Entity != ""
	}
	if strings.TrimSpace(working.Entity) != "" {
		return !strings.EqualFold(strings.TrimSpace(working.Entity), strings.TrimSpace(focus.Entity))
	}
	if strings.TrimSpace(working.DatasourceID) != "" && strings.TrimSpace(working.DatasourceID) != strings.TrimSpace(focus.DatasourceID) {
		return true
	}
	if strings.TrimSpace(working.Database) != "" && strings.TrimSpace(working.Database) != strings.TrimSpace(focus.Database) {
		return true
	}
	return false
}

func (r *einoTurnRuntime) rememberIntent(next *TurnIntent) {
	if r == nil {
		return
	}
	if intent := cloneTurnIntent(next); intent != nil {
		r.req.Intent = intent
	}
}

func (r *einoTurnRuntime) rememberWorkingContext(ctx context.Context, next *WorkingContext) {
	if r == nil || r.service == nil {
		return
	}
	next = mergeWorkingContext(next, r.req.WorkingContext)
	next = r.service.fillWorkingContextDatasourceType(ctx, next)
	if next == nil {
		return
	}
	if sameWorkingContext(next, r.req.WorkingContext) {
		r.req.WorkingContext = next
		return
	}
	r.req.WorkingContext = next
	r.service.appendThreadEvent(r.req.ThreadID, newThreadEvent("working_context_updated", map[string]any{
		"datasourceId":   next.DatasourceID,
		"datasourceType": next.DatasourceType,
		"database":       next.Database,
		"entity":         next.Entity,
		"source":         firstNonEmpty(next.Source, "discovered"),
		"toolName":       next.ToolName,
		"confidence":     next.Confidence,
	}))
}

func (r *einoTurnRuntime) refreshWorkingContextFromThread(ctx context.Context) {
	if r == nil || r.service == nil || r.service.threadStore == nil {
		return
	}
	threadID := strings.TrimSpace(r.req.ThreadID)
	if threadID == "" {
		return
	}
	events, err := r.service.threadStore.LoadEvents(threadID)
	if err != nil || len(events) == 0 {
		return
	}
	if sticky := collectWorkingContext(events); sticky != nil {
		r.req.WorkingContext = r.service.fillWorkingContextDatasourceType(ctx, mergeWorkingContext(r.req.WorkingContext, sticky))
	}
}

func (r *einoTurnRuntime) noteDiscoveryAttempt(datasourceID, pattern string, resultCount int) {
	if r == nil {
		return
	}
	datasourceID = strings.TrimSpace(datasourceID)
	pattern = strings.TrimSpace(pattern)
	focusID := strings.TrimSpace(r.req.PageContext.CurrentDatasourceID)
	if datasourceID == "" || focusID == "" || datasourceID != focusID || pattern == "" {
		return
	}
	if resultCount == 0 {
		r.focusDiscoveryMisses++
		return
	}
	r.focusDiscoveryMisses = 0
}

func (r *einoTurnRuntime) shouldReplanFocusExecute(execReq executeStatementArgs) (string, bool) {
	if r == nil {
		return "", false
	}
	focusID := strings.TrimSpace(r.req.PageContext.CurrentDatasourceID)
	if focusID == "" || strings.TrimSpace(execReq.DatasourceID) != focusID {
		return "", false
	}
	if turnIntentAvoidsCurrentFocus(r.req) {
		if r.locale == uiLocaleZH {
			return "用户已经明确说明目标不在当前页面 datasource。不要继续在当前 datasource 上执行；请扩展 discovery 到其他 datasource/database/entity，或在候选仍不唯一时先确认。", true
		}
		return "The user already said the target is not in the current page datasource. Do not execute on the current datasource; expand discovery to other datasource/database/entity candidates or ask for clarification if multiple targets remain.", true
	}
	if turnIntentPrefersCurrentFocus(r.req) {
		return "", false
	}
	if wc := cloneWorkingContext(r.req.WorkingContext); wc != nil && strings.TrimSpace(wc.DatasourceID) != "" && strings.TrimSpace(wc.DatasourceID) != focusID {
		if r.locale == uiLocaleZH {
			return "已经发现了比当前页面更匹配的 working context。不要继续在当前 datasource 上执行；请沿着已发现的目标继续 describe/list，或在仍有歧义时先确认。", true
		}
		return "A better working context has already been discovered. Do not execute on the current datasource; continue discovery on the discovered target or ask for clarification if ambiguity remains.", true
	}
	if r.focusDiscoveryMisses < 1 {
		return "", false
	}
	if r.locale == uiLocaleZH {
		return "当前页面 datasource 已连续多次探索未命中目标。不要继续在当前 datasource 上执行；请先扩展 discovery（例如 list_datasources、get_datasource、list_entities、describe_entity），定位真实目标后再执行。", true
	}
	return "The current page datasource has already missed the target in repeated discovery. Do not execute on the current datasource yet; expand discovery first (for example list_datasources, get_datasource, list_entities, describe_entity) and execute only after the real target is identified.", true
}

func (r *einoTurnRuntime) toolListDatasources(ctx context.Context, _ map[string]any) (string, error) {
	items, err := r.service.tools.ListDatasources(ctx)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	return marshalToolResult(items)
}

func (r *einoTurnRuntime) toolGetDatasource(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId", "id")
	if id == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId is required"})
	}
	item, err := r.service.tools.GetDatasource(ctx, id)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.rememberWorkingContext(ctx, &WorkingContext{
		DatasourceID:   strings.TrimSpace(item.ID),
		DatasourceType: strings.TrimSpace(item.Type),
		Database:       strings.TrimSpace(item.Database),
		Source:         "discovered",
		ToolName:       "get_datasource",
		Confidence:     0.85,
	})
	return marshalToolResult(item)
}

func (r *einoTurnRuntime) toolListDatabases(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	if id == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId is required"})
	}
	items, err := r.service.tools.ListDatabases(ctx, id, stringArg(cleanArgs, "pattern"))
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.noteDiscoveryAttempt(id, stringArg(cleanArgs, "pattern"), len(items))
	if database := inferWorkingTargetFromListResult(stringArg(cleanArgs, "pattern"), items); database != "" {
		r.rememberWorkingContext(ctx, &WorkingContext{
			DatasourceID: strings.TrimSpace(id),
			Database:     database,
			Source:       "discovered",
			ToolName:     "list_databases",
			Confidence:   0.78,
		})
	}
	return marshalToolResult(items)
}

func (r *einoTurnRuntime) toolListEntities(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	if id == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId is required"})
	}
	items, err := r.service.tools.ListEntities(ctx, id, stringArg(cleanArgs, "pattern"), stringArg(cleanArgs, "database"))
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.noteDiscoveryAttempt(id, stringArg(cleanArgs, "pattern"), len(items))
	if entity := inferWorkingTargetFromListResult(stringArg(cleanArgs, "pattern"), items); entity != "" {
		r.rememberWorkingContext(ctx, &WorkingContext{
			DatasourceID: strings.TrimSpace(id),
			Database:     strings.TrimSpace(stringArg(cleanArgs, "database")),
			Entity:       entity,
			Source:       "discovered",
			ToolName:     "list_entities",
			Confidence:   0.84,
		})
	}
	return marshalToolResult(items)
}

func inferWorkingTargetFromListResult(pattern string, items []string) string {
	pattern = strings.TrimSpace(pattern)
	if len(items) == 0 {
		return ""
	}
	if pattern == "" {
		if len(items) == 1 {
			return strings.TrimSpace(items[0])
		}
		return ""
	}

	normalizedPattern := strings.ToLower(pattern)
	exact := ""
	for _, item := range items {
		candidate := strings.TrimSpace(item)
		if candidate == "" {
			continue
		}
		if strings.EqualFold(candidate, pattern) {
			if exact != "" && !strings.EqualFold(exact, candidate) {
				return ""
			}
			exact = candidate
		}
	}
	if exact != "" {
		return exact
	}
	if len(items) != 1 {
		return ""
	}
	candidate := strings.TrimSpace(items[0])
	if candidate == "" {
		return ""
	}
	normalizedCandidate := strings.ToLower(candidate)
	if strings.Contains(normalizedCandidate, normalizedPattern) || strings.Contains(normalizedPattern, normalizedCandidate) {
		return candidate
	}
	return ""
}

func (r *einoTurnRuntime) toolDescribeEntity(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	name := stringArg(cleanArgs, "name", "entity")
	if id == "" || name == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId and name are required"})
	}
	out, err := r.service.tools.DescribeEntity(ctx, id, name, stringArg(cleanArgs, "database"))
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.rememberWorkingContext(ctx, &WorkingContext{
		DatasourceID: strings.TrimSpace(id),
		Database:     strings.TrimSpace(stringArg(cleanArgs, "database")),
		Entity:       strings.TrimSpace(name),
		Source:       "discovered",
		ToolName:     "describe_entity",
		Confidence:   0.92,
	})
	return marshalToolResult(out)
}

func (r *einoTurnRuntime) toolSearchKnowledge(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	result, err := r.service.searchKnowledge(ctx, r.req, cleanArgs)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolMemorySave(_ context.Context, args map[string]any) (string, error) {
	if r == nil || r.service == nil || r.service.memoryStore == nil {
		return marshalToolResult(map[string]any{"error": "memory store is not initialized"})
	}
	if r.memorySaveCount >= 1 {
		return marshalToolResult(map[string]any{"error": "memory_save may be called at most once per turn"})
	}
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	input := memorySaveInputFromToolArgs(cleanArgs)
	result, err := r.service.memoryStore.SavePattern(input)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.memorySaveCount++
	confidence := input.Confidence
	savedProblem := strings.TrimSpace(input.Problem)
	if result.SavedPattern != nil {
		confidence = result.SavedPattern.Confidence
		savedProblem = strings.TrimSpace(result.SavedPattern.Problem)
	}
	r.service.appendThreadEvent(r.req.ThreadID, newThreadEvent("memory_saved", map[string]any{
		"version":          strings.TrimSpace(result.Version),
		"problem":          savedProblem,
		"confidence":       confidence,
		"evidenceCount":    len(input.EvidenceEventIDs),
		"replaceHintCount": len(input.ReplaceHints),
	}))
	return marshalToolResult(map[string]any{
		"saved":       true,
		"version":     result.Version,
		"problem":     savedProblem,
		"archivedIds": result.ArchivedIDs,
	})
}

func (r *einoTurnRuntime) toolWebSearch(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	r.service.log(ctx, "tool_web_search_start", map[string]any{
		"engine":     strings.TrimSpace(stringArg(cleanArgs, "engine")),
		"query":      strings.TrimSpace(stringArg(cleanArgs, "query", "q")),
		"maxResults": intArg(cleanArgs, "maxResults", 5),
	})
	result, err := r.service.webSearch(ctx, cleanArgs)
	if err != nil {
		r.service.log(ctx, "tool_web_search", map[string]any{
			"error": err.Error(),
		})
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.service.log(ctx, "tool_web_search", map[string]any{
		"error":    "",
		"engine":   result.Engine,
		"query":    result.Query,
		"results":  len(result.Results),
		"warnings": len(result.Warnings),
	})
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolExplainStatement(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	r.refreshWorkingContextFromThread(ctx)
	req, err := explainStatementArgsFromToolArgs(r.req, cleanArgs)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	result, err := r.service.tools.ExplainStatement(ctx, req.DatasourceID, req.Statement, req.Database)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	r.rememberWorkingContext(ctx, &WorkingContext{
		DatasourceID: strings.TrimSpace(req.DatasourceID),
		Database:     strings.TrimSpace(req.Database),
		Source:       "discovered",
		ToolName:     "explain_statement",
		Confidence:   0.88,
	})
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolExecuteStatement(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	if resumedArgs, ok := r.resumeArgsForTool(ctx, "execute_statement"); ok {
		return r.runApprovedExecuteStatementToolResult(ctx, resumedArgs)
	}
	r.refreshWorkingContextFromThread(ctx)
	execReq, err := executeStatementArgsFromToolArgs(r.req, cleanArgs)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	if replanReason, shouldReplan := r.shouldReplanFocusExecute(execReq); shouldReplan {
		r.service.log(ctx, "execute_statement_replan_required", map[string]any{
			"datasourceId":         execReq.DatasourceID,
			"database":             execReq.Database,
			"focusDiscoveryMisses": r.focusDiscoveryMisses,
			"workingDatasourceId": func() string {
				if r.req.WorkingContext == nil {
					return ""
				}
				return strings.TrimSpace(r.req.WorkingContext.DatasourceID)
			}(),
		})
		return marshalToolResult(map[string]any{
			"error":          replanReason,
			"replanRequired": true,
			"suggestedTools": []string{"list_datasources", "get_datasource", "list_entities", "describe_entity"},
		})
	}
	if userRequestedResultAnalysis(lastUserText(r.req.Messages)) {
		if stored, ok := r.service.analysis.GetResult(r.conversationID); ok && len(stored.Rows) > 0 {
			approvalID := newApprovalID()
			storedArgs := map[string]any{
				"aiConfigId": strings.TrimSpace(r.req.AIConfigID),
				"question":   lastUserText(r.req.Messages),
				"lang":       string(r.locale),
			}
			r.service.approvals.put(r.conversationID, approvalID, pendingToolCall{
				ThreadID:  strings.TrimSpace(r.req.ThreadID),
				Name:      "analyze_result",
				Arguments: storedArgs,
			})
			msg := defaultAnalyzeResultAssistantMessage(r.locale, stored)
			r.service.log(ctx, "approval_pending", map[string]any{
				"kind":       string(ApprovalAnalyzeResult),
				"approvalId": approvalID,
				"rowCount":   stored.RowCount,
				"rows":       len(stored.Rows),
				"bytes":      stored.ApproxBytes,
				"truncated":  stored.RowsTruncated,
				"reason":     "user_requested_result_analysis",
			})
			return r.directReturn(ctx, TurnResponse{
				AssistantMessage: msg,
				Approval: &Approval{
					ID:      approvalID,
					Kind:    ApprovalAnalyzeResult,
					Summary: summarizeAnalyzeResult(r.locale, stored),
					Payload: sanitizeAnalyzeResultPayload(stored),
				},
				Agent: agentMeta,
				Plan:  planMeta,
			})
		}
	}

	ds, dsErr := r.service.tools.GetDatasource(ctx, execReq.DatasourceID)
	if dsErr == nil && strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
		if normalized, changed, normalizeErr := normalizeMongoStatementForTool(execReq.Statement); normalizeErr == nil {
			if changed {
				r.service.log(ctx, "mongo_statement_normalized", map[string]any{
					"mode":          "turn_eino",
					"datasourceId":  execReq.DatasourceID,
					"database":      execReq.Database,
					"statementLen":  len(execReq.Statement),
					"normalizedLen": len(normalized),
				})
				execReq.Statement = normalized
			}
		} else {
			r.service.log(ctx, "execute_statement_invalid", map[string]any{
				"datasourceId": execReq.DatasourceID,
				"database":     execReq.Database,
				"error":        normalizeErr.Error(),
				"statementLen": len(execReq.Statement),
				"statement":    previewForLog(execReq.Statement),
			})
			return marshalToolResult(map[string]any{"error": normalizeErr.Error()})
		}
		if validateErr := validateMongoStatementForTool(execReq.Statement); validateErr != nil {
			r.service.log(ctx, "execute_statement_invalid", map[string]any{
				"datasourceId": execReq.DatasourceID,
				"database":     execReq.Database,
				"error":        validateErr.Error(),
				"statementLen": len(execReq.Statement),
				"statement":    previewForLog(execReq.Statement),
			})
			return marshalToolResult(map[string]any{"error": validateErr.Error()})
		}
	}

	msg := defaultExecuteAssistantMessage(r.locale, ds, execReq)

	// Datasource lookup failed — no trust level available, fall back to
	// auto-execution to match legacy behavior.
	if dsErr != nil {
		msg = appendRiskAutoExecuteDetails(r.locale, msg, ds, execReq)
		return r.executeAndReturn(ctx, msg, ds, execReq, nil, agentMeta, planMeta)
	}

	assessment, explain := r.service.assessStatement(ctx, ds, execReq.Statement)
	trust := datasourceSummaryToDataSource(ds).TrustLevel()
	if riskengine.DecideGate(trust, assessment) == riskengine.GateAutoRun {
		msg = appendRiskAutoExecuteDetails(r.locale, msg, ds, execReq)
		meta := map[string]any{
			"riskLevel":    string(assessment.Level),
			"trustLevel":   string(trust),
			"autoApproved": true,
		}
		return r.executeAndReturn(ctx, msg, ds, execReq, meta, agentMeta, planMeta)
	}

	msg = appendRiskApprovalDetails(r.locale, msg, ds, execReq, assessment, explain)

	storedArgs := map[string]any{
		"datasourceId":  execReq.DatasourceID,
		"database":      execReq.Database,
		"statement":     execReq.Statement,
		"pagingToken":   execReq.PagingToken,
		"pageSize":      execReq.PageSize,
		"lang":          string(r.locale),
		"aiConfigId":    strings.TrimSpace(r.req.AIConfigID),
		"question":      r.userText,
		"repairAttempt": 0,
	}
	return r.requestApprovalInterrupt(
		ctx,
		ApprovalExecuteStatement,
		msg,
		summarizeExecuteStatement(r.locale, ds, assessment, explain),
		sanitizeExecuteStatementPayload(ds, execReq, assessment, explain),
		agentMeta,
		planMeta,
		"execute_statement",
		storedArgs,
	)
}

// executeAndReturn runs the statement execution, logs, builds the result, and returns.
// extraLogFields are merged into the "tool_execute_statement_start" log entry.
func (r *einoTurnRuntime) executeAndReturn(
	ctx context.Context, msg string, ds DatasourceSummary, execReq executeStatementArgs,
	extraLogFields map[string]any, agentMeta *AgentDecision, planMeta *AgentPlan,
) (string, error) {
	logFields := map[string]any{
		"mode":         "turn_eino",
		"datasourceId": execReq.DatasourceID,
		"database":     execReq.Database,
		"statementLen": len(execReq.Statement),
	}
	for k, v := range extraLogFields {
		logFields[k] = v
	}
	r.service.log(ctx, "tool_execute_statement_start", logFields)
	execStartAt := time.Now()
	result, execErr := r.service.tools.ExecuteStatement(ctx, execReq.DatasourceID, execReq.Statement, execReq.Database, execReq.PagingToken, execReq.PageSize, true)
	if execErr != nil {
		r.service.log(ctx, "tool_execute_statement", map[string]any{
			"durationMs":   time.Since(execStartAt).Milliseconds(),
			"error":        execErr.Error(),
			"datasourceId": execReq.DatasourceID,
			"database":     execReq.Database,
			"statementLen": len(execReq.Statement),
		})
		return marshalToolResult(executeStatementErrorPayload(ds, execReq, execErr))
	}
	r.service.log(ctx, "tool_execute_statement", map[string]any{
		"durationMs":   time.Since(execStartAt).Milliseconds(),
		"error":        "",
		"datasourceId": execReq.DatasourceID,
		"database":     execReq.Database,
		"statementLen": len(execReq.Statement),
		"rowCount":     result.RowCount,
		"hasMore":      result.HasMore,
		"elapsedMs":    result.ElapsedMs,
	})
	r.rememberWorkingContext(ctx, &WorkingContext{
		DatasourceID: strings.TrimSpace(execReq.DatasourceID),
		Database:     strings.TrimSpace(execReq.Database),
		Source:       "discovered",
		ToolName:     "execute_statement",
		Confidence:   0.9,
	})
	resultMsg := formatExecuteResultMarkdown(r.locale, ds, execReq, result)
	if strings.TrimSpace(msg) != "" {
		msg = strings.TrimSpace(msg) + "\n\n" + resultMsg
	} else {
		msg = resultMsg
	}
	effectStatement := execReq.Statement
	if strings.EqualFold(strings.TrimSpace(ds.Type), "mongodb") {
		if formatted := formatMongoStatementForHuman(effectStatement); formatted != effectStatement {
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
	r.service.analysis.PutResult(r.conversationID, effect)
	return r.directReturn(ctx, TurnResponse{
		AssistantMessage: msg,
		Effects: Effects{
			ConsoleResult: &effect,
		},
		Agent: agentMeta,
		Plan:  planMeta,
	})
}

func (r *einoTurnRuntime) runApprovedExecuteStatementToolResult(ctx context.Context, args map[string]any) (string, error) {
	if r == nil || r.service == nil || r.service.approvals == nil {
		return "", errors.New("approval runtime not initialized")
	}
	approvalID := newApprovalID()
	r.service.approvals.put(r.conversationID, approvalID, pendingToolCall{
		ThreadID:  strings.TrimSpace(r.req.ThreadID),
		Name:      "execute_statement",
		Arguments: cloneMapAny(args),
	})
	resp, err := r.service.Approve(ctx, r.internalApproveRequest(approvalID))
	if err != nil {
		return "", err
	}
	if resp.Approval != nil {
		return r.directReturn(ctx, resp)
	}
	r.rememberWorkingContext(ctx, &WorkingContext{
		DatasourceID: strings.TrimSpace(stringArg(args, "datasourceId", "id")),
		Database:     strings.TrimSpace(stringArg(args, "database")),
		Source:       "discovered",
		ToolName:     "execute_statement",
		Confidence:   0.9,
	})
	r.rememberEffects(resp.Effects)

	payload := map[string]any{
		"assistantMessage": strings.TrimSpace(resp.AssistantMessage),
	}
	if resp.Effects.ConsoleResult != nil {
		payload["consoleResult"] = compactConsoleResultForTool(resp.Effects.ConsoleResult)
		if pagingToken := strings.TrimSpace(stringArg(args, "pagingToken", "pageToken", "nextToken")); pagingToken != "" {
			payload["pagingToken"] = pagingToken
		}
		if pageSize := intArg(args, "pageSize", 100); pageSize > 0 {
			payload["pageSize"] = pageSize
		}
	}
	if resp.Effects.Visualization != nil {
		payload["visualization"] = resp.Effects.Visualization
	}
	if resp.Effects.DatasourcesChanged {
		payload["datasourcesChanged"] = true
	}
	if strings.TrimSpace(resp.Effects.NavigateTo) != "" {
		payload["navigateTo"] = strings.TrimSpace(resp.Effects.NavigateTo)
	}
	return marshalToolResult(payload)
}

func compactConsoleResultForTool(effect *ConsoleResultEffect) map[string]any {
	if effect == nil {
		return nil
	}

	payload := map[string]any{
		"datasourceId": strings.TrimSpace(effect.DatasourceID),
		"statement":    strings.TrimSpace(effect.Statement),
		"result": map[string]any{
			"columns":   append([]string(nil), effect.Result.Columns...),
			"rowCount":  effect.Result.RowCount,
			"hasMore":   effect.Result.HasMore,
			"elapsedMs": effect.Result.ElapsedMs,
		},
	}
	if strings.TrimSpace(effect.DatasourceType) != "" {
		payload["datasourceType"] = strings.TrimSpace(effect.DatasourceType)
	}
	if strings.TrimSpace(effect.Database) != "" {
		payload["database"] = strings.TrimSpace(effect.Database)
	}
	if count := len(effect.Result.Rows); count > 0 {
		payload["result"].(map[string]any)["rowsOmitted"] = count
	}
	if strings.TrimSpace(effect.Result.NextToken) != "" {
		payload["result"].(map[string]any)["nextTokenAvailable"] = true
	}
	if strings.TrimSpace(effect.Result.PrevToken) != "" {
		payload["result"].(map[string]any)["prevTokenAvailable"] = true
	}
	return payload
}

func (r *einoTurnRuntime) toolAnalyzeResult(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	if resumedArgs, ok := r.resumeArgsForTool(ctx, "analyze_result"); ok {
		return r.runApprovedToolByLegacyApprove(ctx, "analyze_result", resumedArgs)
	}
	stored, ok := r.service.analysis.GetResult(r.conversationID)
	if !ok || len(stored.Rows) == 0 {
		return marshalToolResult(map[string]any{"error": "no recent AI console result available"})
	}

	storedArgs := map[string]any{
		"aiConfigId": strings.TrimSpace(r.req.AIConfigID),
		"question":   analysisQuestionFromArgs(r.req, cleanArgs),
		"lang":       string(r.locale),
	}
	return r.requestApprovalInterrupt(
		ctx,
		ApprovalAnalyzeResult,
		defaultAnalyzeResultAssistantMessage(r.locale, stored),
		summarizeAnalyzeResult(r.locale, stored),
		sanitizeAnalyzeResultPayload(stored),
		agentMeta,
		planMeta,
		"analyze_result",
		storedArgs,
	)
}

func (r *einoTurnRuntime) toolCreateVisualization(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	if resumedArgs, ok := r.resumeArgsForTool(ctx, "create_visualization"); ok {
		return r.runApprovedToolByLegacyApprove(ctx, "create_visualization", resumedArgs)
	}
	stored, ok := r.service.analysis.GetResult(r.conversationID)
	if !ok || len(stored.Rows) == 0 {
		return marshalToolResult(map[string]any{"error": "no recent AI console result available"})
	}

	storedArgs := map[string]any{
		"aiConfigId": strings.TrimSpace(r.req.AIConfigID),
		"question":   analysisQuestionFromArgs(r.req, cleanArgs),
		"lang":       string(r.locale),
	}
	return r.requestApprovalInterrupt(
		ctx,
		ApprovalCreateVisualization,
		defaultCreateVisualizationAssistantMessage(r.locale, stored),
		summarizeCreateVisualization(r.locale, stored),
		sanitizeCreateVisualizationPayload(stored),
		agentMeta,
		planMeta,
		"create_visualization",
		storedArgs,
	)
}

func (r *einoTurnRuntime) toolGetRedisCommandDocs(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	command := stringArg(cleanArgs, "command")
	if id == "" || command == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId and command are required"})
	}
	result, err := r.service.tools.GetRedisCommandDocs(ctx, id, command)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolGetSchemaKnowledge(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	if id == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId is required"})
	}
	result, err := r.service.tools.GetSchemaKnowledge(ctx, id, stringArg(cleanArgs, "entity"), stringArg(cleanArgs, "database"))
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolGetERKnowledge(ctx context.Context, args map[string]any) (string, error) {
	_, _, cleanArgs := extractModelMetadataFromArgs(args)
	id := stringArg(cleanArgs, "datasourceId")
	if id == "" {
		return marshalToolResult(map[string]any{"error": "datasourceId is required"})
	}
	result, err := r.service.tools.GetERKnowledge(ctx, id, stringArg(cleanArgs, "database"))
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	return marshalToolResult(result)
}

func (r *einoTurnRuntime) toolCreateDatasource(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	if resumedArgs, ok := r.resumeArgsForTool(ctx, "create_datasource"); ok {
		return r.runApprovedToolByLegacyApprove(ctx, "create_datasource", resumedArgs)
	}
	input, err := datasourceCreateInputFromArgs(cleanArgs)
	if err != nil {
		return r.directReturn(ctx, TurnResponse{AssistantMessage: err.Error(), Agent: agentMeta, Plan: planMeta})
	}
	return r.requestApprovalInterrupt(
		ctx,
		ApprovalCreateDatasource,
		"",
		summarizeCreateDatasource(input),
		sanitizeCreateDatasourceInput(input),
		agentMeta,
		planMeta,
		"create_datasource",
		cleanArgs,
	)
}

func (r *einoTurnRuntime) toolDeleteDatasource(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	if resumedArgs, ok := r.resumeArgsForTool(ctx, "delete_datasource"); ok {
		return r.runApprovedToolByLegacyApprove(ctx, "delete_datasource", resumedArgs)
	}
	target, err := datasourceDeleteTargetFromArgs(cleanArgs)
	if err != nil {
		return r.directReturn(ctx, TurnResponse{AssistantMessage: err.Error(), Agent: agentMeta, Plan: planMeta})
	}
	return r.requestApprovalInterrupt(
		ctx,
		ApprovalDeleteDatasource,
		"",
		summarizeDeleteDatasource(target),
		target,
		agentMeta,
		planMeta,
		"delete_datasource",
		cleanArgs,
	)
}

func (r *einoTurnRuntime) toolNavigateDatasource(ctx context.Context, args map[string]any) (string, error) {
	agentMeta, planMeta, cleanArgs := extractModelMetadataFromArgs(args)
	target, err := navigationTargetFromArgs(cleanArgs)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	path, err := resolveNavigationPath(ctx, r.service.tools, target)
	if err != nil {
		return marshalToolResult(map[string]any{"error": err.Error()})
	}
	msg := fmt.Sprintf("OK, opening %s.", path)
	if r.locale == uiLocaleZH {
		msg = fmt.Sprintf("好的，正在打开 %s。", path)
	}
	resp := TurnResponse{
		AssistantMessage: msg,
		Effects:          Effects{NavigateTo: path},
		Agent:            agentMeta,
		Plan:             planMeta,
	}
	return r.directReturn(ctx, resp)
}

func (r *einoTurnRuntime) directReturn(ctx context.Context, resp TurnResponse) (string, error) {
	if err := einoReact.SetReturnDirectly(ctx); err != nil {
		return "", err
	}
	return encodeEinoTurnEnvelope(resp)
}

func marshalToolResult(value any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	if str, ok := value.(string); ok {
		return str, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func formatToolResultForModel(name string, payload string) string {
	trimmed := strings.TrimSpace(payload)
	if strings.HasPrefix(trimmed, "[tool_result]") {
		return trimmed
	}
	if trimmed == "" {
		trimmed = "{}"
	}
	return fmt.Sprintf("[tool_result] %s\n%s", strings.TrimSpace(name), trimmed)
}

func encodeEinoTurnEnvelope(resp TurnResponse) (string, error) {
	payload, err := json.Marshal(einoTurnEnvelope{Marker: einoEnvelopeMarker, Response: resp})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func decodeEinoTurnEnvelope(content string) (TurnResponse, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return TurnResponse{}, false
	}
	var env einoTurnEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		return TurnResponse{}, false
	}
	if env.Marker != einoEnvelopeMarker {
		return TurnResponse{}, false
	}
	return env.Response, true
}

func extractMetadataFromMessage(msg *einoSchema.Message) (*AgentDecision, *AgentPlan) {
	if msg == nil || len(msg.Extra) == 0 {
		return nil, nil
	}
	agent := decodeAgentFromAny(msg.Extra[einoMessageAgentKey])
	plan := decodePlanFromAny(msg.Extra[einoMessagePlanKey])
	return agent, plan
}

func extractModelMetadataFromArgs(args map[string]any) (*AgentDecision, *AgentPlan, map[string]any) {
	if len(args) == 0 {
		return nil, nil, map[string]any{}
	}
	clean := make(map[string]any, len(args))
	var agent *AgentDecision
	var plan *AgentPlan
	for key, value := range args {
		switch key {
		case einoMetaAgentKey:
			agent = decodeAgentFromAny(value)
		case einoMetaPlanKey:
			plan = decodePlanFromAny(value)
		case einoMetaIntentKey:
			continue
		default:
			clean[key] = value
		}
	}
	return agent, plan, clean
}

func decodeAgentFromAny(value any) *AgentDecision {
	if value == nil {
		return nil
	}
	if typed, ok := value.(*AgentDecision); ok && typed != nil {
		return cloneAgentDecision(typed)
	}
	if typed, ok := value.(AgentDecision); ok {
		return cloneAgentDecision(&typed)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out AgentDecision
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return cloneAgentDecision(&out)
}

func decodePlanFromAny(value any) *AgentPlan {
	if value == nil {
		return nil
	}
	if typed, ok := value.(*AgentPlan); ok && typed != nil {
		return cloneAgentPlan(typed)
	}
	if typed, ok := value.(AgentPlan); ok {
		return cloneAgentPlan(&typed)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out AgentPlan
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return cloneAgentPlan(&out)
}

func mapAnyFromValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return cloneMapAny(typed)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

func decodeApprovalInterruptInfo(value any) (ApprovalKind, string, any, string, *AgentDecision, *AgentPlan, bool) {
	data := mapAnyFromValue(value)
	if len(data) == 0 {
		return "", "", nil, "", nil, nil, false
	}
	kind := ApprovalKind(strings.TrimSpace(stringArg(data, "kind")))
	if kind == "" {
		return "", "", nil, "", nil, nil, false
	}
	summary := strings.TrimSpace(stringArg(data, "summary"))
	assistantMessage := strings.TrimSpace(stringArg(data, "assistantMessage", "assistant_message"))
	payload, _ := data["payload"]
	agent := decodeAgentFromAny(data["agent"])
	plan := decodePlanFromAny(data["plan"])
	return kind, summary, payload, assistantMessage, agent, plan, true
}

func newEinoCheckpointID(conversationID string) string {
	trimmed := strings.TrimSpace(conversationID)
	if trimmed == "" {
		trimmed = "conversation"
	}
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, trimmed)
	return fmt.Sprintf("eino_cp_%s_%x", safe, time.Now().UTC().UnixNano())
}

func (s *Service) approveWithEinoCheckpoint(ctx context.Context, req ApproveRequest, decision string) (TurnResponse, bool, error) {
	if s == nil || s.einoApprovals == nil {
		return TurnResponse{}, false, nil
	}
	conversationID := strings.TrimSpace(req.ConversationID)
	approvalID := strings.TrimSpace(req.ApprovalID)
	item, ok := s.einoApprovals.get(conversationID, approvalID)
	if !ok {
		return TurnResponse{}, false, nil
	}

	if decision == "reject" {
		s.einoApprovals.delete(conversationID, approvalID)
		if s.einoCheckpoints != nil {
			s.einoCheckpoints.Delete(item.CheckpointID)
		}
		if item.Locale == uiLocaleZH {
			return TurnResponse{AssistantMessage: "好的，已取消。", Effects: cloneEffects(item.Effects)}, true, nil
		}
		return TurnResponse{AssistantMessage: "OK, cancelled.", Effects: cloneEffects(item.Effects)}, true, nil
	}

	resp, err := s.resumeEinoFromApproval(ctx, item)
	if err != nil {
		return TurnResponse{}, true, err
	}
	s.einoApprovals.delete(conversationID, approvalID)
	if resp.Approval == nil && s.einoCheckpoints != nil {
		s.einoCheckpoints.Delete(item.CheckpointID)
	}
	return resp, true, nil
}

func (s *Service) resumeEinoFromApproval(ctx context.Context, item einoApprovalResumeItem) (TurnResponse, error) {
	checkpointID := strings.TrimSpace(item.CheckpointID)
	interruptID := strings.TrimSpace(item.InterruptID)
	if checkpointID == "" || interruptID == "" {
		return TurnResponse{}, errors.New("approval checkpoint not found")
	}
	return s.turnEinoWithOptions(
		ctx,
		item.Request,
		false,
		nil,
		checkpointID,
		interruptID,
		map[string]any{"decision": "approve"},
		true,
		item.Effects,
		nil,
	)
}

type einoApprovalResumeItem struct {
	CheckpointID string
	InterruptID  string
	Request      TurnRequest
	Locale       uiLocale
	Kind         ApprovalKind
	Effects      Effects
	CreatedAt    time.Time
}

type einoApprovalResumeStore struct {
	mu    sync.Mutex
	items map[string]map[string]einoApprovalResumeItem
}

func newEinoApprovalResumeStore() *einoApprovalResumeStore {
	return &einoApprovalResumeStore{items: make(map[string]map[string]einoApprovalResumeItem)}
}

func (s *einoApprovalResumeStore) put(conversationID, approvalID string, item einoApprovalResumeItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		byConversation = make(map[string]einoApprovalResumeItem)
		s.items[conversationID] = byConversation
	}
	byConversation[approvalID] = item
}

func (s *einoApprovalResumeStore) get(conversationID, approvalID string) (einoApprovalResumeItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		return einoApprovalResumeItem{}, false
	}
	item, ok := byConversation[approvalID]
	if !ok {
		return einoApprovalResumeItem{}, false
	}
	return item, true
}

func (s *einoApprovalResumeStore) delete(conversationID, approvalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byConversation, ok := s.items[conversationID]
	if !ok {
		return
	}
	delete(byConversation, approvalID)
	if len(byConversation) == 0 {
		delete(s.items, conversationID)
	}
}

type einoCheckpointStore struct {
	mu    sync.Mutex
	items map[string][]byte
}

func newEinoCheckpointStore() *einoCheckpointStore {
	return &einoCheckpointStore{items: make(map[string][]byte)}
}

func (s *einoCheckpointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[checkPointID]
	if !ok {
		return nil, false, nil
	}
	out := make([]byte, len(item))
	copy(out, item)
	return out, true, nil
}

func (s *einoCheckpointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := make([]byte, len(checkPoint))
	copy(data, checkPoint)
	s.items[checkPointID] = data
	return nil
}

func (s *einoCheckpointStore) Delete(checkPointID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, strings.TrimSpace(checkPointID))
}
