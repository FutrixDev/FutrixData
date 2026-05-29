package aichat

import (
	"context"
	"strings"
	"testing"
)

func TestTurn_NilServiceReturnsInitializationError(t *testing.T) {
	var svc *Service

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_nil_turn",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatalf("expected initialization error")
	}
	if !strings.Contains(err.Error(), "ai chat service not initialized") {
		t.Fatalf("expected initialization error, got %v", err)
	}
}

func TestTurnStream_NilServiceReturnsInitializationError(t *testing.T) {
	var svc *Service

	_, err := svc.TurnStream(context.Background(), TurnRequest{
		ConversationID: "chat_nil_stream",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	}, nil)
	if err == nil {
		t.Fatalf("expected initialization error")
	}
	if !strings.Contains(err.Error(), "ai chat service not initialized") {
		t.Fatalf("expected initialization error, got %v", err)
	}
}
