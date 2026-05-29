package aichat

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	webSearchEngineAuto       = "auto"
	webSearchEngineGoogle     = "google"
	webSearchEngineDuckDuckGo = "duckduckgo"
	webSearchEngineBing       = "bing"
	webSearchMaxResultsLimit  = 10
	webSearchBodySizeLimit    = 2 << 20 // 2MB
)

var (
	webTagPattern        = regexp.MustCompile(`(?is)<[^>]+>`)
	webWhitespacePattern = regexp.MustCompile(`\s+`)

	duckResultLinkPattern    = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	duckResultSnippetPattern = regexp.MustCompile(`(?is)<(?:a|div)[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</(?:a|div)>`)

	bingResultBlockPattern   = regexp.MustCompile(`(?is)<li[^>]*class="[^"]*b_algo[^"]*"[^>]*>(.*?)</li>`)
	bingResultLinkPattern    = regexp.MustCompile(`(?is)<h2[^>]*>\s*<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	bingResultSnippetPattern = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)

	googleResultLinkPattern    = regexp.MustCompile(`(?is)<a[^>]*href="([^"]+)"[^>]*>\s*<h3[^>]*>(.*?)</h3>\s*</a>`)
	googleResultSnippetPattern = regexp.MustCompile(`(?is)<div[^>]*class="[^"]*(?:VwiC3b|s3v9rd|lyLwlc)[^"]*"[^>]*>(.*?)</div>`)
)

type WebSearchProvider interface {
	Search(ctx context.Context, req WebSearchRequest) (WebSearchResponse, error)
}

type webSearchEndpoints struct {
	Google        string
	GoogleNewsRSS string
	DuckDuckGo    string
	Bing          string
}

type httpWebSearchProvider struct {
	client    *http.Client
	endpoints webSearchEndpoints
}

func defaultWebSearchEndpoints() webSearchEndpoints {
	return webSearchEndpoints{
		Google:        "https://www.google.com/search",
		GoogleNewsRSS: "https://news.google.com/rss/search",
		DuckDuckGo:    "https://duckduckgo.com/html/",
		Bing:          "https://www.bing.com/search",
	}
}

func newDefaultWebSearchProvider() WebSearchProvider {
	return newHTTPWebSearchProvider(nil, defaultWebSearchEndpoints())
}

func newHTTPWebSearchProvider(client *http.Client, endpoints webSearchEndpoints) *httpWebSearchProvider {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	defaults := defaultWebSearchEndpoints()
	if strings.TrimSpace(endpoints.Google) == "" {
		endpoints.Google = defaults.Google
	}
	if strings.TrimSpace(endpoints.GoogleNewsRSS) == "" {
		endpoints.GoogleNewsRSS = defaults.GoogleNewsRSS
	}
	if strings.TrimSpace(endpoints.DuckDuckGo) == "" {
		endpoints.DuckDuckGo = defaults.DuckDuckGo
	}
	if strings.TrimSpace(endpoints.Bing) == "" {
		endpoints.Bing = defaults.Bing
	}
	return &httpWebSearchProvider{
		client:    client,
		endpoints: endpoints,
	}
}

func (p *httpWebSearchProvider) Search(ctx context.Context, req WebSearchRequest) (WebSearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return WebSearchResponse{}, errors.New("query is required")
	}
	engine, err := normalizeWebSearchEngine(req.Engine)
	if err != nil {
		return WebSearchResponse{}, err
	}
	maxResults := clampWebSearchMaxResults(req.MaxResults)

	if engine != webSearchEngineAuto {
		results, err := p.searchSingleEngine(ctx, engine, query, maxResults)
		if err != nil {
			return WebSearchResponse{}, err
		}
		return WebSearchResponse{
			Query:   query,
			Engine:  engine,
			Results: results,
		}, nil
	}

	engines := []string{webSearchEngineDuckDuckGo, webSearchEngineBing, webSearchEngineGoogle}
	merged := make([]WebSearchResult, 0, maxResults)
	seen := map[string]struct{}{}
	warnings := make([]string, 0, len(engines))
	for _, current := range engines {
		results, searchErr := p.searchSingleEngine(ctx, current, query, maxResults)
		if searchErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %s", current, searchErr.Error()))
			continue
		}
		for _, item := range results {
			if _, ok := seen[item.URL]; ok {
				continue
			}
			seen[item.URL] = struct{}{}
			merged = append(merged, item)
			if len(merged) >= maxResults {
				break
			}
		}
		if len(merged) >= maxResults {
			break
		}
	}
	if len(merged) == 0 {
		return WebSearchResponse{}, errors.New("web search returned no results")
	}
	return WebSearchResponse{
		Query:    query,
		Engine:   webSearchEngineAuto,
		Results:  merged,
		Warnings: warnings,
	}, nil
}

func (p *httpWebSearchProvider) searchSingleEngine(ctx context.Context, engine string, query string, maxResults int) ([]WebSearchResult, error) {
	endpoint, err := p.endpointForEngine(engine)
	if err != nil {
		return nil, err
	}
	body, err := p.fetchSearchHTML(ctx, endpoint, query, nil)
	if err != nil {
		return nil, err
	}

	results := parseWebSearchResults(engine, body)
	if len(results) == 0 && engine == webSearchEngineBing {
		rssBody, rssErr := p.fetchSearchHTML(ctx, endpoint, query, map[string]string{"format": "rss"})
		if rssErr == nil {
			results = parseWebSearchResults(engine, rssBody)
		}
	}
	if len(results) == 0 && engine == webSearchEngineGoogle {
		rssBody, rssErr := p.fetchSearchHTML(ctx, p.endpoints.GoogleNewsRSS, query, map[string]string{
			"hl":   "en-US",
			"gl":   "US",
			"ceid": "US:en",
		})
		if rssErr == nil {
			results = parseRSSResults(rssBody, webSearchEngineGoogle)
		}
	}
	if len(results) == 0 {
		return nil, errors.New("no parseable results")
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func (p *httpWebSearchProvider) endpointForEngine(engine string) (string, error) {
	switch engine {
	case webSearchEngineGoogle:
		return p.endpoints.Google, nil
	case webSearchEngineDuckDuckGo:
		return p.endpoints.DuckDuckGo, nil
	case webSearchEngineBing:
		return p.endpoints.Bing, nil
	default:
		return "", fmt.Errorf("unsupported engine: %s", strings.TrimSpace(engine))
	}
}

func (p *httpWebSearchProvider) fetchSearchHTML(ctx context.Context, endpoint string, query string, extraParams map[string]string) (string, error) {
	baseURL, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("invalid search endpoint: %w", err)
	}
	values := baseURL.Query()
	values.Set("q", query)
	for key, value := range extraParams {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values.Set(key, strings.TrimSpace(value))
	}
	baseURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) FutrixDataAI/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("search http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, webSearchBodySizeLimit))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseWebSearchResults(engine string, body string) []WebSearchResult {
	switch engine {
	case webSearchEngineDuckDuckGo:
		return parseDuckDuckGoResults(body)
	case webSearchEngineBing:
		return parseBingResults(body)
	case webSearchEngineGoogle:
		return parseGoogleResults(body)
	default:
		return nil
	}
}

func parseDuckDuckGoResults(body string) []WebSearchResult {
	links := duckResultLinkPattern.FindAllStringSubmatch(body, webSearchMaxResultsLimit*2)
	snippets := duckResultSnippetPattern.FindAllStringSubmatch(body, webSearchMaxResultsLimit*2)

	out := make([]WebSearchResult, 0, len(links))
	for i, item := range links {
		u := normalizeWebSearchURL(item[1], webSearchEngineDuckDuckGo)
		title := cleanWebSearchHTMLText(item[2])
		if u == "" || title == "" {
			continue
		}
		snippet := ""
		if i < len(snippets) && len(snippets[i]) > 1 {
			snippet = cleanWebSearchHTMLText(snippets[i][1])
		}
		out = append(out, WebSearchResult{
			Engine:  webSearchEngineDuckDuckGo,
			Title:   title,
			URL:     u,
			Snippet: snippet,
		})
	}
	return out
}

func parseBingResults(body string) []WebSearchResult {
	if rss := parseRSSResults(body, webSearchEngineBing); len(rss) > 0 {
		return rss
	}
	blocks := bingResultBlockPattern.FindAllStringSubmatch(body, webSearchMaxResultsLimit*2)
	out := make([]WebSearchResult, 0, len(blocks))
	for _, block := range blocks {
		if len(block) < 2 {
			continue
		}
		link := bingResultLinkPattern.FindStringSubmatch(block[1])
		if len(link) < 3 {
			continue
		}
		u := normalizeWebSearchURL(link[1], webSearchEngineBing)
		title := cleanWebSearchHTMLText(link[2])
		if u == "" || title == "" {
			continue
		}
		snippet := ""
		if parsed := bingResultSnippetPattern.FindStringSubmatch(block[1]); len(parsed) > 1 {
			snippet = cleanWebSearchHTMLText(parsed[1])
		}
		out = append(out, WebSearchResult{
			Engine:  webSearchEngineBing,
			Title:   title,
			URL:     u,
			Snippet: snippet,
		})
	}
	return out
}

func parseGoogleResults(body string) []WebSearchResult {
	links := googleResultLinkPattern.FindAllStringSubmatch(body, webSearchMaxResultsLimit*3)
	snippets := googleResultSnippetPattern.FindAllStringSubmatch(body, webSearchMaxResultsLimit*3)

	out := make([]WebSearchResult, 0, len(links))
	for i, item := range links {
		u := normalizeWebSearchURL(item[1], webSearchEngineGoogle)
		title := cleanWebSearchHTMLText(item[2])
		if u == "" || title == "" {
			continue
		}
		snippet := ""
		if i < len(snippets) && len(snippets[i]) > 1 {
			snippet = cleanWebSearchHTMLText(snippets[i][1])
		}
		out = append(out, WebSearchResult{
			Engine:  webSearchEngineGoogle,
			Title:   title,
			URL:     u,
			Snippet: snippet,
		})
	}
	if len(out) > 0 {
		return out
	}
	return parseRSSResults(body, webSearchEngineGoogle)
}

type webSearchRSS struct {
	Channel webSearchRSSChannel `xml:"channel"`
}

type webSearchRSSChannel struct {
	Items []webSearchRSSItem `xml:"item"`
}

type webSearchRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
}

func parseRSSResults(body string, engine string) []WebSearchResult {
	var payload webSearchRSS
	if err := xml.Unmarshal([]byte(body), &payload); err != nil {
		return nil
	}
	if len(payload.Channel.Items) == 0 {
		return nil
	}
	out := make([]WebSearchResult, 0, len(payload.Channel.Items))
	for _, item := range payload.Channel.Items {
		title := cleanWebSearchHTMLText(item.Title)
		u := normalizeWebSearchURL(item.Link, engine)
		if title == "" || u == "" {
			continue
		}
		out = append(out, WebSearchResult{
			Engine:  engine,
			Title:   title,
			URL:     u,
			Snippet: cleanWebSearchHTMLText(item.Description),
		})
		if len(out) >= webSearchMaxResultsLimit*2 {
			break
		}
	}
	return out
}

func normalizeWebSearchEngine(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", webSearchEngineAuto:
		return webSearchEngineAuto, nil
	case "ddg", "duck", webSearchEngineDuckDuckGo:
		return webSearchEngineDuckDuckGo, nil
	case "ms", webSearchEngineBing:
		return webSearchEngineBing, nil
	case webSearchEngineGoogle:
		return webSearchEngineGoogle, nil
	default:
		return "", errors.New("engine must be one of: auto|google|duckduckgo|bing")
	}
}

func clampWebSearchMaxResults(raw int) int {
	if raw <= 0 {
		return 5
	}
	if raw > webSearchMaxResultsLimit {
		return webSearchMaxResultsLimit
	}
	return raw
}

func cleanWebSearchHTMLText(input string) string {
	text := webTagPattern.ReplaceAllString(input, " ")
	text = html.UnescapeString(text)
	text = strings.TrimSpace(text)
	return webWhitespacePattern.ReplaceAllString(text, " ")
}

func normalizeWebSearchURL(raw string, engine string) string {
	clean := html.UnescapeString(strings.TrimSpace(raw))
	if clean == "" {
		return ""
	}
	if strings.HasPrefix(clean, "//") {
		clean = "https:" + clean
	}
	if strings.HasPrefix(clean, "/url?") {
		parsed, err := url.Parse(clean)
		if err == nil {
			q := strings.TrimSpace(parsed.Query().Get("q"))
			if q != "" {
				clean = q
			}
		}
	}
	if engine == webSearchEngineDuckDuckGo && strings.Contains(clean, "duckduckgo.com/l/?") {
		parsed, err := url.Parse(clean)
		if err == nil {
			target := strings.TrimSpace(parsed.Query().Get("uddg"))
			if target != "" {
				decoded, decodeErr := url.QueryUnescape(target)
				if decodeErr == nil && decoded != "" {
					clean = decoded
				} else {
					clean = target
				}
			}
		}
	}
	if engine == webSearchEngineBing && strings.Contains(clean, "bing.com/ck/a") {
		parsed, err := url.Parse(clean)
		if err == nil {
			target := decodeBingTrackingURL(parsed.Query().Get("u"))
			if target != "" {
				clean = target
			}
		}
	}
	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") {
		return clean
	}
	return ""
}

func decodeBingTrackingURL(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "" {
		return ""
	}
	if unescaped, err := url.QueryUnescape(target); err == nil && strings.TrimSpace(unescaped) != "" {
		target = strings.TrimSpace(unescaped)
	}
	lower := strings.ToLower(target)
	for _, prefix := range []string{"a1", "a2", "u1"} {
		if strings.HasPrefix(lower, prefix) && len(target) > len(prefix) {
			target = target[len(prefix):]
			break
		}
	}
	if decoded := decodeWebSearchBase64URL(target); decoded != "" {
		return decoded
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	return ""
}

func decodeWebSearchBase64URL(raw string) string {
	candidates := []string{raw}
	if mod := len(raw) % 4; mod != 0 {
		candidates = append(candidates, raw+strings.Repeat("=", 4-mod))
	}
	decoders := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, candidate := range candidates {
		for _, dec := range decoders {
			decoded, err := dec.DecodeString(candidate)
			if err != nil {
				continue
			}
			text := strings.TrimSpace(string(decoded))
			if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
				return text
			}
		}
	}
	return ""
}
