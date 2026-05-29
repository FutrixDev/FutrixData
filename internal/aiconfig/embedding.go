package aiconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type embeddingRequest struct {
	Input      string `json:"input"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// ComputeEmbedding calls the OpenAI-compatible /v1/embeddings endpoint to
// convert a text string into a vector using the given AI config.
// The optional dimensions parameter controls the output vector size;
// pass 0 to use the model's default.
func ComputeEmbedding(ctx context.Context, cfg AIConfig, text string, dimensions int) ([]float64, error) {
	baseURL := resolveEmbeddingBaseURL(cfg)
	if baseURL == "" {
		return nil, fmt.Errorf("embedding base URL is empty")
	}
	// For custom providers the user supplies the full endpoint URL
	// (e.g. http://localhost:8901/v1/embeddings), so don't append /embeddings.
	url := baseURL
	if cfg.Provider != ProviderCustom {
		url = strings.TrimRight(baseURL, "/") + "/embeddings"
	}

	reqBody := embeddingRequest{Input: text, Model: cfg.Model}
	if dimensions > 0 {
		reqBody.Dimensions = dimensions
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, truncateBody(body))
	}

	var result embeddingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse embedding response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response contains no vectors")
	}
	return result.Data[0].Embedding, nil
}

// TestEmbeddingConnection sends a small test text to verify the embedding
// endpoint is reachable and returns a valid vector.
func TestEmbeddingConnection(ctx context.Context, cfg AIConfig) TestResult {
	start := time.Now()
	vec, err := ComputeEmbedding(ctx, cfg, "test", 0)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return TestResult{Connected: false, LatencyMs: latency, Error: err.Error()}
	}
	return TestResult{
		Connected: true,
		LatencyMs: latency,
		ModelInfo: fmt.Sprintf("%s (%d dims)", cfg.Model, len(vec)),
	}
}

func resolveEmbeddingBaseURL(cfg AIConfig) string {
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	if info, ok := EmbeddingProviderDefaults[cfg.Provider]; ok {
		return info.BaseURL
	}
	return ""
}

func truncateBody(body []byte) string {
	s := string(body)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
