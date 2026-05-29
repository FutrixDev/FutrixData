package aichat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchKnowledge_RejectsPathTraversalDatasourceID(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		t.Fatalf("mkdir knowledge: %v", err)
	}

	outsideDir := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	secretPath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte(strings.TrimSpace(`
SUPERSECRET
do not leak
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{}, map[string]any{
		"query":        "SUPERSECRET",
		"scope":        "datasource",
		"datasourceId": "../../outside",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("expected 0 hits (should not read outside knowledge root), got %v", res.Hits)
	}
}
