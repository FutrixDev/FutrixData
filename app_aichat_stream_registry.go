package main

import (
	"context"
	"sync"
)

type aiChatStreamRegistry struct {
	mu      sync.Mutex
	streams map[string]context.CancelFunc
}

func newAIChatStreamRegistry() *aiChatStreamRegistry {
	return &aiChatStreamRegistry{streams: make(map[string]context.CancelFunc)}
}

func (r *aiChatStreamRegistry) register(streamID string, cancel context.CancelFunc) {
	if r == nil || streamID == "" || cancel == nil {
		return
	}
	r.mu.Lock()
	r.streams[streamID] = cancel
	r.mu.Unlock()
}

func (r *aiChatStreamRegistry) unregister(streamID string) {
	if r == nil || streamID == "" {
		return
	}
	r.mu.Lock()
	delete(r.streams, streamID)
	r.mu.Unlock()
}

func (r *aiChatStreamRegistry) cancel(streamID string) bool {
	if r == nil || streamID == "" {
		return false
	}
	r.mu.Lock()
	cancel := r.streams[streamID]
	delete(r.streams, streamID)
	r.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}
