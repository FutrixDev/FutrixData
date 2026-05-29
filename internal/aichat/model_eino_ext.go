package aichat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	einoOpenAI "github.com/cloudwego/eino-ext/components/model/openai"
	einoModel "github.com/cloudwego/eino/components/model"
	einoSchema "github.com/cloudwego/eino/schema"
)

var _ Model = (*EinoExtModel)(nil)
var _ StreamingModel = (*EinoExtModel)(nil)

type EinoExtModel struct {
	chatModel einoModel.ToolCallingChatModel
	optsFn    func() []einoModel.Option
	provider  string
	baseURL   string
}

func NewOpenAIEinoExtModel(cfg OpenAICompatibleModelConfig) (*EinoExtModel, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	rawBaseURL := strings.TrimSpace(cfg.BaseURL)
	baseURL, baseURLQuery := normalizeOpenAIBaseURLParts(rawBaseURL)
	modelName := strings.TrimSpace(cfg.Model)
	if apiKey == "" || baseURL == "" || modelName == "" {
		return nil, errors.New("ai provider not configured")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	chatCfg := &einoOpenAI.ChatModelConfig{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Model:      modelName,
		HTTPClient: openAIHTTPClient(timeout, baseURLQuery),
	}
	if usesOpenAIMaxCompletionTokens(modelName) {
		chatCfg.MaxCompletionTokens = &maxTokens
	} else {
		chatCfg.MaxTokens = &maxTokens
	}
	chatCfg.Temperature = openAITemperatureForModel(modelName)

	model, err := einoOpenAI.NewChatModel(context.Background(), chatCfg)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{}
	if strings.Contains(strings.ToLower(rawBaseURL), "openrouter.ai") {
		referer := strings.TrimSpace(cfg.Referer)
		if referer == "" {
			referer = "http://localhost"
		}
		title := strings.TrimSpace(cfg.AppTitle)
		if title == "" {
			title = "FutrixData Platform"
		}
		headers["HTTP-Referer"] = referer
		headers["X-Title"] = title
	}

	optsFn := func() []einoModel.Option {
		if len(headers) == 0 {
			return nil
		}
		return []einoModel.Option{einoOpenAI.WithExtraHeader(headers)}
	}

	return &EinoExtModel{
		chatModel: model,
		optsFn:    optsFn,
		provider:  "openai-compatible",
		baseURL:   rawBaseURL,
	}, nil
}

func (m *EinoExtModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	if m == nil || m.chatModel == nil {
		return "", errors.New("ai provider not configured")
	}
	resp, err := m.chatModel.Generate(ctx, toEinoExtMessages(systemPrompt, messages), m.options()...)
	if err != nil {
		return "", m.wrapError(err)
	}
	if resp == nil {
		return "", errors.New("ai provider returned no response")
	}
	return resp.Content, nil
}

func (m *EinoExtModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	if m == nil || m.chatModel == nil {
		return "", errors.New("ai provider not configured")
	}
	sr, err := m.chatModel.Stream(ctx, toEinoExtMessages(systemPrompt, messages), m.options()...)
	if err != nil {
		return "", m.wrapError(err)
	}
	defer sr.Close()

	var out strings.Builder
	for {
		chunk, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return "", m.wrapError(recvErr)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		out.WriteString(chunk.Content)
		if onDelta != nil {
			onDelta(chunk.Content)
		}
	}
	return out.String(), nil
}

func (m *EinoExtModel) options() []einoModel.Option {
	if m == nil || m.optsFn == nil {
		return nil
	}
	return m.optsFn()
}

func (m *EinoExtModel) wrapError(err error) error {
	if err == nil || m == nil {
		return err
	}
	provider := strings.TrimSpace(m.provider)
	baseURL := strings.TrimSpace(m.baseURL)
	if strings.EqualFold(provider, "anthropic") && baseURL != "" {
		return fmt.Errorf("%s request failed (baseURL=%s, messagesURL=%s): %w", provider, baseURL, anthropicMessagesURL(baseURL), err)
	}
	if strings.EqualFold(provider, "openai-compatible") && baseURL != "" {
		return fmt.Errorf("%s request failed (baseURL=%s, completionsURL=%s): %w", provider, baseURL, openAIChatCompletionsURL(baseURL), err)
	}
	switch {
	case provider == "" && baseURL == "":
		return err
	case provider == "":
		return fmt.Errorf("ai request failed (baseURL=%s): %w", baseURL, err)
	case baseURL == "":
		return fmt.Errorf("%s request failed: %w", provider, err)
	default:
		return fmt.Errorf("%s request failed (baseURL=%s): %w", provider, baseURL, err)
	}
}

func toEinoExtMessages(systemPrompt string, messages []Message) []*einoSchema.Message {
	out := make([]*einoSchema.Message, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		out = append(out, einoSchema.SystemMessage(systemPrompt))
	}
	for _, msg := range messages {
		content := normalizeOpenAICompatibleContent(msg.Content)
		switch strings.ToLower(strings.TrimSpace(msg.Role)) {
		case "system":
			out = append(out, einoSchema.SystemMessage(content))
		case "assistant":
			out = append(out, einoSchema.AssistantMessage(content, nil))
		default:
			out = append(out, einoSchema.UserMessage(content))
		}
	}
	return out
}

func normalizeOpenAICompatibleContent(content string) string {
	// Some OpenAI-compatible gateways reject omitted/null message.content.
	// The upstream SDK omits empty strings due `omitempty`, so we coerce
	// blank content into a whitespace string to keep JSON type as string.
	if strings.TrimSpace(content) == "" {
		return " "
	}
	return content
}

func usesOpenAIMaxCompletionTokens(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return strings.HasPrefix(normalized, "gpt5") || strings.HasPrefix(normalized, "gpt-5")
}

func openAITemperatureForModel(model string) *float32 {
	if usesOpenAIMaxCompletionTokens(model) {
		// GPT-5 class models currently enforce fixed sampling params; omit temperature.
		return nil
	}
	temperature := float32(0.2)
	return &temperature
}

func normalizeOpenAIBaseURL(baseURL string) string {
	normalized, _ := normalizeOpenAIBaseURLParts(baseURL)
	return normalized
}

func normalizeOpenAIBaseURLParts(baseURL string) (string, url.Values) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return normalizeOpenAIBaseURLFallback(trimmed), nil
	}
	query := parsed.Query()
	parsed.RawQuery = ""
	parsed.Path = normalizeOpenAIPath(parsed.Path)
	return parsed.String(), query
}

func normalizeOpenAIBaseURLFallback(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimRight(trimmed, "/")
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed[:len(trimmed)-len("/chat/completions")]
	}
	return trimmed
}

func normalizeOpenAIPath(path string) string {
	trimmedPath := strings.TrimRight(path, "/")
	lowerPath := strings.ToLower(trimmedPath)
	if strings.HasSuffix(lowerPath, "/chat/completions") {
		return trimmedPath[:len(trimmedPath)-len("/chat/completions")]
	}
	return trimmedPath
}

func openAIChatCompletionsURL(baseURL string) string {
	normalized, baseQuery := normalizeOpenAIBaseURLParts(baseURL)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		trimmed := strings.TrimRight(normalizeOpenAIBaseURLFallback(baseURL), "/")
		if strings.HasSuffix(strings.ToLower(trimmed), "/chat/completions") {
			return trimmed
		}
		if trimmed == "" {
			return "/chat/completions"
		}
		return trimmed + "/chat/completions"
	}

	path := strings.TrimRight(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, "/chat/completions"):
		parsed.Path = path
	case path == "":
		parsed.Path = "/chat/completions"
	default:
		parsed.Path = path + "/chat/completions"
	}

	if len(baseQuery) > 0 {
		values := parsed.Query()
		for key, queryValues := range baseQuery {
			if _, exists := values[key]; exists {
				continue
			}
			for _, value := range queryValues {
				values.Add(key, value)
			}
		}
		parsed.RawQuery = values.Encode()
	}
	return parsed.String()
}

func normalizeAnthropicBaseURL(baseURL string) string {
	normalized, _ := normalizeAnthropicBaseURLParts(baseURL)
	return normalized
}

func normalizeAnthropicBaseURLParts(baseURL string) (string, url.Values) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return normalizeAnthropicBaseURLFallback(trimmed), nil
	}
	query := parsed.Query()
	parsed.RawQuery = ""
	parsed.Path = normalizeAnthropicPath(parsed.Path)
	return parsed.String(), query
}

func normalizeAnthropicBaseURLFallback(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/v1/messages"):
		return trimmed[:len(trimmed)-len("/v1/messages")]
	case strings.HasSuffix(lower, "/v1"):
		return trimmed[:len(trimmed)-len("/v1")]
	default:
		return trimmed
	}
}

func normalizeAnthropicPath(path string) string {
	trimmedPath := strings.TrimRight(path, "/")
	lowerPath := strings.ToLower(trimmedPath)
	switch {
	case strings.HasSuffix(lowerPath, "/v1/messages"):
		return trimmedPath[:len(trimmedPath)-len("/v1/messages")]
	case strings.HasSuffix(lowerPath, "/v1"):
		return trimmedPath[:len(trimmedPath)-len("/v1")]
	default:
		return trimmedPath
	}
}

func openAIHTTPClient(timeout time.Duration, baseQuery url.Values) *http.Client {
	return queryAppendingHTTPClient(timeout, baseQuery)
}

func queryAppendingHTTPClient(timeout time.Duration, baseQuery url.Values) *http.Client {
	client := &http.Client{Timeout: timeout}
	if len(baseQuery) == 0 {
		return client
	}
	client.Transport = newQueryAppendingRoundTripper(client.Transport, baseQuery)
	return client
}

type queryAppendingRoundTripper struct {
	base  http.RoundTripper
	query url.Values
}

func newQueryAppendingRoundTripper(base http.RoundTripper, query url.Values) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := make(url.Values, len(query))
	for key, values := range query {
		cloned[key] = append([]string(nil), values...)
	}
	return &queryAppendingRoundTripper{base: base, query: cloned}
}

func (rt *queryAppendingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("invalid request")
	}
	base := http.RoundTripper(http.DefaultTransport)
	if rt != nil && rt.base != nil {
		base = rt.base
	}
	if rt == nil || len(rt.query) == 0 {
		return base.RoundTrip(req)
	}
	clonedReq := req.Clone(req.Context())
	values := clonedReq.URL.Query()
	for key, queryValues := range rt.query {
		if _, exists := values[key]; exists {
			continue
		}
		for _, value := range queryValues {
			values.Add(key, value)
		}
	}
	clonedReq.URL.RawQuery = values.Encode()
	return base.RoundTrip(clonedReq)
}
