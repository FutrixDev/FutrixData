package aichat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

type ThreadSession struct {
	ThreadID       string    `json:"threadId"`
	ConversationID string    `json:"conversationId"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type threadEvent struct {
	ID        string         `json:"id,omitempty"`
	Kind      string         `json:"kind"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type threadStore interface {
	LoadSession(threadID string) (ThreadSession, error)
	SaveSession(session ThreadSession) error
	AppendEvent(threadID string, evt threadEvent) error
	LoadEvents(threadID string) ([]threadEventRecord, error)
	SaveEvents(threadID string, events []threadEventRecord) error
	LoadSummaryBlocks(threadID string) ([]ThreadSummaryBlock, error)
	SaveSummaryBlocks(threadID string, blocks []ThreadSummaryBlock) error
	LoadMemorySnapshot(threadID string) (ThreadMemorySnapshot, error)
	SaveMemorySnapshot(threadID string, snapshot ThreadMemorySnapshot) error
}

type fileThreadStore struct {
	root string
	mu   sync.Mutex
}

var threadIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func newFileThreadStore(root string) *fileThreadStore {
	return &fileThreadStore{root: strings.TrimSpace(root)}
}

func normalizeThreadID(threadID string) (string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "", errors.New("threadId is required")
	}
	if !threadIDPattern.MatchString(threadID) || threadID == "." || threadID == ".." {
		return "", errors.New("threadId contains unsafe characters")
	}
	return threadID, nil
}

func (s *fileThreadStore) threadFilePath(threadID string, name string) (string, error) {
	if s == nil {
		return "", errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, threadID, name), nil
}

func (s *fileThreadStore) threadDirPath(threadID string) (string, error) {
	if s == nil {
		return "", errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, threadID), nil
}

func (s *fileThreadStore) LoadSession(threadID string) (ThreadSession, error) {
	if s == nil {
		return ThreadSession{}, errors.New("thread store is nil")
	}
	path, err := s.threadFilePath(threadID, "session.json")
	if err != nil {
		return ThreadSession{}, err
	}
	data, err := securefile.ReadFile(path)
	if err != nil {
		return ThreadSession{}, err
	}
	var session ThreadSession
	if err := json.Unmarshal(data, &session); err != nil {
		return ThreadSession{}, err
	}
	return session, nil
}

func (s *fileThreadStore) SaveSession(session ThreadSession) error {
	if s == nil {
		return errors.New("thread store is nil")
	}
	session.ThreadID = strings.TrimSpace(session.ThreadID)
	session.ConversationID = strings.TrimSpace(session.ConversationID)
	if session.ThreadID == "" {
		return errors.New("threadId is required")
	}
	if session.ConversationID == "" {
		return errors.New("conversationId is required")
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}
	threadID, err := normalizeThreadID(session.ThreadID)
	if err != nil {
		return err
	}
	session.ThreadID = threadID

	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := s.threadDirPath(session.ThreadID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(filepath.Join(dir, "session.json"), append(data, '\n'), 0o644)
}

func (s *fileThreadStore) AppendEvent(threadID string, evt threadEvent) error {
	if s == nil {
		return errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return err
	}
	evt.Kind = strings.TrimSpace(evt.Kind)
	if evt.Kind == "" {
		return errors.New("event kind is required")
	}
	if strings.TrimSpace(evt.ID) == "" {
		evt.ID = newThreadEventID()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	line, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := s.threadDirPath(threadID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return securefile.AppendFile(filepath.Join(dir, "events.jsonl"), append(line, '\n'), 0o644)
}

func (s *fileThreadStore) LoadEvents(threadID string) ([]threadEventRecord, error) {
	if s == nil {
		return nil, errors.New("thread store is nil")
	}
	path, err := s.threadFilePath(threadID, "events.jsonl")
	if err != nil {
		return nil, err
	}
	data, err := securefile.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := splitNonEmptyLines(string(data))
	out := make([]threadEventRecord, 0, len(lines))
	for idx, line := range lines {
		var evt threadEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return nil, err
		}
		record := threadEventRecord{
			ID:        strings.TrimSpace(evt.ID),
			Kind:      strings.TrimSpace(evt.Kind),
			Timestamp: evt.Timestamp,
			Payload:   cloneMapAny(evt.Payload),
		}
		if record.ID == "" {
			record.ID = fallbackThreadEventRecordID(idx, record.Timestamp)
		}
		record.Summary = summarizeThreadEventRecord(record)
		out = append(out, record)
	}
	return out, nil
}

func (s *fileThreadStore) SaveEvents(threadID string, events []threadEventRecord) error {
	if s == nil {
		return errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := s.threadDirPath(threadID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	lines := make([]byte, 0, len(events)*128)
	for i := range events {
		evt := threadEvent{
			ID:        strings.TrimSpace(events[i].ID),
			Kind:      strings.TrimSpace(events[i].Kind),
			Timestamp: events[i].Timestamp,
			Payload:   cloneMapAny(events[i].Payload),
		}
		if evt.Kind == "" {
			return errors.New("event kind is required")
		}
		if evt.ID == "" {
			evt.ID = newThreadEventID()
		}
		if evt.Timestamp.IsZero() {
			evt.Timestamp = time.Now().UTC()
		}
		line, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		lines = append(lines, line...)
		lines = append(lines, '\n')
	}

	return securefile.WriteFile(filepath.Join(dir, "events.jsonl"), lines, 0o644)
}

func (s *fileThreadStore) LoadSummaryBlocks(threadID string) ([]ThreadSummaryBlock, error) {
	if s == nil {
		return nil, errors.New("thread store is nil")
	}
	path, err := s.threadFilePath(threadID, "summaries.json")
	if err != nil {
		return nil, err
	}
	data, err := securefile.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var blocks []ThreadSummaryBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (s *fileThreadStore) SaveSummaryBlocks(threadID string, blocks []ThreadSummaryBlock) error {
	if s == nil {
		return errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := s.threadDirPath(threadID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(filepath.Join(dir, "summaries.json"), append(data, '\n'), 0o644)
}

func (s *fileThreadStore) LoadMemorySnapshot(threadID string) (ThreadMemorySnapshot, error) {
	if s == nil {
		return ThreadMemorySnapshot{}, errors.New("thread store is nil")
	}
	path, err := s.threadFilePath(threadID, "memory_snapshot.json")
	if err != nil {
		return ThreadMemorySnapshot{}, err
	}
	data, err := securefile.ReadFile(path)
	if err != nil {
		return ThreadMemorySnapshot{}, err
	}
	var snapshot ThreadMemorySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return ThreadMemorySnapshot{}, err
	}
	return snapshot, nil
}

func (s *fileThreadStore) SaveMemorySnapshot(threadID string, snapshot ThreadMemorySnapshot) error {
	if s == nil {
		return errors.New("thread store is nil")
	}
	threadID, err := normalizeThreadID(threadID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := s.threadDirPath(threadID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(filepath.Join(dir, "memory_snapshot.json"), append(data, '\n'), 0o644)
}

func fallbackThreadEventRecordID(idx int, ts time.Time) string {
	return strings.TrimSpace(ts.UTC().Format("20060102T150405.000000000")) + "_" + strings.TrimSpace(strconv.Itoa(idx+1))
}

func splitNonEmptyLines(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
