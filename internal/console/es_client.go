package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"futrixdata/platform/internal/datasource"
)

type elasticsearchClient struct {
	httpClient *http.Client
}

func newElasticsearchClient() *elasticsearchClient {
	return &elasticsearchClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func elasticsearchBaseURL(ds datasource.DataSource) (string, error) {
	rawHost := strings.TrimSpace(ds.Host)
	if rawHost == "" {
		return "", errors.New("host is required")
	}

	scheme := "http"
	if ds.Options != nil {
		if raw, ok := ds.Options["scheme"]; ok {
			value := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
			if value != "" {
				scheme = value
			}
		}
	}

	host := rawHost
	port := ds.Port

	// Allow users to paste a full URL (e.g. https://es.example.com:9243) into the host field.
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("invalid host url: %w", err)
		}
		if parsed.Scheme != "" {
			scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
		}
		if parsed.Hostname() != "" {
			host = parsed.Hostname()
		} else {
			return "", errors.New("host is required")
		}
		if parsed.Port() != "" {
			parsedPort, err := strconv.Atoi(parsed.Port())
			if err != nil {
				return "", fmt.Errorf("invalid host port: %w", err)
			}
			port = parsedPort
		}
	} else {
		parsed, err := url.Parse("http://" + strings.TrimSuffix(host, "/"))
		if err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
			if parsed.Port() != "" {
				parsedPort, err := strconv.Atoi(parsed.Port())
				if err != nil {
					return "", fmt.Errorf("invalid host port: %w", err)
				}
				port = parsedPort
			}
		}
	}

	switch scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported scheme: %s", scheme)
	}

	if port <= 0 {
		return "", errors.New("port is required")
	}
	if host == "" {
		return "", errors.New("host is required")
	}

	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(port))), nil
}

func (c *elasticsearchClient) do(ctx context.Context, ds datasource.DataSource, method string, path string, body string) ([]byte, string, error) {
	baseURL, err := elasticsearchBaseURL(ds)
	if err != nil {
		return nil, "", err
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, "", err
	}
	endpoint := target.ResolveReference(ref).String()

	var reader io.Reader
	if strings.TrimSpace(body) != "" {
		reader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), endpoint, reader)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(ds.Username) != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		if snippet != "" {
			return nil, contentType, fmt.Errorf("elasticsearch request failed: %s: %s", resp.Status, snippet)
		}
		return nil, contentType, fmt.Errorf("elasticsearch request failed: %s", resp.Status)
	}
	return raw, contentType, nil
}
