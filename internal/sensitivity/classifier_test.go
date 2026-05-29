package sensitivity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseClassificationResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "plain JSON",
			input: `[{"entity":"users","fields":[{"name":"email","level":"high","category":"pii","reason":"email","confidence":0.95}]}]`,
			want:  1,
		},
		{
			name:  "markdown fenced",
			input: "```json\n" + `[{"entity":"users","fields":[{"name":"id","level":"low","category":"identifier","reason":"pk","confidence":0.99}]}]` + "\n```",
			want:  1,
		},
		{
			name:  "with preamble text",
			input: "Here are the results:\n\n" + `[{"entity":"t1","fields":[]}]`,
			want:  1,
		},
		{
			name:    "no JSON array",
			input:   "I cannot classify these fields.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parseClassificationResponse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.want {
				t.Errorf("got %d results, want %d", len(results), tt.want)
			}
		})
	}
}

func TestToFieldClassification(t *testing.T) {
	tests := []struct {
		name       string
		input      AIFieldClassResult
		wantLevel  SensitivityLevel
		wantSource ClassificationSource
	}{
		{
			name:       "high confidence legacy maps to L4",
			input:      AIFieldClassResult{Name: "email", Level: "high", Category: "pii", Confidence: 0.95},
			wantLevel:  "L4",
			wantSource: SourceAI,
		},
		{
			name:       "low confidence becomes unconfirmed",
			input:      AIFieldClassResult{Name: "data", Level: "L3", Category: "behavioral", Confidence: 0.5},
			wantLevel:  LevelUnconfirmed,
			wantSource: SourceAI,
		},
		{
			name:       "unknown level becomes unconfirmed",
			input:      AIFieldClassResult{Name: "x", Level: "unknown", Category: "none", Confidence: 0.9},
			wantLevel:  LevelUnconfirmed,
			wantSource: SourceAI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := ToFieldClassification(tt.input, nil)
			if fc.Level != tt.wantLevel {
				t.Errorf("level = %q, want %q", fc.Level, tt.wantLevel)
			}
			if fc.Source != tt.wantSource {
				t.Errorf("source = %q, want %q", fc.Source, tt.wantSource)
			}
		})
	}
}

type mockModel struct {
	response string
	err      error
}

func (m *mockModel) Chat(_ context.Context, _ string, _ []ChatMessage) (string, error) {
	return m.response, m.err
}

type mockModelFunc struct {
	fn func(context.Context, string, []ChatMessage) (string, error)
}

func (m *mockModelFunc) Chat(ctx context.Context, sp string, msgs []ChatMessage) (string, error) {
	return m.fn(ctx, sp, msgs)
}

func TestExpandWideEntities(t *testing.T) {
	// Create an entity with 250 fields (exceeds maxFieldsPerEntityInPrompt=200)
	fields := make([]SchemaField, 250)
	for i := range fields {
		fields[i] = SchemaField{Name: fmt.Sprintf("col_%d", i), DataType: "varchar"}
	}
	entities := []SchemaEntity{
		{Entity: "narrow", Fields: []SchemaField{{Name: "id", DataType: "int"}}},
		{Entity: "wide", Fields: fields},
	}

	expanded := expandWideEntities(entities)

	// narrow stays as-is, wide split into 2 chunks (200 + 50)
	if len(expanded) != 3 {
		t.Fatalf("got %d expanded entities, want 3", len(expanded))
	}
	if expanded[0].Entity != "narrow" || len(expanded[0].Fields) != 1 {
		t.Errorf("narrow entity unexpected: %s %d fields", expanded[0].Entity, len(expanded[0].Fields))
	}
	if expanded[1].Entity != "wide" || len(expanded[1].Fields) != 200 {
		t.Errorf("wide chunk 1: %s %d fields, want wide 200", expanded[1].Entity, len(expanded[1].Fields))
	}
	if expanded[2].Entity != "wide" || len(expanded[2].Fields) != 50 {
		t.Errorf("wide chunk 2: %s %d fields, want wide 50", expanded[2].Entity, len(expanded[2].Fields))
	}
}

func TestMergeChunkedResults(t *testing.T) {
	results := []AIClassificationResult{
		{Entity: "wide", Fields: []AIFieldClassResult{{Name: "a", Level: "low"}}},
		{Entity: "other", Fields: []AIFieldClassResult{{Name: "x", Level: "high"}}},
		{Entity: "wide", Fields: []AIFieldClassResult{{Name: "b", Level: "medium"}}},
	}
	merged := mergeChunkedResults(results)
	if len(merged) != 2 {
		t.Fatalf("got %d merged, want 2", len(merged))
	}
	// Find the "wide" entry
	for _, r := range merged {
		if r.Entity == "wide" {
			if len(r.Fields) != 2 {
				t.Errorf("wide fields = %d, want 2", len(r.Fields))
			}
		}
	}
}

func TestClassifyEntitiesPartialOnError(t *testing.T) {
	callCount := 0
	model := &mockModelFunc{fn: func(_ context.Context, _ string, _ []ChatMessage) (string, error) {
		callCount++
		if callCount == 1 {
			r := []AIClassificationResult{{Entity: "t1", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Confidence: 0.9}}}}
			b, _ := json.Marshal(r)
			return string(b), nil
		}
		return "", errors.New("AI unavailable")
	}}

	// 2 entities → 2 batches (batch size is 20, but we need 2 separate calls)
	// Actually with batch size 20, we need >20 entities for 2 batches.
	// Let's create 21 entities so batch 1 has 20, batch 2 has 1.
	entities := make([]SchemaEntity, 21)
	for i := range entities {
		entities[i] = SchemaEntity{Entity: fmt.Sprintf("t%d", i), Fields: []SchemaField{{Name: "id", DataType: "int"}}}
	}

	results, err := ClassifyEntities(context.Background(), model, entities, "", nil)
	if err == nil {
		t.Fatal("expected error from second batch")
	}
	// Should have partial results from the first batch
	if len(results) != 1 {
		t.Errorf("got %d partial results, want 1 (from first successful batch)", len(results))
	}
}

func TestClassifyEntitiesRetriesSmallerBatchesWhenResponseIsTruncated(t *testing.T) {
	model := &mockModelFunc{fn: func(_ context.Context, _ string, msgs []ChatMessage) (string, error) {
		if len(msgs) != 1 {
			t.Fatalf("expected one message, got %d", len(msgs))
		}
		var entities []SchemaEntity
		raw := strings.TrimPrefix(msgs[0].Content, "Classify the sensitivity of each field in these database entities:\n\n")
		if err := json.Unmarshal([]byte(raw), &entities); err != nil {
			t.Fatalf("unmarshal request entities: %v", err)
		}
		if len(entities) > 5 {
			return `[{"entity":"users","fields":[{"name":"email","level":"high","category":"pii","reason":"email","confidence":0.95}]}`, nil
		}
		results := make([]AIClassificationResult, 0, len(entities))
		for _, entity := range entities {
			results = append(results, AIClassificationResult{
				Entity: entity.Entity,
				Fields: []AIFieldClassResult{
					{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99},
				},
			})
		}
		b, _ := json.Marshal(results)
		return string(b), nil
	}}

	entities := make([]SchemaEntity, 6)
	for i := range entities {
		entities[i] = SchemaEntity{
			Entity: fmt.Sprintf("t%d", i),
			Fields: []SchemaField{{Name: "id", DataType: "int"}},
		}
	}

	results, err := ClassifyEntities(context.Background(), model, entities, "", nil)
	if err != nil {
		t.Fatalf("expected truncation retry to succeed, got %v", err)
	}
	if len(results) != len(entities) {
		t.Fatalf("got %d results, want %d", len(results), len(entities))
	}
}

func TestClassifyEntitiesKeepsRightSideResultsWhenLeftSplitFails(t *testing.T) {
	model := &mockModelFunc{fn: func(_ context.Context, _ string, msgs []ChatMessage) (string, error) {
		if len(msgs) != 1 {
			t.Fatalf("expected one message, got %d", len(msgs))
		}
		var entities []SchemaEntity
		raw := strings.TrimPrefix(msgs[0].Content, "Classify the sensitivity of each field in these database entities:\n\n")
		if err := json.Unmarshal([]byte(raw), &entities); err != nil {
			t.Fatalf("unmarshal request entities: %v", err)
		}
		switch len(entities) {
		case 5:
			return `[{"entity":"broken","fields":[{"name":"id","level":"low","category":"identifier","reason":"pk","confidence":0.99}]}`, nil
		case 2:
			if entities[0].Entity == "t0" {
				return `[{"entity":"broken","fields":[{"name":"id","level":"low","category":"identifier","reason":"pk","confidence":0.99}]}`, nil
			}
		case 1:
			if entities[0].Entity == "t0" {
				return `[{"entity":"broken","fields":[{"name":"id","level":"low","category":"identifier","reason":"pk","confidence":0.99}]}`, nil
			}
		}
		results := make([]AIClassificationResult, 0, len(entities))
		for _, entity := range entities {
			results = append(results, AIClassificationResult{
				Entity: entity.Entity,
				Fields: []AIFieldClassResult{
					{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99},
				},
			})
		}
		b, _ := json.Marshal(results)
		return string(b), nil
	}}

	entities := make([]SchemaEntity, 5)
	for i := range entities {
		entities[i] = SchemaEntity{
			Entity: fmt.Sprintf("t%d", i),
			Fields: []SchemaField{{Name: "id", DataType: "int"}},
		}
	}

	results, err := ClassifyEntities(context.Background(), model, entities, "", nil)
	if err == nil {
		t.Fatal("expected error when left split remains unparseable")
	}
	if len(results) != 4 {
		t.Fatalf("got %d partial results, want 4 from right side and left-success branch", len(results))
	}
	for _, want := range []string{"t1", "t2", "t3", "t4"} {
		found := false
		for _, result := range results {
			if result.Entity == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected partial results to keep %s", want)
		}
	}
}

func TestClassifyEntitiesCustomRulesInPrompt(t *testing.T) {
	expected := []AIClassificationResult{
		{Entity: "t1", Fields: []AIFieldClassResult{{Name: "wechat_id", Level: "high", Category: "contact", Reason: "user rule", Confidence: 0.95}}},
	}
	responseJSON, _ := json.Marshal(expected)

	var capturedPrompt string
	model := &mockModelFunc{fn: func(_ context.Context, sp string, _ []ChatMessage) (string, error) {
		capturedPrompt = sp
		return string(responseJSON), nil
	}}
	entities := []SchemaEntity{{Entity: "t1", Fields: []SchemaField{{Name: "wechat_id", DataType: "varchar"}}}}

	_, err := ClassifyEntities(context.Background(), model, entities, "Fields containing wechat are PII contact info", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedPrompt, "User-Defined Rules") {
		t.Error("system prompt missing User-Defined Rules section")
	}
	if !strings.Contains(capturedPrompt, "wechat are PII") {
		t.Error("system prompt missing custom rule content")
	}
}

func TestClassifyEntitiesNoCustomRules(t *testing.T) {
	expected := []AIClassificationResult{
		{Entity: "t1", Fields: []AIFieldClassResult{{Name: "id", Level: "low", Category: "identifier", Reason: "pk", Confidence: 0.99}}},
	}
	responseJSON, _ := json.Marshal(expected)

	var capturedPrompt string
	model := &mockModelFunc{fn: func(_ context.Context, sp string, _ []ChatMessage) (string, error) {
		capturedPrompt = sp
		return string(responseJSON), nil
	}}
	entities := []SchemaEntity{{Entity: "t1", Fields: []SchemaField{{Name: "id", DataType: "int"}}}}

	_, err := ClassifyEntities(context.Background(), model, entities, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(capturedPrompt, "User-Defined Rules") {
		t.Error("system prompt should not contain User-Defined Rules when no custom rules provided")
	}
}

func TestClassifyEntities(t *testing.T) {
	expected := []AIClassificationResult{
		{
			Entity: "users",
			Fields: []AIFieldClassResult{
				{Name: "email", Level: "high", Category: "pii", Reason: "email field", Confidence: 0.95},
				{Name: "id", Level: "low", Category: "identifier", Reason: "primary key", Confidence: 0.99},
			},
		},
	}
	responseJSON, _ := json.Marshal(expected)

	model := &mockModel{response: string(responseJSON)}
	entities := []SchemaEntity{
		{Entity: "users", Fields: []SchemaField{{Name: "email", DataType: "varchar(255)"}, {Name: "id", DataType: "int"}}},
	}

	results, err := ClassifyEntities(context.Background(), model, entities, "", nil)
	if err != nil {
		t.Fatalf("ClassifyEntities: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if len(results[0].Fields) != 2 {
		t.Errorf("got %d fields, want 2", len(results[0].Fields))
	}
}
