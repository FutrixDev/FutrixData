package aiconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSave_OmitsIsActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiconfigs.json")
	store := NewStore(path)
	_, err := store.Create(AIConfig{
		ID:       "ai_1",
		Name:     "Primary",
		Provider: ProviderOpenAI,
		APIKey:   "sk-test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if bytes.Contains(payload, []byte(`"isActive"`)) {
		t.Fatalf("expected isActive to be removed from persisted data")
	}
}
