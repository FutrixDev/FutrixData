package schemaprivacy

import (
	"context"
	"strings"
)

// aiConfigIDKey is an unexported type so other packages cannot collide on the
// context value. The active AI config flows from the chat turn (which knows
// the user's per-request override) down to the gate call site, where it
// otherwise would have to be threaded through every tool method signature.
type aiConfigIDKey struct{}

// ContextWithAIConfigID stamps the context with the AI config that the
// current turn is using. The chat runtime calls this once per turn; the gate
// reads it via AIConfigIDFromContext when recording an audit entry, so the
// log reflects where the schema actually went rather than the user's
// preferred default.
func ContextWithAIConfigID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, aiConfigIDKey{}, id)
}

// AIConfigIDFromContext retrieves the AI config ID stamped onto the context.
// Returns "" if no ID was attached, which the provider lookup treats as
// "fall back to the preferred config".
func AIConfigIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(aiConfigIDKey{}).(string); ok {
		return v
	}
	return ""
}
