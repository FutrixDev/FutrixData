package aichat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubWebSearchProvider struct {
	requests []WebSearchRequest
	response WebSearchResponse
	err      error
}

func (p *stubWebSearchProvider) Search(_ context.Context, req WebSearchRequest) (WebSearchResponse, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return WebSearchResponse{}, p.err
	}
	out := p.response
	if strings.TrimSpace(out.Query) == "" {
		out.Query = req.Query
	}
	if strings.TrimSpace(out.Engine) == "" {
		out.Engine = req.Engine
	}
	return out, nil
}

func TestTurn_WebSearch_ToolCallIncludesResults(t *testing.T) {
	model := &scriptedModel{
		responses: []string{
			`{"assistantMessage":"","toolCalls":[{"name":"web_search","arguments":{"query":"latest k8s release date","engine":"bing","maxResults":2}}]}`,
			`{"assistantMessage":"I found the references.","toolCalls":[]}`,
		},
	}
	provider := &stubWebSearchProvider{
		response: WebSearchResponse{
			Results: []WebSearchResult{
				{
					Engine:  "bing",
					Title:   "Kubernetes v1.31 release",
					URL:     "https://kubernetes.io/releases/",
					Snippet: "Release information.",
				},
			},
		},
	}

	svc := NewService(fakeResolver{model: model}, &fakeTools{})
	svc.SetWebSearchProvider(provider)

	resp, err := svc.Turn(context.Background(), TurnRequest{
		ConversationID: "chat_1",
		Messages:       []Message{{Role: "user", Content: "latest k8s release date"}},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(resp.AssistantMessage) != "I found the references." {
		t.Fatalf("unexpected assistant message: %q", resp.AssistantMessage)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected 1 web search request, got %d", len(provider.requests))
	}
	req := provider.requests[0]
	if req.Query != "latest k8s release date" {
		t.Fatalf("unexpected query: %q", req.Query)
	}
	if req.Engine != "bing" {
		t.Fatalf("unexpected engine: %q", req.Engine)
	}
	if req.MaxResults != 2 {
		t.Fatalf("unexpected maxResults: %d", req.MaxResults)
	}

	if len(model.received) < 2 {
		t.Fatalf("expected second model call with tool_result")
	}
	var saw bool
	for _, msg := range model.received[1] {
		if !strings.Contains(msg.Content, "[tool_result] web_search") {
			continue
		}
		if !strings.Contains(msg.Content, "kubernetes.io/releases") {
			continue
		}
		saw = true
		break
	}
	if !saw {
		t.Fatalf("expected web_search tool result to be included in second model call messages")
	}
}

func TestHTTPWebSearchProvider_Search_DuckDuckGo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=k8s") {
			t.Fatalf("expected q=k8s query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`
<html><body>
<div class="result">
  <a class="result__a" href="https://duck.example/a">Duck Result A</a>
  <a class="result__snippet">Duck snippet A</a>
</div>
</body></html>`))
	}))
	defer server.Close()

	provider := newHTTPWebSearchProvider(&http.Client{Timeout: 2 * time.Second}, webSearchEndpoints{
		DuckDuckGo: server.URL,
	})
	resp, err := provider.Search(context.Background(), WebSearchRequest{
		Query:      "k8s",
		Engine:     "duckduckgo",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Duck Result A" {
		t.Fatalf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://duck.example/a" {
		t.Fatalf("unexpected url: %q", resp.Results[0].URL)
	}
}

func TestHTTPWebSearchProvider_Search_Bing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=k8s") {
			t.Fatalf("expected q=k8s query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`
<html><body>
<li class="b_algo">
  <h2><a href="https://bing.example/a">Bing Result A</a></h2>
  <div class="b_caption"><p>Bing snippet A</p></div>
</li>
</body></html>`))
	}))
	defer server.Close()

	provider := newHTTPWebSearchProvider(&http.Client{Timeout: 2 * time.Second}, webSearchEndpoints{
		Bing: server.URL,
	})
	resp, err := provider.Search(context.Background(), WebSearchRequest{
		Query:      "k8s",
		Engine:     "bing",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Bing Result A" {
		t.Fatalf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://bing.example/a" {
		t.Fatalf("unexpected url: %q", resp.Results[0].URL)
	}
}

func TestNormalizeWebSearchURL_BingRedirect(t *testing.T) {
	raw := "https://www.bing.com/ck/a?!&&p=hash&u=a1aHR0cHM6Ly9vcGVuYWkuY29tLw&ntb=1"
	got := normalizeWebSearchURL(raw, webSearchEngineBing)
	if got != "https://openai.com/" {
		t.Fatalf("expected decoded url, got %q", got)
	}
}

func TestHTTPWebSearchProvider_Search_BingRSSFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=k8s") {
			t.Fatalf("expected q=k8s query, got %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("format") == "rss" {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
  <item><title>Bing RSS Result</title><link>https://bing.example/rss</link><description>Bing rss snippet</description></item>
</channel></rss>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>No parseable html blocks</body></html>`))
	}))
	defer server.Close()

	provider := newHTTPWebSearchProvider(&http.Client{Timeout: 2 * time.Second}, webSearchEndpoints{
		Bing: server.URL,
	})
	resp, err := provider.Search(context.Background(), WebSearchRequest{
		Query:      "k8s",
		Engine:     "bing",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Bing RSS Result" {
		t.Fatalf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://bing.example/rss" {
		t.Fatalf("unexpected url: %q", resp.Results[0].URL)
	}
}

func TestHTTPWebSearchProvider_Search_Google(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=k8s") {
			t.Fatalf("expected q=k8s query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`
<html><body>
<div class="g">
  <a href="/url?q=https://google.example/a&sa=U&ved=1"><h3>Google Result A</h3></a>
  <div class="VwiC3b">Google snippet A</div>
</div>
</body></html>`))
	}))
	defer server.Close()

	provider := newHTTPWebSearchProvider(&http.Client{Timeout: 2 * time.Second}, webSearchEndpoints{
		Google: server.URL,
	})
	resp, err := provider.Search(context.Background(), WebSearchRequest{
		Query:      "k8s",
		Engine:     "google",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Google Result A" {
		t.Fatalf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://google.example/a" {
		t.Fatalf("unexpected url: %q", resp.Results[0].URL)
	}
}

func TestHTTPWebSearchProvider_Search_GoogleNewsRSSFallback(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>No parseable google html</body></html>`))
	}))
	defer primary.Close()

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=k8s") {
			t.Fatalf("expected q=k8s query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
  <item><title>Google RSS Result</title><link>https://google.example/rss</link><description>Google rss snippet</description></item>
</channel></rss>`))
	}))
	defer rss.Close()

	provider := newHTTPWebSearchProvider(&http.Client{Timeout: 2 * time.Second}, webSearchEndpoints{
		Google:        primary.URL,
		GoogleNewsRSS: rss.URL,
	})
	resp, err := provider.Search(context.Background(), WebSearchRequest{
		Query:      "k8s",
		Engine:     "google",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "Google RSS Result" {
		t.Fatalf("unexpected title: %q", resp.Results[0].Title)
	}
	if resp.Results[0].URL != "https://google.example/rss" {
		t.Fatalf("unexpected url: %q", resp.Results[0].URL)
	}
}
