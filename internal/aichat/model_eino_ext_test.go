package aichat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestUsesOpenAIMaxCompletionTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "gpt 5 short", model: "gpt-5.2", want: true},
		{name: "gpt 5 no dash", model: "gpt5-mini", want: true},
		{name: "gpt 5 with underscore", model: "gpt_5_chat", want: true},
		{name: "gpt 4", model: "gpt-4.1-mini", want: false},
		{name: "claude", model: "claude-sonnet-4-5-20250929", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := usesOpenAIMaxCompletionTokens(tc.model); got != tc.want {
				t.Fatalf("usesOpenAIMaxCompletionTokens(%q)=%v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestOpenAITemperatureForModel(t *testing.T) {
	t.Parallel()

	if got := openAITemperatureForModel("gpt-5.2"); got != nil {
		t.Fatalf("expected nil temperature for gpt-5.2, got %v", *got)
	}

	got := openAITemperatureForModel("gpt-4.1-mini")
	if got == nil {
		t.Fatalf("expected non-nil temperature for gpt-4.1-mini")
	}
	if *got != float32(0.2) {
		t.Fatalf("expected default temperature 0.2, got %v", *got)
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "https://api.openai.com/v1", want: "https://api.openai.com/v1"},
		{name: "with completions", in: "https://api.openai.com/v1/chat/completions", want: "https://api.openai.com/v1"},
		{name: "with completions slash", in: "https://api.openai.com/v1/chat/completions/", want: "https://api.openai.com/v1"},
		{name: "proxy with completions", in: "https://proxy.example.com/openai/v1/chat/completions", want: "https://proxy.example.com/openai/v1"},
		{name: "with query", in: "https://proxy.example.com/openai/v1/chat/completions?api-version=2024-02-01", want: "https://proxy.example.com/openai/v1"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeOpenAIBaseURL(tc.in); got != tc.want {
				t.Fatalf("normalizeOpenAIBaseURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestOpenAIChatCompletionsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain v1", in: "https://api.openai.com/v1", want: "https://api.openai.com/v1/chat/completions"},
		{name: "already completions", in: "https://api.openai.com/v1/chat/completions", want: "https://api.openai.com/v1/chat/completions"},
		{name: "proxy v1", in: "https://proxy.example.com/openai/v1", want: "https://proxy.example.com/openai/v1/chat/completions"},
		{name: "proxy completions query", in: "https://proxy.example.com/openai/v1/chat/completions?api-version=2024-02-01", want: "https://proxy.example.com/openai/v1/chat/completions?api-version=2024-02-01"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := openAIChatCompletionsURL(tc.in); got != tc.want {
				t.Fatalf("openAIChatCompletionsURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewOpenAIEinoExtModel_normalizesBaseURLWithCompletionsAndQuery(t *testing.T) {
	t.Parallel()

	var (
		gotPath    string
		gotQuery   string
		gotPayload map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)

		if r.URL.Path != "/proxy/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	model, err := NewOpenAIEinoExtModel(OpenAICompatibleModelConfig{
		BaseURL:   srv.URL + "/proxy/v1/chat/completions?api-version=2024-02-01",
		APIKey:    "test-key",
		Model:     "gpt-5.2",
		Timeout:   5 * time.Second,
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("NewOpenAIEinoExtModel error: %v", err)
	}

	got, err := model.Chat(context.Background(), "", []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Chat returned %q, want %q", got, "ok")
	}
	if gotPath != "/proxy/v1/chat/completions" {
		t.Fatalf("request path=%q, want %q", gotPath, "/proxy/v1/chat/completions")
	}
	if gotQuery != "api-version=2024-02-01" {
		t.Fatalf("request query=%q, want %q", gotQuery, "api-version=2024-02-01")
	}
	if _, ok := gotPayload["max_completion_tokens"]; !ok {
		t.Fatalf("expected max_completion_tokens for gpt-5.2, got payload=%v", gotPayload)
	}
	if _, ok := gotPayload["temperature"]; ok {
		t.Fatalf("expected temperature omitted for gpt-5.2, got payload=%v", gotPayload)
	}
}

func TestNewOpenAIEinoExtModel_coercesEmptyAssistantContentToString(t *testing.T) {
	t.Parallel()

	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","object":"chat.completion","created":1,"model":"gpt-4.1-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	model, err := NewOpenAIEinoExtModel(OpenAICompatibleModelConfig{
		BaseURL:   srv.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-4.1-mini",
		Timeout:   5 * time.Second,
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("NewOpenAIEinoExtModel error: %v", err)
	}

	_, err = model.Chat(context.Background(), "", []Message{
		{Role: "assistant", Content: ""},
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	rawMessages, ok := gotPayload["messages"].([]any)
	if !ok || len(rawMessages) < 2 {
		t.Fatalf("expected request payload to include messages, got=%v", gotPayload)
	}
	first, ok := rawMessages[0].(map[string]any)
	if !ok {
		t.Fatalf("first message should be object, got=%T", rawMessages[0])
	}
	content, ok := first["content"].(string)
	if !ok {
		t.Fatalf("expected first message content to be string, got=%v", first["content"])
	}
	if strings.TrimSpace(content) != "" {
		t.Fatalf("expected coerced blank content, got=%q", content)
	}
}

func TestNormalizeAnthropicBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "https://api.anthropic.com", want: "https://api.anthropic.com"},
		{name: "with v1", in: "https://api.anthropic.com/v1", want: "https://api.anthropic.com"},
		{name: "with v1 slash", in: "https://api.anthropic.com/v1/", want: "https://api.anthropic.com"},
		{name: "with messages path", in: "https://api.anthropic.com/v1/messages", want: "https://api.anthropic.com"},
		{name: "proxy with v1", in: "https://proxy.example.com/anthropic/v1", want: "https://proxy.example.com/anthropic"},
		{name: "proxy with v1 slash", in: "https://proxy.example.com/anthropic/v1/", want: "https://proxy.example.com/anthropic"},
		{name: "with query", in: "https://proxy.example.com/anthropic/v1?api-version=2023-06-01", want: "https://proxy.example.com/anthropic"},
		{name: "with messages query", in: "https://proxy.example.com/anthropic/v1/messages?api-version=2023-06-01", want: "https://proxy.example.com/anthropic"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeAnthropicBaseURL(tc.in); got != tc.want {
				t.Fatalf("normalizeAnthropicBaseURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeAnthropicBaseURL_preservesProxyPathForSDKResolution(t *testing.T) {
	t.Parallel()

	base := normalizeAnthropicBaseURL("https://proxy.example.com/anthropic/v1")
	if got, want := resolveAnthropicSDKMessagesURL(t, base), "https://proxy.example.com/anthropic/v1/messages"; got != want {
		t.Fatalf("resolved endpoint=%q, want %q", got, want)
	}
}

func TestAnthropicMessagesURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "https://api.anthropic.com", want: "https://api.anthropic.com/v1/messages"},
		{name: "with v1", in: "https://api.anthropic.com/v1", want: "https://api.anthropic.com/v1/messages"},
		{name: "with v1 slash", in: "https://api.anthropic.com/v1/", want: "https://api.anthropic.com/v1/messages"},
		{name: "with messages", in: "https://api.anthropic.com/v1/messages", want: "https://api.anthropic.com/v1/messages"},
		{name: "proxy with v1", in: "https://proxy.example.com/anthropic/v1", want: "https://proxy.example.com/anthropic/v1/messages"},
		{name: "proxy with messages", in: "https://proxy.example.com/anthropic/v1/messages", want: "https://proxy.example.com/anthropic/v1/messages"},
		{name: "proxy with query", in: "https://proxy.example.com/anthropic/v1?api-version=2023-06-01", want: "https://proxy.example.com/anthropic/v1/messages?api-version=2023-06-01"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := anthropicMessagesURL(tc.in); got != tc.want {
				t.Fatalf("anthropicMessagesURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeAnthropicBaseURLParts_extractsQueryForSDKTransport(t *testing.T) {
	t.Parallel()

	base, query := normalizeAnthropicBaseURLParts("https://proxy.example.com/anthropic/v1?api-version=2023-06-01")
	if got, want := base, "https://proxy.example.com/anthropic"; got != want {
		t.Fatalf("normalized base=%q, want %q", got, want)
	}
	if got, want := query.Get("api-version"), "2023-06-01"; got != want {
		t.Fatalf("query api-version=%q, want %q", got, want)
	}
}

func TestQueryAppendingRoundTripper(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotQuery string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotQuery = req.URL.RawQuery
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	rt := newQueryAppendingRoundTripper(base, url.Values{"api-version": {"2023-06-01"}})
	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/anthropic/v1/messages?stream=true", nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	if gotPath != "/anthropic/v1/messages" {
		t.Fatalf("path=%q, want /anthropic/v1/messages", gotPath)
	}
	if gotQuery != "api-version=2023-06-01&stream=true" && gotQuery != "stream=true&api-version=2023-06-01" {
		t.Fatalf("query=%q, want stream=true + api-version=2023-06-01", gotQuery)
	}
}

func resolveAnthropicSDKMessagesURL(t *testing.T, base string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse base url failed: %v", err)
	}
	if u.Path != "" && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	resolved, err := u.Parse("v1/messages")
	if err != nil {
		t.Fatalf("resolve anthropic endpoint failed: %v", err)
	}
	return resolved.String()
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
