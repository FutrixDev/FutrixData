package aichat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchKnowledge_ScansTypeFilePack(t *testing.T) {
	tmp := t.TempDir()
	rawKnowledgeDir := filepath.Join(tmp, "kb")
	if err := os.MkdirAll(rawKnowledgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	knowledgeDir := rawKnowledgeDir
	if resolved, err := filepath.EvalSymlinks(rawKnowledgeDir); err == nil && strings.TrimSpace(resolved) != "" {
		knowledgeDir = resolved
	}

	if err := os.MkdirAll(filepath.Join(knowledgeDir, "types"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	token := "TYPE_FILE_PACK_TOKEN"
	if err := os.WriteFile(filepath.Join(knowledgeDir, "types", "mysql.md"), []byte(strings.TrimSpace(token)+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query":          token,
		"scope":          "type",
		"datasourceType": "mysql",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("expected hits for type file pack, got 0 (roots=%v notes=%v)", res.Roots, res.Notes)
	}
}

func TestSearchKnowledge_ScansDatasourceFilePack(t *testing.T) {
	tmp := t.TempDir()
	rawKnowledgeDir := filepath.Join(tmp, "kb")
	if err := os.MkdirAll(rawKnowledgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	knowledgeDir := rawKnowledgeDir
	if resolved, err := filepath.EvalSymlinks(rawKnowledgeDir); err == nil && strings.TrimSpace(resolved) != "" {
		knowledgeDir = resolved
	}

	if err := os.MkdirAll(filepath.Join(knowledgeDir, "datasources"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	token := "DATASOURCE_FILE_PACK_TOKEN"
	if err := os.WriteFile(filepath.Join(knowledgeDir, "datasources", "ds_1.md"), []byte(strings.TrimSpace(token)+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query":        token,
		"scope":        "datasource",
		"datasourceId": "ds_1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("expected hits for datasource file pack, got 0 (roots=%v notes=%v)", res.Roots, res.Notes)
	}
}
