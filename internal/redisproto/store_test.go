package redisproto

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "redis-protobuf.json"))
	step := int64(0)
	s.now = func() time.Time {
		step++
		return time.Unix(0, step*1000).UTC()
	}
	return s
}

func mustSave(t *testing.T, s *Store, req SaveRequest) Schema {
	t.Helper()
	got, err := s.Save(req)
	if err != nil {
		t.Fatalf("Save(%+v) returned error: %v", req, err)
	}
	return got
}

func TestSaveCreateAndUpdate(t *testing.T) {
	s := newTestStore(t)

	created := mustSave(t, s, SaveRequest{
		DatasourceID: "ds_1",
		Name:         "user.proto",
		Content:      "syntax = \"proto3\"; message User { string name = 1; }",
	})
	if created.ID == "" {
		t.Fatalf("expected generated id, got empty")
	}
	if created.CreatedAt != created.UpdatedAt {
		t.Fatalf("new schema should have CreatedAt == UpdatedAt, got %v vs %v", created.CreatedAt, created.UpdatedAt)
	}

	updated, err := s.Save(SaveRequest{
		ID:           created.ID,
		DatasourceID: "ds_1",
		Name:         "user.proto",
		Content:      "syntax = \"proto3\"; message User { string name = 1; int32 age = 2; }",
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Fatalf("CreatedAt should be preserved on update, got %v want %v", updated.CreatedAt, created.CreatedAt)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdatedAt should advance, got %v not after %v", updated.UpdatedAt, created.UpdatedAt)
	}
	if !strings.Contains(updated.Content, "int32 age = 2") {
		t.Fatalf("content not updated: %s", updated.Content)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s := newTestStore(t)
	cases := []SaveRequest{
		{Name: "", Content: "syntax = \"proto3\";"},
		{Name: "x", Content: ""},
		{Name: "x", Content: "   "},
	}
	for _, c := range cases {
		if _, err := s.Save(c); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected ErrInvalid for %+v, got %v", c, err)
		}
	}
}

func TestSaveUpdateMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Save(SaveRequest{ID: "rps_missing", Name: "x", Content: "syntax;"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListAndListByDatasource(t *testing.T) {
	s := newTestStore(t)
	a := mustSave(t, s, SaveRequest{DatasourceID: "ds_1", Name: "a.proto", Content: "syntax = \"proto3\";"})
	b := mustSave(t, s, SaveRequest{DatasourceID: "ds_2", Name: "b.proto", Content: "syntax = \"proto3\";"})
	c := mustSave(t, s, SaveRequest{DatasourceID: "", Name: "global.proto", Content: "syntax = \"proto3\";"})

	all := s.List()
	if len(all) != 3 {
		t.Fatalf("expected 3 schemas, got %d", len(all))
	}
	if all[0].ID != c.ID || all[1].ID != b.ID || all[2].ID != a.ID {
		t.Fatalf("expected newest-first ordering, got %v", []string{all[0].ID, all[1].ID, all[2].ID})
	}

	byDS := s.ListByDatasource("ds_1")
	if len(byDS) != 1 || byDS[0].ID != a.ID {
		t.Fatalf("ListByDatasource(ds_1) = %v, want %s", byDS, a.ID)
	}
	global := s.ListByDatasource("")
	if len(global) != 1 || global[0].ID != c.ID {
		t.Fatalf("ListByDatasource('') = %v, want %s", global, c.ID)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	a := mustSave(t, s, SaveRequest{DatasourceID: "ds_1", Name: "a.proto", Content: "syntax;"})
	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if err := s.Delete(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestDeleteByDatasourceCascades(t *testing.T) {
	s := newTestStore(t)
	a := mustSave(t, s, SaveRequest{DatasourceID: "ds_1", Name: "a.proto", Content: "syntax;"})
	mustSave(t, s, SaveRequest{DatasourceID: "ds_1", Name: "b.proto", Content: "syntax;"})
	mustSave(t, s, SaveRequest{DatasourceID: "ds_2", Name: "c.proto", Content: "syntax;"})

	removed, err := s.DeleteByDatasource("ds_1")
	if err != nil {
		t.Fatalf("DeleteByDatasource error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}
	if len(s.ListByDatasource("ds_1")) != 0 {
		t.Fatalf("ds_1 schemas should be empty")
	}
	if len(s.ListByDatasource("ds_2")) != 1 {
		t.Fatalf("ds_2 schemas should be untouched")
	}
	// Empty datasourceID is a no-op.
	if n, _ := s.DeleteByDatasource(""); n != 0 {
		t.Fatalf("DeleteByDatasource('') should be no-op, got %d", n)
	}
	// Re-deleting yields zero.
	if n, _ := s.DeleteByDatasource("ds_1"); n != 0 {
		t.Fatalf("re-delete should be no-op, got %d", n)
	}
	_ = a
}

func TestPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis-protobuf.json")

	s1 := NewStore(path)
	s1.now = func() time.Time { return time.Unix(0, 1000).UTC() }
	created, err := s1.Save(SaveRequest{DatasourceID: "ds_1", Name: "user.proto", Content: "syntax = \"proto3\";"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var list []Schema
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("on-disk list mismatch: %v", list)
	}

	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := s2.Get(created.ID)
	if !ok {
		t.Fatalf("schema missing after reload")
	}
	if got.Content != created.Content {
		t.Fatalf("reloaded content mismatch")
	}
}

func TestLoadMissingFileIsNoop(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file should be nil, got %v", err)
	}
}

func TestLoadResetsItemsWhenFileMissing(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Save(SaveRequest{Name: "a.proto", Content: "syntax = \"proto3\"; message A {}"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(s.List()) == 0 {
		t.Fatalf("expected at least one schema before reload")
	}
	if err := os.Remove(s.path); err != nil {
		t.Fatalf("remove path: %v", err)
	}
	if err := s.Load(); err != nil {
		t.Fatalf("Load after removing file: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("expected items cleared after reload of missing file, got %d", len(got))
	}
}

func TestSaveContentSizeLimit(t *testing.T) {
	s := newTestStore(t)
	huge := strings.Repeat("a", maxContentBytes+1)
	if _, err := s.Save(SaveRequest{Name: "huge.proto", Content: huge}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for oversized content, got %v", err)
	}
}

// breakWritablePath makes the configured store path un-writable so that
// saveLocked fails. We do this by replacing the path with a directory that the
// .tmp sibling cannot resolve (parent is a file).
func breakStorePath(t *testing.T, s *Store) {
	t.Helper()
	parent := filepath.Dir(s.path)
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup blocker: %v", err)
	}
	s.path = filepath.Join(blocker, "child.json")
}

func TestSaveUpdateRollsBackOnPersistFailure(t *testing.T) {
	s := newTestStore(t)
	created, err := s.Save(SaveRequest{Name: "user.proto", Content: "syntax = \"proto3\";"})
	if err != nil {
		t.Fatalf("seed save: %v", err)
	}

	breakStorePath(t, s)
	if _, err := s.Save(SaveRequest{ID: created.ID, Name: "renamed.proto", Content: created.Content}); err == nil {
		t.Fatalf("expected persist failure")
	}

	got, ok := s.Get(created.ID)
	if !ok {
		t.Fatalf("schema disappeared on persist failure")
	}
	if got.Name != "user.proto" {
		t.Fatalf("in-memory state should not be mutated on persist failure; got name=%q", got.Name)
	}
}

func TestDeleteRollsBackOnPersistFailure(t *testing.T) {
	s := newTestStore(t)
	created, err := s.Save(SaveRequest{Name: "user.proto", Content: "syntax = \"proto3\";"})
	if err != nil {
		t.Fatalf("seed save: %v", err)
	}

	breakStorePath(t, s)
	if err := s.Delete(created.ID); err == nil {
		t.Fatalf("expected persist failure")
	}
	if _, ok := s.Get(created.ID); !ok {
		t.Fatalf("schema removed despite persist failure")
	}
}

func TestDeleteByDatasourceRollsBackOnPersistFailure(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Save(SaveRequest{DatasourceID: "ds_1", Name: "a.proto", Content: "syntax = \"proto3\";"}); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	if _, err := s.Save(SaveRequest{DatasourceID: "ds_1", Name: "b.proto", Content: "syntax = \"proto3\";"}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	breakStorePath(t, s)
	if _, err := s.DeleteByDatasource("ds_1"); err == nil {
		t.Fatalf("expected persist failure")
	}
	if got := len(s.ListByDatasource("ds_1")); got != 2 {
		t.Fatalf("expected schemas restored after persist failure, got %d", got)
	}
}
