package ai

import (
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

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

type MongoAIRequest struct {
	Action     string
	Statement  string
	Error      string
	Prompt     string
	Collection string
	Database   string
	Fields     []string
	Indexes    []string
}

type MongoAIResponse struct {
	Statement   string   `json:"statement"`
	Explanation string   `json:"explanation,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) Configured() bool {
	return strings.TrimSpace(c.cfg.APIKey) != "" && strings.TrimSpace(c.cfg.BaseURL) != "" && strings.TrimSpace(c.cfg.Model) != ""
}

func (c *Client) AssistMongo(ctx context.Context, req MongoAIRequest) (MongoAIResponse, error) {
	if !c.Configured() {
		return MongoAIResponse{}, errors.New("ai client is not configured")
	}
	prompt, actionLabel := buildMongoPrompt(req)
	requestBody := openAIChatRequest{
		Model: c.cfg.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: mongoSystemPrompt()},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return MongoAIResponse{}, err
	}

	endpoint := chatCompletionsURL(c.cfg.BaseURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return MongoAIResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	request.Header.Set("Content-Type", "application/json")
	if strings.Contains(strings.ToLower(c.cfg.BaseURL), "openrouter.ai") {
		request.Header.Set("HTTP-Referer", "http://localhost")
		request.Header.Set("X-Title", "FutrixData Platform")
	}

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return MongoAIResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MongoAIResponse{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return MongoAIResponse{}, fmt.Errorf("ai provider error: %s", strings.TrimSpace(string(body)))
	}

	var response openAIChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return MongoAIResponse{}, err
	}
	if len(response.Choices) == 0 {
		return MongoAIResponse{}, errors.New("ai provider returned no choices")
	}
	content := response.Choices[0].Message.Content

	var result MongoAIResponse
	if err := parseModelJSON(content, &result); err == nil {
		if strings.TrimSpace(result.Statement) == "" {
			result.Statement = strings.TrimSpace(content)
			if result.Explanation == "" {
				result.Explanation = "AI response did not include a statement; using raw output."
			}
		}
		return result, nil
	}

	fallback := strings.TrimSpace(stripCodeFence(content))
	if fallback == "" {
		return MongoAIResponse{}, fmt.Errorf("ai response (%s) was empty", actionLabel)
	}
	return MongoAIResponse{
		Statement:   fallback,
		Explanation: "AI response was not JSON; using raw output.",
		Warnings:    []string{"Validate the statement before executing."},
	}, nil
}

func mongoSystemPrompt() string {
	return "You are a MongoDB console assistant. Return only JSON with keys: statement, explanation, warnings. The statement must be a runnable Mongo shell command (e.g. db.collection.find())."
}

func buildMongoPrompt(req MongoAIRequest) (string, string) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	actionLabel := "assist"
	actionDesc := "Provide the best Mongo shell statement for the request."
	switch action {
	case "complete":
		actionLabel = "complete"
		actionDesc = "Complete or generate a Mongo shell statement based on the context."
	case "diagnose":
		actionLabel = "diagnose"
		actionDesc = "Diagnose the error and provide a corrected Mongo shell statement."
	case "fix":
		actionLabel = "fix"
		actionDesc = "Fix syntax or logic issues in the statement and return a corrected version."
	}

	var b strings.Builder
	b.WriteString("Task: " + actionDesc + "\n")
	if req.Database != "" {
		b.WriteString("Database: " + req.Database + "\n")
	}
	if req.Collection != "" {
		b.WriteString("Collection: " + req.Collection + "\n")
	}
	if len(req.Fields) > 0 {
		b.WriteString("Fields: " + strings.Join(req.Fields, ", ") + "\n")
	}
	if len(req.Indexes) > 0 {
		b.WriteString("Indexes: " + strings.Join(req.Indexes, ", ") + "\n")
	}
	if req.Statement != "" {
		b.WriteString("Statement:\n" + req.Statement + "\n")
	} else {
		b.WriteString("Statement: <empty>\n")
	}
	if req.Error != "" {
		b.WriteString("Error:\n" + req.Error + "\n")
	}
	if req.Prompt != "" {
		b.WriteString("User prompt:\n" + req.Prompt + "\n")
	}
	b.WriteString("Return JSON only.")
	return b.String(), actionLabel
}

func parseModelJSON(content string, out any) error {
	trimmed := stripCodeFence(content)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end <= start {
		return errors.New("json payload not found")
	}
	payload := trimmed[start : end+1]
	return json.Unmarshal([]byte(payload), out)
}

func chatCompletionsURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
		if end := strings.LastIndex(trimmed, "```"); end != -1 {
			trimmed = trimmed[:end]
		}
	}
	return strings.TrimSpace(trimmed)
}
