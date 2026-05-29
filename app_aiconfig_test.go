package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/aiconfig"
)

func TestUpdateAIConfig_PreservesExistingKeyWhenPayloadIsMasked(t *testing.T) {
	dir := t.TempDir()
	store := aiconfig.NewStore(filepath.Join(dir, "aiconfigs.json"))

	created, err := store.Create(aiconfig.AIConfig{
		Name:     "Test Config",
		Provider: aiconfig.ProviderCustom,
		BaseURL:  "http://localhost:9999/v1",
		APIKey:   "sk-real-1234",
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	app := &App{aiConfigStore: store}
	masked := maskAIKey(created).APIKey

	_, err = app.UpdateAIConfig(created.ID, AIConfigPayload{
		Name:     created.Name,
		Provider: created.Provider,
		BaseURL:  created.BaseURL,
		APIKey:   masked,
		Model:    created.Model,
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}

	updated, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("updated config not found")
	}
	if updated.APIKey != created.APIKey {
		t.Fatalf("expected stored api key to remain %q, got %q", created.APIKey, updated.APIKey)
	}
}

func TestUpdateEmbeddingConfig_DoesNotPreserveExistingKeyWhenEndpointChanges(t *testing.T) {
	dir := t.TempDir()
	store := aiconfig.NewStore(filepath.Join(dir, "aiconfigs.json"))

	created, err := store.Create(aiconfig.AIConfig{
		Name:     "Embedding Config",
		Provider: aiconfig.ProviderOpenAI,
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-real-1234",
		Model:    "text-embedding-3-small",
		Purpose:  aiconfig.PurposeEmbedding,
	})
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	app := &App{aiConfigStore: store}
	masked := maskAIKey(created).APIKey

	_, err = app.UpdateEmbeddingConfig(created.ID, AIConfigPayload{
		Name:     created.Name,
		Provider: aiconfig.ProviderCustom,
		BaseURL:  "https://embeddings.example.com/v1",
		APIKey:   masked,
		Model:    "text-embedding-3-small",
	})
	if err != nil {
		t.Fatalf("update embedding config: %v", err)
	}

	updated, ok := store.Get(created.ID)
	if !ok {
		t.Fatalf("updated config not found")
	}
	if updated.APIKey != "" {
		t.Fatalf("expected stored api key to be cleared after endpoint change, got %q", updated.APIKey)
	}
}

func TestTestAIConfigPreview_UsesStoredKeyWhenPayloadIsMasked(t *testing.T) {
	const storedKey = "sk-real-1234"
	const expectedAuth = "Bearer " + storedKey

	gotAuth := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if gotAuth != expectedAuth {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"test-model"}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	store := aiconfig.NewStore(filepath.Join(dir, "aiconfigs.json"))
	created, err := store.Create(aiconfig.AIConfig{
		Name:     "Test Config",
		Provider: aiconfig.ProviderCustom,
		BaseURL:  server.URL,
		APIKey:   storedKey,
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	app := &App{aiConfigStore: store}
	masked := maskAIKey(created).APIKey

	_, err = app.TestAIConfigPreview(created.ID, AIConfigPayload{
		APIKey: masked,
	})
	if err != nil {
		t.Fatalf("expected preview test to succeed, got error: %v (auth=%q)", err, gotAuth)
	}
	if gotAuth != expectedAuth {
		t.Fatalf("expected auth %q, got %q", expectedAuth, gotAuth)
	}
}
