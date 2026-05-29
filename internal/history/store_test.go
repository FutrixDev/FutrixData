package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreAppendAndCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := 0; i < 1001; i++ {
		_, err := store.Append(AppendInput{
			DatasourceID:   "ds",
			DatasourceName: "DS",
			DatasourceType: "mysql",
			Statement:      "SELECT " + string(rune('a'+(i%26))),
			Targets:        []string{"t"},
		})
		if err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	items := store.List(Filter{})
	if len(items) != 1000 {
		t.Fatalf("cap failed: %d", len(items))
	}
}

func TestStoreDedup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_ = store.Load()
	_, _ = store.Append(AppendInput{DatasourceID: "ds", DatasourceName: "DS", DatasourceType: "mysql", Statement: "SELECT 1", Targets: []string{"t"}})
	_, _ = store.Append(AppendInput{DatasourceID: "ds", DatasourceName: "DS", DatasourceType: "mysql", Statement: "SELECT 1", Targets: []string{"t"}})
	if len(store.List(Filter{})) != 1 {
		t.Fatalf("dedup failed")
	}
}

func TestStoreFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_ = store.Load()
	_, _ = store.Append(AppendInput{DatasourceID: "a", DatasourceName: "Alpha", DatasourceType: "mysql", Statement: "SELECT * FROM users", Targets: []string{"users"}})
	_, _ = store.Append(AppendInput{DatasourceID: "b", DatasourceName: "Beta", DatasourceType: "mongodb", Statement: "db.orders.find({})", Targets: []string{"orders"}})
	out := store.List(Filter{DatasourceID: "a", Target: "users", Keyword: "select"})
	if len(out) != 1 {
		t.Fatalf("filter failed: %v", out)
	}
}

func TestStoreListEmptyReturnsSlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	out := store.List(Filter{})
	if out == nil {
		t.Fatalf("expected empty slice, got nil")
	}
}

func TestStoreLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_, _ = store.Append(AppendInput{DatasourceID: "ds", DatasourceName: "DS", DatasourceType: "mysql", Statement: "SELECT 1", Targets: []string{"t"}})
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("save failed")
	}
}


func TestStoreGetByID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	entry, err := store.Append(AppendInput{
		DatasourceID:   "ds",
		DatasourceName: "DS",
		DatasourceType: "mysql",
		Statement:      "SELECT 1",
		Targets:        []string{"t"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	got, ok := store.GetByID(entry.ID)
	if !ok || got.ID != entry.ID {
		t.Fatalf("get by id failed: %v", got)
	}
}

func TestStoreDeleteByID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_ = store.Load()
	entry, _ := store.Append(AppendInput{
		DatasourceID:   "ds",
		DatasourceName: "DS",
		DatasourceType: "mysql",
		Statement:      "SELECT 1",
		Targets:        []string{"t"},
	})
	if !store.DeleteByID(entry.ID) {
		t.Fatalf("delete failed")
	}
	if _, ok := store.GetByID(entry.ID); ok {
		t.Fatalf("entry not deleted")
	}
}

func TestStoreClearFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_ = store.Load()
	_, _ = store.Append(AppendInput{DatasourceID: "a", DatasourceName: "A", DatasourceType: "mysql", Statement: "SELECT 1"})
	_, _ = store.Append(AppendInput{DatasourceID: "b", DatasourceName: "B", DatasourceType: "mysql", Statement: "SELECT 2"})
	cleared := store.Clear(Filter{DatasourceID: "a"})
	if cleared != 1 || len(store.List(Filter{})) != 1 {
		t.Fatalf("clear filter failed: %d", cleared)
	}
}
