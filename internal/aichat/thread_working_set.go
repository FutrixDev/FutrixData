package aichat

import (
	"strings"
	"time"
)

type ToolSummary struct {
	ToolName       string    `json:"toolName,omitempty"`
	Kind           string    `json:"kind,omitempty"`
	DatasourceID   string    `json:"datasourceId,omitempty"`
	DatasourceType string    `json:"datasourceType,omitempty"`
	Database       string    `json:"database,omitempty"`
	Statement      string    `json:"statement,omitempty"`
	RowCount       int64     `json:"rowCount,omitempty"`
	HasMore        bool      `json:"hasMore,omitempty"`
	Timestamp      time.Time `json:"timestamp,omitempty"`
	Summary        string    `json:"summary,omitempty"`
}

type ApprovalSummary struct {
	ApprovalID string    `json:"approvalId,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Decision   string    `json:"decision,omitempty"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
}

type WorkingSet struct {
	RecentMessages      []Message             `json:"recentMessages,omitempty"`
	ToolSummaries       []ToolSummary         `json:"toolSummaries,omitempty"`
	ApprovalSummaries   []ApprovalSummary     `json:"approvalSummaries,omitempty"`
	ThreadSummaries     []ThreadSummaryBlock  `json:"threadSummaries,omitempty"`
	WorkingContext      *WorkingContext       `json:"workingContext,omitempty"`
	PageContext         PageContext           `json:"pageContext,omitempty"`
	ImplicitStatement   string                `json:"implicitStatement,omitempty"`
	MemorySnapshot      *ThreadMemorySnapshot `json:"memorySnapshot,omitempty"`
	RecalledMemoryNotes []MemoryNote          `json:"recalledMemoryNotes,omitempty"`
}

type workingSetBuildInput struct {
	Messages            []Message
	Events              []threadEventRecord
	SummaryBlocks       []ThreadSummaryBlock
	WorkingContext      *WorkingContext
	PageContext         PageContext
	ImplicitStatement   string
	MemorySnapshot      *ThreadMemorySnapshot
	RecalledMemoryNotes []MemoryNote
	Config              workingSetConfig
}

type workingSetConfig struct {
	MaxRecentMessages    int
	MaxToolSummaries     int
	MaxApprovalSummaries int
	MaxThreadSummaries   int
	MaxRecalledNotes     int
	Compactor            threadCompactorConfig
}

type workingSetBuildResult struct {
	WorkingSet     WorkingSet
	RetainedEvents []threadEventRecord
	SummaryBlocks  []ThreadSummaryBlock
	Compacted      bool
	LoadedEventIDs []string
}

func assembleWorkingSet(
	req TurnRequest,
	events []threadEventRecord,
	summaryBlocks []ThreadSummaryBlock,
	recalled []MemoryNote,
	cfg workingSetConfig,
) WorkingSet {
	result := buildWorkingSet(workingSetBuildInput{
		Messages:            req.Messages,
		Events:              events,
		SummaryBlocks:       summaryBlocks,
		WorkingContext:      req.WorkingContext,
		PageContext:         req.PageContext,
		ImplicitStatement:   req.ImplicitStatement,
		RecalledMemoryNotes: recalled,
		Config:              cfg,
	})
	return result.WorkingSet
}

func buildWorkingSet(input workingSetBuildInput) workingSetBuildResult {
	cfg := normalizeWorkingSetConfig(input.Config)
	compaction := compactThreadEvents(input.Events, cfg.Compactor)
	summaryBlocks := mergeThreadSummaryBlocks(input.SummaryBlocks, compaction.SummaryBlocks, cfg.MaxThreadSummaries)
	recentMessages := collectRecentMessages(compaction.RetainedEvents, cfg.MaxRecentMessages)
	if len(recentMessages) == 0 {
		recentMessages = lastMessages(input.Messages, cfg.MaxRecentMessages)
	}

	return workingSetBuildResult{
		WorkingSet: WorkingSet{
			RecentMessages:      recentMessages,
			ToolSummaries:       collectToolSummaries(compaction.RetainedEvents, cfg.MaxToolSummaries),
			ApprovalSummaries:   collectApprovalSummaries(compaction.RetainedEvents, cfg.MaxApprovalSummaries),
			ThreadSummaries:     summaryBlocks,
			WorkingContext:      mergeWorkingContext(input.WorkingContext, collectWorkingContext(compaction.RetainedEvents)),
			PageContext:         input.PageContext,
			ImplicitStatement:   strings.TrimSpace(input.ImplicitStatement),
			MemorySnapshot:      cloneThreadMemorySnapshot(input.MemorySnapshot),
			RecalledMemoryNotes: lastMemoryNotes(input.RecalledMemoryNotes, cfg.MaxRecalledNotes),
		},
		RetainedEvents: cloneThreadEventRecords(compaction.RetainedEvents),
		SummaryBlocks:  summaryBlocks,
		Compacted:      compaction.Compacted,
		LoadedEventIDs: threadEventIDs(input.Events),
	}
}

func threadEventIDs(events []threadEventRecord) []string {
	if len(events) == 0 {
		return nil
	}
	ids := make([]string, 0, len(events))
	for i := range events {
		if id := strings.TrimSpace(events[i].ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func cloneThreadMemorySnapshot(snapshot *ThreadMemorySnapshot) *ThreadMemorySnapshot {
	if snapshot == nil {
		return nil
	}
	copyValue := *snapshot
	return &copyValue
}

func collectRecentMessages(events []threadEventRecord, max int) []Message {
	if len(events) == 0 || max <= 0 {
		return nil
	}
	messages := make([]Message, 0, max)
	for i := len(events) - 1; i >= 0 && len(messages) < max; i-- {
		content := strings.TrimSpace(stringPayload(events[i].Payload, "content", "text"))
		if content == "" {
			content = strings.TrimSpace(events[i].Summary)
		}
		if content == "" {
			continue
		}
		switch events[i].Kind {
		case "user_message":
			messages = append(messages, Message{Role: "user", Content: content})
		case "assistant_message":
			messages = append(messages, Message{Role: "assistant", Content: content})
		case "memory_snapshot_seeded":
			messages = append(messages, Message{Role: "system", Content: content})
		}
	}
	reverseMessages(messages)
	return messages
}

func normalizeWorkingSetConfig(cfg workingSetConfig) workingSetConfig {
	if cfg.MaxRecentMessages < 1 {
		cfg.MaxRecentMessages = 6
	}
	if cfg.MaxToolSummaries < 1 {
		cfg.MaxToolSummaries = 3
	}
	if cfg.MaxApprovalSummaries < 1 {
		cfg.MaxApprovalSummaries = 3
	}
	if cfg.MaxThreadSummaries < 1 {
		cfg.MaxThreadSummaries = 3
	}
	if cfg.MaxRecalledNotes < 1 {
		cfg.MaxRecalledNotes = 3
	}
	cfg.Compactor = normalizeThreadCompactorConfig(cfg.Compactor)
	return cfg
}

func lastMessages(messages []Message, max int) []Message {
	if len(messages) == 0 || max <= 0 {
		return nil
	}
	if len(messages) <= max {
		out := make([]Message, len(messages))
		copy(out, messages)
		return out
	}
	out := make([]Message, max)
	copy(out, messages[len(messages)-max:])
	return out
}

func lastMemoryNotes(notes []MemoryNote, max int) []MemoryNote {
	if len(notes) == 0 || max <= 0 {
		return nil
	}
	if len(notes) <= max {
		out := make([]MemoryNote, len(notes))
		copy(out, notes)
		return out
	}
	out := make([]MemoryNote, max)
	copy(out, notes[len(notes)-max:])
	return out
}

func collectToolSummaries(events []threadEventRecord, max int) []ToolSummary {
	if len(events) == 0 || max <= 0 {
		return nil
	}
	summaries := make([]ToolSummary, 0, max)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Kind != "tool_result_summary" {
			continue
		}
		summaries = append(summaries, ToolSummary{
			ToolName:       stringPayload(event.Payload, "toolName"),
			Kind:           event.Kind,
			DatasourceID:   stringPayload(event.Payload, "datasourceId"),
			DatasourceType: stringPayload(event.Payload, "datasourceType"),
			Database:       stringPayload(event.Payload, "database"),
			Statement:      stringPayload(event.Payload, "statement"),
			RowCount:       intPayload(event.Payload, "rowCount"),
			HasMore:        boolPayload(event.Payload, "hasMore"),
			Timestamp:      event.Timestamp,
			Summary:        strings.TrimSpace(event.Summary),
		})
		if len(summaries) >= max {
			break
		}
	}
	reverseToolSummaries(summaries)
	return summaries
}

func collectApprovalSummaries(events []threadEventRecord, max int) []ApprovalSummary {
	if len(events) == 0 || max <= 0 {
		return nil
	}
	summaries := make([]ApprovalSummary, 0, max)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		switch event.Kind {
		case "approval_pending":
			summaries = append(summaries, ApprovalSummary{
				ApprovalID: stringPayload(event.Payload, "id", "approvalId"),
				Kind:       stringPayload(event.Payload, "kind"),
				Summary:    firstNonEmpty(strings.TrimSpace(event.Summary), stringPayload(event.Payload, "summary")),
				Timestamp:  event.Timestamp,
			})
		case "approval_decision":
			summaries = append(summaries, ApprovalSummary{
				ApprovalID: stringPayload(event.Payload, "approvalId", "id"),
				Decision:   stringPayload(event.Payload, "decision"),
				Summary:    strings.TrimSpace(event.Summary),
				Timestamp:  event.Timestamp,
			})
		default:
			continue
		}
		if len(summaries) >= max {
			break
		}
	}
	reverseApprovalSummaries(summaries)
	return summaries
}

func mergeThreadSummaryBlocks(existing []ThreadSummaryBlock, generated []ThreadSummaryBlock, max int) []ThreadSummaryBlock {
	if max <= 0 {
		return nil
	}
	merged := make([]ThreadSummaryBlock, 0, len(existing)+len(generated))
	seen := make(map[string]struct{}, len(existing)+len(generated))
	for _, block := range existing {
		if id := strings.TrimSpace(block.ID); id != "" {
			seen[id] = struct{}{}
		}
		merged = append(merged, block)
	}
	for _, block := range generated {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		merged = append(merged, block)
	}
	if len(merged) <= max {
		return merged
	}
	out := make([]ThreadSummaryBlock, max)
	copy(out, merged[len(merged)-max:])
	return out
}

func reverseToolSummaries(items []ToolSummary) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseApprovalSummaries(items []ApprovalSummary) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseMessages(items []Message) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
