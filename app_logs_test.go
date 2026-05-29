package main

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/diagnostics"
	"futrixdata/platform/internal/observability"
)

func TestRecordClientErrorWritesErrorLog(t *testing.T) {
	root := t.TempDir()
	app := &App{
		errorLog: log.New(observability.NewLevelWriter(observability.Config{
			RootDir:  root,
			FileName: "error.log",
			MaxBytes: 1024,
		}), "", 0),
	}

	if err := app.RecordClientError("error", "boom", "stack"); err != nil {
		t.Fatalf("record client error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "error.log"))
	if err != nil {
		t.Fatalf("read error.log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "source=client") || !strings.Contains(content, `message="boom"`) {
		t.Fatalf("unexpected error log content: %q", content)
	}
}

func TestDiagnosticsSettingsTogglePersists(t *testing.T) {
	root := t.TempDir()
	app := &App{
		diagnostics: diagnostics.NewStore(filepath.Join(root, "diagnostics-settings.json")),
		infoLog: log.New(observability.NewLevelWriter(observability.Config{
			RootDir:  root,
			FileName: "info.log",
			MaxBytes: 1024,
		}), "", 0),
	}

	settings, err := app.GetDiagnosticsSettings()
	if err != nil {
		t.Fatalf("GetDiagnosticsSettings: %v", err)
	}
	if settings.DatasourceTimingLogEnabled {
		t.Fatal("default DatasourceTimingLogEnabled = true, want false")
	}

	settings, err = app.SetDatasourceTimingLogEnabled(true)
	if err != nil {
		t.Fatalf("SetDatasourceTimingLogEnabled: %v", err)
	}
	if !settings.DatasourceTimingLogEnabled {
		t.Fatal("DatasourceTimingLogEnabled = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(root, "info.log"))
	if err != nil {
		t.Fatalf("read info.log: %v", err)
	}
	if !strings.Contains(string(data), "event=datasource_timing_log_toggled enabled=true") {
		t.Fatalf("expected toggle log, got %q", string(data))
	}
}

func TestDatasourceTimingFinishLogsErrorDetails(t *testing.T) {
	root := t.TempDir()
	store := diagnostics.NewStore(filepath.Join(root, "diagnostics-settings.json"))
	if _, err := store.SetDatasourceTimingLogEnabled(true); err != nil {
		t.Fatalf("enable datasource timing: %v", err)
	}
	logger := log.New(observability.NewLevelWriter(observability.Config{
		RootDir:  root,
		FileName: "info.log",
		MaxBytes: 1024,
	}), "", 0)

	_, finish := newAppDatasourceTimingStarter(store, logger)(
		context.Background(),
		"app.test_datasource",
		datasource.DataSource{ID: "d1-prod", Type: datasource.TypeD1},
		"",
		console.ExecuteOptions{},
		false,
	)
	finish(errors.New("dial tcp 127.0.0.1:443: i/o timeout"))

	data, err := os.ReadFile(filepath.Join(root, "info.log"))
	if err != nil {
		t.Fatalf("read info.log: %v", err)
	}
	content := string(data)
	for _, want := range []string{`event="finish"`, `status="error"`, `error_kind="error"`, `error_message="dial tcp 127.0.0.1:443: i/o timeout"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %q in datasource timing finish log:\n%s", want, content)
		}
	}
}

func TestExportLogsBundlesExpectedFiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "info.log"), []byte("info"), 0o644); err != nil {
		t.Fatalf("write info.log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "error.log"), []byte("error"), 0o644); err != nil {
		t.Fatalf("write error.log: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "aichat"), 0o755); err != nil {
		t.Fatalf("mkdir aichat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "aichat", "2026-03-10.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write aichat log: %v", err)
	}

	app := &App{logsRoot: root}
	zipPath, err := app.ExportLogs()
	if err != nil {
		t.Fatalf("export logs: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
		if file.Name == "manifest.json" {
			rc, err := file.Open()
			if err != nil {
				t.Fatalf("open manifest: %v", err)
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			if !strings.Contains(string(body), `"logRoot"`) {
				t.Fatalf("expected manifest content, got %q", string(body))
			}
		}
	}

	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "info.log") || !strings.Contains(joined, "error.log") || !strings.Contains(joined, "aichat/2026-03-10.jsonl") {
		t.Fatalf("unexpected zip entries: %v", names)
	}
}

func TestExportLogsSkipsVanishedFiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "info.log"), []byte("info"), 0o644); err != nil {
		t.Fatalf("write info.log: %v", err)
	}
	broken := filepath.Join(root, "missing.log")
	if err := os.Symlink(filepath.Join(root, "rotated-away.log"), broken); err != nil {
		t.Fatalf("create broken symlink: %v", err)
	}

	app := &App{logsRoot: root}
	zipPath, err := app.ExportLogs()
	if err != nil {
		t.Fatalf("export logs with vanished file: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == "missing.log" {
			t.Fatalf("expected vanished file to be skipped from archive")
		}
	}
}

func TestExportLogsSucceedsWhenLogRootDoesNotExist(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	root := filepath.Join(t.TempDir(), "missing-logs")
	app := &App{logsRoot: root}

	zipPath, err := app.ExportLogs()
	if err != nil {
		t.Fatalf("export logs without log root: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
	}

	if len(names) != 1 || names[0] != "manifest.json" {
		t.Fatalf("expected manifest-only archive, got %v", names)
	}
}

func TestExportLogsSkipsSymlinkTargets(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	downloads := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "info.log"), []byte("info"), 0o644); err != nil {
		t.Fatalf("write info.log: %v", err)
	}

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "outside.log")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	app := &App{logsRoot: root}
	zipPath, err := app.ExportLogs()
	if err != nil {
		t.Fatalf("export logs with symlink: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == "outside.log" {
			t.Fatalf("expected symlink target to be skipped from archive")
		}
	}
}
