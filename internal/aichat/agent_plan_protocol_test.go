package aichat

import (
	"context"
	"testing"
)

func TestParseModelOutputDetailed_WithAgentAndPlan(t *testing.T) {
	t.Parallel()

	raw := `{"assistantMessage":"I will plan this.","toolCalls":[],"agent":{"mode":"plan_executor","complexity":"complex","reason":"Needs multiple steps.","confidence":0.88},"plan":{"title":"Investigate request","summary":"Break down and execute safely.","markdown":"1. Inspect context\n2. Plan\n3. Execute","steps":[{"id":"step_1","title":"Inspect context","description":"Read datasource schema.","status":"pending"},{"id":"step_2","title":"Plan SQL","description":"Draft safe query.","status":"pending"}]}}`

	parsed, ok, err := parseModelOutputDetailed(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected parse ok")
	}
	if parsed.Agent == nil {
		t.Fatalf("expected agent metadata")
	}
	if parsed.Agent.Mode != "plan_executor" {
		t.Fatalf("expected plan_executor mode, got %q", parsed.Agent.Mode)
	}
	if parsed.Agent.Complexity != "complex" {
		t.Fatalf("expected complexity=complex, got %q", parsed.Agent.Complexity)
	}
	if parsed.Plan == nil {
		t.Fatalf("expected plan metadata")
	}
	if parsed.Plan.Title != "Investigate request" {
		t.Fatalf("expected plan title, got %q", parsed.Plan.Title)
	}
	if len(parsed.Plan.Steps) != 2 {
		t.Fatalf("expected 2 plan steps, got %d", len(parsed.Plan.Steps))
	}
	if parsed.Plan.Steps[0].Status != "pending" {
		t.Fatalf("expected first step pending, got %q", parsed.Plan.Steps[0].Status)
	}
}

func TestParseModelOutputDetailed_WithIntent(t *testing.T) {
	t.Parallel()

	raw := `{"assistantMessage":"","toolCalls":[{"name":"list_datasources","arguments":{}}],"intent":{"currentFocus":"avoid_current","reason":"user says target is elsewhere","confidence":0.91}}`

	parsed, ok, err := parseModelOutputDetailed(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected parse ok")
	}
	if parsed.Intent == nil {
		t.Fatalf("expected intent metadata")
	}
	if parsed.Intent.CurrentFocus != "avoid_current" {
		t.Fatalf("expected avoid_current intent, got %#v", parsed.Intent)
	}
	if parsed.Intent.Confidence != 0.91 {
		t.Fatalf("expected confidence 0.91, got %#v", parsed.Intent)
	}
}

func TestTurn_PropagatesAgentAndPlanFromModelOutput(t *testing.T) {
	t.Parallel()

	model := fakeModel{response: `{"assistantMessage":"Let's do this.","toolCalls":[],"agent":{"mode":"deepagent","complexity":"hard"},"plan":{"title":"Solve task","markdown":"- step A","steps":[{"id":"a","title":"step A","status":"in_progress"}]}}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "help me debug this"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Agent == nil {
		t.Fatalf("expected response agent metadata")
	}
	if resp.Agent.Mode != "deepagent" {
		t.Fatalf("expected deepagent mode, got %q", resp.Agent.Mode)
	}
	if resp.Plan == nil {
		t.Fatalf("expected response plan metadata")
	}
	if resp.Plan.Title != "Solve task" {
		t.Fatalf("expected plan title, got %q", resp.Plan.Title)
	}
}

func TestTurnStream_PropagatesAgentAndPlanFromModelOutput(t *testing.T) {
	t.Parallel()

	model := fakeStreamingModel{response: `{"assistantMessage":"Streaming done.","toolCalls":[],"agent":{"mode":"plan_executor","complexity":"complex"},"plan":{"title":"Run plan","markdown":"- one","steps":[{"id":"one","title":"one","status":"pending"}]}}`}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})

	resp, err := svc.TurnStream(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "please plan this task"}},
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Agent == nil {
		t.Fatalf("expected response agent metadata")
	}
	if resp.Agent.Mode != "plan_executor" {
		t.Fatalf("expected plan_executor mode, got %q", resp.Agent.Mode)
	}
	if resp.Plan == nil || resp.Plan.Title != "Run plan" {
		t.Fatalf("expected propagated plan title, got %#v", resp.Plan)
	}
}
