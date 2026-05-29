package sensitivity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	classifyBatchSize          = 5
	confidenceThreshold        = 0.7
	maxFieldsPerEntityInPrompt = 200
)

// Model is the AI model interface for sensitivity classification.
type Model interface {
	Chat(ctx context.Context, systemPrompt string, messages []ChatMessage) (string, error)
}

// ChatMessage is a chat message for the AI model.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ModelResolver resolves an AI model by config ID.
type ModelResolver interface {
	Resolve(aiConfigID string) (Model, error)
}

// buildClassificationSystemPrompt constructs the AI system prompt dynamically
// from the configured level definitions so the AI uses the correct level keys.
func buildClassificationSystemPrompt(levels []LevelDefinition) string {
	var b strings.Builder
	b.WriteString(`You are a data security expert specializing in sensitive data classification.

Your task: analyze database schema field names and data types to classify each field's sensitivity level.

## Classification Levels (from lowest to highest sensitivity)

`)
	for _, l := range levels {
		// Always use Name/Description (user-editable) as the primary values so
		// user renames take effect in classification prompts.  Append the English
		// shadow fields only when they differ — this gives the AI bilingual
		// context without ignoring user edits.
		name := l.Name
		desc := l.Description
		if l.NameEn != "" && l.NameEn != l.Name {
			name = fmt.Sprintf("%s / %s", l.Name, l.NameEn)
		}
		if l.DescriptionEn != "" && l.DescriptionEn != l.Description {
			desc = fmt.Sprintf("%s (%s)", l.Description, l.DescriptionEn)
		}
		b.WriteString(fmt.Sprintf("- %s (%s): %s", l.Key, name, desc))
		if len(l.Examples) > 0 {
			b.WriteString(fmt.Sprintf(" (e.g. %s)", strings.Join(l.Examples, ", ")))
		}
		b.WriteString("\n")
	}

	// Collect valid level keys for the prompt
	keys := make([]string, 0, len(levels))
	for _, l := range levels {
		keys = append(keys, l.Key)
	}

	b.WriteString(`
## Categories

pii, credential, financial, behavioral, medical, location, contact, identifier, none

## Rules

1. Classify based on field NAME and DATA TYPE only — you have no access to actual data values.
2. When uncertain, err on the side of higher sensitivity.
3. Provide a confidence score (0.0–1.0) for each classification.
4. Use ONLY these level keys: `)
	b.WriteString(strings.Join(keys, ", "))
	b.WriteString(`.
5. Be especially strict with: names containing "password", "secret", "token", "key", "ssn", "credit", "card", "email", "phone", "address", "salary", "income", "medical", "diagnosis", "dob", "birth".

## Output Format

Return ONLY a valid JSON array (no markdown, no explanation):
[
  {
    "entity": "table_name",
    "fields": [
      {"name": "field_name", "level": "`)
	if len(keys) > 0 {
		b.WriteString(keys[len(keys)-1])
	} else {
		b.WriteString("L5")
	}
	b.WriteString(`", "category": "pii", "reason": "brief reason", "confidence": 0.95}
    ]
  }
]`)
	return b.String()
}

// ClassifyEntities sends schema entities to the AI model for sensitivity classification.
// customRules is an optional user-provided hint (e.g. "fields with 'phone' are PII")
// that is appended to the system prompt.
// levels defines the sensitivity levels the AI should use; if nil, defaults are used.
func ClassifyEntities(ctx context.Context, model Model, entities []SchemaEntity, customRules string, levels []LevelDefinition) ([]AIClassificationResult, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	if len(levels) == 0 {
		defaults := DefaultLevelConfig()
		levels = defaults.Levels
	}

	systemPrompt := buildClassificationSystemPrompt(levels)
	if rules := strings.TrimSpace(customRules); rules != "" {
		systemPrompt += "\n\n## User-Defined Rules\n\nThe user has provided additional classification guidance. Apply these rules with higher priority than the defaults:\n\n" + rules
	}

	// Expand wide entities into multiple chunks so every field gets classified.
	expanded := expandWideEntities(entities)

	var allResults []AIClassificationResult
	for i := 0; i < len(expanded); i += classifyBatchSize {
		end := i + classifyBatchSize
		if end > len(expanded) {
			end = len(expanded)
		}
		batch := expanded[i:end]
		results, err := classifyBatch(ctx, model, systemPrompt, batch)
		if err != nil {
			return mergeChunkedResults(append(allResults, results...)), err
		}
		allResults = append(allResults, results...)
	}

	// Merge chunks for the same entity back together.
	return mergeChunkedResults(allResults), nil
}

func classifyBatch(ctx context.Context, model Model, systemPrompt string, batch []SchemaEntity) ([]AIClassificationResult, error) {
	payload, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal schema batch: %w", err)
	}

	userMsg := fmt.Sprintf("Classify the sensitivity of each field in these database entities:\n\n%s", string(payload))
	response, err := model.Chat(ctx, systemPrompt, []ChatMessage{{Role: "user", Content: userMsg}})
	if err != nil {
		return nil, fmt.Errorf("AI classification failed: %w", err)
	}

	results, err := parseClassificationResponse(response)
	if err == nil {
		return results, nil
	}
	if !shouldRetryWithSmallerBatch(err, len(batch)) {
		return nil, fmt.Errorf("parse AI response: %w", err)
	}

	mid := len(batch) / 2
	leftResults, leftErr := classifyBatch(ctx, model, systemPrompt, batch[:mid])
	rightResults, rightErr := classifyBatch(ctx, model, systemPrompt, batch[mid:])
	combined := append(leftResults, rightResults...)
	if leftErr != nil {
		return mergeChunkedResults(combined), leftErr
	}
	if rightErr != nil {
		return mergeChunkedResults(combined), rightErr
	}
	return mergeChunkedResults(combined), nil
}

func shouldRetryWithSmallerBatch(err error, batchSize int) bool {
	if err == nil || batchSize <= 1 {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unexpected end of json input") ||
		strings.Contains(msg, "no json array found")
}

// expandWideEntities splits entities with more than maxFieldsPerEntityInPrompt fields
// into multiple SchemaEntity entries so all fields are classified.
func expandWideEntities(entities []SchemaEntity) []SchemaEntity {
	var result []SchemaEntity
	for _, e := range entities {
		if len(e.Fields) <= maxFieldsPerEntityInPrompt {
			result = append(result, e)
			continue
		}
		for j := 0; j < len(e.Fields); j += maxFieldsPerEntityInPrompt {
			end := j + maxFieldsPerEntityInPrompt
			if end > len(e.Fields) {
				end = len(e.Fields)
			}
			chunk := SchemaEntity{
				Entity: e.Entity,
				Fields: make([]SchemaField, end-j),
			}
			copy(chunk.Fields, e.Fields[j:end])
			result = append(result, chunk)
		}
	}
	return result
}

// mergeChunkedResults combines AI results for the same entity (from field chunking).
func mergeChunkedResults(results []AIClassificationResult) []AIClassificationResult {
	seen := make(map[string]int) // entity name → index in merged
	var merged []AIClassificationResult
	for _, r := range results {
		if idx, ok := seen[r.Entity]; ok {
			merged[idx].Fields = append(merged[idx].Fields, r.Fields...)
		} else {
			seen[r.Entity] = len(merged)
			merged = append(merged, r)
		}
	}
	return merged
}

// parseClassificationResponse extracts the JSON array from the AI response.
func parseClassificationResponse(response string) ([]AIClassificationResult, error) {
	trimmed := strings.TrimSpace(response)

	// Strip markdown code fences if present
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		start, end := 0, len(lines)
		for i, line := range lines {
			if i == 0 && strings.HasPrefix(strings.TrimSpace(line), "```") {
				start = i + 1
				continue
			}
			if i > 0 && strings.TrimSpace(line) == "```" {
				end = i
				break
			}
		}
		trimmed = strings.Join(lines[start:end], "\n")
		trimmed = strings.TrimSpace(trimmed)
	}

	// Find JSON array boundaries
	startIdx := strings.Index(trimmed, "[")
	endIdx := strings.LastIndex(trimmed, "]")
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return nil, fmt.Errorf("no JSON array found in response")
	}
	jsonStr := trimmed[startIdx : endIdx+1]

	var results []AIClassificationResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil, fmt.Errorf("unmarshal classification results: %w", err)
	}
	return results, nil
}

// ToFieldClassification converts an AI result to our stored classification,
// marking low-confidence results as unconfirmed.
// validKeys are the configured level keys (e.g. "L1", "L2", ...); if nil, defaults are used.
func ToFieldClassification(r AIFieldClassResult, validKeys map[string]bool) FieldClassification {
	level := normalizeSensitivityLevel(r.Level, validKeys)
	category := normalizeCategory(r.Category)

	if r.Confidence < confidenceThreshold {
		level = LevelUnconfirmed
	}

	return FieldClassification{
		Level:    level,
		Category: category,
		Reason:   strings.TrimSpace(r.Reason),
		Source:   SourceAI,
	}
}

// normalizeSensitivityLevel matches the AI output against configured level keys.
// Supports both new keys (L1-L5) and legacy keys (critical/high/medium/low).
func normalizeSensitivityLevel(s string, validKeys map[string]bool) SensitivityLevel {
	if validKeys == nil {
		defaults := DefaultLevelConfig()
		validKeys = make(map[string]bool, len(defaults.Levels))
		for _, l := range defaults.Levels {
			validKeys[l.Key] = true
		}
	}
	trimmed := strings.TrimSpace(s)
	// Exact match
	if validKeys[trimmed] {
		return SensitivityLevel(trimmed)
	}
	// Case-insensitive match
	upper := strings.ToUpper(trimmed)
	for k := range validKeys {
		if strings.ToUpper(k) == upper {
			return SensitivityLevel(k)
		}
	}
	// Legacy fallback
	legacyMap := map[string]string{
		"critical": "L5", "high": "L4", "medium": "L3", "low": "L2",
	}
	if mapped, ok := legacyMap[strings.ToLower(trimmed)]; ok && validKeys[mapped] {
		return SensitivityLevel(mapped)
	}
	return LevelUnconfirmed
}

func normalizeCategory(s string) Category {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pii":
		return CategoryPII
	case "credential":
		return CategoryCredential
	case "financial":
		return CategoryFinancial
	case "behavioral":
		return CategoryBehavioral
	case "medical":
		return CategoryMedical
	case "location":
		return CategoryLocation
	case "contact":
		return CategoryContact
	case "identifier":
		return CategoryIdentifier
	case "none":
		return CategoryNone
	default:
		return CategoryNone
	}
}
