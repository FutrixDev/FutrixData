package aichat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThreadStore_PutAndGetSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileThreadStore(root)

	want := ThreadSession{
		ThreadID:       "thread_1",
		ConversationID: "chat_1",
		UpdatedAt:      time.Unix(123, 0).UTC(),
	}
	if err := store.SaveSession(want); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got, err := store.LoadSession("thread_1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got.ThreadID != want.ThreadID {
		t.Fatalf("expected thread id %q, got %q", want.ThreadID, got.ThreadID)
	}
	if got.ConversationID != want.ConversationID {
		t.Fatalf("expected conversation id %q, got %q", want.ConversationID, got.ConversationID)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("expected updated at %s, got %s", want.UpdatedAt, got.UpdatedAt)
	}
}

func TestThreadStore_AppendsEventsInOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileThreadStore(root)

	first := threadEvent{Kind: "user_message", Timestamp: time.Unix(100, 0).UTC(), Payload: map[string]any{"text": "hi"}}
	second := threadEvent{Kind: "assistant_message", Timestamp: time.Unix(101, 0).UTC(), Payload: map[string]any{"text": "hello"}}
	if err := store.AppendEvent("thread_1", first); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if err := store.AppendEvent("thread_1", second); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "thread_1", "events.jsonl"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	lines := splitNonEmptyLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("expected 2 event lines, got %d", len(lines))
	}

	var gotFirst threadEvent
	if err := json.Unmarshal([]byte(lines[0]), &gotFirst); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotFirst.Kind != first.Kind {
		t.Fatalf("expected first event kind %q, got %q", first.Kind, gotFirst.Kind)
	}

	var gotSecond threadEvent
	if err := json.Unmarshal([]byte(lines[1]), &gotSecond); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotSecond.Kind != second.Kind {
		t.Fatalf("expected second event kind %q, got %q", second.Kind, gotSecond.Kind)
	}
}

func TestThreadStore_RejectsUnsafeThreadIDs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileThreadStore(root)
	unsafeThreadID := "../escape"

	if err := store.SaveSession(ThreadSession{
		ThreadID:       unsafeThreadID,
		ConversationID: "chat_1",
		UpdatedAt:      time.Unix(123, 0).UTC(),
	}); err == nil {
		t.Fatalf("expected error for unsafe thread id")
	}
	if err := store.AppendEvent(unsafeThreadID, threadEvent{Kind: "user_message"}); err == nil {
		t.Fatalf("expected append event error for unsafe thread id")
	}
	if _, err := store.LoadSession(unsafeThreadID); err == nil {
		t.Fatalf("expected load session error for unsafe thread id")
	}
	if _, err := store.LoadEvents(unsafeThreadID); err == nil {
		t.Fatalf("expected load events error for unsafe thread id")
	}
	if _, err := store.LoadSummaryBlocks(unsafeThreadID); err == nil {
		t.Fatalf("expected load summaries error for unsafe thread id")
	}
	if err := store.SaveSummaryBlocks(unsafeThreadID, []ThreadSummaryBlock{{Summary: "summary"}}); err == nil {
		t.Fatalf("expected save summaries error for unsafe thread id")
	}
	if _, err := store.LoadMemorySnapshot(unsafeThreadID); err == nil {
		t.Fatalf("expected load memory snapshot error for unsafe thread id")
	}
	if err := store.SaveMemorySnapshot(unsafeThreadID, ThreadMemorySnapshot{Version: "mem_1"}); err == nil {
		t.Fatalf("expected save memory snapshot error for unsafe thread id")
	}

	escapedSession := filepath.Join(filepath.Dir(root), "escape", "session.json")
	if _, err := os.Stat(escapedSession); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no escaped session file, got err=%v", err)
	}
	escapedEvents := filepath.Join(filepath.Dir(root), "escape", "events.jsonl")
	if _, err := os.Stat(escapedEvents); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no escaped events file, got err=%v", err)
	}
}
