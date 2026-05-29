package aichat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

func (s *Service) SetMemoryRecallProvider(provider MemoryRecallProvider) {
	if s == nil {
		return
	}
	if provider == nil {
		s.memoryRecall = noopMemoryRecallProvider{}
		return
	}
	s.memoryRecall = provider
}

func (s *Service) SetMemoryCapturePlanner(planner memoryCapturePlanner) {
	if s == nil {
		return
	}
	if planner == nil {
		s.memoryCapture = defaultMemoryCapturePlanner{}
		return
	}
	s.memoryCapture = planner
}

func (s *Service) loadThreadWorkingSet(ctx context.Context, req TurnRequest) workingSetBuildResult {
	input := workingSetBuildInput{
		Messages:          req.Messages,
		WorkingContext:    req.WorkingContext,
		PageContext:       req.PageContext,
		ImplicitStatement: req.ImplicitStatement,
	}
	if s == nil {
		return buildWorkingSet(input)
	}
	input.Config = s.workingSetConfig
	input.RecalledMemoryNotes = recallMemoryNotes(ctx, s.memoryRecall, buildRecallRequest(req, input.Config.MaxRecalledNotes))

	if s.threadStore == nil {
		return buildWorkingSet(input)
	}

	threadID := resolveThreadID(req.ThreadID, req.ConversationID)
	if threadID == "" {
		return buildWorkingSet(input)
	}

	events, err := s.threadStore.LoadEvents(threadID)
	if err != nil {
		s.logThreadStoreError("load_events", threadID, err)
	}
	summaryBlocks, err := s.threadStore.LoadSummaryBlocks(threadID)
	if err != nil {
		s.logThreadStoreError("load_summaries", threadID, err)
	}
	snapshot, created, err := s.ensureThreadMemorySnapshot(threadID)
	if err != nil {
		s.logThreadStoreError("ensure_memory_snapshot", threadID, err)
	} else if snapshot.Rendered != "" && created && len(events) <= 1 {
		input.MemorySnapshot = &snapshot
	}
	if created {
		summary := summarizeThreadMemorySnapshot(snapshot)
		seeded := newThreadEvent("memory_snapshot_seeded", map[string]any{
			"version": strings.TrimSpace(snapshot.Version),
			"summary": summary,
			"content": summary,
		})
		s.appendThreadEvent(threadID, seeded)
	}
	input.Events = events
	input.SummaryBlocks = summaryBlocks
	return buildWorkingSet(input)
}

func buildThreadWorkingSetSection(result workingSetBuildResult) string {
	const maxRenderedRecentMessages = 4
	const maxRenderedMessageRunes = 48

	ws := result.WorkingSet
	if len(ws.RecentMessages) == 0 && len(ws.ToolSummaries) == 0 && len(ws.ApprovalSummaries) == 0 && len(ws.ThreadSummaries) == 0 && len(ws.RecalledMemoryNotes) == 0 && ws.MemorySnapshot == nil && ws.WorkingContext == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Thread working set:\n")
	if len(ws.RecentMessages) > 0 {
		b.WriteString("Recent thread messages:\n")
		start := 0
		if len(ws.RecentMessages) > maxRenderedRecentMessages {
			start = len(ws.RecentMessages) - maxRenderedRecentMessages
		}
		for _, item := range ws.RecentMessages[start:] {
			b.WriteString(fmt.Sprintf("- %s: %s\n", strings.TrimSpace(item.Role), truncateThreadPreview(strings.TrimSpace(item.Content), maxRenderedMessageRunes)))
		}
	}
	if len(ws.ToolSummaries) > 0 {
		b.WriteString("Recent tool results:\n")
		for _, item := range ws.ToolSummaries {
			statement := strings.TrimSpace(item.Statement)
			if statement == "" {
				statement = strings.TrimSpace(item.Summary)
			}
			if statement == "" {
				statement = "tool result"
			}
			b.WriteString(fmt.Sprintf("- %s | rowCount=%d | hasMore=%t\n", statement, item.RowCount, item.HasMore))
		}
	}
	if len(ws.ApprovalSummaries) > 0 {
		b.WriteString("Recent approvals:\n")
		for _, item := range ws.ApprovalSummaries {
			parts := []string{}
			if strings.TrimSpace(item.Kind) != "" {
				parts = append(parts, item.Kind)
			}
			if strings.TrimSpace(item.Decision) != "" {
				parts = append(parts, item.Decision)
			}
			if strings.TrimSpace(item.Summary) != "" {
				parts = append(parts, item.Summary)
			}
			b.WriteString(fmt.Sprintf("- %s\n", strings.Join(parts, " | ")))
		}
	}
	if ws.WorkingContext != nil {
		parts := []string{}
		if ws.WorkingContext.DatasourceID != "" {
			parts = append(parts, "datasourceId="+ws.WorkingContext.DatasourceID)
		}
		if ws.WorkingContext.Database != "" {
			parts = append(parts, "database="+ws.WorkingContext.Database)
		}
		if ws.WorkingContext.Entity != "" {
			parts = append(parts, "entity="+ws.WorkingContext.Entity)
		}
		if ws.WorkingContext.Source != "" {
			parts = append(parts, "source="+ws.WorkingContext.Source)
		}
		if len(parts) > 0 {
			b.WriteString("Working context:\n")
			b.WriteString("- " + strings.Join(parts, " | ") + "\n")
		}
	}
	if len(ws.ThreadSummaries) > 0 {
		b.WriteString("Older thread summaries:\n")
		for _, item := range ws.ThreadSummaries {
			b.WriteString(fmt.Sprintf("- %s\n", strings.TrimSpace(item.Summary)))
		}
	}
	if ws.MemorySnapshot != nil && strings.TrimSpace(ws.MemorySnapshot.Rendered) != "" {
		b.WriteString("Pinned memory snapshot:\n")
		b.WriteString(strings.TrimSpace(ws.MemorySnapshot.Rendered))
		b.WriteString("\n")
	}
	if len(ws.RecalledMemoryNotes) > 0 {
		b.WriteString("Recalled memory:\n")
		for _, note := range ws.RecalledMemoryNotes {
			title := firstNonEmpty(note.Title, note.ID, "memory")
			content := strings.TrimSpace(note.Content)
			if content == "" {
				content = title
			}
			b.WriteString(fmt.Sprintf("- %s: %s\n", title, content))
		}
	}
	return strings.TrimSpace(b.String())
}

func truncateThreadPreview(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return strings.TrimSpace(text)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var runeCount int
	for i := range text {
		if runeCount >= maxRunes {
			return strings.TrimSpace(text[:i]) + "…"
		}
		runeCount++
	}
	return text
}

func toolNamesFromWorkingSet(result workingSetBuildResult) []string {
	names := make([]string, 0, len(result.WorkingSet.ToolSummaries)+len(result.WorkingSet.ApprovalSummaries))
	for _, item := range result.WorkingSet.ToolSummaries {
		name := strings.TrimSpace(item.ToolName)
		if name == "" {
			name = strings.TrimSpace(item.Kind)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	for _, item := range result.WorkingSet.ApprovalSummaries {
		if strings.TrimSpace(item.Kind) != "" {
			names = append(names, strings.TrimSpace(item.Kind))
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}

func (s *Service) buildResponseMemory(threadID string) *MemoryEnvelope {
	if s == nil || s.threadStore == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	events, err := s.threadStore.LoadEvents(threadID)
	if err != nil {
		events = nil
	}
	var candidates []MemoryCandidate
	if s.memoryCapture != nil {
		candidates = s.memoryCapture.BuildCandidates(events)
	}
	snapshot, _, err := s.ensureThreadMemorySnapshot(threadID)
	if err != nil && len(candidates) == 0 {
		return nil
	}
	var snapshotCopy *ThreadMemorySnapshot
	if strings.TrimSpace(snapshot.Rendered) != "" {
		snapshotCopy = &snapshot
	}
	if snapshotCopy == nil && len(candidates) == 0 {
		return nil
	}
	return &MemoryEnvelope{
		Snapshot:   snapshotCopy,
		Candidates: candidates,
	}
}

func memoryEnvelopeFromWorkingSet(ws *WorkingSet) *MemoryEnvelope {
	if ws == nil {
		return nil
	}
	envelope := &MemoryEnvelope{}
	if ws.MemorySnapshot != nil {
		envelope.Snapshot = cloneThreadMemorySnapshot(ws.MemorySnapshot)
	}
	if len(ws.RecalledMemoryNotes) > 0 {
		out := make([]MemoryNote, len(ws.RecalledMemoryNotes))
		copy(out, ws.RecalledMemoryNotes)
		envelope.Recalled = out
	}
	if envelope.Snapshot == nil && len(envelope.Recalled) == 0 {
		return nil
	}
	return envelope
}

func mergeMemoryEnvelopes(base *MemoryEnvelope, next *MemoryEnvelope) *MemoryEnvelope {
	if base == nil && next == nil {
		return nil
	}
	merged := &MemoryEnvelope{}
	if base != nil {
		if base.Snapshot != nil {
			merged.Snapshot = cloneThreadMemorySnapshot(base.Snapshot)
		}
		if len(base.Recalled) > 0 {
			merged.Recalled = append(merged.Recalled, base.Recalled...)
		}
		if len(base.Candidates) > 0 {
			merged.Candidates = append(merged.Candidates, base.Candidates...)
		}
	}
	if next != nil {
		if next.Snapshot != nil {
			merged.Snapshot = cloneThreadMemorySnapshot(next.Snapshot)
		}
		if len(next.Recalled) > 0 {
			merged.Recalled = append(merged.Recalled, next.Recalled...)
		}
		if len(next.Candidates) > 0 {
			merged.Candidates = append(merged.Candidates, next.Candidates...)
		}
	}
	if merged.Snapshot == nil && len(merged.Recalled) == 0 && len(merged.Candidates) == 0 {
		return nil
	}
	return merged
}

func (s *Service) persistThreadWorkingSet(threadID string, result workingSetBuildResult) {
	if s == nil || s.threadStore == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	if result.Compacted {
		events := result.RetainedEvents
		if appended, err := s.eventsAppendedAfterWorkingSetLoad(threadID, result.LoadedEventIDs); err != nil {
			s.logThreadStoreError("load_events_for_compaction_merge", threadID, err)
		} else if len(appended) > 0 {
			events = append(cloneThreadEventRecords(events), appended...)
		}
		if err := s.threadStore.SaveEvents(threadID, events); err != nil {
			s.logThreadStoreError("save_events", threadID, err)
		}
	}
	if len(result.SummaryBlocks) > 0 {
		if err := s.threadStore.SaveSummaryBlocks(threadID, result.SummaryBlocks); err != nil {
			s.logThreadStoreError("save_summaries", threadID, err)
		}
	}
}

func (s *Service) eventsAppendedAfterWorkingSetLoad(threadID string, loadedEventIDs []string) ([]threadEventRecord, error) {
	if s == nil || s.threadStore == nil {
		return nil, nil
	}
	current, err := s.threadStore.LoadEvents(threadID)
	if err != nil || len(current) == 0 {
		return nil, err
	}
	loaded := make(map[string]struct{}, len(loadedEventIDs))
	for _, id := range loadedEventIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			loaded[trimmed] = struct{}{}
		}
	}
	appended := make([]threadEventRecord, 0, len(current))
	for i := range current {
		if _, ok := loaded[strings.TrimSpace(current[i].ID)]; ok {
			continue
		}
		appended = append(appended, current[i])
	}
	return appended, nil
}

func (s *Service) ensureThreadMemorySnapshot(threadID string) (ThreadMemorySnapshot, bool, error) {
	if s == nil || s.threadStore == nil || s.memoryStore == nil {
		return ThreadMemorySnapshot{}, false, nil
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ThreadMemorySnapshot{}, false, nil
	}
	snapshot, err := s.threadStore.LoadMemorySnapshot(threadID)
	if err == nil {
		return snapshot, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ThreadMemorySnapshot{}, false, err
	}
	snapshot, err = s.memoryStore.BuildThreadSnapshot()
	if err != nil {
		return ThreadMemorySnapshot{}, false, err
	}
	if strings.TrimSpace(snapshot.Rendered) == "" {
		return ThreadMemorySnapshot{}, false, nil
	}
	if err := s.threadStore.SaveMemorySnapshot(threadID, snapshot); err != nil {
		return ThreadMemorySnapshot{}, false, err
	}
	return snapshot, true, nil
}

func summarizeThreadMemorySnapshot(snapshot ThreadMemorySnapshot) string {
	version := strings.TrimSpace(snapshot.Version)
	titles := extractPatternTitles(snapshot.Rendered)
	if len(titles) == 0 {
		return firstNonEmpty(
			"Pinned memory carryover ("+version+")",
			"Pinned memory carryover",
		)
	}
	return strings.TrimSpace(firstNonEmpty(
		"Pinned memory carryover ("+version+"): "+strings.Join(titles, " | "),
		"Pinned memory carryover: "+strings.Join(titles, " | "),
	))
}

func extractPatternTitles(rendered string) []string {
	lines := strings.Split(strings.ReplaceAll(rendered, "\r\n", "\n"), "\n")
	out := make([]string, 0, 4)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "### Pattern: ") {
			continue
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "### Pattern: ")))
		if len(out) >= 3 {
			break
		}
	}
	return out
}
