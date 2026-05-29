package aichat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestThreadStore_SavesAndLoadsMemorySnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileThreadStore(root)

	want := ThreadMemorySnapshot{
		Version:  "mem_v1",
		Rendered: "## Active Patterns\n- Avoid duplicate execute loops",
		Tokens:   42,
	}
	if err := store.SaveMemorySnapshot("thread_1", want); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	got, err := store.LoadMemorySnapshot("thread_1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got.Version != want.Version {
		t.Fatalf("expected version %q, got %q", want.Version, got.Version)
	}
	if got.Rendered != want.Rendered {
		t.Fatalf("expected rendered snapshot %q, got %q", want.Rendered, got.Rendered)
	}
}

func TestTurn_UsesPinnedThreadMemorySnapshotAcrossTurns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &promptSequenceModel{
		responses: []string{
			`{"assistantMessage":"first","toolCalls":[]}`,
			`{"assistantMessage":"second","toolCalls":[]}`,
			`{"assistantMessage":"third","toolCalls":[]}`,
		},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	store := svc.memoryStore.(*fileMemoryStore)
	if _, err := store.SavePattern(MemorySaveInput{
		Problem:    "avoid duplicate execute approvals",
		Signals:    []string{"same statement", "same datasource"},
		Avoid:      []string{"repeat execute_statement"},
		Do:         []string{"reuse prior evidence"},
		Why:        "Repeated execution adds no new signal.",
		Confidence: 0.9,
	}); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	_, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_same",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "first question"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}

	if _, err := store.SavePattern(MemorySaveInput{
		Problem:    "new thread should prefer memory_save for reusable patterns",
		Signals:    []string{"successful correction path"},
		Avoid:      []string{"store raw event log"},
		Do:         []string{"store abstract troubleshooting pattern"},
		Why:        "Long-term memory should capture reusable methods only.",
		Confidence: 0.91,
	}); err != nil {
		t.Fatalf("update memory: %v", err)
	}

	_, err = svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_same",
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "second question"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}

	_, err = svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_new",
		ConversationID: "chat_2",
		Messages:       []Message{{Role: "user", Content: "new thread question"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("third turn: %v", err)
	}

	if len(model.prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(model.prompts))
	}
	if !strings.Contains(model.prompts[0], "Pinned memory snapshot:") {
		t.Fatalf("expected first thread prompt to include pinned snapshot section, got: %s", model.prompts[0])
	}
	if !strings.Contains(model.prompts[0], "avoid duplicate execute approvals") {
		t.Fatalf("expected first thread prompt to include initial memory snapshot, got: %s", model.prompts[0])
	}
	if strings.Contains(model.prompts[1], "Pinned memory snapshot:") {
		t.Fatalf("expected same-thread prompt to avoid re-injecting the full snapshot, got: %s", model.prompts[1])
	}
	if !strings.Contains(model.prompts[1], "Pinned memory carryover") {
		t.Fatalf("expected same-thread prompt to preserve a continuity signal, got: %s", model.prompts[1])
	}
	if !strings.Contains(model.prompts[1], "avoid dupli") {
		t.Fatalf("expected same-thread prompt to keep the original memory theme via the carryover summary, got: %s", model.prompts[1])
	}
	if strings.Contains(model.prompts[1], "new thread should prefer memory_save") {
		t.Fatalf("expected same-thread prompt to reuse pinned snapshot without reloading new memory, got: %s", model.prompts[1])
	}
	if !strings.Contains(model.prompts[2], "Pinned memory snapshot:") {
		t.Fatalf("expected new thread prompt to load the latest pinned snapshot, got: %s", model.prompts[2])
	}
	if !strings.Contains(model.prompts[2], "new thread should prefer memory_save") {
		t.Fatalf("expected new thread prompt to use refreshed global memory, got: %s", model.prompts[2])
	}
}
