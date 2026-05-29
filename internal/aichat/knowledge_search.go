package aichat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type KnowledgeSearchHit struct {
	Source    string `json:"source"`
	LineStart int    `json:"lineStart"`
	LineEnd   int    `json:"lineEnd"`
	Snippet   string `json:"snippet"`
	Score     int    `json:"score"`
}

type KnowledgeSearchResult struct {
	Query          string               `json:"query"`
	Scope          string               `json:"scope"`
	DatasourceID   string               `json:"datasourceId,omitempty"`
	DatasourceType string               `json:"datasourceType,omitempty"`
	Roots          []string             `json:"roots"`
	FilesScanned   int                  `json:"filesScanned"`
	FilesTruncated int                  `json:"filesTruncated"`
	Hits           []KnowledgeSearchHit `json:"hits"`
	Notes          []string             `json:"notes,omitempty"`
}

func (s *Service) searchKnowledge(ctx context.Context, req TurnRequest, args map[string]any) (KnowledgeSearchResult, error) {
	scope := strings.ToLower(strings.TrimSpace(stringArg(args, "scope")))

	datasourceID := strings.TrimSpace(stringArg(args, "datasourceId"))
	if datasourceID == "" {
		datasourceID = strings.TrimSpace(defaultDatasourceIDForTool(req))
	}

	datasourceType := strings.TrimSpace(stringArg(args, "datasourceType"))
	if datasourceType == "" {
		if shouldPreferPageContextForExecuteDefaults(req) {
			datasourceType = strings.TrimSpace(req.PageContext.CurrentDatasourceType)
		}
		if datasourceType == "" {
			if working := establishedWorkingContext(req); working != nil {
				datasourceType = strings.TrimSpace(working.DatasourceType)
			}
		}
	}
	if datasourceType == "" && !turnIntentAvoidsCurrentFocus(req) {
		datasourceType = strings.TrimSpace(req.PageContext.CurrentDatasourceType)
	}
	if datasourceType == "" && datasourceID != "" {
		if ds, err := s.getDatasourceCached(ctx, datasourceID); err == nil {
			datasourceType = strings.TrimSpace(ds.Type)
		}
	}
	datasourceType = normalizeDatasourceType(datasourceType)
	query, queryGenerated := resolveKnowledgeSearchQuery(req, args, datasourceType)
	if datasourceType == "" {
		if inferred := inferDatasourceTypesFromText(query); len(inferred) == 1 {
			datasourceType = inferred[0]
		}
	}
	if scope == "" {
		scope = defaultKnowledgeSearchScope(datasourceID, datasourceType)
	}
	if query == "" {
		return KnowledgeSearchResult{}, errors.New("query is required")
	}

	builtinRoot := strings.TrimSpace(s.knowledgeDir)
	if builtinRoot == "" {
		builtinRoot = "data/ai-chat-knowledge"
	}

	builtinRoots, notes := resolveKnowledgeRoots(builtinRoot, scope, datasourceID, datasourceType)
	if queryGenerated {
		notes = append(notes, "query generated from datasource-aware template")
	}
	if len(builtinRoots) == 0 {
		builtinRoots = []string{builtinRoot}
		notes = append(notes, "no scoped roots resolved; fell back to root search")
	}

	userRoot := strings.TrimSpace(s.userKnowledgeDir)
	userRoots, userNotes := resolveUserKnowledgeRoots(userRoot, scope, datasourceID)
	notes = append(notes, userNotes...)

	roots := make([]string, 0, len(builtinRoots)+len(userRoots))
	roots = append(roots, builtinRoots...)
	roots = append(roots, userRoots...)

	maxFiles := clampInt(intArg(args, "maxFiles", 40), 1, 200)
	maxFileBytes := clampInt(intArg(args, "maxFileBytes", 200_000), 10_000, 1_000_000)
	maxHits := clampInt(intArg(args, "maxHits", 6), 1, 25)
	contextLines := clampInt(intArg(args, "contextLines", 2), 0, 8)

	files, skipped := collectKnowledgeFiles(roots, maxFiles)
	if skipped > 0 {
		notes = append(notes, fmt.Sprintf("file list truncated to %d files", maxFiles))
	}
	notes = append(notes, "scope escalation order: current -> type -> all")

	terms := extractKnowledgeSearchTerms(query)
	if len(terms) == 0 {
		return KnowledgeSearchResult{}, errors.New("query terms are empty after normalization")
	}

	var hits []KnowledgeSearchHit
	filesTruncated := 0
	for _, path := range files {
		lines, truncated, err := readTextLinesLimited(path, maxFileBytes)
		if err != nil {
			continue
		}
		if truncated {
			filesTruncated++
		}
		added := 0
		for i := 0; i < len(lines); i++ {
			score := scoreKnowledgeLine(lines[i], terms)
			if score == 0 {
				continue
			}
			snippet, lineStart, lineEnd := snippetWithContext(lines, i, contextLines)
			hits = append(hits, KnowledgeSearchHit{
				Source:    formatKnowledgeSourceMulti(builtinRoot, userRoot, path),
				LineStart: lineStart,
				LineEnd:   lineEnd,
				Snippet:   snippet,
				Score:     score,
			})
			added++
			if added >= 3 {
				break
			}
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Source != hits[j].Source {
			return hits[i].Source < hits[j].Source
		}
		return hits[i].LineStart < hits[j].LineStart
	})

	if len(hits) > maxHits {
		hits = hits[:maxHits]
		notes = append(notes, fmt.Sprintf("hits truncated to top %d", maxHits))
	}

	return KnowledgeSearchResult{
		Query:          query,
		Scope:          scope,
		DatasourceID:   datasourceID,
		DatasourceType: datasourceType,
		Roots:          roots,
		FilesScanned:   len(files),
		FilesTruncated: filesTruncated,
		Hits:           hits,
		Notes:          notes,
	}, nil
}

func defaultKnowledgeSearchScope(datasourceID string, datasourceType string) string {
	switch {
	case strings.TrimSpace(datasourceID) != "":
		return "current"
	case strings.TrimSpace(datasourceType) != "":
		return "type"
	default:
		return "all"
	}
}

func resolveKnowledgeSearchQuery(req TurnRequest, args map[string]any, datasourceType string) (string, bool) {
	query := strings.TrimSpace(stringArg(args, "query", "q"))
	if query != "" {
		return query, false
	}
	topic := strings.TrimSpace(stringArg(args, "topic", "intent", "hint"))
	contextHint := strings.TrimSpace(lastUserText(req.Messages))
	if topic == "" {
		topic = contextHint
		contextHint = ""
	}
	if topic == "" {
		topic = strings.TrimSpace(req.ImplicitStatement)
	}
	if contextHint != "" && !strings.Contains(strings.ToLower(topic), strings.ToLower(contextHint)) {
		topic = strings.TrimSpace(topic + " " + contextHint)
	}
	return buildKnowledgeSearchTemplate(datasourceType, topic), true
}

func buildKnowledgeSearchTemplate(datasourceType string, topic string) string {
	topic = strings.TrimSpace(topic)
	switch normalizeDatasourceType(datasourceType) {
	case "dynamodb":
		return strings.TrimSpace("dynamodb partiql partition key sort key gsi lsi predicate/index rule quoting " + topic)
	case "mongodb":
		return strings.TrimSpace("mongodb query syntax index filter projection aggregation field naming " + topic)
	case "mysql":
		return strings.TrimSpace("mysql sql quoting index predicate rule field naming " + topic)
	case "postgresql":
		return strings.TrimSpace("postgresql sql quoting index predicate rule field naming " + topic)
	case "redis", "redis_cluster":
		return strings.TrimSpace("redis command syntax key pattern scan safety " + topic)
	case "elasticsearch":
		return strings.TrimSpace("elasticsearch query dsl mapping filter request shape " + topic)
	default:
		return topic
	}
}

func resolveKnowledgeRoots(root string, scope string, datasourceID string, datasourceType string) ([]string, []string) {
	notes := []string{}
	out := make([]string, 0, 2)

	addRoot := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		out = append(out, path)
	}

	addDatasourcePack := func(id string) {
		if strings.TrimSpace(id) == "" {
			return
		}
		addRoot(filepath.Join(root, "datasources", id))
		addRoot(filepath.Join(root, "datasources", id+".md"))
	}

	addTypePack := func(t string) {
		if strings.TrimSpace(t) == "" {
			return
		}
		addRoot(filepath.Join(root, "types", t))
		addRoot(filepath.Join(root, "types", t+".md"))
	}

	switch scope {
	case "datasource":
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		} else {
			notes = append(notes, "scope=datasource but datasourceId is empty")
		}
	case "type":
		if datasourceType != "" {
			addTypePack(datasourceType)
			if datasourceType == "redis_cluster" {
				addTypePack("redis")
			}
		} else {
			notes = append(notes, "scope=type but datasourceType is empty")
		}
	case "all":
		addRoot(root)
	case "current", "":
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		}
		if datasourceType != "" {
			addTypePack(datasourceType)
			if datasourceType == "redis_cluster" {
				addTypePack("redis")
			}
		}
	default:
		notes = append(notes, fmt.Sprintf("unknown scope %q; defaulted to current", scope))
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		}
		if datasourceType != "" {
			addTypePack(datasourceType)
			if datasourceType == "redis_cluster" {
				addTypePack("redis")
			}
		}
	}

	dedup := make([]string, 0, len(out))
	seen := map[string]struct{}{}
	for _, path := range out {
		key := filepath.Clean(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, key)
	}

	// Guard against path traversal: datasourceId/datasourceType are identifiers, not paths.
	// Tool args are model-controlled and user-influenced, so we must ensure resolved roots
	// stay under the knowledge directory root.
	safe := make([]string, 0, len(dedup))
	for _, path := range dedup {
		if !pathWithinRoot(root, path) {
			notes = append(notes, "ignored unsafe root outside knowledge directory")
			continue
		}
		safe = append(safe, path)
	}

	return safe, notes
}

func resolveUserKnowledgeRoots(root string, scope string, datasourceID string) ([]string, []string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}

	notes := []string{}
	out := make([]string, 0, 2)

	addRoot := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		out = append(out, path)
	}

	addAllPack := func() {
		addRoot(filepath.Join(root, "all"))
	}

	addDatasourcePack := func(id string) {
		if strings.TrimSpace(id) == "" {
			return
		}
		addRoot(filepath.Join(root, "datasources", id))
	}

	switch scope {
	case "datasource":
		addAllPack()
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		} else {
			notes = append(notes, "scope=datasource but datasourceId is empty")
		}
	case "type":
		addAllPack()
	case "all":
		addRoot(root)
	case "current", "":
		addAllPack()
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		}
	default:
		notes = append(notes, fmt.Sprintf("unknown scope %q; defaulted to current", scope))
		addAllPack()
		if datasourceID != "" {
			addDatasourcePack(datasourceID)
		}
	}

	dedup := make([]string, 0, len(out))
	seen := map[string]struct{}{}
	for _, path := range out {
		key := filepath.Clean(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, key)
	}

	safe := make([]string, 0, len(dedup))
	for _, path := range dedup {
		if !pathWithinRoot(root, path) {
			notes = append(notes, "ignored unsafe root outside user knowledge directory")
			continue
		}
		safe = append(safe, path)
	}
	return safe, notes
}

func collectKnowledgeFiles(roots []string, maxFiles int) ([]string, int) {
	seen := map[string]struct{}{}
	files := make([]string, 0, min(maxFiles, 64))
	truncated := 0

	addFile := func(path string) bool {
		if len(files) >= maxFiles {
			truncated++
			return false
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return true
		}
		seen[clean] = struct{}{}
		files = append(files, clean)
		return true
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil || info == nil {
			continue
		}
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				ext := strings.ToLower(filepath.Ext(root))
				if ext == ".md" || ext == ".txt" {
					_ = addFile(root)
				}
			}
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if strings.HasPrefix(name, ".") || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".md" && ext != ".txt" {
				return nil
			}
			if !addFile(path) {
				return fs.SkipAll
			}
			return nil
		})
		if len(files) >= maxFiles {
			break
		}
	}

	sort.Strings(files)
	return files, truncated
}

func extractKnowledgeSearchTerms(query string) []string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil
	}

	lower := strings.ToLower(trimmed)
	parts := []string{}

	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parts = append(parts, value)
	}

	add(lower)

	fields := strings.FieldsFunc(lower, func(r rune) bool {
		if unicode.IsSpace(r) {
			return true
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return true
		}
		return false
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if containsCJK(f) {
			add(f)
			continue
		}
		if len(f) < 2 {
			continue
		}
		add(f)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, term := range parts {
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

func scoreKnowledgeLine(line string, terms []string) int {
	lower := strings.ToLower(line)
	score := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(lower, term) {
			score++
		}
	}
	return score
}

func snippetWithContext(lines []string, idx int, contextLines int) (string, int, int) {
	start := idx - contextLines
	if start < 0 {
		start = 0
	}
	end := idx + contextLines
	if end >= len(lines) {
		end = len(lines) - 1
	}

	maxLineChars := 240
	var b strings.Builder
	for i := start; i <= end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		line := strings.TrimRight(lines[i], "\r")
		if len(line) > maxLineChars {
			line = line[:maxLineChars] + "…"
		}
		b.WriteString(fmt.Sprintf("%d| %s", i+1, line))
	}
	return b.String(), start + 1, end + 1
}

func readTextLinesLimited(path string, maxBytes int) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()

	limit := int64(maxBytes) + 1
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, false, err
	}

	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	text := string(data)
	lines := strings.Split(text, "\n")
	return lines, truncated, nil
}

func formatKnowledgeSource(root string, path string) string {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root != "" && path != "" {
		if rel, err := filepath.Rel(root, path); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func formatKnowledgeSourceMulti(builtinRoot string, userRoot string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	userRoot = strings.TrimSpace(userRoot)
	if userRoot != "" && pathWithinRoot(userRoot, path) {
		return "user/" + formatKnowledgeSource(userRoot, path)
	}
	builtinRoot = strings.TrimSpace(builtinRoot)
	if builtinRoot != "" && pathWithinRoot(builtinRoot, path) {
		return formatKnowledgeSource(builtinRoot, path)
	}
	return filepath.ToSlash(path)
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func pathWithinRoot(root string, path string) bool {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return false
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	rootEval := rootAbs
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil && strings.TrimSpace(resolved) != "" {
		rootEval = resolved
	}
	pathEval := pathAbs
	if resolved, err := filepath.EvalSymlinks(pathAbs); err == nil && strings.TrimSpace(resolved) != "" {
		pathEval = resolved
	}

	rel, err := filepath.Rel(rootEval, pathEval)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	sep := string(filepath.Separator)
	return !strings.HasPrefix(rel, ".."+sep)
}
