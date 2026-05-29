package aiconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TestConnection tests the AI configuration by making a minimal API call
func TestConnection(ctx context.Context, cfg AIConfig) TestResult {
	start := time.Now()

	// Resolve base URL from provider defaults if not specified
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if defaults, ok := ProviderDefaults[cfg.Provider]; ok {
			baseURL = defaults.BaseURL
		}
	}
	if baseURL == "" {
		return TestResult{
			Connected: false,
			Error:     "base URL not configured",
		}
	}

	var result TestResult

	switch cfg.Provider {
	case ProviderAnthropic:
		result = testAnthropic(ctx, baseURL, cfg.APIKey, cfg.Model)
	case ProviderOpenRouter:
		result = testOpenAICompatible(ctx, baseURL, cfg.APIKey, cfg.Model, map[string]string{
			"HTTP-Referer": "http://localhost",
			"X-Title":      "FutrixData Platform",
		})
	default:
		// OpenAI-compatible API (OpenAI, Qwen, Zhipu, DeepSeek, Custom)
		result = testOpenAICompatible(ctx, baseURL, cfg.APIKey, cfg.Model, nil)
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

// testOpenAICompatible tests OpenAI-compatible APIs
func testOpenAICompatible(ctx context.Context, baseURL, apiKey, model string, extraHeaders map[string]string) TestResult {
	url := chatCompletionsURL(baseURL)

	maxTokensKey := openAIMaxTokensKey(model)
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	}
	payload[maxTokensKey] = 16

	body, err := json.Marshal(payload)
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("marshal error: %v", err)}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("request error: %v", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("connection error: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Try to extract error message
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return TestResult{Connected: false, Error: errResp.Error.Message}
		}
		return TestResult{Connected: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}
	}

	var result struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(respBody, &result) == nil {
		return TestResult{Connected: true, ModelInfo: result.Model}
	}

	return TestResult{Connected: true, ModelInfo: model}
}

// testAnthropic tests Anthropic's Messages API
func testAnthropic(ctx context.Context, baseURL, apiKey, model string) TestResult {
	url := anthropicMessagesURL(baseURL)

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 16,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("marshal error: %v", err)}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("request error: %v", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TestResult{Connected: false, Error: fmt.Sprintf("connection error: %v", err)}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Try to extract error message
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return TestResult{Connected: false, Error: errResp.Error.Message}
		}
		return TestResult{Connected: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}
	}

	var result struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(respBody, &result) == nil {
		return TestResult{Connected: true, ModelInfo: result.Model}
	}

	return TestResult{Connected: true, ModelInfo: model}
}

func chatCompletionsURL(baseURL string) string {
	normalized, baseQuery := normalizeOpenAIBaseURLParts(baseURL)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		trimmed := strings.TrimRight(normalizeOpenAIBaseURLFallback(baseURL), "/")
		if strings.HasSuffix(strings.ToLower(trimmed), "/chat/completions") {
			return trimmed
		}
		if trimmed == "" {
			return "/chat/completions"
		}
		return trimmed + "/chat/completions"
	}

	path := strings.TrimRight(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, "/chat/completions"):
		parsed.Path = path
	case path == "":
		parsed.Path = "/chat/completions"
	default:
		parsed.Path = path + "/chat/completions"
	}
	if len(baseQuery) > 0 {
		values := parsed.Query()
		for key, queryValues := range baseQuery {
			if _, exists := values[key]; exists {
				continue
			}
			for _, value := range queryValues {
				values.Add(key, value)
			}
		}
		parsed.RawQuery = values.Encode()
	}
	return parsed.String()
}

func normalizeOpenAIBaseURLParts(baseURL string) (string, url.Values) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return normalizeOpenAIBaseURLFallback(trimmed), nil
	}
	query := parsed.Query()
	parsed.RawQuery = ""
	parsed.Path = normalizeOpenAIPath(parsed.Path)
	return parsed.String(), query
}

func normalizeOpenAIBaseURLFallback(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimRight(trimmed, "/")
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed[:len(trimmed)-len("/chat/completions")]
	}
	return trimmed
}

func normalizeOpenAIPath(path string) string {
	trimmedPath := strings.TrimRight(path, "/")
	lowerPath := strings.ToLower(trimmedPath)
	if strings.HasSuffix(lowerPath, "/chat/completions") {
		return trimmedPath[:len(trimmedPath)-len("/chat/completions")]
	}
	return trimmedPath
}

func anthropicMessagesURL(baseURL string) string {
	normalized := normalizeAnthropicBaseURL(baseURL)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed == nil {
		lower := strings.ToLower(normalized)
		switch {
		case strings.HasSuffix(lower, "/v1/messages"):
			return normalized
		case strings.HasSuffix(lower, "/v1"):
			return normalized + "/messages"
		default:
			return normalized + "/v1/messages"
		}
	}
	path := strings.TrimRight(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, "/v1/messages"):
		parsed.Path = path
	case strings.HasSuffix(lowerPath, "/v1"):
		parsed.Path = path + "/messages"
	default:
		if path == "" {
			parsed.Path = "/v1/messages"
		} else {
			parsed.Path = path + "/v1/messages"
		}
	}
	return parsed.String()
}

func normalizeAnthropicBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil {
		return normalizeAnthropicBaseURLFallback(trimmed)
	}
	parsed.Path = normalizeAnthropicPath(parsed.Path)
	return parsed.String()
}

func normalizeAnthropicBaseURLFallback(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/v1/messages"):
		return trimmed[:len(trimmed)-len("/v1/messages")]
	case strings.HasSuffix(lower, "/v1"):
		return trimmed[:len(trimmed)-len("/v1")]
	default:
		return trimmed
	}
}

func normalizeAnthropicPath(path string) string {
	trimmedPath := strings.TrimRight(path, "/")
	lowerPath := strings.ToLower(trimmedPath)
	switch {
	case strings.HasSuffix(lowerPath, "/v1/messages"):
		return trimmedPath[:len(trimmedPath)-len("/v1/messages")]
	case strings.HasSuffix(lowerPath, "/v1"):
		return trimmedPath[:len(trimmedPath)-len("/v1")]
	default:
		return trimmedPath
	}
}

func openAIMaxTokensKey(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if strings.HasPrefix(normalized, "gpt5") || strings.HasPrefix(normalized, "gpt-5") {
		return "max_completion_tokens"
	}
	return "max_tokens"
}
