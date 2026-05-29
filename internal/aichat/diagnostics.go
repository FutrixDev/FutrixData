package aichat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Diagnostics is an optional hook for emitting structured debug events.
// Implementations must be safe for concurrent use.
type Diagnostics interface {
	Log(event string, fields map[string]any)
}

type FileDiagnostics struct {
	dir        string
	includeRaw bool
	afterWrite func()
	mu         sync.Mutex
}

type FileDiagnosticsConfig struct {
	Dir        string
	IncludeRaw bool
	AfterWrite func()
}

func NewFileDiagnostics(cfg FileDiagnosticsConfig) *FileDiagnostics {
	return &FileDiagnostics{
		dir:        stringsTrimSpace(cfg.Dir),
		includeRaw: cfg.IncludeRaw,
		afterWrite: cfg.AfterWrite,
	}
}

func (d *FileDiagnostics) IncludeRaw() bool {
	if d == nil {
		return false
	}
	return d.includeRaw
}

func (d *FileDiagnostics) Log(event string, fields map[string]any) {
	if d == nil {
		return
	}
	dir := stringsTrimSpace(d.dir)
	if dir == "" {
		return
	}

	record := make(map[string]any, 8+len(fields))
	record["ts"] = time.Now().Format(time.RFC3339Nano)
	record["event"] = event
	for k, v := range fields {
		if k == "" {
			continue
		}
		record[k] = v
	}

	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(line, '\n'))
	if d.afterWrite != nil {
		d.afterWrite()
	}
}

func stringsTrimSpace(value string) string {
	start := 0
	end := len(value)
	for start < end {
		ch := value[start]
		if ch != ' ' && ch != '\n' && ch != '\r' && ch != '\t' {
			break
		}
		start++
	}
	for end > start {
		ch := value[end-1]
		if ch != ' ' && ch != '\n' && ch != '\r' && ch != '\t' {
			break
		}
		end--
	}
	if start == 0 && end == len(value) {
		return value
	}
	return value[start:end]
}
