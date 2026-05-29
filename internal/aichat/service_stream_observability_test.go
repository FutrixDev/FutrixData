package aichat

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedEvent struct {
	name   string
	fields map[string]any
}

type captureDiagnostics struct {
	mu     sync.Mutex
	events []capturedEvent
}

func (d *captureDiagnostics) Log(event string, fields map[string]any) {
	if d == nil {
		return
	}
	copied := make(map[string]any, len(fields))
	for k, v := range fields {
		copied[k] = v
	}
	d.mu.Lock()
	d.events = append(d.events, capturedEvent{name: event, fields: copied})
	d.mu.Unlock()
}

func (d *captureDiagnostics) Events() []capturedEvent {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]capturedEvent, len(d.events))
	copy(out, d.events)
	return out
}

type chunkedStreamingModel struct {
	chunks []string
	delays []time.Duration
}

func (m chunkedStreamingModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	return strings.Join(m.chunks, ""), nil
}

func (m chunkedStreamingModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	var full strings.Builder
	for i, chunk := range m.chunks {
		if i < len(m.delays) && m.delays[i] > 0 {
			time.Sleep(m.delays[i])
		}
		full.WriteString(chunk)
		if onDelta != nil && chunk != "" {
			onDelta(chunk)
		}
	}
	return full.String(), nil
}

func findFirstEvent(events []capturedEvent, name string) (capturedEvent, bool) {
	for _, evt := range events {
		if evt.name == name {
			return evt, true
		}
	}
	return capturedEvent{}, false
}

func fieldString(fields map[string]any, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	return s, ok
}

func fieldInt64(fields map[string]any, key string) (int64, bool) {
	raw, ok := fields[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func TestTurnStream_EmitsStreamingTimingEvents(t *testing.T) {
	t.Parallel()

	diag := &captureDiagnostics{}
	model := chunkedStreamingModel{
		chunks: []string{`{"assistantMessage":"hi","toolCalls":[]}`},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetDiagnostics(diag)

	ctx := WithDiagnosticsContext(context.Background(), "chat_1", "stream_1")
	resp, err := svc.TurnStream(ctx, TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(resp.AssistantMessage) == "" {
		t.Fatalf("expected assistant message, got empty")
	}

	events := diag.Events()

	firstDelta, ok := findFirstEvent(events, "model_stream_first_delta")
	if !ok {
		t.Fatalf("expected model_stream_first_delta event")
	}
	if conv, ok := fieldString(firstDelta.fields, "conversationId"); !ok || conv != "chat_1" {
		t.Fatalf("expected conversationId %q, got %q", "chat_1", conv)
	}
	if stream, ok := fieldString(firstDelta.fields, "streamId"); !ok || stream != "stream_1" {
		t.Fatalf("expected streamId %q, got %q", "stream_1", stream)
	}
	if ms, ok := fieldInt64(firstDelta.fields, "firstDeltaMs"); !ok || ms < 0 {
		t.Fatalf("expected firstDeltaMs >= 0, got %v (ok=%v)", ms, ok)
	}

	if _, ok := findFirstEvent(events, "model_stream_first_assistant_delta"); !ok {
		t.Fatalf("expected model_stream_first_assistant_delta event")
	}

	modelCall, ok := findFirstEvent(events, "model_call")
	if !ok {
		t.Fatalf("expected model_call event")
	}
	if _, ok := modelCall.fields["rawDeltaChunks"]; !ok {
		t.Fatalf("expected model_call.rawDeltaChunks field")
	}
	if _, ok := modelCall.fields["assistantDeltaChunks"]; !ok {
		t.Fatalf("expected model_call.assistantDeltaChunks field")
	}
	if _, ok := modelCall.fields["firstDeltaMs"]; !ok {
		t.Fatalf("expected model_call.firstDeltaMs field")
	}
	if _, ok := modelCall.fields["firstAssistantDeltaMs"]; !ok {
		t.Fatalf("expected model_call.firstAssistantDeltaMs field")
	}
}

func TestTurnStream_FirstAssistantDeltaCanLagWhenAssistantMessageKeyArrivesLate(t *testing.T) {
	t.Parallel()

	diag := &captureDiagnostics{}
	model := chunkedStreamingModel{
		chunks: []string{
			`{"toolCalls":[]`,
			`,"assistantMessage":"hi"`,
			`}`,
		},
		delays: []time.Duration{0, 20 * time.Millisecond, 0},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetDiagnostics(diag)

	ctx := WithDiagnosticsContext(context.Background(), "chat_1", "stream_1")
	_, err := svc.TurnStream(ctx, TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	events := diag.Events()
	modelCall, ok := findFirstEvent(events, "model_call")
	if !ok {
		t.Fatalf("expected model_call event")
	}

	firstDeltaMs, ok := fieldInt64(modelCall.fields, "firstDeltaMs")
	if !ok {
		t.Fatalf("expected model_call.firstDeltaMs")
	}
	firstAssistantDeltaMs, ok := fieldInt64(modelCall.fields, "firstAssistantDeltaMs")
	if !ok {
		t.Fatalf("expected model_call.firstAssistantDeltaMs")
	}
	if firstAssistantDeltaMs < firstDeltaMs+10 {
		t.Fatalf("expected firstAssistantDeltaMs (%d) >= firstDeltaMs (%d) + 10ms", firstAssistantDeltaMs, firstDeltaMs)
	}
}

func TestTurnStream_NonStreamingModelDoesNotEmitStreamingTimingEvents(t *testing.T) {
	t.Parallel()

	diag := &captureDiagnostics{}
	model := fakeModel{response: `{"assistantMessage":"hi","toolCalls":[]}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetDiagnostics(diag)

	ctx := WithDiagnosticsContext(context.Background(), "chat_1", "stream_1")
	_, err := svc.TurnStream(ctx, TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	events := diag.Events()
	if _, ok := findFirstEvent(events, "model_stream_first_delta"); ok {
		t.Fatalf("did not expect model_stream_first_delta event")
	}
	if _, ok := findFirstEvent(events, "model_stream_first_assistant_delta"); ok {
		t.Fatalf("did not expect model_stream_first_assistant_delta event")
	}
}
