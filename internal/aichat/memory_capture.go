package aichat

import (
	"fmt"
	"strings"
)

type MemoryCandidate struct {
	ID      string         `json:"id,omitempty"`
	Kind    string         `json:"kind"`
	Title   string         `json:"title"`
	Content string         `json:"content"`
	Tags    []string       `json:"tags,omitempty"`
	Source  map[string]any `json:"source,omitempty"`
	Status  string         `json:"status,omitempty"`
}

type memoryCapturePlanner interface {
	BuildCandidates(events []threadEventRecord) []MemoryCandidate
}

type defaultMemoryCapturePlanner struct{}

func (defaultMemoryCapturePlanner) BuildCandidates(events []threadEventRecord) []MemoryCandidate {
	return buildMemoryCaptureCandidates(events)
}

func buildMemoryCaptureCandidates(events []threadEventRecord) []MemoryCandidate {
	if len(events) == 0 {
		return nil
	}
	candidates := make([]MemoryCandidate, 0, 4)
	for i := range events {
		event := events[i]
		switch event.Kind {
		case "user_message":
			content := strings.TrimSpace(stringPayload(event.Payload, "content", "text"))
			if !looksLikeStablePreference(content) {
				continue
			}
			candidates = appendIfMissingCandidate(candidates, MemoryCandidate{
				ID:      "memory_pref_" + strings.TrimSpace(event.ID),
				Kind:    "user_preference",
				Title:   "User preference",
				Content: content,
				Tags:    []string{"thread_preference"},
				Source: map[string]any{
					"eventId": event.ID,
					"kind":    event.Kind,
				},
				Status: "pending_review",
			})
		case "tool_result_summary":
			if !boolPayload(event.Payload, "stableFact") {
				continue
			}
			statement := strings.TrimSpace(firstNonEmpty(event.Summary, stringPayload(event.Payload, "statement")))
			if statement == "" {
				continue
			}
			candidates = appendIfMissingCandidate(candidates, MemoryCandidate{
				ID:      "memory_tool_" + strings.TrimSpace(event.ID),
				Kind:    "stable_fact",
				Title:   "Stable datasource fact",
				Content: fmt.Sprintf("Verified fact: %s", statement),
				Tags:    []string{"tool_result", "stable_fact"},
				Source: map[string]any{
					"eventId": event.ID,
					"kind":    event.Kind,
				},
				Status: "pending_review",
			})
		case "assistant_message":
			content := strings.TrimSpace(stringPayload(event.Payload, "content", "text"))
			if isSpeculativeMemoryContent(content) {
				continue
			}
			if !boolPayload(event.Payload, "stable") && !strings.Contains(strings.ToLower(content), "原因") && !strings.Contains(strings.ToLower(content), "root cause") {
				continue
			}
			candidates = appendIfMissingCandidate(candidates, MemoryCandidate{
				ID:      "memory_conclusion_" + strings.TrimSpace(event.ID),
				Kind:    "stable_conclusion",
				Title:   "Stable conclusion",
				Content: content,
				Tags:    []string{"assistant_summary"},
				Source: map[string]any{
					"eventId": event.ID,
					"kind":    event.Kind,
				},
				Status: "pending_review",
			})
		}
	}
	return candidates
}

func appendIfMissingCandidate(candidates []MemoryCandidate, next MemoryCandidate) []MemoryCandidate {
	for _, existing := range candidates {
		if existing.Kind == next.Kind && existing.Content == next.Content {
			return candidates
		}
	}
	return append(candidates, next)
}

func isSpeculativeMemoryContent(content string) bool {
	if content == "" {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return true
	}
	speculative := []string{
		"maybe", "might", "probably", "possibly", "could be",
		"可能", "也许", "大概", "猜测",
	}
	for _, marker := range speculative {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeStablePreference(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	markers := []string{
		"prefer ", "default ", "以后", "默认", "请始终", "always", "use utc",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
