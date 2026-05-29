package main

import (
	"context"
	"errors"
	"strings"

	"futrixdata/platform/internal/aiconfig"
)

// --- Embedding config bindings ---

func (a *App) ListEmbeddingConfigs() ([]aiconfig.AIConfig, error) {
	configs := a.aiConfigStore.ListByPurpose(aiconfig.PurposeEmbedding)
	masked := make([]aiconfig.AIConfig, len(configs))
	for i, cfg := range configs {
		masked[i] = maskAIKey(cfg)
	}
	return masked, nil
}

func (a *App) CreateEmbeddingConfig(payload AIConfigPayload) (aiconfig.AIConfig, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return aiconfig.AIConfig{}, errors.New("name is required")
	}
	if strings.TrimSpace(payload.Model) == "" {
		return aiconfig.AIConfig{}, errors.New("model is required")
	}
	cfg := payload.toAIConfig("")
	cfg.Purpose = aiconfig.PurposeEmbedding
	created, err := a.aiConfigStore.Create(cfg)
	if err != nil {
		return aiconfig.AIConfig{}, err
	}
	return maskAIKey(created), nil
}

func (a *App) UpdateEmbeddingConfig(id string, payload AIConfigPayload) (aiconfig.AIConfig, error) {
	existing, ok := a.aiConfigStore.Get(id)
	if !ok {
		return aiconfig.AIConfig{}, errors.New("configuration not found")
	}
	if strings.TrimSpace(payload.APIKey) == "" || isMaskedAIKey(payload.APIKey, existing.APIKey) {
		payload.APIKey = ""
		nextProvider := existing.Provider
		if strings.TrimSpace(string(payload.Provider)) != "" {
			nextProvider = payload.Provider
		}
		nextBaseURL := existing.BaseURL
		if strings.TrimSpace(payload.BaseURL) != "" {
			nextBaseURL = strings.TrimSpace(payload.BaseURL)
		}
		if nextProvider == existing.Provider && strings.TrimSpace(nextBaseURL) == strings.TrimSpace(existing.BaseURL) {
			payload.APIKey = existing.APIKey
		}
	}
	cfg := payload.toAIConfig(id)
	cfg.Purpose = aiconfig.PurposeEmbedding
	updated, err := a.aiConfigStore.Update(id, cfg)
	if err != nil {
		return aiconfig.AIConfig{}, err
	}
	return maskAIKey(updated), nil
}

func (a *App) DeleteEmbeddingConfig(id string) (bool, error) {
	return a.DeleteAIConfig(id)
}

func (a *App) ListEmbeddingProviders() (map[string]aiconfig.ProviderInfo, error) {
	providers := make(map[string]aiconfig.ProviderInfo)
	for key, value := range aiconfig.EmbeddingProviderDefaults {
		providers[string(key)] = value
	}
	return providers, nil
}

func (a *App) TestEmbeddingConfig(id string) (aiconfig.TestResult, error) {
	cfg, ok := a.aiConfigStore.Get(id)
	if !ok {
		return aiconfig.TestResult{}, errors.New("configuration not found")
	}
	result := aiconfig.TestEmbeddingConnection(context.Background(), cfg)
	if !result.Connected {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = "connection failed"
		}
		return result, errors.New(msg)
	}
	_, _ = a.aiConfigStore.UpdateStatus(id, result)
	return result, nil
}

func (a *App) TestEmbeddingConfigPayload(payload AIConfigPayload) (aiconfig.TestResult, error) {
	if strings.TrimSpace(payload.Model) == "" {
		return aiconfig.TestResult{}, errors.New("model is required")
	}
	cfg := payload.toAIConfig("")
	cfg.Purpose = aiconfig.PurposeEmbedding
	result := aiconfig.TestEmbeddingConnection(context.Background(), cfg)
	if !result.Connected {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = "connection failed"
		}
		return result, errors.New(msg)
	}
	return result, nil
}

// ComputeEmbeddingForSearch converts text to a vector using a configured
// embedding provider. Called by the ChromaDB console for text-based search.
// The dimensions parameter controls the output vector size (0 = model default).
func (a *App) ComputeEmbeddingForSearch(embeddingConfigID, text string, dimensions int) ([]float64, error) {
	cfg, ok := a.aiConfigStore.Get(embeddingConfigID)
	if !ok {
		return nil, errors.New("embedding configuration not found")
	}
	return aiconfig.ComputeEmbedding(context.Background(), cfg, text, dimensions)
}

type AIConfigPayload struct {
	Name     string                `json:"name"`
	Provider aiconfig.ProviderType `json:"provider"`
	BaseURL  string                `json:"baseUrl"`
	APIKey   string                `json:"apiKey"`
	Model    string                `json:"model"`
	Options  map[string]any        `json:"options"`
}

func (p AIConfigPayload) toAIConfig(id string) aiconfig.AIConfig {
	return aiconfig.AIConfig{
		ID:       id,
		Name:     strings.TrimSpace(p.Name),
		Provider: p.Provider,
		BaseURL:  strings.TrimSpace(p.BaseURL),
		APIKey:   p.APIKey,
		Model:    strings.TrimSpace(p.Model),
		Options:  p.Options,
	}
}

func validateAIConfigPayload(p AIConfigPayload) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if p.Provider == "" {
		return errors.New("provider is required")
	}
	switch p.Provider {
	case aiconfig.ProviderOpenAI, aiconfig.ProviderAnthropic, aiconfig.ProviderGemini,
		aiconfig.ProviderQwen, aiconfig.ProviderZhipu, aiconfig.ProviderDeepSeek,
		aiconfig.ProviderOpenRouter, aiconfig.ProviderOllama, aiconfig.ProviderLMStudio,
		aiconfig.ProviderCustom:
	default:
		return errors.New("unsupported provider")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return errors.New("apiKey is required")
	}
	if p.Provider == aiconfig.ProviderCustom && strings.TrimSpace(p.BaseURL) == "" {
		return errors.New("baseUrl is required for custom provider")
	}
	return nil
}

func validateAITestPayload(p AIConfigPayload) error {
	if err := validateAIConfigPayload(p); err != nil {
		return err
	}
	if strings.TrimSpace(p.Model) == "" {
		return errors.New("model is required")
	}
	return nil
}

func maskAIKey(cfg aiconfig.AIConfig) aiconfig.AIConfig {
	if len(cfg.APIKey) > 8 {
		cfg.APIKey = cfg.APIKey[:4] + "***" + cfg.APIKey[len(cfg.APIKey)-4:]
	} else if len(cfg.APIKey) > 0 {
		cfg.APIKey = "***"
	}
	return cfg
}

func maskAIKeyString(apiKey string) string {
	if len(apiKey) > 8 {
		return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
	}
	if len(apiKey) > 0 {
		return "***"
	}
	return ""
}

func isMaskedAIKey(apiKey, existingKey string) bool {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "***") {
		return true
	}
	return trimmed == maskAIKeyString(existingKey)
}

func (a *App) ListAIConfigs() ([]aiconfig.AIConfig, error) {
	configs := a.aiConfigStore.ListByPurpose(aiconfig.PurposeChat)
	masked := make([]aiconfig.AIConfig, len(configs))
	for i, cfg := range configs {
		masked[i] = maskAIKey(cfg)
	}
	return masked, nil
}

func (a *App) CreateAIConfig(payload AIConfigPayload) (aiconfig.AIConfig, error) {
	if err := validateAIConfigPayload(payload); err != nil {
		return aiconfig.AIConfig{}, err
	}
	created, err := a.aiConfigStore.Create(payload.toAIConfig(""))
	if err != nil {
		return aiconfig.AIConfig{}, err
	}
	return maskAIKey(created), nil
}

func (a *App) UpdateAIConfig(id string, payload AIConfigPayload) (aiconfig.AIConfig, error) {
	existing, ok := a.aiConfigStore.Get(id)
	if !ok {
		return aiconfig.AIConfig{}, errors.New("configuration not found")
	}
	if strings.TrimSpace(string(payload.Provider)) == "" {
		payload.Provider = existing.Provider
	}
	if strings.TrimSpace(payload.APIKey) == "" || isMaskedAIKey(payload.APIKey, existing.APIKey) {
		if payload.Provider == existing.Provider {
			payload.APIKey = existing.APIKey
		}
	}
	if err := validateAIConfigPayload(payload); err != nil {
		return aiconfig.AIConfig{}, err
	}
	updated, err := a.aiConfigStore.Update(id, payload.toAIConfig(id))
	if err != nil {
		return aiconfig.AIConfig{}, err
	}
	return maskAIKey(updated), nil
}

func (a *App) DeleteAIConfig(id string) (bool, error) {
	if err := a.aiConfigStore.Delete(id); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) GetAIConfigAPIKey(id string) (string, error) {
	cfg, ok := a.aiConfigStore.Get(id)
	if !ok {
		return "", errors.New("configuration not found")
	}
	return cfg.APIKey, nil
}

func (a *App) ListAIProviders() (map[string]aiconfig.ProviderInfo, error) {
	providers := make(map[string]aiconfig.ProviderInfo)
	for key, value := range aiconfig.ProviderDefaults {
		providers[string(key)] = value
	}
	return providers, nil
}

func (a *App) TestAIConfig(id string) (aiconfig.TestResult, error) {
	cfg, ok := a.aiConfigStore.Get(id)
	if !ok {
		return aiconfig.TestResult{}, errors.New("configuration not found")
	}
	result := aiconfig.TestConnection(context.Background(), cfg)
	if !result.Connected {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "connection failed"
		}
		return result, errors.New(message)
	}
	updated, err := a.aiConfigStore.UpdateStatus(id, result)
	if err == nil {
		_, _ = a.store.AssignAIConfigIfUnset(updated.ID)
	}
	return result, nil
}

func (a *App) TestAIConfigPayload(payload AIConfigPayload) (aiconfig.TestResult, error) {
	if err := validateAITestPayload(payload); err != nil {
		return aiconfig.TestResult{}, err
	}
	result := aiconfig.TestConnection(context.Background(), payload.toAIConfig(""))
	if !result.Connected {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "connection failed"
		}
		return result, errors.New(message)
	}
	return result, nil
}

func (a *App) TestAIConfigPreview(id string, payload AIConfigPayload) (aiconfig.TestResult, error) {
	cfg, ok := a.aiConfigStore.Get(id)
	if !ok {
		return aiconfig.TestResult{}, errors.New("configuration not found")
	}
	if payload.Provider != "" {
		cfg.Provider = payload.Provider
		if strings.TrimSpace(payload.BaseURL) == "" {
			cfg.BaseURL = ""
		}
	}
	if strings.TrimSpace(payload.BaseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(payload.BaseURL)
	}
	if strings.TrimSpace(payload.Model) != "" {
		cfg.Model = strings.TrimSpace(payload.Model)
	}
	apiKey := strings.TrimSpace(payload.APIKey)
	if apiKey != "" && !isMaskedAIKey(apiKey, cfg.APIKey) {
		cfg.APIKey = payload.APIKey
	}

	result := aiconfig.TestConnection(context.Background(), cfg)
	if !result.Connected {
		message := strings.TrimSpace(result.Error)
		if message == "" {
			message = "connection failed"
		}
		return result, errors.New(message)
	}
	return result, nil
}
