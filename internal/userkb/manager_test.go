package userkb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_CategoryCRUDAndIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{Root: tmp})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	state, err := m.CreateCategory(ctx, CategoryCreateInput{
		Name:  "Global Notes",
		Scope: ScopeAll,
	})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	if len(state.State.Categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(state.State.Categories))
	}
	cat := state.State.Categories[0]
	if cat.ID == "" {
		t.Fatalf("expected category id to be set")
	}

	state, err = m.UpdateCategory(ctx, cat.ID, CategoryUpdateInput{
		Name:        "Global Notes 2",
		Description: "desc",
		Scope:       ScopeAll,
	})
	if err != nil {
		t.Fatalf("UpdateCategory error: %v", err)
	}
	if state.State.Categories[0].Name != "Global Notes 2" {
		t.Fatalf("expected updated name, got %q", state.State.Categories[0].Name)
	}
	if state.State.Categories[0].Description != "desc" {
		t.Fatalf("expected updated description, got %q", state.State.Categories[0].Description)
	}

	state, err = m.DeleteCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("DeleteCategory error: %v", err)
	}
	if len(state.State.Categories) != 0 {
		t.Fatalf("expected 0 categories after delete, got %d", len(state.State.Categories))
	}
}

func TestManager_UploadFiles_MarksNeedsProviderWhenUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{Root: tmp})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	state, err := m.CreateCategory(ctx, CategoryCreateInput{Name: "Global", Scope: ScopeAll})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	catID := state.State.Categories[0].ID

	payload := base64.StdEncoding.EncodeToString([]byte("KB_TOKEN_123\nusers(id,name)\n"))
	vs, err := m.UploadFiles(ctx, catID, []UploadFileInput{{Name: "schema.txt", Base64: payload}}, "")
	if err != nil {
		t.Fatalf("UploadFiles error: %v", err)
	}
	if len(vs.State.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(vs.State.Files))
	}
	f := vs.State.Files[0]
	if f.ParseStatus != ParseOK {
		t.Fatalf("expected parse ok, got %q (err=%q)", f.ParseStatus, f.ParseError)
	}
	if f.SummaryStatus != SummaryNeedsProvider {
		t.Fatalf("expected needs_provider, got %q (err=%q)", f.SummaryStatus, f.SummaryError)
	}
}

func TestManager_UploadFiles_GeneratesAISummaryWhenProviderReady(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{
		Root: tmp,
		ModelResolver: fakeResolver{
			model: fakeModel{response: `{"summary":"Schema notes","keywords":["users","id"]}`},
		},
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	state, err := m.CreateCategory(ctx, CategoryCreateInput{Name: "Global", Scope: ScopeAll})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	catID := state.State.Categories[0].ID

	payload := base64.StdEncoding.EncodeToString([]byte("KB_TOKEN_SUMMARY\nusers(id,name)\n"))
	vs, err := m.UploadFiles(ctx, catID, []UploadFileInput{{Name: "schema.txt", Base64: payload}}, "")
	if err != nil {
		t.Fatalf("UploadFiles error: %v", err)
	}
	if len(vs.State.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(vs.State.Files))
	}
	f := vs.State.Files[0]
	if f.ParseStatus != ParseOK {
		t.Fatalf("expected parse ok, got %q (err=%q)", f.ParseStatus, f.ParseError)
	}
	if f.SummaryStatus != SummaryOK {
		t.Fatalf("expected summary ok, got %q (err=%q)", f.SummaryStatus, f.SummaryError)
	}
	if f.AISummary != "Schema notes" {
		t.Fatalf("expected aiSummary to be populated, got %q", f.AISummary)
	}
	if len(f.Keywords) != 2 || f.Keywords[0] != "users" || f.Keywords[1] != "id" {
		t.Fatalf("unexpected keywords: %v", f.Keywords)
	}
}

func TestManager_DeleteFile_RemovesFromState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{Root: tmp})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	state, err := m.CreateCategory(ctx, CategoryCreateInput{Name: "Global", Scope: ScopeAll})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	catID := state.State.Categories[0].ID

	payload := base64.StdEncoding.EncodeToString([]byte("KB_DELETE_TOKEN\n"))
	vs, err := m.UploadFiles(ctx, catID, []UploadFileInput{{Name: "note.txt", Base64: payload}}, "")
	if err != nil {
		t.Fatalf("UploadFiles error: %v", err)
	}
	if len(vs.State.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(vs.State.Files))
	}

	fileID := vs.State.Files[0].ID
	vs, err = m.DeleteFile(ctx, fileID)
	if err != nil {
		t.Fatalf("DeleteFile error: %v", err)
	}
	if len(vs.State.Files) != 0 {
		t.Fatalf("expected 0 files after delete, got %d", len(vs.State.Files))
	}
}

func TestManager_DatasourceCategory_BindsMultipleDatasources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{Root: tmp})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	state, err := m.CreateCategory(ctx, CategoryCreateInput{
		Name:          "Orders Schema",
		Scope:         ScopeDatasource,
		DatasourceIDs: []string{"ds_a", "ds_b"},
	})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	catID := state.State.Categories[0].ID

	payload := base64.StdEncoding.EncodeToString([]byte("ORDER_TOKEN\norders(id,user_id)\n"))
	vs, err := m.UploadFiles(ctx, catID, []UploadFileInput{{Name: "orders.md", Base64: payload}}, "")
	if err != nil {
		t.Fatalf("UploadFiles error: %v", err)
	}
	if len(vs.State.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(vs.State.Files))
	}
	fileID := vs.State.Files[0].ID

	for _, dsID := range []string{"ds_a", "ds_b"} {
		path := filepath.Join(tmp, "parsed", "scopes", "datasources", dsID, catID, fileID+".txt")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected scope file for %s: %v", dsID, err)
		}
		if !strings.Contains(string(data), "ORDER_TOKEN") {
			t.Fatalf("expected scope file to contain token for %s", dsID)
		}

		indexPath := filepath.Join(tmp, "parsed", "scopes", "datasources", dsID, "data_structure.md")
		indexData, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("expected datasource index for %s: %v", dsID, err)
		}
		if !strings.Contains(string(indexData), catID) {
			t.Fatalf("expected datasource index to mention category for %s", dsID)
		}
	}
}

func TestManager_UpdateCategory_UpdatesDatasourceBindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmp := t.TempDir()

	m, err := NewManager(ManagerConfig{Root: tmp})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	vs, err := m.CreateCategory(ctx, CategoryCreateInput{
		Name:          "Scoped",
		Scope:         ScopeDatasource,
		DatasourceIDs: []string{"ds_a"},
	})
	if err != nil {
		t.Fatalf("CreateCategory error: %v", err)
	}
	catID := vs.State.Categories[0].ID

	payload := base64.StdEncoding.EncodeToString([]byte("BIND_TOKEN\n"))
	vs, err = m.UploadFiles(ctx, catID, []UploadFileInput{{Name: "x.txt", Base64: payload}}, "")
	if err != nil {
		t.Fatalf("UploadFiles error: %v", err)
	}
	fileID := vs.State.Files[0].ID

	oldPath := filepath.Join(tmp, "parsed", "scopes", "datasources", "ds_a", catID, fileID+".txt")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected old scope file to exist: %v", err)
	}

	vs, err = m.UpdateCategory(ctx, catID, CategoryUpdateInput{
		Name:          "Scoped",
		Scope:         ScopeDatasource,
		DatasourceIDs: []string{"ds_b"},
	})
	if err != nil {
		t.Fatalf("UpdateCategory error: %v", err)
	}

	if _, err := os.Stat(oldPath); err == nil {
		t.Fatalf("expected old scope file to be removed")
	}
	newPath := filepath.Join(tmp, "parsed", "scopes", "datasources", "ds_b", catID, fileID+".txt")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected new scope file to exist: %v", err)
	}
}

type fakeResolver struct {
	model Model
	err   error
}

func (r fakeResolver) Resolve(aiConfigID string) (Model, error) {
	_ = aiConfigID
	if r.err != nil {
		return nil, r.err
	}
	if r.model == nil {
		return nil, errors.New("ai provider not configured")
	}
	return r.model, nil
}

type fakeModel struct {
	response string
	err      error
}

func (m fakeModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	_ = ctx
	_ = systemPrompt
	_ = messages
	if m.err != nil {
		return "", m.err
	}
	// Ensure the response is valid JSON-ish for our parser.
	var tmp any
	if err := json.Unmarshal([]byte(m.response), &tmp); err != nil {
		return `{"summary":"invalid","keywords":[]}`, nil
	}
	return m.response, nil
}
