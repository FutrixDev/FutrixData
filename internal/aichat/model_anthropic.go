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
	"net/url"
	"strings"
	"time"
)

type AnthropicModelConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	Timeout   time.Duration
	MaxTokens int
}

type AnthropicModel struct {
	cfg        AnthropicModelConfig
	httpClient *http.Client
}

func NewAnthropicModel(cfg AnthropicModelConfig) *AnthropicModel {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	return &AnthropicModel{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: timeout},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
}

func (m *AnthropicModel) Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error) {
	if strings.TrimSpace(m.cfg.APIKey) == "" || strings.TrimSpace(m.cfg.BaseURL) == "" || strings.TrimSpace(m.cfg.Model) == "" {
		return "", errors.New("ai provider not configured")
	}

	msgs := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: msg.Content})
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       m.cfg.Model,
		System:      systemPrompt,
		Messages:    msgs,
		Temperature: 0.2,
		MaxTokens:   m.cfg.MaxTokens,
	})
	if err != nil {
		return "", err
	}

	endpoint := anthropicMessagesURL(m.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("ai provider error: %s", strings.TrimSpace(string(respBody)))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" && block.Text != "" {
			b.WriteString(block.Text)
		}
	}
	return b.String(), nil
}

type anthropicStreamEvent struct {
	Type string `json:"type"`
	// content_block_delta -> delta.text
	Delta struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	// content_block_start -> content_block.text (sometimes empty)
	ContentBlock struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"content_block,omitempty"`
}

func (m *AnthropicModel) ChatStream(ctx context.Context, systemPrompt string, messages []Message, onDelta func(delta string)) (string, error) {
	if strings.TrimSpace(m.cfg.APIKey) == "" || strings.TrimSpace(m.cfg.BaseURL) == "" || strings.TrimSpace(m.cfg.Model) == "" {
		return "", errors.New("ai provider not configured")
	}

	msgs := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: msg.Content})
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       m.cfg.Model,
		System:      systemPrompt,
		Messages:    msgs,
		Stream:      true,
		Temperature: 0.2,
		MaxTokens:   m.cfg.MaxTokens,
	})
	if err != nil {
		return "", err
	}

	endpoint := anthropicMessagesURL(m.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", m.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
		if payload == "" || payload == "[DONE]" {
			return false, nil
		}
		var evt anthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			return false, fmt.Errorf("ai provider stream decode error: %w", err)
		}
		switch evt.Type {
		case "content_block_delta":
			if evt.Delta.Text == "" {
				return false, nil
			}
			full.WriteString(evt.Delta.Text)
			if onDelta != nil {
				onDelta(evt.Delta.Text)
			}
			return false, nil
		case "content_block_start":
			// Some implementations may include initial text here.
			if evt.ContentBlock.Text == "" {
				return false, nil
			}
			full.WriteString(evt.ContentBlock.Text)
			if onDelta != nil {
				onDelta(evt.ContentBlock.Text)
			}
			return false, nil
		case "message_stop":
			return true, nil
		default:
			return false, nil
		}
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
				return full.String(), nil
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

func anthropicMessagesURL(baseURL string) string {
	normalized, baseQuery := normalizeAnthropicBaseURLParts(baseURL)
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
