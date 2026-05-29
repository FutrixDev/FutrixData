package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SessionRecord struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

type SessionTracker struct {
	root         string
	path         string
	currentPID   func() int
	ownsPath     bool
	processAlive func(pid int) bool
}

func NewSessionTracker(root string) *SessionTracker {
	return &SessionTracker{
		root:         root,
		path:         filepath.Join(root, "runtime", "session.json"),
		currentPID:   os.Getpid,
		processAlive: processAlive,
	}
}

func (t *SessionTracker) Start() (bool, SessionRecord, error) {
	if t == nil {
		return false, SessionRecord{}, os.ErrInvalid
	}

	var previous SessionRecord
	abnormal := false
	currentPID := t.pid()
	markers, err := t.loadMarkers()
	if err != nil {
		return false, SessionRecord{}, err
	}
	for _, marker := range markers {
		if marker.record.PID == currentPID {
			continue
		}
		if marker.record.PID > 0 && t.processAlive != nil && t.processAlive(marker.record.PID) {
			if previous.PID == 0 {
				previous = marker.record
			}
			continue
		}
		if !abnormal || sessionRecordTime(marker.record).After(sessionRecordTime(previous)) {
			previous = marker.record
		}
		abnormal = true
		if err := os.Remove(marker.path); err != nil && !os.IsNotExist(err) {
			return false, SessionRecord{}, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return false, SessionRecord{}, err
	}

	current := SessionRecord{
		PID:       currentPID,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(current)
	if err != nil {
		return false, SessionRecord{}, err
	}
	if err := os.WriteFile(t.markerPath(currentPID), data, 0o644); err != nil {
		return false, SessionRecord{}, err
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		return false, SessionRecord{}, err
	}
	t.ownsPath = true
	return abnormal, previous, nil
}

func (t *SessionTracker) Close() error {
	if t == nil {
		return os.ErrInvalid
	}
	if !t.ownsPath {
		return nil
	}
	currentPID := t.pid()
	if err := os.Remove(t.markerPath(currentPID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := t.cleanupCurrentSessionFile(currentPID); err != nil {
		return err
	}
	t.ownsPath = false
	return nil
}

type sessionMarker struct {
	path   string
	record SessionRecord
}

func (t *SessionTracker) pid() int {
	if t != nil && t.currentPID != nil {
		if pid := t.currentPID(); pid > 0 {
			return pid
		}
	}
	return os.Getpid()
}

func (t *SessionTracker) runtimeDir() string {
	if t == nil {
		return ""
	}
	return filepath.Dir(t.path)
}

func (t *SessionTracker) markerPath(pid int) string {
	return filepath.Join(t.runtimeDir(), fmt.Sprintf("session-%d.json", pid))
}

func (t *SessionTracker) loadMarkers() ([]sessionMarker, error) {
	runtimeDir := t.runtimeDir()
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	markers := make([]sessionMarker, 0, len(entries)+1)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name != "session.json" && !isPerProcessSessionMarker(name) {
			continue
		}
		path := filepath.Join(runtimeDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var record SessionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		markers = append(markers, sessionMarker{path: path, record: record})
	}
	return markers, nil
}

func (t *SessionTracker) cleanupCurrentSessionFile(currentPID int) error {
	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var current SessionRecord
	if err := json.Unmarshal(data, &current); err != nil {
		return nil
	}
	if current.PID != currentPID {
		return nil
	}

	markers, err := t.loadMarkers()
	if err != nil {
		return err
	}

	var replacement SessionRecord
	for _, marker := range markers {
		if marker.path == t.path || marker.record.PID == currentPID {
			continue
		}
		if t.processAlive != nil && !t.processAlive(marker.record.PID) {
			continue
		}
		if replacement.PID == 0 || sessionRecordTime(marker.record).After(sessionRecordTime(replacement)) {
			replacement = marker.record
		}
	}
	if replacement.PID == 0 {
		if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	payload, err := json.Marshal(replacement)
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, payload, 0o644)
}

func isPerProcessSessionMarker(name string) bool {
	return strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json")
}

func sessionRecordTime(record SessionRecord) time.Time {
	startedAt, err := time.Parse(time.RFC3339Nano, record.StartedAt)
	if err != nil {
		return time.Time{}
	}
	return startedAt
}
