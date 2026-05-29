package aichat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type threadEventRecord struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Timestamp time.Time      `json:"timestamp"`
	Summary   string         `json:"summary,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type ThreadSummaryBlock struct {
	ID         string    `json:"id"`
	StartAt    time.Time `json:"startAt"`
	EndAt      time.Time `json:"endAt"`
	Summary    string    `json:"summary"`
	EventRefs  []string  `json:"eventRefs,omitempty"`
	EventCount int       `json:"eventCount"`
}

type threadCompactorConfig struct {
	MaxRecentEvents        int
	MaxEventsBeforeCompact int
	MaxSerializedBytes     int
}

type threadCompactionResult struct {
	RetainedEvents []threadEventRecord  `json:"retainedEvents"`
	SummaryBlocks  []ThreadSummaryBlock `json:"summaryBlocks,omitempty"`
	Compacted      bool                 `json:"compacted"`
}

func compactThreadEvents(events []threadEventRecord, cfg threadCompactorConfig) threadCompactionResult {
	if len(events) == 0 {
		return threadCompactionResult{}
	}
	normalized := normalizeThreadCompactorConfig(cfg)
	if !shouldCompactThreadEvents(events, normalized) {
		return threadCompactionResult{
			RetainedEvents: cloneThreadEventRecords(events),
			Compacted:      false,
		}
	}

	cutoff := len(events) - normalized.MaxRecentEvents
	if cutoff <= 0 {
		return threadCompactionResult{
			RetainedEvents: cloneThreadEventRecords(events),
			Compacted:      false,
		}
	}

	compacted := cloneThreadEventRecords(events[:cutoff])
	retained := cloneThreadEventRecords(events[cutoff:])
	block := summarizeCompactedThreadEvents(compacted)
	if block.ID == "" {
		return threadCompactionResult{
			RetainedEvents: retained,
			Compacted:      false,
		}
	}

	return threadCompactionResult{
		RetainedEvents: retained,
		SummaryBlocks:  []ThreadSummaryBlock{block},
		Compacted:      true,
	}
}

func normalizeThreadCompactorConfig(cfg threadCompactorConfig) threadCompactorConfig {
	if cfg.MaxRecentEvents < 1 {
		cfg.MaxRecentEvents = 6
	}
	if cfg.MaxEventsBeforeCompact < cfg.MaxRecentEvents+1 {
		cfg.MaxEventsBeforeCompact = cfg.MaxRecentEvents + 4
	}
	if cfg.MaxSerializedBytes < 0 {
		cfg.MaxSerializedBytes = 0
	}
	return cfg
}

func shouldCompactThreadEvents(events []threadEventRecord, cfg threadCompactorConfig) bool {
	if len(events) > cfg.MaxEventsBeforeCompact {
		return true
	}
	if cfg.MaxSerializedBytes == 0 {
		return false
	}
	data, err := json.Marshal(events)
	if err != nil {
		return false
	}
	return len(data) > cfg.MaxSerializedBytes
}

func summarizeCompactedThreadEvents(events []threadEventRecord) ThreadSummaryBlock {
	if len(events) == 0 {
		return ThreadSummaryBlock{}
	}

	refs := make([]string, 0, len(events))
	kindCounts := map[string]int{}
	startAt := events[0].Timestamp
	endAt := events[0].Timestamp
	for i := range events {
		if id := strings.TrimSpace(events[i].ID); id != "" {
			refs = append(refs, id)
		}
		kind := strings.TrimSpace(events[i].Kind)
		if kind == "" {
			kind = "unknown"
		}
		kindCounts[kind]++
		if events[i].Timestamp.Before(startAt) {
			startAt = events[i].Timestamp
		}
		if events[i].Timestamp.After(endAt) {
			endAt = events[i].Timestamp
		}
	}

	summaryParts := make([]string, 0, len(kindCounts))
	for kind, count := range kindCounts {
		summaryParts = append(summaryParts, fmt.Sprintf("%s=%d", kind, count))
	}
	sort.Strings(summaryParts)

	return ThreadSummaryBlock{
		ID:         buildThreadSummaryBlockID(startAt, endAt, refs),
		StartAt:    startAt,
		EndAt:      endAt,
		Summary:    fmt.Sprintf("Compacted %d thread events (%s)", len(events), strings.Join(summaryParts, ", ")),
		EventRefs:  refs,
		EventCount: len(events),
	}
}

func buildThreadSummaryBlockID(startAt, endAt time.Time, refs []string) string {
	start := startAt.UTC().Format("20060102T150405")
	end := endAt.UTC().Format("20060102T150405")
	if len(refs) == 0 {
		return "summary_" + start + "_" + end
	}
	return "summary_" + start + "_" + end + "_" + strings.TrimSpace(refs[0])
}

func cloneThreadEventRecords(events []threadEventRecord) []threadEventRecord {
	if len(events) == 0 {
		return nil
	}
	out := make([]threadEventRecord, len(events))
	copy(out, events)
	for i := range out {
		if len(out[i].Payload) == 0 {
			continue
		}
		cloned := make(map[string]any, len(out[i].Payload))
		for k, v := range out[i].Payload {
			cloned[k] = v
		}
		out[i].Payload = cloned
	}
	return out
}
