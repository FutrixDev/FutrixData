package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"futrixdata/platform/internal/ai"
	"futrixdata/platform/internal/aiconfig"
	"futrixdata/platform/internal/datasource"
)

func buildAIConfig(cfg Config) ai.Config {
	baseURL := firstNonEmpty(cfg.AIBaseURL, os.Getenv("FUTRIX_AI_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := firstNonEmpty(cfg.AIAPIKey, os.Getenv("FUTRIX_AI_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(cfg.AIModel, os.Getenv("FUTRIX_AI_MODEL"))
	if model == "" {
		model = "gpt-5.2"
	}
	timeout := time.Duration(cfg.AITimeoutSeconds) * time.Second
	return ai.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
	}
}

func (a *App) AssistMongo(req ai.MongoRequest) (ai.MongoAIResponse, error) {
	if strings.TrimSpace(req.DatasourceID) == "" {
		return ai.MongoAIResponse{}, errors.New("datasourceId is required")
	}
	ds, ok := a.store.Get(req.DatasourceID)
	if !ok {
		return ai.MongoAIResponse{}, errors.New("datasource not found")
	}
	if ds.Type != datasource.TypeMongoDB {
		return ai.MongoAIResponse{}, errors.New("ai mongo assistant only supports mongodb")
	}
	client := a.getClientForDatasource(ds)
	if client == nil || !client.Configured() {
		return ai.MongoAIResponse{}, errors.New("ai provider not configured")
	}
	if req.Database == "" {
		req.Database = ds.Database
	}
	return client.AssistMongo(context.Background(), ai.MongoAIRequest{
		Action:     req.Action,
		Statement:  req.Statement,
		Error:      req.Error,
		Prompt:     req.Prompt,
		Collection: req.Collection,
		Database:   req.Database,
		Fields:     req.Fields,
		Indexes:    req.Indexes,
	})
}

func (a *App) getClientForDatasource(ds datasource.DataSource) *ai.Client {
	if a.aiConfigStore != nil {
		if selectedID := aiConfigIDFromOptions(ds.Options); selectedID != "" {
			if cfg, ok := a.aiConfigStore.Get(selectedID); ok {
				if client := a.buildClientFromConfig(cfg); client != nil {
					return client
				}
			}
		}
	}
	return a.getClient()
}

func (a *App) getClient() *ai.Client {
	if a.aiConfigStore != nil {
		if cfg, ok := a.aiConfigStore.GetPreferred(); ok {
			if client := a.buildClientFromConfig(cfg); client != nil {
				return client
			}
		}
	}
	return a.fallbackAI
}

func (a *App) buildClientFromConfig(cfg aiconfig.AIConfig) *ai.Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if defaults, ok := aiconfig.ProviderDefaults[cfg.Provider]; ok {
			baseURL = defaults.BaseURL
		}
	}
	client := ai.NewClient(ai.Config{
		BaseURL: baseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		Timeout: 15 * time.Second,
	})
	if !client.Configured() {
		return nil
	}
	return client
}

func aiConfigIDFromOptions(options map[string]any) string {
	if options == nil {
		return ""
	}
	raw, ok := options["aiConfigId"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
