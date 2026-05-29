package aiconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// aiConfigRequest represents the request body for creating/updating AI configs.
type aiConfigRequest struct {
	Name     string         `json:"name"`
	Provider ProviderType   `json:"provider"`
	BaseURL  string         `json:"baseUrl"`
	APIKey   string         `json:"apiKey"`
	Model    string         `json:"model"`
	Options  map[string]any `json:"options"`
}

func decodeAIConfigRequest(r *http.Request) (aiConfigRequest, error) {
	decoder := json.NewDecoder(r.Body)
	var req aiConfigRequest
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func (r aiConfigRequest) toAIConfig(id string) AIConfig {
	return AIConfig{
		ID:       id,
		Name:     strings.TrimSpace(r.Name),
		Provider: r.Provider,
		BaseURL:  strings.TrimSpace(r.BaseURL),
		APIKey:   r.APIKey,
		Model:    strings.TrimSpace(r.Model),
		Options:  r.Options,
	}
}

func validateAIConfigRequest(r aiConfigRequest) error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if r.Provider == "" {
		return errors.New("provider is required")
	}
	switch r.Provider {
	case ProviderOpenAI,
		ProviderAnthropic,
		ProviderGemini,
		ProviderQwen,
		ProviderZhipu,
		ProviderDeepSeek,
		ProviderOpenRouter,
		ProviderOllama,
		ProviderLMStudio,
		ProviderCustom:
	default:
		return errors.New("unsupported provider")
	}
	if strings.TrimSpace(r.APIKey) == "" {
		return errors.New("apiKey is required")
	}
	if r.Provider == ProviderCustom && strings.TrimSpace(r.BaseURL) == "" {
		return errors.New("baseUrl is required for custom provider")
	}
	return nil
}

func validateAITestRequest(r aiConfigRequest) error {
	if err := validateAIConfigRequest(r); err != nil {
		return err
	}
	if strings.TrimSpace(r.Model) == "" {
		return errors.New("model is required")
	}
	return nil
}

// maskAPIKey returns a copy of the config with the API key masked.
func maskAPIKey(cfg AIConfig) AIConfig {
	if len(cfg.APIKey) > 8 {
		cfg.APIKey = cfg.APIKey[:4] + "***" + cfg.APIKey[len(cfg.APIKey)-4:]
	} else if len(cfg.APIKey) > 0 {
		cfg.APIKey = "***"
	}
	return cfg
}
