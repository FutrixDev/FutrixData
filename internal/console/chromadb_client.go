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

const (
	chromaDBDefaultTenant   = "default_tenant"
	chromaDBDefaultDatabase = "default_database"
)

type chromaDBClient struct {
	httpClient *http.Client
}

func newChromaDBClient() *chromaDBClient {
	return &chromaDBClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func chromaDBOptionString(ds datasource.DataSource, key string) string {
	if ds.Options == nil {
		return ""
	}
	raw, ok := ds.Options[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func chromaDBTenant(ds datasource.DataSource) string {
	if value := chromaDBOptionString(ds, "tenant"); value != "" {
		return value
	}
	return chromaDBDefaultTenant
}

func chromaDBDatabase(ds datasource.DataSource) string {
	if value := chromaDBOptionString(ds, "database"); value != "" {
		return value
	}
	return chromaDBDefaultDatabase
}

func chromaDBBaseURL(ds datasource.DataSource) (string, error) {
	rawHost := strings.TrimSpace(ds.Host)
	if rawHost == "" {
		return "", errors.New("host is required")
	}

	scheme := "http"
	if value := strings.ToLower(chromaDBOptionString(ds, "scheme")); value != "" {
		scheme = value
	}

	host := rawHost
	port := ds.Port
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("invalid host url: %w", err)
		}
		if parsed.Scheme != "" {
			scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
		}
		if parsed.Hostname() == "" {
			return "", errors.New("host is required")
		}
		host = parsed.Hostname()
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

func chromaDBAPIPrefix(ds datasource.DataSource) string {
	return "/api/v2/tenants/" + url.PathEscape(chromaDBTenant(ds)) + "/databases/" + url.PathEscape(chromaDBDatabase(ds))
}

func (c *chromaDBClient) do(ctx context.Context, ds datasource.DataSource, method string, path string, body string) ([]byte, string, error) {
	baseURL, err := chromaDBBaseURL(ds)
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
	if token := chromaDBOptionString(ds, "apiToken"); token != "" {
		req.Header.Set("x-chroma-token", token)
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
			return nil, contentType, fmt.Errorf("chromadb request failed: %s: %s", resp.Status, snippet)
		}
		return nil, contentType, fmt.Errorf("chromadb request failed: %s", resp.Status)
	}
	return raw, contentType, nil
}
