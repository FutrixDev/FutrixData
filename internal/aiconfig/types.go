package aiconfig

// ConfigPurpose distinguishes chat models from embedding models.
type ConfigPurpose string

const (
	PurposeChat      ConfigPurpose = "chat"
	PurposeEmbedding ConfigPurpose = "embedding"
)

// ProviderType represents the AI provider type
type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderGemini     ProviderType = "gemini"
	ProviderQwen       ProviderType = "qwen"  // Alibaba Tongyi Qianwen
	ProviderZhipu      ProviderType = "zhipu" // ChatGLM
	ProviderDeepSeek   ProviderType = "deepseek"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderOllama     ProviderType = "ollama"
	ProviderLMStudio   ProviderType = "lmstudio"
	ProviderCustom     ProviderType = "custom" // Custom OpenAI-compatible
)

// AIConfig represents a configured AI provider
type AIConfig struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Provider      ProviderType   `json:"provider"`
	BaseURL       string         `json:"baseUrl"`
	APIKey        string         `json:"apiKey"`
	Model         string         `json:"model"`
	Purpose       ConfigPurpose  `json:"purpose,omitempty"` // "chat" (default) or "embedding"
	Status        string         `json:"status"`
	StatusDetail  string         `json:"statusDetail,omitempty"`
	LastCheckedAt int64          `json:"lastCheckedAt,omitempty"`
	LastLatencyMs int64          `json:"lastLatencyMs,omitempty"`
	LastModelInfo string         `json:"lastModelInfo,omitempty"`
	CreatedAt     int64          `json:"createdAt"`
	Options       map[string]any `json:"options,omitempty"`
}

// ProviderInfo contains default configuration for a provider
type ProviderInfo struct {
	Name         string   `json:"name"`
	BaseURL      string   `json:"baseUrl"`
	DefaultModel string   `json:"defaultModel"`
	Models       []string `json:"models"`
}

// ProviderDefaults provides default configurations for each provider
var ProviderDefaults = map[ProviderType]ProviderInfo{
	ProviderOpenAI: {
		Name:         "OpenAI",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-5.2",
		Models: []string{
			"gpt-5.2",
			"gpt-5.2-chat",
			"gpt-5.2-pro",
			"gpt-5.1",
			"gpt-5.1-chat",
			"gpt-5.1-codex",
			"gpt-5.1-codex-mini",
			"gpt-5.1-codex-max",
			"gpt-5",
			"gpt-5-chat",
			"gpt-5-pro",
			"gpt-5-mini",
			"gpt-5-nano",
			"gpt-5-codex",
			"gpt-4.1",
			"gpt-4.1-mini",
			"gpt-4.1-nano",
			"gpt-4o",
			"gpt-4o-mini",
			"o3",
			"o3-mini",
			"o4-mini",
		},
	},
	ProviderAnthropic: {
		Name:         "Anthropic Claude",
		BaseURL:      "https://api.anthropic.com",
		DefaultModel: "claude-sonnet-4-5-20250929",
		Models:       []string{"claude-opus-4-5-20251124", "claude-sonnet-4-5-20250929", "claude-haiku-4-5-20251022", "claude-opus-4-1-20250805", "claude-sonnet-4-20250514", "claude-opus-4-20250514"},
	},
	ProviderGemini: {
		Name:         "Google Gemini",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
		DefaultModel: "gemini-3-flash",
		Models:       []string{"gemini-3-flash", "gemini-3-pro", "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
	},
	ProviderQwen: {
		Name:         "Alibaba Qwen",
		BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModel: "qwen-turbo",
		Models:       []string{"qwen-turbo", "qwen-plus", "qwen-max"},
	},
	ProviderZhipu: {
		Name:         "Zhipu AI (ChatGLM)",
		BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
		DefaultModel: "glm-4",
		Models:       []string{"glm-4", "glm-4-flash", "glm-3-turbo"},
	},
	ProviderDeepSeek: {
		Name:         "DeepSeek",
		BaseURL:      "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
		Models:       []string{"deepseek-chat", "deepseek-reasoner"},
	},
	ProviderOpenRouter: {
		Name:         "OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		DefaultModel: "anthropic/claude-sonnet-4.5",
		Models: []string{
			"anthropic/claude-opus-4.5",
			"anthropic/claude-sonnet-4.5",
			"anthropic/claude-haiku-4.5",
			"anthropic/claude-opus-4.1",
			"anthropic/claude-sonnet-4",
			"anthropic/claude-opus-4",
			"openai/gpt-5.2",
			"openai/gpt-5.2-chat",
			"openai/gpt-5.2-pro",
			"openai/gpt-5.1",
			"openai/gpt-5.1-chat",
			"openai/gpt-5.1-codex",
			"openai/gpt-5.1-codex-mini",
			"openai/gpt-5.1-codex-max",
			"openai/gpt-5",
			"openai/gpt-5-chat",
			"openai/gpt-5-pro",
			"openai/gpt-5-mini",
			"openai/gpt-5-nano",
			"openai/gpt-5-codex",
			"openai/gpt-4.1",
			"openai/gpt-4.1-mini",
			"openai/gpt-4.1-nano",
			"openai/gpt-4o",
			"openai/gpt-4o-mini",
			"openai/o3",
			"openai/o3-mini",
			"openai/o4-mini",
			"google/gemini-3-flash",
			"google/gemini-3-pro",
			"google/gemini-2.5-pro",
			"google/gemini-2.5-flash",
			"deepseek/deepseek-chat",
			"deepseek/deepseek-reasoner",
		},
	},
	ProviderCustom: {
		Name:         "Custom",
		BaseURL:      "",
		DefaultModel: "",
		Models:       []string{},
	},
}

// EmbeddingProviderDefaults provides default models for embedding providers.
var EmbeddingProviderDefaults = map[ProviderType]ProviderInfo{
	ProviderOpenAI: {
		Name:         "OpenAI",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "text-embedding-3-small",
		Models:       []string{"text-embedding-3-small", "text-embedding-3-large", "text-embedding-ada-002"},
	},
	ProviderGemini: {
		Name:         "Google Gemini",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
		DefaultModel: "text-embedding-004",
		Models:       []string{"text-embedding-004"},
	},
	ProviderQwen: {
		Name:         "Alibaba Qwen",
		BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModel: "text-embedding-v3",
		Models:       []string{"text-embedding-v3", "text-embedding-v2"},
	},
	ProviderDeepSeek: {
		Name:         "DeepSeek",
		BaseURL:      "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-embedding",
		Models:       []string{"deepseek-embedding"},
	},
	ProviderOpenRouter: {
		Name:         "OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		DefaultModel: "openai/text-embedding-3-small",
		Models:       []string{"openai/text-embedding-3-small", "openai/text-embedding-3-large"},
	},
	ProviderCustom: {
		Name:         "Custom",
		BaseURL:      "",
		DefaultModel: "",
		Models:       []string{},
	},
}

// TestResult contains the result of a connection test
type TestResult struct {
	Connected bool   `json:"connected"`
	LatencyMs int64  `json:"latencyMs"`
	ModelInfo string `json:"modelInfo,omitempty"`
	Error     string `json:"error,omitempty"`
}
