package console

import "testing"

func TestParseElasticsearchTargets_SingleIndex(t *testing.T) {
	targets, err := ParseElasticsearchTargets("POST /futrixdata-demo-1/_search\n{\"query\":{\"match_all\":{}}}")
	if err != nil {
		t.Fatalf("ParseElasticsearchTargets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "futrixdata-demo-1" {
		t.Fatalf("expected [futrixdata-demo-1], got %#v", targets)
	}
}

func TestParseElasticsearchTargets_MultiIndex(t *testing.T) {
	targets, err := ParseElasticsearchTargets("GET /idx1,idx2/_search\n{}")
	if err != nil {
		t.Fatalf("ParseElasticsearchTargets: %v", err)
	}
	if len(targets) != 2 || targets[0] != "idx1" || targets[1] != "idx2" {
		t.Fatalf("expected [idx1 idx2], got %#v", targets)
	}
}

func TestParseElasticsearchTargets_NoIndex(t *testing.T) {
	targets, err := ParseElasticsearchTargets("GET /_cat/indices?v")
	if err != nil {
		t.Fatalf("ParseElasticsearchTargets: %v", err)
	}
	if targets != nil {
		t.Fatalf("expected nil targets, got %#v", targets)
	}
}
