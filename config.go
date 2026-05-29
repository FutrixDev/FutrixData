package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"futrixdata/platform/internal/appdata"
	"futrixdata/platform/internal/auth"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Address                string `json:"address" yaml:"address"`
	DataPath               string `json:"data_path" yaml:"data_path"`
	AuthBaseURL            string `json:"auth_base_url" yaml:"auth_base_url"`
	AIBaseURL              string `json:"ai_base_url" yaml:"ai_base_url"`
	AIAPIKey               string `json:"ai_api_key" yaml:"ai_api_key"`
	AIModel                string `json:"ai_model" yaml:"ai_model"`
	AITimeoutSeconds       int    `json:"ai_timeout_seconds" yaml:"ai_timeout_seconds"`
	AIChatPromptPath       string `json:"ai_chat_prompt_path" yaml:"ai_chat_prompt_path"`
	AIChatPromptModulesDir string `json:"ai_chat_prompt_modules_dir" yaml:"ai_chat_prompt_modules_dir"`
	AIChatKnowledgeDir     string `json:"ai_chat_knowledge_dir" yaml:"ai_chat_knowledge_dir"`
}

func loadConfig(path string) (Config, error) {
	defaultDataPath := appdata.DevDataPath("FutrixData")
	cfg := Config{
		Address:                ":8080",
		DataPath:               defaultDataPath,
		AuthBaseURL:            auth.DefaultBaseURL,
		AIBaseURL:              "https://api.openai.com/v1",
		AIModel:                "gpt-5.2",
		AITimeoutSeconds:       15,
		AIChatPromptPath:       "data/ai-chat-system-prompt.md",
		AIChatPromptModulesDir: "data/ai-chat-prompts",
		AIChatKnowledgeDir:     "data/ai-chat-knowledge",
	}
	if path == "" {
		return cfg, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	switch ext := filepath.Ext(path); ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			return cfg, err
		}
	default:
		if err := json.Unmarshal(content, &cfg); err != nil {
			return cfg, err
		}
	}

	if cfg.DataPath == "" {
		cfg.DataPath = defaultDataPath
	}
	if !filepath.IsAbs(cfg.DataPath) {
		cfg.DataPath = filepath.Join(filepath.Dir(path), cfg.DataPath)
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.AuthBaseURL == "" {
		cfg.AuthBaseURL = os.Getenv("FUTRIX_AUTH_BASE_URL")
	}
	if cfg.AuthBaseURL == "" {
		cfg.AuthBaseURL = auth.DefaultBaseURL
	}
	if cfg.AIModel == "" {
		cfg.AIModel = "gpt-5.2"
	}
	if cfg.AITimeoutSeconds == 0 {
		cfg.AITimeoutSeconds = 15
	}
	if cfg.AIChatPromptPath == "" {
		cfg.AIChatPromptPath = "data/ai-chat-system-prompt.md"
	}
	if cfg.AIChatPromptModulesDir == "" {
		cfg.AIChatPromptModulesDir = "data/ai-chat-prompts"
	}
	if cfg.AIChatKnowledgeDir == "" {
		cfg.AIChatKnowledgeDir = "data/ai-chat-knowledge"
	}
	return cfg, nil
}
