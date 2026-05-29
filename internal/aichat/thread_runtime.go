package aichat

import (
	"context"
	"strings"
	"time"
)

func (s *Service) SetThreadStoreDir(dir string) {
	if s == nil {
		return
	}
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		s.threadStore = nil
		s.memoryStore = nil
		return
	}
	s.threadStore = newFileThreadStore(trimmed)
	s.memoryStore = newFileMemoryStore(trimmed, 8_000)
}

func resolveThreadID(threadID string, conversationID string) string {
	if trimmed := strings.TrimSpace(threadID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(conversationID)
}

func (s *Service) persistThreadSession(threadID string, conversationID string) {
	if s == nil || s.threadStore == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	conversationID = strings.TrimSpace(conversationID)
	if threadID == "" || conversationID == "" {
		return
	}
	if err := s.threadStore.SaveSession(ThreadSession{
		ThreadID:       threadID,
		ConversationID: conversationID,
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		s.logThreadStoreError("save_session", threadID, err)
	}
}

func (s *Service) appendThreadEvent(threadID string, evt threadEvent) {
	if s == nil || s.threadStore == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	if err := s.threadStore.AppendEvent(threadID, evt); err != nil {
		s.logThreadStoreError("append_event", threadID, err)
	}
}

func (s *Service) appendThreadUserMessage(threadID string, req TurnRequest) {
	if s == nil {
		return
	}
	text := strings.TrimSpace(lastUserText(req.Messages))
	if text == "" {
		return
	}
	s.appendThreadEvent(threadID, newThreadEvent("user_message", map[string]any{
		"content": text,
	}))
}

func (s *Service) appendThreadApprovalDecision(threadID string, approvalID string, decision string) {
	if s == nil {
		return
	}
	approvalID = strings.TrimSpace(approvalID)
	decision = strings.TrimSpace(decision)
	if approvalID == "" || decision == "" {
		return
	}
	s.appendThreadEvent(threadID, newThreadEvent("approval_decision", map[string]any{
		"approvalId": approvalID,
		"decision":   decision,
	}))
}

func (s *Service) appendThreadResponseEvents(threadID string, resp TurnResponse) {
	if s == nil {
		return
	}
	if msg := strings.TrimSpace(resp.AssistantMessage); msg != "" {
		s.appendThreadEvent(threadID, newThreadEvent("assistant_message", map[string]any{
			"content": msg,
		}))
	}
	if resp.Approval != nil {
		s.appendThreadEvent(threadID, newThreadEvent("approval_pending", summarizeThreadApproval(resp.Approval)))
	}
	if payload := summarizeThreadConsoleResult(resp.Effects.ConsoleResult); len(payload) > 0 {
		s.appendThreadEvent(threadID, newThreadEvent("tool_result_summary", payload))
	}
}

func (s *Service) logThreadStoreError(operation string, threadID string, err error) {
	if s == nil || err == nil {
		return
	}
	s.log(context.Background(), "thread_store_error", map[string]any{
		"operation": strings.TrimSpace(operation),
		"threadId":  strings.TrimSpace(threadID),
		"error":     err.Error(),
	})
}
