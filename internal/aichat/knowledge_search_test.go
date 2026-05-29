package aichat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurn_SearchKnowledge_ToolCallIncludesSnippets(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")

	mysqlDir := filepath.Join(knowledgeDir, "types", "mysql")
	if err := os.MkdirAll(mysqlDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mysqlDir, "cheatsheet.md"), []byte(strings.TrimSpace(`
# MySQL cheat
Use an indexed order + LIMIT:
SELECT * FROM orders ORDER BY id ASC LIMIT 10;
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"search_knowledge","arguments":{"query":"ORDER BY id LIMIT"}}]}`,
			`{"assistantMessage":"ok","toolCalls":[]}`,
		},
	}
	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	_, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "How to list first 10 rows safely?"}},
		PageContext: PageContext{
			RouteName:             "console",
			RoutePath:             "/console/ds_test",
			CurrentDatasourceID:   "ds_test",
			CurrentDatasourceType: "mysql",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(model.received) < 2 {
		t.Fatalf("expected recorded model calls")
	}

	var sawToolResult bool
	for _, msg := range model.received[1] {
		if !strings.Contains(msg.Content, "[tool_result] search_knowledge") {
			continue
		}
		if !strings.Contains(msg.Content, "ORDER BY id") || !strings.Contains(msg.Content, "LIMIT 10") {
			continue
		}
		sawToolResult = true
		break
	}
	if !sawToolResult {
		t.Fatalf("expected search_knowledge tool result to be included in second model call messages")
	}
}

func TestSearchKnowledge_DefaultsToCurrentScopeFirst(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")
	currentFile := filepath.Join(knowledgeDir, "datasources", "ds_current", "notes.md")
	typeFile := filepath.Join(knowledgeDir, "types", "mysql", "notes.md")
	if err := os.MkdirAll(filepath.Dir(currentFile), 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(typeFile), 0o755); err != nil {
		t.Fatalf("mkdir type: %v", err)
	}
	token := "CURRENT_SCOPE_TOKEN"
	if err := os.WriteFile(currentFile, []byte(token+"\n"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}
	if err := os.WriteFile(typeFile, []byte(token+"\n"), 0o644); err != nil {
		t.Fatalf("write type: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{
		PageContext: PageContext{
			CurrentDatasourceID:   "ds_current",
			CurrentDatasourceType: "mysql",
		},
		Messages: []Message{{Role: "user", Content: "show mysql quoting guidance"}},
	}, map[string]any{
		"query": "index limit guidance",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res.Scope != "current" {
		t.Fatalf("expected default scope current, got %q", res.Scope)
	}
	if res.DatasourceID != "ds_current" {
		t.Fatalf("expected datasource id from page context, got %q", res.DatasourceID)
	}
	if res.DatasourceType != "mysql" {
		t.Fatalf("expected datasource type from page context, got %q", res.DatasourceType)
	}
	if len(res.Roots) < 2 {
		t.Fatalf("expected current scope roots for datasource + type, got %v", res.Roots)
	}
	if !strings.Contains(res.Roots[0], filepath.Join("datasources", "ds_current")) {
		t.Fatalf("expected datasource root first, got %v", res.Roots)
	}
	if len(res.Notes) == 0 || !strings.Contains(strings.Join(res.Notes, " "), "current -> type -> all") {
		t.Fatalf("expected progressive scope note, got %v", res.Notes)
	}
}

func TestSearchKnowledge_DefaultsToDatasourceTypeScopeWhenDatasourceIsUnknown(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")
	typeFile := filepath.Join(knowledgeDir, "types", "mysql", "notes.md")
	if err := os.MkdirAll(filepath.Dir(typeFile), 0o755); err != nil {
		t.Fatalf("mkdir type: %v", err)
	}
	if err := os.WriteFile(typeFile, []byte("MYSQL_TYPE_SCOPE_TOKEN\n"), 0o644); err != nil {
		t.Fatalf("write type: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{
		PageContext: PageContext{
			CurrentDatasourceType: "mysql",
		},
		Messages: []Message{{Role: "user", Content: "find mysql guidance for a table that is not in the current datasource"}},
	}, map[string]any{
		"query": "mysql field naming guidance",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res.Scope != "type" {
		t.Fatalf("expected default scope type when datasource id is unavailable, got %q", res.Scope)
	}
	if res.DatasourceID != "" {
		t.Fatalf("expected empty datasource id, got %q", res.DatasourceID)
	}
	if res.DatasourceType != "mysql" {
		t.Fatalf("expected datasource type mysql, got %q", res.DatasourceType)
	}
	if len(res.Roots) == 0 || !strings.Contains(res.Roots[0], filepath.Join("types", "mysql")) {
		t.Fatalf("expected mysql type root, got %v", res.Roots)
	}
}

func TestSearchKnowledge_IntentAvoidCurrentDoesNotFallbackToPageFocus(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")
	allFile := filepath.Join(knowledgeDir, "general.md")
	if err := os.MkdirAll(filepath.Dir(allFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(allFile, []byte("GENERAL_SCOPE_TOKEN\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), TurnRequest{
		Intent: &TurnIntent{
			CurrentFocus: "avoid_current",
			Confidence:   0.9,
		},
		PageContext: PageContext{
			CurrentDatasourceID:   "ds_current",
			CurrentDatasourceType: "mysql",
		},
		Messages: []Message{{Role: "user", Content: "find guidance for another datasource"}},
	}, map[string]any{
		"query": "guidance",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res.Scope != "all" {
		t.Fatalf("expected avoid_current intent to fall back to all-scope discovery, got %q", res.Scope)
	}
	if res.DatasourceID != "" {
		t.Fatalf("expected datasource id to stay empty when current focus is explicitly avoided, got %q", res.DatasourceID)
	}
	if res.DatasourceType != "" {
		t.Fatalf("expected datasource type to stay empty when current focus is explicitly avoided, got %q", res.DatasourceType)
	}
}

func TestSearchKnowledge_UsesDatasourceTypeTemplatesWhenNoExplicitQuery(t *testing.T) {
	tmp := t.TempDir()
	knowledgeDir := filepath.Join(tmp, "kb")
	dynamoDir := filepath.Join(knowledgeDir, "types", "dynamodb")
	if err := os.MkdirAll(dynamoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dynamoDir, "constraints.md"), []byte(strings.TrimSpace(`
predicate index rule
Partition Key predicates are required for efficient point access.
Queries that only filter on a non-key attribute may require a scan or another access path.
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := TurnRequest{
		Messages: []Message{{Role: "user", Content: "why does filtering by aid miss data"}},
		PageContext: PageContext{
			CurrentDatasourceType: "dynamodb",
			CurrentEntity:         "orders",
		},
	}
	query, generated := resolveKnowledgeSearchQuery(req, map[string]any{"topic": "predicate_index_rule"}, "dynamodb")

	if !strings.Contains(query, "partition key") {
		t.Fatalf("expected dynamodb template terms in query, got %q", query)
	}
	if !strings.Contains(query, "aid") {
		t.Fatalf("expected original topic to be preserved, got %q", query)
	}
	if !generated {
		t.Fatalf("expected query to be generated from datasource template")
	}

	svc := NewService(fakeResolver{model: &scriptedModel{}}, &fakeTools{})
	svc.SetKnowledgeDir(knowledgeDir)

	res, err := svc.searchKnowledge(context.Background(), req, map[string]any{
		"topic": "predicate_index_rule",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("expected hits from templated query, got 0 (query=%q roots=%v notes=%v)", res.Query, res.Roots, res.Notes)
	}
	if !strings.Contains(strings.Join(res.Notes, " "), "query generated from datasource-aware template") {
		t.Fatalf("expected generated-query note, got %v", res.Notes)
	}
}
