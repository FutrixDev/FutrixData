package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	RootDir     string
	FileName    string
	MaxBytes    int64
	RotateBytes int64
}

type LevelWriter struct {
	rootDir     string
	fileName    string
	maxBytes    int64
	rotateBytes int64
	mu          *sync.Mutex
}

var levelWriterLocks sync.Map

func NewLevelWriter(cfg Config) *LevelWriter {
	rootDir := strings.TrimSpace(cfg.RootDir)
	fileName := strings.TrimSpace(cfg.FileName)
	lockPath := ""
	if rootDir != "" && fileName != "" {
		lockPath = filepath.Join(rootDir, fileName)
	}
	return &LevelWriter{
		rootDir:     rootDir,
		fileName:    fileName,
		maxBytes:    cfg.MaxBytes,
		rotateBytes: cfg.RotateBytes,
		mu:          sharedFileLock(lockPath),
	}
}

func sharedFileLock(path string) *sync.Mutex {
	key := strings.TrimSpace(path)
	if key == "" {
		return &sync.Mutex{}
	}
	key = filepath.Clean(key)
	if existing, ok := levelWriterLocks.Load(key); ok {
		return existing.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	actual, _ := levelWriterLocks.LoadOrStore(key, mu)
	return actual.(*sync.Mutex)
}

func (w *LevelWriter) Write(p []byte) (int, error) {
	if w == nil || w.rootDir == "" || w.fileName == "" {
		return 0, os.ErrInvalid
	}

	if w.mu == nil {
		w.mu = sharedFileLock(filepath.Join(w.rootDir, w.fileName))
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(w.rootDir, 0o755); err != nil {
		return 0, err
	}

	path := filepath.Join(w.rootDir, w.fileName)
	if err := w.rotateIfNeeded(path, int64(len(p))); err != nil {
		return 0, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := f.Write(p)
	if err != nil {
		return n, err
	}
	if err := PruneLogs(w.rootDir, w.maxBytes, DefaultPreserveBaseNames()); err != nil {
		return n, err
	}
	return n, nil
}

func (w *LevelWriter) rotateIfNeeded(path string, incoming int64) error {
	if w.rotateBytes <= 0 {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size()+incoming <= w.rotateBytes {
		return nil
	}

	ext := filepath.Ext(w.fileName)
	base := strings.TrimSuffix(w.fileName, ext)
	archive := filepath.Join(w.rootDir, fmt.Sprintf("%s-%s%s", base, time.Now().UTC().Format("20060102T150405.000000000"), ext))
	return os.Rename(path, archive)
}
