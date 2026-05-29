package console

import (
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestElasticsearchBaseURL_DefaultsToHTTP(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "example.com",
		Port: 9200,
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "http://example.com:9200" {
		t.Fatalf("expected http://example.com:9200, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_UsesOptionsScheme(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type:    datasource.TypeElasticsearch,
		Host:    "example.com",
		Port:    9200,
		Options: map[string]any{"scheme": "https"},
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "https://example.com:9200" {
		t.Fatalf("expected https://example.com:9200, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_AllowsSchemeInHost(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "https://example.com",
		Port: 443,
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "https://example.com:443" {
		t.Fatalf("expected https://example.com:443, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_AllowsHostPortInHost(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "example.com:9243",
		Port: 9200,
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "http://example.com:9243" {
		t.Fatalf("expected http://example.com:9243, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_HostURLPortOverridesPortField(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "https://example.com:9243",
		Port: 9200,
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "https://example.com:9243" {
		t.Fatalf("expected https://example.com:9243, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_HostSchemeOverridesOptionsScheme(t *testing.T) {
	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type:    datasource.TypeElasticsearch,
		Host:    "http://example.com",
		Port:    9200,
		Options: map[string]any{"scheme": "https"},
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "http://example.com:9200" {
		t.Fatalf("expected http://example.com:9200, got %q", baseURL)
	}
}

func TestElasticsearchBaseURL_RejectsUnsupportedScheme(t *testing.T) {
	_, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "ftp://example.com",
		Port: 21,
	})
	if err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
}

func TestElasticsearchBaseURL_PortRequiredUnlessProvidedInHost(t *testing.T) {
	_, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "example.com",
		Port: 0,
	})
	if err == nil {
		t.Fatalf("expected error for missing port")
	}

	baseURL, err := elasticsearchBaseURL(datasource.DataSource{
		Type: datasource.TypeElasticsearch,
		Host: "example.com:9200",
		Port: 0,
	})
	if err != nil {
		t.Fatalf("elasticsearchBaseURL: %v", err)
	}
	if baseURL != "http://example.com:9200" {
		t.Fatalf("expected http://example.com:9200, got %q", baseURL)
	}
}
