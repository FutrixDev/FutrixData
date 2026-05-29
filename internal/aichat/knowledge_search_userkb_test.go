package aichat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchKnowledge_ScansUserKnowledge_AllScope(t *testing.T) {
	tmp := t.TempDir()

	builtinDir := filepath.Join(tmp, "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatalf("mkdir builtin: %v", err)
	}

	userRoot := filepath.Join(tmp, "userkb", "parsed", "scopes")
	userFile := filepath.Join(userRoot, "all", "kbcat_1", "kbfile_1.txt")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}

	token := "USER_KB_GLOBAL_TOKEN"
	if err := os.WriteFile(userFile, []byte(strings.TrimSpace("hello "+token)+"\n"), 0o644); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(builtinDir)
	svc.SetUserKnowledgeDir(userRoot)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query": token,
		"scope": "all",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var sawUser bool
	for _, hit := range res.Hits {
		if strings.Contains(hit.Source, "user/") {
			sawUser = true
			break
		}
	}
	if !sawUser {
		t.Fatalf("expected user knowledge hit (roots=%v notes=%v hits=%v)", res.Roots, res.Notes, res.Hits)
	}
}

func TestSearchKnowledge_ScansUserKnowledge_DatasourceScope(t *testing.T) {
	tmp := t.TempDir()

	builtinDir := filepath.Join(tmp, "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatalf("mkdir builtin: %v", err)
	}

	userRoot := filepath.Join(tmp, "userkb", "parsed", "scopes")
	userFile := filepath.Join(userRoot, "datasources", "ds_1", "kbcat_1", "kbfile_1.txt")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}

	token := "USER_KB_DS_TOKEN"
	if err := os.WriteFile(userFile, []byte(strings.TrimSpace("hello "+token)+"\n"), 0o644); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(builtinDir)
	svc.SetUserKnowledgeDir(userRoot)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query":        token,
		"scope":        "datasource",
		"datasourceId": "ds_1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("expected hits for user datasource scope, got 0 (roots=%v notes=%v)", res.Roots, res.Notes)
	}
}

func TestSearchKnowledge_UserKnowledge_RejectsPathTraversalDatasourceID(t *testing.T) {
	tmp := t.TempDir()

	builtinDir := filepath.Join(tmp, "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatalf("mkdir builtin: %v", err)
	}

	userRoot := filepath.Join(tmp, "userkb", "parsed", "scopes")
	if err := os.MkdirAll(userRoot, 0o755); err != nil {
		t.Fatalf("mkdir user root: %v", err)
	}

	outsideDir := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	secretPath := filepath.Join(outsideDir, "secret.txt")
	token := "USER_KB_SECRET_TOKEN"
	if err := os.WriteFile(secretPath, []byte(strings.TrimSpace(token)+"\n"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(builtinDir)
	svc.SetUserKnowledgeDir(userRoot)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query":        token,
		"scope":        "datasource",
		"datasourceId": "../../outside",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("expected 0 hits (should not read outside user knowledge root), got %v", res.Hits)
	}
}

func TestSearchKnowledge_UserKnowledge_DefaultCurrentScopeUsesDatasourcePack(t *testing.T) {
	tmp := t.TempDir()

	builtinDir := filepath.Join(tmp, "builtin")
	if err := os.MkdirAll(builtinDir, 0o755); err != nil {
		t.Fatalf("mkdir builtin: %v", err)
	}

	userRoot := filepath.Join(tmp, "userkb", "parsed", "scopes")
	userFile := filepath.Join(userRoot, "datasources", "ds_current", "kbcat_1", "kbfile_1.txt")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatalf("mkdir user datasource: %v", err)
	}
	token := "USER_KB_CURRENT_TOKEN"
	if err := os.WriteFile(userFile, []byte(token+"\n"), 0o644); err != nil {
		t.Fatalf("write user file: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(builtinDir)
	svc.SetUserKnowledgeDir(userRoot)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{
		PageContext: PageContext{
			CurrentDatasourceID: "ds_current",
		},
	}, map[string]any{
		"query": token,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res.Scope != "current" {
		t.Fatalf("expected default scope current, got %q", res.Scope)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("expected hits from current datasource pack, got 0")
	}
}
