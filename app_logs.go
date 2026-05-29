package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"futrixdata/platform/internal/observability"
)

const (
	defaultLogsMaxBytes   int64 = 50 * 1024 * 1024
	defaultRotateMaxBytes int64 = 5 * 1024 * 1024
)

func resolveLogsRoot(cfg Config) string {
	return filepath.Join(filepath.Dir(cfg.DataPath), "logs")
}

func newAppLogger(root, fileName string) *log.Logger {
	return log.New(observability.NewLevelWriter(observability.Config{
		RootDir:     root,
		FileName:    fileName,
		MaxBytes:    defaultLogsMaxBytes,
		RotateBytes: defaultRotateMaxBytes,
	}), "", log.LstdFlags|log.Lmicroseconds)
}

func configureProcessInfoLog(cfg Config) {
	root := resolveLogsRoot(cfg)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(observability.NewLevelWriter(observability.Config{
		RootDir:     root,
		FileName:    "info.log",
		MaxBytes:    defaultLogsMaxBytes,
		RotateBytes: defaultRotateMaxBytes,
	}))
}

func writeProcessErrorLog(cfg Config, format string, args ...any) {
	logger := newAppLogger(resolveLogsRoot(cfg), "error.log")
	logger.Printf(format, args...)
}

func (a *App) RecordClientError(kind, message, detail string) error {
	if a == nil {
		return os.ErrInvalid
	}
	logger := a.errorLog
	if logger == nil {
		logger = newAppLogger(a.logsRoot, "error.log")
		a.errorLog = logger
	}
	logger.Printf("level=error source=client kind=%s message=%s detail=%s",
		logField(kind),
		logField(message),
		logField(detail),
	)
	return nil
}

func (a *App) ExportLogs() (string, error) {
	if a == nil {
		return "", os.ErrInvalid
	}
	root := strings.TrimSpace(a.logsRoot)
	if root == "" {
		root = resolveLogsRoot(a.cfg)
	}
	exportDir, err := resolveExportDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("futrixdata-logs-%s.zip", time.Now().UTC().Format("20060102T150405"))
	target, err := nextExportFilePath(exportDir, name)
	if err != nil {
		return "", err
	}

	file, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	if err := writeLogManifest(zipWriter, root); err != nil {
		zipWriter.Close()
		return "", err
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		src, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer src.Close()
		w, err := zipWriter.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		return err
	}); err != nil {
		zipWriter.Close()
		return "", err
	}
	if err := zipWriter.Close(); err != nil {
		a.logErrorf("source=export event=export_logs_failed error=%s", logField(err.Error()))
		return "", err
	}
	a.logInfof("source=export event=export_logs_success path=%s", logField(target))
	return target, nil
}

func writeLogManifest(zipWriter *zip.Writer, root string) error {
	entry, err := zipWriter.Create("manifest.json")
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"exportedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"logRoot":    root,
	})
	if err != nil {
		return err
	}
	_, err = entry.Write(payload)
	return err
}

func logField(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return `""`
	}
	return strconvQuote(trimmed)
}

func strconvQuote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func (a *App) logInfof(format string, args ...any) {
	if a != nil && a.infoLog != nil {
		a.infoLog.Printf(format, args...)
	}
}

func (a *App) logErrorf(format string, args ...any) {
	if a != nil && a.errorLog != nil {
		a.errorLog.Printf(format, args...)
	}
}

func parseSessionStartedAt(value string) time.Time {
	startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return startedAt
}
