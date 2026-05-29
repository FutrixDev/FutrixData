package aichat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleModel_ChatStream_supportsMultilineSSEDataEvents(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// One SSE event split into multiple data lines. When concatenated with "\n",
		// the JSON remains valid (newlines are whitespace outside strings).
		_, _ = w.Write([]byte("data: {\"choices\":[\n"))
		_, _ = w.Write([]byte("data: {\"delta\":{\"content\":\"hello\"}}\n"))
		_, _ = w.Write([]byte("data: ]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	model := NewOpenAICompatibleModel(OpenAICompatibleModelConfig{
		BaseURL:  srv.URL,
		APIKey:   "test",
		Model:    "test-model",
		Timeout:  2 * time.Second,
		Referer:  "http://localhost",
		AppTitle: "FutrixData Platform",
	})

	var deltas strings.Builder
	got, err := model.ChatStream(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, func(delta string) {
		deltas.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected full content %q, got %q", "hello", got)
	}
	if deltas.String() != "hello" {
		t.Fatalf("expected deltas %q, got %q", "hello", deltas.String())
	}
}

func TestOpenAICompatibleModel_ChatStream_returnsErrorWhenProviderSendsErrorEvent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"invalid api key\"}}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	model := NewOpenAICompatibleModel(OpenAICompatibleModelConfig{
		BaseURL:  srv.URL,
		APIKey:   "test",
		Model:    "test-model",
		Timeout:  2 * time.Second,
		Referer:  "http://localhost",
		AppTitle: "FutrixData Platform",
	})

	got, err := model.ChatStream(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatalf("expected ChatStream error, got nil (content=%q)", got)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid api key") {
		t.Fatalf("expected error to include %q, got %q", "invalid api key", err.Error())
	}
}

func TestOpenAICompatibleModel_ChatStream_sendsMaxCompletionTokensForGPT52(t *testing.T) {
	t.Parallel()

	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	model := NewOpenAICompatibleModel(OpenAICompatibleModelConfig{
		BaseURL:   srv.URL,
		APIKey:    "test",
		Model:     "gpt-5.2",
		MaxTokens: 64,
		Timeout:   2 * time.Second,
		Referer:   "http://localhost",
		AppTitle:  "FutrixData Platform",
	})

	_, err := model.ChatStream(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	if _, ok := gotPayload["max_completion_tokens"]; !ok {
		t.Fatalf("expected request payload to include max_completion_tokens, got %v", gotPayload)
	}
	if _, ok := gotPayload["max_tokens"]; ok {
		t.Fatalf("expected request payload to omit max_tokens when using max_completion_tokens, got %v", gotPayload)
	}
}

func TestAnthropicModel_ChatStream_supportsMultilineSSEDataEvents(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// One SSE event split into multiple data lines; JSON remains valid when joined.
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\n"))
		_, _ = w.Write([]byte("data: \"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	model := NewAnthropicModel(AnthropicModelConfig{
		BaseURL: srv.URL,
		APIKey:  "test",
		Model:   "claude-test",
		Timeout: 2 * time.Second,
	})

	var deltas strings.Builder
	got, err := model.ChatStream(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, func(delta string) {
		deltas.WriteString(delta)
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected full content %q, got %q", "hello", got)
	}
	if deltas.String() != "hello" {
		t.Fatalf("expected deltas %q, got %q", "hello", deltas.String())
	}
}
