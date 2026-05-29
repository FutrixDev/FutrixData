package schemaprivacy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

const maxAuditLineBytes = 4 * 1024 * 1024

// AuditStore is a tiny append-only jsonl log dedicated to schema-to-LLM
// events. It deliberately does not depend on agentaudit because the egress
// being recorded happens with or without an agent access key (in-app AI Chat,
// SensitivityScan, schema knowledge ER generation), and because the event
// shape — entity counts, provider, decision — has nothing in common with a
// tool call.
type AuditStore struct {
	path string
	mu   sync.Mutex
	now  func() time.Time
}

// NewAuditStore returns a store backed by the given jsonl path. The file is
// created on first Append.
func NewAuditStore(path string) *AuditStore {
	return &AuditStore{path: strings.TrimSpace(path), now: time.Now}
}

// Append writes one entry. ID and CreatedAt are filled when blank so callers
// don't have to. Returns nil silently when the store has no path configured —
// this lets headless test harnesses skip wiring without the production code
// growing a sprinkling of nil checks.
func (s *AuditStore) Append(entry AuditEntry) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = newAuditID(s.now)
	}
	if strings.TrimSpace(entry.CreatedAt) == "" {
		entry.CreatedAt = s.now().UTC().Format(time.RFC3339)
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return securefile.AppendFile(s.path, payload, 0o644)
}

// List returns audit entries in newest-first order. Filtering happens after
// JSON decoding to keep the file format simple; the log size stays small
// because we record summaries, not payloads.
func (s *AuditStore) List(filter AuditFilter) ([]AuditEntry, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]AuditEntry, 0)
	err := securefile.WithPathLock(s.path, func() error {
		data, err := securefile.ReadFile(s.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), maxAuditLineBytes)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var entry AuditEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if !matchAuditFilter(entry, filter) {
				continue
			}
			items = append(items, entry)
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

// LastForDatasource returns the most recent entry for a datasource, or false
// when the log has none. Used by ConsentSummary to render "last sent at" in
// the UI without forcing the caller to scan the whole list itself.
func (s *AuditStore) LastForDatasource(datasourceID string) (AuditEntry, bool, error) {
	trimmed := strings.TrimSpace(datasourceID)
	if trimmed == "" {
		return AuditEntry{}, false, nil
	}
	items, err := s.List(AuditFilter{DatasourceID: trimmed, Limit: 1})
	if err != nil {
		return AuditEntry{}, false, err
	}
	if len(items) == 0 {
		return AuditEntry{}, false, nil
	}
	return items[0], true, nil
}

// LatestByDatasource scans the audit log once and returns the most recent
// entry per datasource. Callers rendering one row per datasource (the schema
// egress overview) should use this in place of a per-row LastForDatasource
// loop — N datasources × full-file scan becomes a single pass that holds the
// audit lock for one read.
func (s *AuditStore) LatestByDatasource() (map[string]AuditEntry, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return map[string]AuditEntry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	latest := map[string]AuditEntry{}
	err := securefile.WithPathLock(s.path, func() error {
		data, err := securefile.ReadFile(s.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), maxAuditLineBytes)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var entry AuditEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			id := strings.TrimSpace(entry.DatasourceID)
			if id == "" {
				continue
			}
			// Append-only with monotonically advancing CreatedAt: the last
			// line wins. We compare on CreatedAt anyway so out-of-order
			// writes (clock skew, manual edits) still settle on the newest
			// timestamp instead of file order.
			if prev, ok := latest[id]; ok && prev.CreatedAt > entry.CreatedAt {
				continue
			}
			latest[id] = entry
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func matchAuditFilter(entry AuditEntry, filter AuditFilter) bool {
	if filter.DatasourceID != "" && entry.DatasourceID != filter.DatasourceID {
		return false
	}
	if filter.TriggerSource != "" && entry.TriggerSource != filter.TriggerSource {
		return false
	}
	if filter.Status != "" && entry.Status != filter.Status {
		return false
	}
	return true
}

func newAuditID(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return "schemegress_" + strings.ReplaceAll(now().UTC().Format("20060102T150405.000000000"), ".", "")
}
