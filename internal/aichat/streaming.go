package aichat

import "context"

// StreamingModel is an optional extension interface for models that can stream tokens.
// The callback receives raw text deltas (not yet parsed/decoded by the agent).
type StreamingModel interface {
	ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error)
}
