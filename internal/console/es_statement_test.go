package console

import "testing"

func TestParseElasticsearchStatement_NoBody(t *testing.T) {
	stmt, err := parseElasticsearchStatement("GET /_cat/indices?v")
	if err != nil {
		t.Fatalf("parseElasticsearchStatement: %v", err)
	}
	if stmt.Method != "GET" {
		t.Fatalf("expected method GET, got %q", stmt.Method)
	}
	if stmt.Path != "/_cat/indices?v" {
		t.Fatalf("expected path /_cat/indices?v, got %q", stmt.Path)
	}
	if stmt.Body != "" {
		t.Fatalf("expected empty body, got %q", stmt.Body)
	}
}

func TestParseElasticsearchStatement_WithBody(t *testing.T) {
	raw := "POST /futrixdata-demo-1/_search\n{\n  \"query\": {\"match_all\": {}},\n  \"size\": 2\n}"
	stmt, err := parseElasticsearchStatement(raw)
	if err != nil {
		t.Fatalf("parseElasticsearchStatement: %v", err)
	}
	if stmt.Method != "POST" {
		t.Fatalf("expected method POST, got %q", stmt.Method)
	}
	if stmt.Path != "/futrixdata-demo-1/_search" {
		t.Fatalf("expected path /futrixdata-demo-1/_search, got %q", stmt.Path)
	}
	if stmt.Body == "" {
		t.Fatalf("expected body, got empty")
	}
}

func TestParseElasticsearchStatement_InvalidFirstLine(t *testing.T) {
	_, err := parseElasticsearchStatement("/_search")
	if err == nil {
		t.Fatalf("expected error")
	}
}
