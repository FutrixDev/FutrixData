package aichat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleModelConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	Timeout   time.Duration
	MaxTokens int
	Referer   string
	AppTitle  string
}

type OpenAICompatibleModel struct {
	cfg        OpenAICompatibleModelConfig
	httpClient *http.Client
}

func NewOpenAICompatibleModel(cfg OpenAICompatibleModelConfig) *OpenAICompatibleModel {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	return &OpenAICompatibleModel{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: timeout},
	}
}

type openAIChatRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	Stream              bool            `json:"stream,omitempty"`
	Temperature         float64         `json:"temperature,omitempty"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

func (m *OpenAICompatibleModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	if strings.TrimSpace(m.cfg.APIKey) == "" || strings.TrimSpace(m.cfg.BaseURL) == "" || strings.TrimSpace(m.cfg.Model) == "" {
		return "", errors.New("ai provider not configured")
	}

	msgs := make([]openAIMessage, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "user"
		}
		msgs = append(msgs, openAIMessage{Role: role, Content: msg.Content})
	}

	body, err := json.Marshal(openAIChatRequestForModel(m.cfg.Model, msgs, true, 0.2, m.cfg.MaxTokens))
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(m.cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	if strings.Contains(strings.ToLower(m.cfg.BaseURL), "openrouter.ai") {
		referer := strings.TrimSpace(m.cfg.Referer)
		if referer == "" {
			referer = "http://localhost"
		}
		title := strings.TrimSpace(m.cfg.AppTitle)
		if title == "" {
			title = "FutrixData Platform"
		}
		req.Header.Set("HTTP-Referer", referer)
		req.Header.Set("X-Title", title)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai provider error: %s", strings.TrimSpace(string(respBody)))
	}

	var (
		full      strings.Builder
		reader    = bufio.NewReader(resp.Body)
		dataLines []string
	)

	flushEvent := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			return true, nil
		}
		if msg := openAIStreamErrorMessage(payload); msg != "" {
			return false, errors.New(msg)
		}
		var chunk openAIStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return false, fmt.Errorf("ai provider stream decode error: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			full.WriteString(choice.Delta.Content)
			if onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				_, flushErr := flushEvent()
				if flushErr != nil {
					return "", flushErr
				}
				break
			}
			return "", err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			done, flushErr := flushEvent()
			if flushErr != nil {
				return "", flushErr
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	}
	return full.String(), nil
}

func openAIChatRequestForModel(model string, messages []openAIMessage, stream bool, temperature float64, maxTokens int) openAIChatRequest {
	req := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      stream,
		Temperature: temperature,
	}
	if usesMaxCompletionTokens(model) {
		req.MaxCompletionTokens = maxTokens
	} else {
		req.MaxTokens = maxTokens
	}
	return req
}

func usesMaxCompletionTokens(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return strings.HasPrefix(normalized, "gpt5") || strings.HasPrefix(normalized, "gpt-5")
}

func openAIStreamErrorMessage(payload string) string {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return ""
	}
	if _, ok := envelope["choices"]; ok {
		return ""
	}

	switch raw := envelope["error"].(type) {
	case map[string]any:
		if msg, ok := raw["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	case string:
		if strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}

	if msg, ok := envelope["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	return ""
}

func chatCompletionsURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}
