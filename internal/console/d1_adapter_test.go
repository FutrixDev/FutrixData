package console

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/datasource"
)

func TestD1Adapter_CloudExecute_WithAPIToken(t *testing.T) {
	var capturedAuth string
	var capturedPath string
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &capturedBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"value": 1},
					},
					"meta": map[string]any{"duration": 3},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.Execute(context.Background(), ds, "SELECT 1 AS value", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedAuth != "Bearer token_123" {
		t.Fatalf("expected bearer token, got %q", capturedAuth)
	}
	if capturedPath != "/client/v4/accounts/acc_123/d1/database/db_123/query" {
		t.Fatalf("unexpected path: %q", capturedPath)
	}
	if sql := strings.TrimSpace(asString(capturedBody["sql"])); !strings.HasPrefix(sql, "SELECT 1 AS value") {
		t.Fatalf("expected sql payload prefix, got %q", sql)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected row count 1, got %d", result.RowCount)
	}
	if len(result.Rows) != 1 || asString(result.Rows[0]["value"]) != "1" {
		t.Fatalf("unexpected rows: %#v", result.Rows)
	}
}

func TestD1Adapter_CloudExecute_UsesWranglerToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"ok": true},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()
	adapter.resolveWranglerToken = func(_ context.Context, command []string) (string, error) {
		if len(command) == 0 {
			t.Fatalf("expected wrangler command")
		}
		return "wrangler_token_123", nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "wrangler",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "SELECT 1", ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedAuth != "Bearer wrangler_token_123" {
		t.Fatalf("expected wrangler token, got %q", capturedAuth)
	}
}

func TestD1Adapter_LocalExecute_RunsWranglerCommand(t *testing.T) {
	var captured []string

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = append([]string{}, command...)
		return []byte(`[{"success":true,"results":[{"value":1}],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	result, err := adapter.Execute(context.Background(), ds, "SELECT 1 AS value", ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "d1 execute DB") {
		t.Fatalf("expected d1 execute command, got %q", joined)
	}
	if !strings.Contains(joined, "--local") || !strings.Contains(joined, "--json") || !strings.Contains(joined, "--command") {
		t.Fatalf("expected wrangler local json command flags, got %q", joined)
	}
	if !strings.Contains(joined, "SELECT 1 AS value") {
		t.Fatalf("expected statement in command, got %q", joined)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected row count 1, got %d", result.RowCount)
	}
}

func TestD1Adapter_Execute_DefaultsToRemoteWithoutMode(t *testing.T) {
	var capturedAuth string
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"value": 1},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "SELECT 1 AS value", ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedAuth != "Bearer token_123" {
		t.Fatalf("expected bearer token, got %q", capturedAuth)
	}
	if capturedPath != "/client/v4/accounts/acc_123/d1/database/db_123/query" {
		t.Fatalf("unexpected path: %q", capturedPath)
	}
}

func TestD1Adapter_ExecuteQuery_PopulatesSourceEntityFallback(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, _ []string) ([]byte, error) {
		return []byte(`[{"success":true,"results":[{"email":"user@example.com","total":120}],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	result, err := adapter.Execute(
		context.Background(),
		ds,
		"SELECT u.email, o.total FROM users u JOIN orders o ON u.id = o.user_id",
		ExecuteOptions{},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.SourceEntity != "users,orders" {
		t.Fatalf("expected fallback source entity hint, got %q", result.SourceEntity)
	}
}

func TestD1Adapter_Execute_UsesDevExecutionMode(t *testing.T) {
	var captured []string

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = append([]string{}, command...)
		return []byte(`[{"success":true,"results":[{"value":1}],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     "/tmp",
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": "/tmp/wrangler.toml",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "SELECT 1 AS value", ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "d1 execute analytics") {
		t.Fatalf("expected d1 execute analytics command, got %q", joined)
	}
	if !strings.Contains(joined, "--local") || !strings.Contains(joined, "--command") {
		t.Fatalf("expected wrangler local command flags, got %q", joined)
	}
	if !strings.Contains(joined, "--config /tmp/wrangler.toml") {
		t.Fatalf("expected wrangler config flag, got %q", joined)
	}
}

func TestD1Adapter_Execute_DevSchemaChangeWritesMigrationFile(t *testing.T) {
	var captured []string

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	if err := os.WriteFile(configPath, []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = append([]string{}, command...)
		return []byte(`[{"success":true,"results":[],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     projectDir,
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/app-logs",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "CREATE TABLE metrics (id INTEGER PRIMARY KEY);", ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "d1 execute analytics") {
		t.Fatalf("expected d1 execute command, got %q", joined)
	}

	files, err := filepath.Glob(filepath.Join(projectDir, "migrations", "app-logs", "*.sql"))
	if err != nil {
		t.Fatalf("glob migration files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one migration file, got %d (%#v)", len(files), files)
	}
	filename := filepath.Base(files[0])
	if !strings.Contains(filename, "create_table_metrics") {
		t.Fatalf("expected migration filename to include create_table_metrics, got %q", filename)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}
	content := strings.TrimSpace(string(raw))
	if !strings.Contains(content, "CREATE TABLE metrics") {
		t.Fatalf("expected migration file to include CREATE TABLE statement, got %q", content)
	}
}

func TestD1Adapter_Execute_DevNonSchemaStatementDoesNotWriteMigrationFile(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	if err := os.WriteFile(configPath, []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		return []byte(`[{"success":true,"results":[{"id":1}],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     projectDir,
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/app-logs",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "INSERT INTO metrics(id) VALUES (1);", ExecuteOptions{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(projectDir, "migrations", "app-logs", "*.sql"))
	if err != nil {
		t.Fatalf("glob migration files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no migration files for non-schema statement, got %#v", files)
	}
}

func TestD1Adapter_Execute_DevSchemaFailureDoesNotWriteMigrationFile(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	configPath := filepath.Join(projectDir, "wrangler.toml")
	if err := os.WriteFile(configPath, []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		return nil, errors.New("sqlite syntax error")
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     projectDir,
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/app-logs",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "CREATE TABLE metrics (id INTEGER PRIMARY KEY);", ExecuteOptions{}); err == nil {
		t.Fatalf("expected execute error")
	}

	files, err := filepath.Glob(filepath.Join(projectDir, "migrations", "app-logs", "*.sql"))
	if err != nil {
		t.Fatalf("glob migration files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no migration files when schema execution fails, got %#v", files)
	}
}

func TestD1Adapter_DeployMigrations_UsesRemoteApplyCommand(t *testing.T) {
	var captured []string

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = append([]string{}, command...)
		return []byte("deployed"), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     "/tmp/project",
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": "/tmp/project/wrangler.toml",
			"migrationsDir":      "migrations/app-logs",
		},
	}

	if err := adapter.DeployMigrations(context.Background(), ds); err != nil {
		t.Fatalf("DeployMigrations: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "d1 migrations apply analytics") {
		t.Fatalf("expected migrations apply command, got %q", joined)
	}
	if !strings.Contains(joined, "--remote") {
		t.Fatalf("expected --remote flag, got %q", joined)
	}
	if !strings.Contains(joined, "--config /tmp/project/wrangler.toml") {
		t.Fatalf("expected config flag, got %q", joined)
	}
}

func TestD1ExecutionMode_DevRequestedWithoutSupportFallsBackToRemote(t *testing.T) {
	mode := d1ExecutionMode(map[string]any{
		"executionMode": "dev",
		"databaseId":    "db_123",
		"databaseName":  "analytics",
	})
	if mode != d1ExecutionRemote {
		t.Fatalf("expected remote mode when datasource does not support dev, got %q", mode)
	}
}

func TestD1ExecutionMode_DevLegacyWranglerConfigKeepsDev(t *testing.T) {
	mode := d1ExecutionMode(map[string]any{
		"executionMode":      "dev",
		"wranglerConfigPath": "/tmp/project/wrangler.toml",
		"databaseId":         "db_legacy",
		"databaseName":       "legacy-db",
	})
	if mode != d1ExecutionDev {
		t.Fatalf("expected dev mode for legacy datasource with wranglerConfigPath, got %q", mode)
	}
}

func TestD1Adapter_Execute_DevSchemaStatErrorReturnsQuickly(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	migrationsDir := filepath.Join(projectDir, "migrations", "app-logs")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("mkdir migrations dir: %v", err)
	}
	if err := os.Chmod(migrationsDir, 0o000); err != nil {
		t.Fatalf("chmod migrations dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(migrationsDir, 0o755)
	})
	configPath := filepath.Join(projectDir, "wrangler.toml")
	if err := os.WriteFile(configPath, []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		return []byte(`[{"success":true,"results":[],"meta":{"duration":2}}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_dev",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"executionMode":      "dev",
			"supportDev":         true,
			"devProjectPath":     projectDir,
			"databaseId":         "db_123",
			"databaseName":       "analytics",
			"wranglerConfigPath": configPath,
			"migrationsDir":      "migrations/app-logs",
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := adapter.Execute(context.Background(), ds, "CREATE TABLE metrics (id INTEGER PRIMARY KEY);", ExecuteOptions{})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected execute to fail when stat returns permission error")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("execute hung when stat returned unexpected error")
	}
}

func TestD1Adapter_LocalExecute_RejectsUnsafeWranglerCommand(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		return []byte(`[{"success":true,"results":[{"value":1}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":            "local",
			"binding":         "DB",
			"databaseId":      "local-db-id",
			"wranglerCommand": "bash -lc",
		},
	}

	_, err := adapter.Execute(context.Background(), ds, "SELECT 1", ExecuteOptions{})
	if err == nil {
		t.Fatalf("expected error for unsafe wrangler command")
	}
	if !strings.Contains(err.Error(), "wrangler command") {
		t.Fatalf("expected wrangler command validation error, got %v", err)
	}
}

func TestD1Adapter_Explain_ParsesIndexUsage(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		joined := strings.Join(command, " ")
		if !strings.Contains(strings.ToUpper(joined), "EXPLAIN QUERY PLAN") {
			t.Fatalf("expected explain query plan command, got %q", joined)
		}
		return []byte(`[{"success":true,"results":[{"detail":"SEARCH users USING COVERING INDEX idx_users_name (name=?)"}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	result, err := adapter.Explain(context.Background(), ds, "SELECT * FROM users WHERE name = 'alice'")
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !result.UsesIndex {
		t.Fatalf("expected UsesIndex=true, got false")
	}
	if len(result.Indexes) != 1 || result.Indexes[0] != "idx_users_name" {
		t.Fatalf("unexpected indexes: %#v", result.Indexes)
	}
}

func TestD1Adapter_Explain_QueryPlanPrefix_NotDuplicated(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		joinedUpper := strings.ToUpper(strings.Join(command, " "))
		if strings.Count(joinedUpper, "QUERY PLAN") != 1 {
			t.Fatalf("expected single QUERY PLAN prefix, got %q", joinedUpper)
		}
		return []byte(`[{"success":true,"results":[{"detail":"SCAN users"}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	if _, err := adapter.Explain(context.Background(), ds, "QUERY PLAN SELECT * FROM users"); err != nil {
		t.Fatalf("Explain: %v", err)
	}
}

func TestD1Adapter_Explain_ExistingExplainPrefix_WithNewline_NotDuplicated(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		joinedUpper := strings.ToUpper(strings.Join(command, " "))
		if strings.Count(joinedUpper, "EXPLAIN") != 1 {
			t.Fatalf("expected single EXPLAIN prefix, got %q", joinedUpper)
		}
		return []byte(`[{"success":true,"results":[{"detail":"SCAN users"}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	if _, err := adapter.Explain(context.Background(), ds, "EXPLAIN\nSELECT * FROM users"); err != nil {
		t.Fatalf("Explain: %v", err)
	}
}

func TestD1Adapter_ListEntities_HidesSystemTablesByDefault(t *testing.T) {
	var capturedSQL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		capturedSQL = strings.TrimSpace(asString(payload["sql"]))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"name": "_cf_KV"},
						{"name": "orders"},
						{"name": "sqlite_sequence"},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	entities, err := adapter.ListEntities(context.Background(), ds, ListOptions{})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(entities) != 1 || entities[0] != "orders" {
		t.Fatalf("expected only business tables, got %#v", entities)
	}
	if !strings.Contains(capturedSQL, "name NOT LIKE 'sqlite_%'") {
		t.Fatalf("expected sqlite system table filter in statement, got %q", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "name NOT LIKE '\\_cf\\_%' ESCAPE '\\'") {
		t.Fatalf("expected cloudflare system table filter in statement, got %q", capturedSQL)
	}
}

func TestD1Adapter_ListEntitiesPage_HidesSystemTablesByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"name": "_cf_KV"},
						{"name": "users"},
						{"name": "sqlite_sequence"},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	page, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 2}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != "users" {
		t.Fatalf("expected only business tables, got %#v", page.Items)
	}
	if !page.Done || page.Cursor != "" {
		t.Fatalf("expected paging done for one visible table, got %#v", page)
	}
}

func TestD1Adapter_ListEntitiesPage_ReturnsViewKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": []map[string]any{
						{"name": "conversion_stats", "type": "view"},
						{"name": "conversions", "type": "table"},
						{"name": "rate_limits", "type": "table"},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	page, err := adapter.ListEntitiesPage(context.Background(), ds, ListOptions{Limit: 200}, "")
	if err != nil {
		t.Fatalf("ListEntitiesPage: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(page.Items), page.Items)
	}
	if len(page.Kinds) != 1 {
		t.Fatalf("expected 1 kind entry for view, got %d: %v", len(page.Kinds), page.Kinds)
	}
	if page.Kinds["conversion_stats"] != "view" {
		t.Fatalf("expected conversion_stats to be view, got %q", page.Kinds["conversion_stats"])
	}
}

func TestD1Adapter_DescribeEntity_IncludesPrimaryKeyFromTableInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		var results []map[string]any
		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('users') LIMIT 1":
			results = []map[string]any{
				{"type": "table", "sql": "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"},
			}
		case "PRAGMA table_info('users')":
			results = []map[string]any{
				{"name": "id", "type": "INTEGER", "notnull": 1, "pk": 1},
				{"name": "name", "type": "TEXT", "notnull": 0, "pk": 0},
			}
		case "PRAGMA index_list('users')":
			results = []map[string]any{
				{"name": "idx_users_name", "unique": 0},
			}
		case "PRAGMA index_info('idx_users_name')":
			results = []map[string]any{
				{"seqno": 0, "name": "name"},
			}
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": results,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.DescribeEntity(context.Background(), ds, "users")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}

	var primary IndexInfo
	hasPrimary := false
	var secondary IndexInfo
	hasSecondary := false
	for _, idx := range result.Indexes {
		if idx.Name == "PRIMARY" {
			primary = idx
			hasPrimary = true
		}
		if idx.Name == "idx_users_name" {
			secondary = idx
			hasSecondary = true
		}
	}
	if !hasPrimary {
		t.Fatalf("expected PRIMARY index in describe result, got %#v", result.Indexes)
	}
	if primary.Column != "id" {
		t.Fatalf("expected PRIMARY column id, got %q", primary.Column)
	}
	if !primary.Unique {
		t.Fatalf("expected PRIMARY index to be unique")
	}
	if !hasSecondary {
		t.Fatalf("expected secondary index to remain in describe result, got %#v", result.Indexes)
	}
	if secondary.Column != "name" {
		t.Fatalf("expected secondary index column name, got %q", secondary.Column)
	}
	if result.EntityKind != "table" {
		t.Fatalf("EntityKind = %q, want table", result.EntityKind)
	}
	if result.DefinitionSQL != "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)" {
		t.Fatalf("DefinitionSQL = %q", result.DefinitionSQL)
	}
}

func TestD1Adapter_DescribeEntity_ReturnsViewMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		var results []map[string]any
		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('conversion_stats') LIMIT 1":
			results = []map[string]any{
				{
					"type": "view",
					"sql":  "CREATE VIEW conversion_stats AS SELECT format, COUNT(*) AS total_count FROM conversions GROUP BY format",
				},
			}
		case "PRAGMA table_info('conversion_stats')":
			results = []map[string]any{
				{"name": "format", "type": "TEXT", "notnull": 0, "pk": 0},
				{"name": "total_count", "type": "INTEGER", "notnull": 0, "pk": 0},
			}
		case "PRAGMA index_list('conversion_stats')":
			results = []map[string]any{}
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": results,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.DescribeEntity(context.Background(), ds, "conversion_stats")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if result.EntityKind != "view" {
		t.Fatalf("EntityKind = %q, want view", result.EntityKind)
	}
	if result.DefinitionSQL == "" {
		t.Fatal("expected DefinitionSQL for view")
	}
	if len(result.Columns) != 2 {
		t.Fatalf("expected view columns, got %#v", result.Columns)
	}
	if len(result.Indexes) != 0 {
		t.Fatalf("expected no indexes for view, got %#v", result.Indexes)
	}
}

func TestD1Adapter_DescribeEntity_MatchesMixedCaseMetadataName(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		statement := ""
		for i := 0; i < len(command)-1; i++ {
			if command[i] == "--command" {
				statement = strings.TrimSpace(command[i+1])
				break
			}
		}
		switch statement {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('conversionstats') LIMIT 1":
			return []byte(`[{"results":[{"type":"view","sql":"CREATE VIEW ConversionStats AS SELECT * FROM conversions"}],"success":true}]`), nil
		case "PRAGMA table_info('conversionstats')":
			return []byte(`[{"results":[{"name":"id","type":"INTEGER","notnull":1,"pk":1}],"success":true}]`), nil
		case "PRAGMA index_list('conversionstats')":
			return []byte(`[{"results":[],"success":true}]`), nil
		default:
			t.Fatalf("unexpected statement: %s", statement)
			return nil, nil
		}
	}

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}
	result, err := adapter.DescribeEntity(context.Background(), ds, "conversionstats")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if result.EntityKind != "view" {
		t.Fatalf("EntityKind = %q, want view", result.EntityKind)
	}
	if !strings.Contains(result.DefinitionSQL, "ConversionStats") {
		t.Fatalf("DefinitionSQL = %q, want mixed-case view definition", result.DefinitionSQL)
	}
}

func TestD1Adapter_DescribeEntity_StripsSchemaQualificationForMetadataLookup(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		statement := ""
		for i := 0; i < len(command)-1; i++ {
			if command[i] == "--command" {
				statement = strings.TrimSpace(command[i+1])
				break
			}
		}
		switch statement {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('conversion_stats') LIMIT 1":
			return []byte(`[{"results":[{"type":"view","sql":"CREATE VIEW conversion_stats AS SELECT * FROM conversions"}],"success":true}]`), nil
		case "PRAGMA table_info('conversion_stats')":
			return []byte(`[{"results":[{"name":"format","type":"TEXT","notnull":0,"pk":0}],"success":true}]`), nil
		case "PRAGMA index_list('conversion_stats')":
			return []byte(`[{"results":[],"success":true}]`), nil
		default:
			t.Fatalf("unexpected statement: %s", statement)
			return nil, nil
		}
	}

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}
	result, err := adapter.DescribeEntity(context.Background(), ds, "main.conversion_stats")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if result.EntityKind != "view" {
		t.Fatalf("EntityKind = %q, want view", result.EntityKind)
	}
	if result.DefinitionSQL == "" {
		t.Fatal("expected DefinitionSQL for schema-qualified view")
	}
}

func TestD1Adapter_DescribeEntity_PreservesQuotedDottedNameForMetadataLookup(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		statement := ""
		for i := 0; i < len(command)-1; i++ {
			if command[i] == "--command" {
				statement = strings.TrimSpace(command[i+1])
				break
			}
		}
		switch statement {
		case `SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('foo.bar') LIMIT 1`:
			return []byte(`[{"results":[{"type":"view","sql":"CREATE VIEW \"foo.bar\" AS SELECT * FROM conversions"}],"success":true}]`), nil
		case `PRAGMA table_info('foo.bar')`:
			return []byte(`[{"results":[{"name":"format","type":"TEXT","notnull":0,"pk":0}],"success":true}]`), nil
		case `PRAGMA index_list('foo.bar')`:
			return []byte(`[{"results":[],"success":true}]`), nil
		default:
			t.Fatalf("unexpected statement: %s", statement)
			return nil, nil
		}
	}

	ds := datasource.DataSource{
		ID:   "ds1",
		Type: datasource.TypeD1,
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}
	result, err := adapter.DescribeEntity(context.Background(), ds, `"foo.bar"`)
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}
	if result.EntityKind != "view" {
		t.Fatalf("EntityKind = %q, want view", result.EntityKind)
	}
	if !strings.Contains(result.DefinitionSQL, `"foo.bar"`) {
		t.Fatalf("DefinitionSQL = %q, want quoted dotted name", result.DefinitionSQL)
	}
	if len(result.Columns) != 1 {
		t.Fatalf("expected quoted dotted view columns, got %#v", result.Columns)
	}
}

func TestD1Adapter_DescribeEntity_PrimaryKeyColumnOrderByPkOrdinal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		var results []map[string]any
		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('events') LIMIT 1":
			results = []map[string]any{
				{"type": "table", "sql": "CREATE TABLE events (tenant_id TEXT, id INTEGER, payload TEXT, PRIMARY KEY (tenant_id, id))"},
			}
		case "PRAGMA table_info('events')":
			results = []map[string]any{
				{"name": "id", "type": "INTEGER", "notnull": 1, "pk": 2},
				{"name": "tenant_id", "type": "TEXT", "notnull": 1, "pk": 1},
				{"name": "payload", "type": "TEXT", "notnull": 0, "pk": 0},
			}
		case "PRAGMA index_list('events')":
			results = []map[string]any{}
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result": []any{
				map[string]any{
					"success": true,
					"results": results,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.DescribeEntity(context.Background(), ds, "events")
	if err != nil {
		t.Fatalf("DescribeEntity: %v", err)
	}

	for _, idx := range result.Indexes {
		if idx.Name != "PRIMARY" {
			continue
		}
		if idx.Column != "tenant_id, id" {
			t.Fatalf("expected PRIMARY columns ordered by pk ordinal, got %q", idx.Column)
		}
		return
	}
	t.Fatalf("expected PRIMARY index in describe result, got %#v", result.Indexes)
}

func TestD1Adapter_DescribeEntity_IgnoresSQLiteAuthOnIndexList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('_cf_KV') LIMIT 1":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		case "PRAGMA table_info('_cf_KV')":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []any{
					map[string]any{
						"success": true,
						"results": []map[string]any{
							{"name": "key", "type": "TEXT", "notnull": 1, "pk": 1},
							{"name": "value", "type": "BLOB", "notnull": 0, "pk": 0},
						},
					},
				},
			})
		case "PRAGMA index_list('_cf_KV')":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.DescribeEntity(context.Background(), ds, "_cf_KV")
	if err != nil {
		t.Fatalf("DescribeEntity should tolerate SQLITE_AUTH during index introspection: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Fatalf("expected columns from PRAGMA table_info, got %#v", result.Columns)
	}
	if len(result.Indexes) != 1 || result.Indexes[0].Name != "PRIMARY" {
		t.Fatalf("expected synthesized PRIMARY index from PK metadata, got %#v", result.Indexes)
	}
}

func TestD1Adapter_DescribeEntity_IgnoresSQLiteAuthOnTableInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('_cf_KV') LIMIT 1":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		case "PRAGMA table_info('_cf_KV')":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		case "PRAGMA index_list('_cf_KV')":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	result, err := adapter.DescribeEntity(context.Background(), ds, "_cf_KV")
	if err != nil {
		t.Fatalf("DescribeEntity should tolerate SQLITE_AUTH during table introspection: %v", err)
	}

	if len(result.Columns) != 0 {
		t.Fatalf("expected empty columns when table_info is unauthorized, got %#v", result.Columns)
	}
	if len(result.Indexes) != 0 {
		t.Fatalf("expected empty indexes when index_list is unauthorized, got %#v", result.Indexes)
	}
}

func TestD1Adapter_DescribeEntity_ReturnsSQLiteAuthOnUserTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		sql := strings.TrimSpace(asString(payload["sql"]))

		switch sql {
		case "SELECT type, sql FROM sqlite_master WHERE lower(name) = lower('users') LIMIT 1":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		case "PRAGMA table_info('users')":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		case "PRAGMA index_list('users')":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"code":7500,"message":"not authorized: SQLITE_AUTH"}],"success":false,"messages":[],"result":[]}`))
		default:
			t.Fatalf("unexpected sql: %q", sql)
		}
	}))
	t.Cleanup(srv.Close)

	adapter := NewD1Adapter()
	adapter.httpClient = srv.Client()

	ds := datasource.DataSource{
		ID:   "ds_d1",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "cloud",
			"accountId":  "acc_123",
			"databaseId": "db_123",
			"authMode":   "token",
			"apiToken":   "token_123",
			"apiBaseURL": srv.URL + "/client/v4",
		},
	}

	_, err := adapter.DescribeEntity(context.Background(), ds, "users")
	if err == nil {
		t.Fatalf("DescribeEntity should return auth error for non-system tables")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "SQLITE_AUTH") {
		t.Fatalf("expected SQLITE_AUTH error, got %v", err)
	}
}

func TestD1Adapter_Explain_RecognizesIntegerPrimaryKeyAccess(t *testing.T) {
	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, _ []string) ([]byte, error) {
		return []byte(`[{"success":true,"results":[{"detail":"SEARCH users USING INTEGER PRIMARY KEY (rowid=?)"}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	result, err := adapter.Explain(context.Background(), ds, "SELECT * FROM users WHERE rowid = 1")
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !result.UsesIndex {
		t.Fatalf("expected UsesIndex=true for INTEGER PRIMARY KEY access")
	}
}

func TestD1IsQueryStatement_PragmaIsNotPaginatedQuery(t *testing.T) {
	if d1IsQueryStatement("pragma table_info('users')") {
		t.Fatalf("expected pragma to bypass paginated query path")
	}
	if !d1IsQueryStatement("select * from users") {
		t.Fatalf("expected select to use query path")
	}
}

func TestD1RunCommand_IgnoresStderrWhenSuccessful(t *testing.T) {
	output, err := d1RunCommand(context.Background(), []string{
		"sh",
		"-c",
		"echo 'warn: noisy stderr' 1>&2; printf '[{\"success\":true}]'",
	})
	if err != nil {
		t.Fatalf("d1RunCommand: %v", err)
	}
	raw := strings.TrimSpace(string(output))
	if raw != `[{"success":true}]` {
		t.Fatalf("expected stdout json only, got %q", raw)
	}
	if strings.Contains(raw, "warn: noisy stderr") {
		t.Fatalf("stderr noise should not be mixed into stdout: %q", raw)
	}
}

func TestD1IsQueryStatement_WithCteWriteBypassesPagination(t *testing.T) {
	if !d1IsQueryStatement("with cte as (select 1 as id) select * from cte") {
		t.Fatalf("expected CTE select to use query path")
	}
	if d1IsQueryStatement("with cte as (select 1 as id) insert into users(id) select id from cte") {
		t.Fatalf("expected CTE insert to bypass paginated query path")
	}
	if d1IsQueryStatement("with recursive cte(x) as (select 1 union all select x + 1 from cte where x < 3) update users set id = id") {
		t.Fatalf("expected CTE update to bypass paginated query path")
	}
	if !d1IsQueryStatement("with cte as (select 1 as id) -- update\nselect * from cte") {
		t.Fatalf("expected CTE select with inline line comment to use query path")
	}
	if d1IsQueryStatement("with cte as (select 1 as id) /* select */ update users set id = id") {
		t.Fatalf("expected CTE update with inline block comment to bypass paginated query path")
	}
}

func TestD1Adapter_Execute_ClampsPageSize(t *testing.T) {
	var captured string

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = strings.Join(command, " ")
		return []byte(`[{"success":true,"results":[{"id":1}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}

	if _, err := adapter.Execute(context.Background(), ds, "SELECT * FROM users", ExecuteOptions{PageSize: 5000}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(captured, "LIMIT 2001 OFFSET 0") {
		t.Fatalf("expected clamped LIMIT, got %q", captured)
	}
}

func TestD1Adapter_Execute_ClampsPagingTokenPageSize(t *testing.T) {
	var captured string

	adapter := NewD1Adapter()
	adapter.runCommand = func(_ context.Context, command []string) ([]byte, error) {
		captured = strings.Join(command, " ")
		return []byte(`[{"success":true,"results":[{"id":1}]}]`), nil
	}

	ds := datasource.DataSource{
		ID:   "ds_d1_local",
		Type: datasource.DataSourceType("d1"),
		Options: map[string]any{
			"mode":       "local",
			"binding":    "DB",
			"databaseId": "local-db-id",
		},
	}
	statement := "SELECT * FROM users"
	token, err := d1EncodePagingToken(d1PagingToken{
		Version:      1,
		DatasourceID: ds.ID,
		QueryHash:    d1StatementHash(statement),
		Offset:       0,
		PageSize:     5000,
	})
	if err != nil {
		t.Fatalf("d1EncodePagingToken: %v", err)
	}

	if _, err := adapter.Execute(context.Background(), ds, statement, ExecuteOptions{PagingToken: token}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(captured, "LIMIT 2001 OFFSET 0") {
		t.Fatalf("expected clamped LIMIT from token page size, got %q", captured)
	}
}

func TestD1AppendLimitOffset_TrailingLineComment(t *testing.T) {
	got := d1AppendLimitOffset("SELECT * FROM users -- trailing comment", 11, 0)
	if !strings.Contains(got, "-- trailing comment\nLIMIT 11 OFFSET 0") {
		t.Fatalf("expected LIMIT clause appended on a new line, got %q", got)
	}
}

func TestD1IsQueryStatement_IgnoresLeadingSQLComments(t *testing.T) {
	if !d1IsQueryStatement("-- warm-up comment\nSELECT * FROM users") {
		t.Fatalf("expected SELECT with leading line comment to be treated as query")
	}
	if !d1IsQueryStatement("/* preflight */\nSELECT * FROM users") {
		t.Fatalf("expected SELECT with leading block comment to be treated as query")
	}
}
