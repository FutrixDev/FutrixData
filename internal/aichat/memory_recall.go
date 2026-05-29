package aichat

import (
	"context"
	"strings"
)

type MemoryNote struct {
	ID      string         `json:"id,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Title   string         `json:"title,omitempty"`
	Content string         `json:"content,omitempty"`
	Tags    []string       `json:"tags,omitempty"`
	Source  map[string]any `json:"source,omitempty"`
	Score   float64        `json:"score,omitempty"`
}

type RecallRequest struct {
	ThreadID          string      `json:"threadId,omitempty"`
	ConversationID    string      `json:"conversationId,omitempty"`
	Query             string      `json:"query,omitempty"`
	PageContext       PageContext `json:"pageContext,omitempty"`
	ImplicitStatement string      `json:"implicitStatement,omitempty"`
	MaxNotes          int         `json:"maxNotes,omitempty"`
	Limit             int         `json:"limit,omitempty"`
}

type MemoryRecallProvider interface {
	Recall(ctx context.Context, req RecallRequest) ([]MemoryNote, error)
}

type noopMemoryRecallProvider struct{}

func (noopMemoryRecallProvider) Recall(_ context.Context, _ RecallRequest) ([]MemoryNote, error) {
	return nil, nil
}

func buildRecallRequest(req TurnRequest, maxNotes int) RecallRequest {
	return RecallRequest{
		ThreadID:          resolveThreadID(req.ThreadID, req.ConversationID),
		ConversationID:    strings.TrimSpace(req.ConversationID),
		Query:             strings.TrimSpace(lastUserText(req.Messages)),
		PageContext:       req.PageContext,
		ImplicitStatement: strings.TrimSpace(req.ImplicitStatement),
		MaxNotes:          maxNotes,
	}
}

func recallMemoryNotes(ctx context.Context, provider MemoryRecallProvider, req RecallRequest) []MemoryNote {
	if provider == nil {
		provider = noopMemoryRecallProvider{}
	}
	notes, err := provider.Recall(ctx, req)
	if err != nil || len(notes) == 0 {
		return nil
	}
	maxNotes := req.MaxNotes
	if maxNotes < 1 {
		maxNotes = req.Limit
	}
	if maxNotes < 1 {
		maxNotes = 3
	}
	if len(notes) > maxNotes {
		notes = notes[:maxNotes]
	}
	out := make([]MemoryNote, len(notes))
	copy(out, notes)
	return out
}
