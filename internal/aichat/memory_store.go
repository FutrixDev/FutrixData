package aichat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"futrixdata/platform/internal/securefile"
)

type MemorySaveInput struct {
	Problem          string   `json:"problem"`
	Signals          []string `json:"signals,omitempty"`
	Avoid            []string `json:"avoid,omitempty"`
	Do               []string `json:"do,omitempty"`
	Why              string   `json:"why,omitempty"`
	Confidence       float64  `json:"confidence,omitempty"`
	EvidenceEventIDs []string `json:"evidenceEventIds,omitempty"`
	ReplaceHints     []string `json:"replaceHints,omitempty"`
}

type MemorySaveResult struct {
	Version      string         `json:"version,omitempty"`
	SavedPattern *MemoryPattern `json:"savedPattern,omitempty"`
	ArchivedIDs  []string       `json:"archivedIds,omitempty"`
}

type MemoryPattern struct {
	ID         string    `json:"id,omitempty"`
	Problem    string    `json:"problem"`
	Signals    []string  `json:"signals,omitempty"`
	Avoid      []string  `json:"avoid,omitempty"`
	Do         []string  `json:"do,omitempty"`
	Why        string    `json:"why,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	UseCount   int       `json:"useCount,omitempty"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	Archived   bool      `json:"archived,omitempty"`
}

type MemoryArchiveRef struct {
	ID       string `json:"id,omitempty"`
	File     string `json:"file,omitempty"`
	Problem  string `json:"problem,omitempty"`
	Summary  string `json:"summary,omitempty"`
	LastUsed int64  `json:"lastUsed,omitempty"`
	UseCount int    `json:"useCount,omitempty"`
}

type MemoryIndex struct {
	Version  string             `json:"version,omitempty"`
	Archives []MemoryArchiveRef `json:"archives,omitempty"`
}

type MemoryState struct {
	Version        string          `json:"version,omitempty"`
	ActivePatterns []MemoryPattern `json:"activePatterns,omitempty"`
}

type memoryStore interface {
	Load() (MemoryState, error)
	LoadIndex() (MemoryIndex, error)
	SavePattern(input MemorySaveInput) (MemorySaveResult, error)
	BuildThreadSnapshot() (ThreadMemorySnapshot, error)
}

type fileMemoryStore struct {
	root        string
	tokenBudget int
	mu          sync.Mutex
}

var (
	memoryCaseIDPattern       = regexp.MustCompile(`\b(?:ds|evt|thread|chat|approval|appr|conversation|stream|mem|mempat|ckpt)_[A-Za-z0-9_-]+\b`)
	memoryInlineSQLPattern    = regexp.MustCompile(`(?is)\bselect\b[^.;。；!?]{0,240}\bfrom\b[^.;。；!?]{0,240}|\binsert\b[^.;。；!?]{0,200}\binto\b[^.;。；!?]{0,200}|\bupdate\b[^.;。；!?]{0,200}\bset\b[^.;。；!?]{0,200}|\bdelete\b[^.;。；!?]{0,200}\bfrom\b[^.;。；!?]{0,200}`)
	memoryJSONPayloadPattern  = regexp.MustCompile(`\{[^{}\n]{0,240}:[^{}\n]{0,240}\}`)
	memoryKeyValuePattern     = regexp.MustCompile(`\b(?:datasourceId|threadId|eventId|approvalId|conversationId|streamId|database|entity|table|payload|error|code|exception)\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	memoryNamedObjectPattern  = regexp.MustCompile(`(?i)\b(?:datasource|database|schema|entity|table|collection|index|field|column)\s+[A-Za-z][A-Za-z0-9_]{0,63}\b`)
	memoryPredicatePattern    = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]{1,48}\s*=\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	memoryExceptionPattern    = regexp.MustCompile(`\b[A-Za-z]+Exception\b`)
	memoryDoubleQuotedPattern = regexp.MustCompile(`"[^"\n]{1,120}"`)
	memorySingleQuotedPattern = regexp.MustCompile(`'[^'\n]{1,120}'`)
	memoryBacktickPattern     = regexp.MustCompile("`[^`\\n]{1,120}`")
)

func newFileMemoryStore(root string, tokenBudget int) *fileMemoryStore {
	if tokenBudget < 1 {
		tokenBudget = 8_000
	}
	return &fileMemoryStore{root: strings.TrimSpace(root), tokenBudget: tokenBudget}
}

func (s *fileMemoryStore) Load() (MemoryState, error) {
	if s == nil {
		return MemoryState{}, errors.New("memory store is nil")
	}
	patterns, err := s.loadPatterns()
	if err != nil {
		return MemoryState{}, err
	}
	version := memoryVersion(patterns)
	return MemoryState{Version: version, ActivePatterns: patterns}, nil
}

func (s *fileMemoryStore) LoadIndex() (MemoryIndex, error) {
	if s == nil {
		return MemoryIndex{}, errors.New("memory store is nil")
	}
	data, err := securefile.ReadFile(filepath.Join(s.root, "memory-index.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return MemoryIndex{}, nil
		}
		return MemoryIndex{}, err
	}
	var index MemoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return MemoryIndex{}, err
	}
	return index, nil
}

func (s *fileMemoryStore) SavePattern(input MemorySaveInput) (MemorySaveResult, error) {
	if s == nil {
		return MemorySaveResult{}, errors.New("memory store is nil")
	}
	pattern, ok := normalizeMemorySaveInput(input)
	if !ok {
		return MemorySaveResult{}, errors.New("memory_save input is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	patterns, err := s.loadPatterns()
	if err != nil {
		return MemorySaveResult{}, err
	}
	now := time.Now().UTC()
	pattern.LastUsedAt = now
	pattern.UseCount = 1

	merged, saved := mergeMemoryPattern(patterns, pattern, input.ReplaceHints)
	compacted, archived := s.compactKnownPatterns(merged, composeMemoryMarkdown(merged))
	if err := s.persist(compacted, archived); err != nil {
		return MemorySaveResult{}, err
	}
	result := MemorySaveResult{
		Version:      memoryVersion(compacted),
		SavedPattern: &saved,
	}
	if len(archived) > 0 {
		result.ArchivedIDs = make([]string, 0, len(archived))
		for _, item := range archived {
			result.ArchivedIDs = append(result.ArchivedIDs, item.ID)
		}
	}
	return result, nil
}

func (s *fileMemoryStore) BuildThreadSnapshot() (ThreadMemorySnapshot, error) {
	if s == nil {
		return ThreadMemorySnapshot{}, errors.New("memory store is nil")
	}
	state, err := s.Load()
	if err != nil {
		return ThreadMemorySnapshot{}, err
	}
	if len(state.ActivePatterns) == 0 {
		return ThreadMemorySnapshot{}, nil
	}
	rendered := ""
	if data, readErr := securefile.ReadFile(filepath.Join(s.root, "MEMORY.md")); readErr == nil {
		rendered = strings.TrimSpace(string(data))
	}
	if rendered == "" {
		rendered = strings.TrimSpace(composeMemoryMarkdown(state.ActivePatterns))
	}
	return ThreadMemorySnapshot{
		Version:   state.Version,
		Rendered:  rendered,
		Tokens:    approximateTokenCount(rendered),
		CreatedAt: time.Now().UTC().Unix(),
	}, nil
}

func (s *fileMemoryStore) persist(patterns []MemoryPattern, archived []MemoryPattern) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}

	rendered := composeMemoryMarkdown(patterns)
	if err := securefile.WriteFile(filepath.Join(s.root, "MEMORY.md"), []byte(rendered), 0o644); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(patterns, "", "  ")
	if err != nil {
		return err
	}
	if err := securefile.WriteFile(filepath.Join(s.root, "memory-active.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}

	index, err := s.LoadIndex()
	if err != nil {
		return err
	}
	index.Version = memoryVersion(patterns)
	if len(archived) > 0 {
		archiveDir := filepath.Join(s.root, "memory")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return err
		}
		fileName := fmt.Sprintf("archived-%d.md", time.Now().UTC().UnixNano())
		if err := securefile.WriteFile(filepath.Join(archiveDir, fileName), []byte(composeMemoryMarkdown(archived)), 0o644); err != nil {
			return err
		}
		existing := make(map[string]struct{}, len(index.Archives))
		for _, item := range index.Archives {
			if trimmed := strings.TrimSpace(item.ID); trimmed != "" {
				existing[trimmed] = struct{}{}
			}
		}
		for _, item := range archived {
			if _, ok := existing[item.ID]; ok {
				continue
			}
			index.Archives = append(index.Archives, MemoryArchiveRef{
				ID:       item.ID,
				File:     fileName,
				Problem:  item.Problem,
				Summary:  summarizePattern(item),
				LastUsed: item.LastUsedAt.Unix(),
				UseCount: item.UseCount,
			})
		}
	}
	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(filepath.Join(s.root, "memory-index.json"), append(indexData, '\n'), 0o644)
}

func (s *fileMemoryStore) loadPatternsJSON() ([]MemoryPattern, error) {
	if s == nil {
		return nil, errors.New("memory store is nil")
	}
	data, err := securefile.ReadFile(filepath.Join(s.root, "memory-active.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var patterns []MemoryPattern
	if err := json.Unmarshal(data, &patterns); err != nil {
		return nil, err
	}
	return patterns, nil
}

func (s *fileMemoryStore) loadPatterns() ([]MemoryPattern, error) {
	if s == nil {
		return nil, errors.New("memory store is nil")
	}
	patterns, err := s.loadPatternsMarkdown()
	if err == nil {
		return patterns, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s.loadPatternsJSON()
}

func (s *fileMemoryStore) loadPatternsMarkdown() ([]MemoryPattern, error) {
	data, err := securefile.ReadFile(filepath.Join(s.root, "MEMORY.md"))
	if err != nil {
		return nil, err
	}
	return parseMemoryMarkdown(string(data)), nil
}

func (s *fileMemoryStore) compactKnownPatterns(patterns []MemoryPattern, rendered string) ([]MemoryPattern, []MemoryPattern) {
	if approximateTokenCount(rendered) <= s.tokenBudget || len(patterns) <= 1 {
		out := make([]MemoryPattern, len(patterns))
		copy(out, patterns)
		return out, nil
	}
	active := append([]MemoryPattern(nil), patterns...)
	archived := make([]MemoryPattern, 0, len(patterns))
	for approximateTokenCount(composeMemoryMarkdown(active)) > s.tokenBudget && len(active) > 1 {
		scored := append([]MemoryPattern(nil), active...)
		sort.Slice(scored, func(i, j int) bool {
			if scored[i].UseCount == scored[j].UseCount {
				return scored[i].LastUsedAt.Before(scored[j].LastUsedAt)
			}
			return scored[i].UseCount < scored[j].UseCount
		})
		archiveID := strings.TrimSpace(scored[0].ID)
		if archiveID == "" {
			break
		}
		for i := range active {
			if strings.TrimSpace(active[i].ID) != archiveID {
				continue
			}
			item := active[i]
			item.Archived = true
			archived = append(archived, item)
			active = append(active[:i], active[i+1:]...)
			break
		}
	}
	return active, archived
}

func normalizeMemorySaveInput(input MemorySaveInput) (MemoryPattern, bool) {
	problem := normalizePatternText(input.Problem)
	if problem == "" {
		return MemoryPattern{}, false
	}
	pattern := MemoryPattern{
		ID:         newMemoryPatternID(),
		Problem:    problem,
		Signals:    normalizePatternList(input.Signals),
		Avoid:      normalizePatternList(input.Avoid),
		Do:         normalizePatternList(input.Do),
		Why:        normalizePatternText(input.Why),
		Confidence: normalizeConfidence(input.Confidence),
	}
	return pattern, true
}

func mergeMemoryPattern(existing []MemoryPattern, next MemoryPattern, replaceHints []string) ([]MemoryPattern, MemoryPattern) {
	out := append([]MemoryPattern(nil), existing...)
	keys := []string{normalizePatternKey(next.Problem)}
	for _, hint := range replaceHints {
		if normalized := normalizePatternKey(hint); normalized != "" {
			keys = append(keys, normalized)
		}
	}
	for i := range out {
		if !containsPatternKey(keys, normalizePatternKey(out[i].Problem)) {
			continue
		}
		next.ID = out[i].ID
		next.UseCount = out[i].UseCount + 1
		if next.UseCount < 1 {
			next.UseCount = 1
		}
		if next.Confidence < out[i].Confidence {
			next.Confidence = out[i].Confidence
		}
		if len(next.Signals) == 0 {
			next.Signals = out[i].Signals
		}
		if len(next.Avoid) == 0 {
			next.Avoid = out[i].Avoid
		}
		if len(next.Do) == 0 {
			next.Do = out[i].Do
		}
		if next.Why == "" {
			next.Why = out[i].Why
		}
		out[i] = next
		return out, next
	}
	out = append(out, next)
	return out, next
}

func containsPatternKey(keys []string, target string) bool {
	for _, key := range keys {
		if key == target && key != "" {
			return true
		}
	}
	return false
}

func composeMemoryMarkdown(patterns []MemoryPattern) string {
	var b strings.Builder
	b.WriteString("# MEMORY\n\n")
	b.WriteString("## Core Principles\n")
	b.WriteString("- Keep long-term memory as reusable patterns, not raw event logs.\n")
	b.WriteString("- Prefer the minimal sufficient evidence source before using higher-cost tools.\n\n")
	b.WriteString("## Active Patterns\n")
	if len(patterns) == 0 {
		b.WriteString("- No active patterns yet.\n")
	} else {
		for _, pattern := range patterns {
			b.WriteString(renderMemoryPattern(pattern))
		}
	}
	b.WriteString("\n## Recent Adjustments\n")
	b.WriteString("- Automatic memory evolution is active.\n\n")
	b.WriteString("## Archive Hints\n")
	b.WriteString("- Cold patterns may be moved to memory/*.md when the active budget is exceeded.\n")
	return strings.TrimSpace(b.String()) + "\n"
}

func parseMemoryMarkdown(raw string) []MemoryPattern {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	patterns := make([]MemoryPattern, 0, 8)
	var current *MemoryPattern
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "### Pattern: "):
			if current != nil && strings.TrimSpace(current.Problem) != "" {
				finalized := finalizeParsedPattern(*current)
				if strings.TrimSpace(finalized.Problem) != "" {
					patterns = append(patterns, finalized)
				}
			}
			current = &MemoryPattern{Problem: strings.TrimSpace(strings.TrimPrefix(trimmed, "### Pattern: "))}
		case current == nil:
			continue
		case strings.HasPrefix(trimmed, "- Signals: "):
			current.Signals = splitPatternList(strings.TrimSpace(strings.TrimPrefix(trimmed, "- Signals: ")))
		case strings.HasPrefix(trimmed, "- Avoid: "):
			current.Avoid = splitPatternList(strings.TrimSpace(strings.TrimPrefix(trimmed, "- Avoid: ")))
		case strings.HasPrefix(trimmed, "- Do: "):
			current.Do = splitPatternList(strings.TrimSpace(strings.TrimPrefix(trimmed, "- Do: ")))
		case strings.HasPrefix(trimmed, "- Why: "):
			current.Why = strings.TrimSpace(strings.TrimPrefix(trimmed, "- Why: "))
		case strings.HasPrefix(trimmed, "- Use: "):
			parsePatternUseLine(strings.TrimSpace(strings.TrimPrefix(trimmed, "- Use: ")), current)
		}
	}
	if current != nil && strings.TrimSpace(current.Problem) != "" {
		finalized := finalizeParsedPattern(*current)
		if strings.TrimSpace(finalized.Problem) != "" {
			patterns = append(patterns, finalized)
		}
	}
	return patterns
}

func finalizeParsedPattern(pattern MemoryPattern) MemoryPattern {
	pattern.Problem = normalizePatternText(pattern.Problem)
	pattern.Signals = normalizePatternList(pattern.Signals)
	pattern.Avoid = normalizePatternList(pattern.Avoid)
	pattern.Do = normalizePatternList(pattern.Do)
	pattern.Why = normalizePatternText(pattern.Why)
	if strings.TrimSpace(pattern.ID) == "" {
		pattern.ID = "parsed_" + strings.ReplaceAll(normalizePatternKey(pattern.Problem), " ", "_")
	}
	if pattern.UseCount < 1 {
		pattern.UseCount = 1
	}
	if pattern.Confidence <= 0 {
		pattern.Confidence = 0.7
	}
	return pattern
}

func splitPatternList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" {
		return nil
	}
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parsePatternUseLine(raw string, pattern *MemoryPattern) {
	if pattern == nil {
		return
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		segment := strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(segment, "confidence="):
			var value float64
			if _, err := fmt.Sscanf(strings.TrimPrefix(segment, "confidence="), "%f", &value); err == nil {
				pattern.Confidence = value
			}
		case strings.HasPrefix(segment, "use_count="):
			var value int
			if _, err := fmt.Sscanf(strings.TrimPrefix(segment, "use_count="), "%d", &value); err == nil {
				pattern.UseCount = value
			}
		case strings.HasPrefix(segment, "last_used="):
			if parsed, err := time.Parse(time.RFC3339, strings.TrimPrefix(segment, "last_used=")); err == nil {
				pattern.LastUsedAt = parsed.UTC()
			}
		}
	}
}

func renderMemoryPattern(pattern MemoryPattern) string {
	return fmt.Sprintf("### Pattern: %s\n- Signals: %s\n- Avoid: %s\n- Do: %s\n- Why: %s\n- Use: confidence=%.2f, use_count=%d, last_used=%s\n\n",
		pattern.Problem,
		joinPatternList(pattern.Signals),
		joinPatternList(pattern.Avoid),
		joinPatternList(pattern.Do),
		firstNonEmpty(pattern.Why, "Reusable troubleshooting guidance."),
		pattern.Confidence,
		maxInt(pattern.UseCount, 1),
		pattern.LastUsedAt.UTC().Format(time.RFC3339),
	)
}

func joinPatternList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, " | ")
}

func summarizePattern(pattern MemoryPattern) string {
	return firstNonEmpty(pattern.Problem, pattern.Why)
}

func normalizePatternList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := normalizePatternText(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizePatternText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = sanitizeCaseSpecificFragments(text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(stripCaseSpecificTokens(strings.Join(fields, " ")))
}

func sanitizeCaseSpecificFragments(text string) string {
	if text == "" {
		return ""
	}
	text = memoryInlineSQLPattern.ReplaceAllString(text, " query shape ")
	text = memoryJSONPayloadPattern.ReplaceAllString(text, " payload detail ")
	text = memoryKeyValuePattern.ReplaceAllString(text, " detail ")
	text = memoryNamedObjectPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := strings.Fields(match)
		if len(parts) == 0 {
			return " detail "
		}
		return strings.ToLower(parts[0]) + " detail"
	})
	text = memoryPredicatePattern.ReplaceAllString(text, " predicate detail ")
	text = memoryCaseIDPattern.ReplaceAllString(text, " id ")
	text = memoryExceptionPattern.ReplaceAllString(text, " error detail ")
	text = replaceQuotedCaseSpecificFragments(text, memoryDoubleQuotedPattern)
	text = replaceQuotedCaseSpecificFragments(text, memorySingleQuotedPattern)
	text = replaceQuotedCaseSpecificFragments(text, memoryBacktickPattern)
	return text
}

func replaceQuotedCaseSpecificFragments(text string, pattern *regexp.Regexp) string {
	if pattern == nil || text == "" {
		return text
	}
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := strings.TrimSpace(match[1 : len(match)-1])
		if !looksCaseSpecificQuotedText(inner) {
			return match
		}
		return " detail "
	})
}

func looksCaseSpecificQuotedText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	switch {
	case memoryCaseIDPattern.MatchString(text):
		return true
	case strings.ContainsAny(text, "{}[]:=<>"):
		return true
	case strings.Contains(text, "_"):
		return true
	case strings.ContainsAny(text, "0123456789"):
		return true
	case strings.Contains(lower, "select ") || strings.Contains(lower, " from ") || strings.Contains(lower, " where "):
		return true
	case strings.Contains(lower, "exception"):
		return true
	case len(text) > 24 && strings.Contains(text, " "):
		return true
	default:
		return false
	}
}

func stripCaseSpecificTokens(text string) string {
	if text == "" {
		return ""
	}
	words := strings.Fields(text)
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		lower := strings.ToLower(strings.Trim(word, "\"'`,.;:()[]{}"))
		switch {
		case strings.HasPrefix(lower, "evt_"):
			continue
		case strings.HasPrefix(lower, "thread_"):
			continue
		case strings.HasPrefix(lower, "ds_"):
			continue
		case strings.HasPrefix(lower, "chat_"):
			continue
		case strings.HasPrefix(lower, "appr_"):
			continue
		case strings.HasPrefix(lower, "approval_"):
			continue
		case lower == "select" || lower == "insert" || lower == "update" || lower == "delete":
			continue
		case lower == "<id>" || lower == "<query>" || lower == "<payload>":
			continue
		}
		filtered = append(filtered, word)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, " ")
}

func normalizePatternKey(text string) string {
	text = strings.ToLower(normalizePatternText(text))
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if r == ' ' {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func normalizeConfidence(value float64) float64 {
	if value <= 0 {
		return 0.7
	}
	if value > 1 {
		return 1
	}
	return value
}

func approximateTokenCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	words := len(strings.Fields(text))
	runes := len([]rune(text))
	charBased := runes / 4
	if charBased < 1 {
		charBased = 1
	}
	if words > charBased {
		return words
	}
	return charBased
}

func memoryVersion(patterns []MemoryPattern) string {
	return fmt.Sprintf("mem_%d_%d", len(patterns), approximateTokenCount(composeMemoryMarkdown(patterns)))
}

func newMemoryPatternID() string {
	return fmt.Sprintf("mempat_%d", time.Now().UTC().UnixNano())
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
