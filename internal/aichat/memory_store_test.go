package aichat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryStore_SaveAndLoadActiveMemory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 8_000)

	_, err := store.SavePattern(MemorySaveInput{
		Problem:    "duplicate execute approvals in the same diagnostic loop",
		Signals:    []string{"same datasource", "same statement", "no new evidence"},
		Avoid:      []string{"re-run execute_statement repeatedly"},
		Do:         []string{"reuse the existing result and answer from accumulated evidence"},
		Why:        "Repeated execution adds cost but does not reduce ambiguity.",
		Confidence: 0.92,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(state.ActivePatterns) != 1 {
		t.Fatalf("expected 1 active pattern, got %d", len(state.ActivePatterns))
	}
	if !strings.Contains(state.ActivePatterns[0].Problem, "duplicate execute approvals") {
		t.Fatalf("unexpected active pattern: %+v", state.ActivePatterns[0])
	}

	data, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("expected MEMORY.md to exist, got %v", err)
	}
	if !strings.Contains(string(data), "Pattern: duplicate execute approvals in the same diagnostic loop") {
		t.Fatalf("expected rendered memory pattern, got: %s", string(data))
	}
}

func TestMemoryStore_CompactsArchivedPatternsWhenBudgetExceeded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 220)

	for i := 0; i < 3; i++ {
		_, err := store.SavePattern(MemorySaveInput{
			Problem:    "pattern " + strings.Repeat("x", 40) + string(rune('a'+i)),
			Signals:    []string{strings.Repeat("signal ", 12)},
			Avoid:      []string{strings.Repeat("avoid ", 12)},
			Do:         []string{strings.Repeat("do ", 12)},
			Why:        strings.Repeat("why ", 18),
			Confidence: 0.8,
		})
		if err != nil {
			t.Fatalf("save pattern %d: %v", i, err)
		}
	}

	index, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(index.Archives) == 0 {
		t.Fatalf("expected archived entries after compaction, got %+v", index)
	}

	memoryDir := filepath.Join(root, "memory")
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		t.Fatalf("expected archive dir, got %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected archive files to be created")
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if got := approximateTokenCount(composeMemoryMarkdown(state.ActivePatterns)); got > 220 {
		t.Fatalf("expected active memory to stay within token budget, got %d", got)
	}
}

func TestMemoryStore_PreservesPreviousArchiveIndexEntries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 220)

	for i := 0; i < 4; i++ {
		_, err := store.SavePattern(MemorySaveInput{
			Problem:    "archive-preserve pattern " + strings.Repeat("y", 30) + string(rune('a'+i)),
			Signals:    []string{strings.Repeat("signal ", 10)},
			Avoid:      []string{strings.Repeat("avoid ", 10)},
			Do:         []string{strings.Repeat("do ", 10)},
			Why:        strings.Repeat("why ", 16),
			Confidence: 0.8,
		})
		if err != nil {
			t.Fatalf("save pattern %d: %v", i, err)
		}
	}

	index, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(index.Archives) < 2 {
		t.Fatalf("expected archive index to preserve accumulated refs, got %+v", index)
	}
}

func TestMemoryStore_SavePatternUsesMemoryMarkdownAsSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 8_000)

	if _, err := store.SavePattern(MemorySaveInput{
		Problem:    "old json-backed pattern",
		Signals:    []string{"s1"},
		Avoid:      []string{"a1"},
		Do:         []string{"d1"},
		Why:        "w1",
		Confidence: 0.8,
	}); err != nil {
		t.Fatalf("seed pattern: %v", err)
	}

	custom := strings.TrimSpace(`# MEMORY

## Core Principles
- Keep long-term memory as reusable patterns, not raw event logs.
- Prefer the minimal sufficient evidence source before using higher-cost tools.

## Active Patterns
### Pattern: markdown-edited source pattern
- Signals: from markdown
- Avoid: stale json source
- Do: prefer MEMORY.md
- Why: markdown should be treated as the active source.
- Use: confidence=0.85, use_count=3, last_used=2026-03-10T00:00:00Z

## Recent Adjustments
- Automatic memory evolution is active.

## Archive Hints
- Cold patterns may be moved to memory/*.md when the active budget is exceeded.
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom markdown: %v", err)
	}

	if _, err := store.SavePattern(MemorySaveInput{
		Problem:    "new merged pattern",
		Signals:    []string{"new signal"},
		Avoid:      []string{"new avoid"},
		Do:         []string{"new do"},
		Why:        "new why",
		Confidence: 0.9,
	}); err != nil {
		t.Fatalf("save after markdown edit: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(state.ActivePatterns) != 2 {
		t.Fatalf("expected markdown source plus new pattern to be preserved, got %+v", state.ActivePatterns)
	}
	if state.ActivePatterns[0].Problem != "markdown-edited source pattern" {
		t.Fatalf("expected markdown pattern to remain authoritative, got %+v", state.ActivePatterns)
	}
	if state.ActivePatterns[0].UseCount != 3 || !state.ActivePatterns[0].LastUsedAt.Equal(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected markdown use metadata to be parsed, got %+v", state.ActivePatterns[0])
	}
}

func TestNormalizeMemorySaveInput_StripsCaseSpecificFragments(t *testing.T) {
	pattern, ok := normalizeMemorySaveInput(MemorySaveInput{
		Problem:    `avoid persisting ds_18986cade5c502f0 failures from SELECT * FROM "orders_2026" WHERE "uid" = 'u_123' after evt_1`,
		Signals:    []string{`thread_42 retried payload {"uid":"u_123","status":"PENDING"} with approval_99`},
		Avoid:      []string{`store datasourceId="ds_18986cade5c502f0" and raw query SELECT * FROM "orders_2026" WHERE "uid" = 'u_123'`},
		Do:         []string{`save only the reusable troubleshooting pattern instead of the JSON payload {"uid":"u_123"}`},
		Why:        `Case-specific ids, payloads, and SQL statements do not generalize.`,
		Confidence: 0.9,
	})
	if !ok {
		t.Fatalf("expected normalized pattern")
	}

	joined := strings.Join([]string{
		pattern.Problem,
		strings.Join(pattern.Signals, " "),
		strings.Join(pattern.Avoid, " "),
		strings.Join(pattern.Do, " "),
		pattern.Why,
	}, " ")

	for _, forbidden := range []string{
		"ds_18986cade5c502f0",
		"evt_1",
		"thread_42",
		"approval_99",
		"orders_2026",
		"u_123",
		`{"uid":"u_123"`,
		"SELECT * FROM",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("expected normalized pattern to remove %q, got %q", forbidden, joined)
		}
	}

	for _, expected := range []string{"query shape", "payload detail", "id"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected normalized pattern to retain generic %q marker, got %q", expected, joined)
		}
	}
}

func TestMemoryStore_StripsCaseSpecificFragmentsBeforePersisting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 8_000)

	_, err := store.SavePattern(MemorySaveInput{
		Problem:    `for datasource ds_18986cade5c502f0, SELECT * FROM orders WHERE aid = 'vvv' returned 0 rows`,
		Signals:    []string{`approvalId=appr_123 repeated in conversation chat_88`},
		Avoid:      []string{`copy {"datasourceId":"ds_1","threadId":"thread_7","eventId":"evt_9","error":"ValidationException: pk mismatch"}`},
		Do:         []string{`reuse the learned troubleshooting pattern instead of preserving SELECT * FROM orders WHERE aid = 'vvv'`},
		Why:        `raw payload {"datasourceId":"ds_1","error":"ValidationException: pk mismatch"} is case-specific`,
		Confidence: 0.93,
	})
	if err != nil {
		t.Fatalf("save pattern: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read memory markdown: %v", err)
	}
	raw := string(data)
	for _, forbidden := range []string{
		"ds_18986cade5c502f0",
		"SELECT * FROM orders",
		"aid = 'vvv'",
		"appr_123",
		"chat_88",
		"thread_7",
		"evt_9",
		"ValidationException",
		`"datasourceId"`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("expected MEMORY.md to omit case-specific fragment %q, got: %s", forbidden, raw)
		}
	}
	if !strings.Contains(raw, "query shape") {
		t.Fatalf("expected MEMORY.md to retain generalized query guidance, got: %s", raw)
	}
}

func TestMemoryStore_SavePatternCleansDirtyMemoryMarkdownOnReload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ai-chat")
	store := newFileMemoryStore(root, 8_000)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create memory root: %v", err)
	}

	dirty := strings.TrimSpace(`# MEMORY

## Core Principles
- Keep long-term memory as reusable patterns, not raw event logs.
- Prefer the minimal sufficient evidence source before using higher-cost tools.

## Active Patterns
### Pattern: for datasource ds_18986cade5c502f0, SELECT * FROM orders WHERE aid = 'vvv' returned 0 rows
- Signals: retry approvalId=appr_123 in conversation chat_88
- Avoid: {"datasourceId":"ds_1","threadId":"thread_7","eventId":"evt_9","error":"ValidationException: pk mismatch"}
- Do: prefer describe_entity before re-running SELECT * FROM orders WHERE aid = 'vvv'
- Why: the raw payload {"error":"ValidationException"} is too case-specific.
- Use: confidence=0.80, use_count=2, last_used=2026-03-10T00:00:00Z

## Recent Adjustments
- Automatic memory evolution is active.

## Archive Hints
- Cold patterns may be moved to memory/*.md when the active budget is exceeded.
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte(dirty), 0o644); err != nil {
		t.Fatalf("write dirty memory markdown: %v", err)
	}

	if _, err := store.SavePattern(MemorySaveInput{
		Problem:    "prefer abstract troubleshooting patterns over raw case logs",
		Signals:    []string{"old path failed"},
		Avoid:      []string{"copy raw payloads into memory"},
		Do:         []string{"persist only reusable troubleshooting patterns"},
		Why:        "clean memory stays reusable across threads",
		Confidence: 0.9,
	}); err != nil {
		t.Fatalf("save after dirty markdown: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read cleaned memory markdown: %v", err)
	}
	raw := string(data)
	for _, forbidden := range []string{
		"ds_18986cade5c502f0",
		"SELECT * FROM orders",
		"aid = 'vvv'",
		"appr_123",
		"chat_88",
		"thread_7",
		"evt_9",
		"ValidationException",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("expected dirty fragment %q to be scrubbed on round-trip, got: %s", forbidden, raw)
		}
	}
	if !strings.Contains(raw, "prefer abstract troubleshooting patterns over raw case logs") {
		t.Fatalf("expected new pattern to be persisted after cleanup, got: %s", raw)
	}
}
