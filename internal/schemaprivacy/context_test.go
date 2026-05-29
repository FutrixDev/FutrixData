package schemaprivacy

import (
	"context"
	"testing"
)

func TestContextWithAIConfigID_RoundTrip(t *testing.T) {
	ctx := ContextWithAIConfigID(context.Background(), "cfg-123")
	if got := AIConfigIDFromContext(ctx); got != "cfg-123" {
		t.Fatalf("expected cfg-123, got %q", got)
	}
}

func TestContextWithAIConfigID_EmptyDoesNotAttachKey(t *testing.T) {
	parent := context.Background()
	ctx := ContextWithAIConfigID(parent, "")
	if ctx != parent {
		t.Fatalf("expected ContextWithAIConfigID with blank id to return the parent ctx unchanged")
	}
	ctx = ContextWithAIConfigID(parent, "   ")
	if ctx != parent {
		t.Fatalf("expected ContextWithAIConfigID with whitespace-only id to return the parent ctx unchanged")
	}
}

func TestAIConfigIDFromContext_NilContextReturnsEmpty(t *testing.T) {
	//nolint:staticcheck // intentionally exercising the nil branch
	if got := AIConfigIDFromContext(nil); got != "" {
		t.Fatalf("expected empty string for nil ctx, got %q", got)
	}
}

func TestAIConfigIDFromContext_MissingKeyReturnsEmpty(t *testing.T) {
	if got := AIConfigIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty string when no id stamped on ctx, got %q", got)
	}
}
