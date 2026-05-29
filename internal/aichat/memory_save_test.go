package aichat

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntime_RegisterMemorySaveTool(t *testing.T) {
	rt := &einoTurnRuntime{
		service:        NewService(fakeResolver{model: &promptRecordingModel{}}, &fakeTools{}),
		req:            TurnRequest{ConversationID: "chat_1"},
		conversationID: "chat_1",
	}
	tools := rt.buildTools()

	var found bool
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info: %v", err)
		}
		if info.Name == "memory_save" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected memory_save tool to be registered")
	}
}

func TestTurn_MemorySaveWritesPatternAndThreadEvent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"memory_save","arguments":{"problem":"avoid storing raw case logs in long-term memory","signals":["same issue repeats","old path failed"],"avoid":["persist raw SQL and ids"],"do":["persist abstract troubleshooting pattern"],"why":"Patterns generalize better than case logs.","confidence":0.95,"evidenceEventIds":["evt_1"]}}]}`,
			`{"assistantMessage":"saved","toolCalls":[]}`,
		},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetThreadStoreDir(root)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ThreadID:       "thread_memsave",
		ConversationID: "chat_memsave",
		Messages:       []Message{{Role: "user", Content: "记住这次从错误路径纠正到正确路径的模式"}},
		PageContext:    PageContext{RouteName: "console"},
	})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !strings.Contains(resp.AssistantMessage, "saved") {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}

	state, err := svc.memoryStore.(*fileMemoryStore).Load()
	if err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if len(state.ActivePatterns) == 0 {
		t.Fatalf("expected memory pattern to be saved")
	}
	if strings.Contains(strings.Join(state.ActivePatterns[0].Do, " "), "evt_1") {
		t.Fatalf("expected normalized memory pattern without raw evidence ids, got %+v", state.ActivePatterns[0])
	}

	events, err := svc.threadStore.LoadEvents("thread_memsave")
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	var sawMemorySaved bool
	for _, event := range events {
		if event.Kind == "memory_saved" {
			sawMemorySaved = true
			break
		}
	}
	if !sawMemorySaved {
		t.Fatalf("expected memory_saved event, got %+v", events)
	}
}

func TestMemorySave_NormalizesWorkingContextCaseDetails(t *testing.T) {
	store := newFileMemoryStore(t.TempDir(), 8_000)
	result, err := store.SavePattern(MemorySaveInput{
		Problem:    "when table MM is not in datasource ds_k3s_mongo, expand discovery before execution",
		Signals:    []string{"current table detail is missing", "datasource detail does not contain table MM"},
		Avoid:      []string{"stay on datasource ds_test and keep executing query shape"},
		Do:         []string{"rebuild working context from discovery results instead of case detail"},
		Why:        "Generalize the correction path instead of storing one case.",
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("save pattern: %v", err)
	}
	if result.SavedPattern == nil {
		t.Fatalf("expected saved pattern")
	}
	rendered := strings.Join([]string{
		result.SavedPattern.Problem,
		strings.Join(result.SavedPattern.Signals, " "),
		strings.Join(result.SavedPattern.Avoid, " "),
		strings.Join(result.SavedPattern.Do, " "),
	}, " ")
	if strings.Contains(rendered, "ds_k3s_mongo") || strings.Contains(rendered, "ds_test") {
		t.Fatalf("expected datasource ids to be removed, got %+v", result.SavedPattern)
	}
	if strings.Contains(rendered, " MM ") || strings.HasSuffix(rendered, " MM") || strings.HasPrefix(rendered, "MM ") {
		t.Fatalf("expected case-specific entity names to be generalized, got %+v", result.SavedPattern)
	}
}
