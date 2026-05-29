package aiconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTestOpenAICompatible_usesMaxCompletionTokensForGPT52(t *testing.T) {
	t.Parallel()

	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"gpt-5.2"}`))
	}))
	t.Cleanup(srv.Close)

	result := testOpenAICompatible(context.Background(), srv.URL, "key", "gpt-5.2", nil)
	if !result.Connected {
		t.Fatalf("expected connection success, got error: %s", result.Error)
	}
	if _, ok := gotPayload["max_completion_tokens"]; !ok {
		t.Fatalf("expected request payload to include max_completion_tokens, got %v", gotPayload)
	}
	if _, ok := gotPayload["max_tokens"]; ok {
		t.Fatalf("expected request payload to omit max_tokens when using max_completion_tokens, got %v", gotPayload)
	}
}

func TestTestOpenAICompatible_usesMaxTokensForNonGPT5(t *testing.T) {
	t.Parallel()

	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"gpt-4.1-mini"}`))
	}))
	t.Cleanup(srv.Close)

	result := testOpenAICompatible(context.Background(), srv.URL, "key", "gpt-4.1-mini", nil)
	if !result.Connected {
		t.Fatalf("expected connection success, got error: %s", result.Error)
	}
	if _, ok := gotPayload["max_tokens"]; !ok {
		t.Fatalf("expected request payload to include max_tokens, got %v", gotPayload)
	}
	if _, ok := gotPayload["max_completion_tokens"]; ok {
		t.Fatalf("expected request payload to omit max_completion_tokens for non-gpt5 models, got %v", gotPayload)
	}
}

func TestTestOpenAICompatible_normalizesCompletionsEndpointAndQuery(t *testing.T) {
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"gpt-5.2"}`))
	}))
	t.Cleanup(srv.Close)

	result := testOpenAICompatible(context.Background(), srv.URL+"/proxy/v1/chat/completions?api-version=2024-02-01", "key", "gpt-5.2", nil)
	if !result.Connected {
		t.Fatalf("expected connection success, got error: %s", result.Error)
	}
	if gotPath != "/proxy/v1/chat/completions" {
		t.Fatalf("expected request path /proxy/v1/chat/completions, got %q", gotPath)
	}
	if gotQuery != "api-version=2024-02-01" {
		t.Fatalf("expected query api-version=2024-02-01, got %q", gotQuery)
	}
	if _, ok := gotPayload["max_completion_tokens"]; !ok {
		t.Fatalf("expected request payload to include max_completion_tokens, got %v", gotPayload)
	}
}

func TestTestAnthropic_normalizesBaseURLWithV1(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"claude-sonnet-4-5-20250929"}`))
	}))
	t.Cleanup(srv.Close)

	result := testAnthropic(context.Background(), srv.URL+"/v1", "key", "claude-sonnet-4-5-20250929")
	if !result.Connected {
		t.Fatalf("expected connection success, got error: %s", result.Error)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("expected request path /v1/messages, got %q", gotPath)
	}
}

func TestTestAnthropic_normalizesBaseURLWithV1AndQuery(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/proxy/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"claude-sonnet-4-5-20250929"}`))
	}))
	t.Cleanup(srv.Close)

	result := testAnthropic(context.Background(), srv.URL+"/proxy/v1?api-version=2023-06-01", "key", "claude-sonnet-4-5-20250929")
	if !result.Connected {
		t.Fatalf("expected connection success, got error: %s", result.Error)
	}
	if gotPath != "/proxy/v1/messages" {
		t.Fatalf("expected request path /proxy/v1/messages, got %q", gotPath)
	}
	if gotQuery != "api-version=2023-06-01" {
		t.Fatalf("expected query api-version=2023-06-01, got %q", gotQuery)
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

func TestChatCompletionsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain v1", in: "https://api.openai.com/v1", want: "https://api.openai.com/v1/chat/completions"},
		{name: "already completions", in: "https://api.openai.com/v1/chat/completions", want: "https://api.openai.com/v1/chat/completions"},
		{name: "proxy path", in: "https://proxy.example.com/openai/v1", want: "https://proxy.example.com/openai/v1/chat/completions"},
		{name: "completions with query", in: "https://proxy.example.com/openai/v1/chat/completions?api-version=2024-02-01", want: "https://proxy.example.com/openai/v1/chat/completions?api-version=2024-02-01"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := chatCompletionsURL(tc.in); got != tc.want {
				t.Fatalf("chatCompletionsURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
