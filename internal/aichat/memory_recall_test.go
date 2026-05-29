package aichat

import (
	"context"
	"testing"
)

type stubMemoryRecallProvider struct {
	notes []MemoryNote
}

func (s stubMemoryRecallProvider) Recall(_ context.Context, _ RecallRequest) ([]MemoryNote, error) {
	return s.notes, nil
}

func TestRecallAssembler_AddsSmallRelevantMemoryNotesToWorkingSet(t *testing.T) {
	notes := recallMemoryNotes(context.Background(), stubMemoryRecallProvider{
		notes: []MemoryNote{
			{ID: "n1", Title: "pk rule", Content: "Orders uses uid as the primary partition key."},
			{ID: "n2", Title: "naming", Content: "aid is a business attribute, not a table key."},
		},
	}, RecallRequest{MaxNotes: 3})

	if len(notes) != 2 {
		t.Fatalf("expected 2 recall notes, got %d", len(notes))
	}
	if notes[0].Title != "pk rule" {
		t.Fatalf("expected first recall note preserved, got %+v", notes[0])
	}
}

func TestRecallAssembler_DoesNotInjectFullLongTermHistory(t *testing.T) {
	notes := recallMemoryNotes(context.Background(), stubMemoryRecallProvider{
		notes: []MemoryNote{
			{ID: "n1", Content: "1"},
			{ID: "n2", Content: "2"},
			{ID: "n3", Content: "3"},
			{ID: "n4", Content: "4"},
		},
	}, RecallRequest{MaxNotes: 2})

	if len(notes) != 2 {
		t.Fatalf("expected recall notes to be capped at 2, got %d", len(notes))
	}
	if notes[0].Content != "1" || notes[1].Content != "2" {
		t.Fatalf("expected recall notes to keep top relevant subset, got %+v", notes)
	}
}
