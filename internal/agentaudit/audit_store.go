package agentaudit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

const (
	StatusSuccess          = "success"
	StatusError            = "error"
	StatusApprovalRequired = "approval_required"
	maxAuditLineBytes      = 8 * 1024 * 1024
)

type AuditStore struct {
	path string
	mu   sync.Mutex
}

func NewAuditStore(path string) *AuditStore {
	return &AuditStore{path: path}
}

func (s *AuditStore) Append(entry AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = newAuditID()
	}
	if strings.TrimSpace(entry.ExecutedAt) == "" {
		entry.ExecutedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return securefile.WithPathLock(s.path, func() error {
		existing, err := securefile.ReadFile(s.path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		state, err := scanAuditChainForAppend(existing)
		if err != nil {
			return err
		}
		prevHash := AuditChainGenesisHash
		if state.chainStarted {
			prevHash = state.lastChainHash
		}
		entry, err = addHashChain(entry, state.totalRecords+1, prevHash)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		combined := append([]byte(nil), existing...)
		if len(bytes.TrimSpace(combined)) > 0 && !bytes.HasSuffix(combined, []byte("\n")) {
			combined = append(combined, '\n')
		}
		combined = append(combined, payload...)
		combined = append(combined, '\n')
		return securefile.WriteFile(s.path, combined, 0o644)
	})
}

func (s *AuditStore) List(filter AuditFilter) ([]AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]AuditEntry, 0)
	err := securefile.WithPathLock(s.path, func() error {
		data, err := securefile.ReadFile(s.path)
		if err != nil {
			if os.IsNotExist(err) {
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
			if !matchesAuditFilter(entry, filter) {
				continue
			}
			items = append(items, entry)
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	reverseAuditEntries(items)
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func matchesAuditFilter(entry AuditEntry, filter AuditFilter) bool {
	if filter.AccessKey != "" && entry.AccessKey != filter.AccessKey {
		return false
	}
	if filter.Protocol != "" && entry.Protocol != filter.Protocol {
		return false
	}
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	if keyword == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		entry.AccessKey,
		entry.ToolName,
		entry.Protocol,
		entry.Summary,
		entry.Statement,
		entry.DatasourceName,
		entry.DatasourceType,
		entry.Target,
		entry.Status,
		entry.Message,
	}, " "))
	return strings.Contains(haystack, keyword)
}

func reverseAuditEntries(items []AuditEntry) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func newAuditID() string {
	return "audit_" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}
