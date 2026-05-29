package userkb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

type Model interface {
	Chat(ctx context.Context, systemPrompt string, messages []Message) (string, error)
}

type ModelResolver interface {
	Resolve(aiConfigID string) (Model, error)
}

type Message struct {
	Role    string
	Content string
}

type Manager struct {
	mu           sync.RWMutex
	root         string
	storePath    string
	models       ModelResolver
	state        StoreState
	parsedDir    string
	scopesAllDir string
	scopesDSDir  string
}

type ManagerConfig struct {
	Root          string
	ModelResolver ModelResolver
}

func NewManager(cfg ManagerConfig) (*Manager, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, errors.New("root is required")
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	storePath := filepath.Join(root, "store.json")
	parsedDir := filepath.Join(root, "parsed")
	scopesAllDir := filepath.Join(parsedDir, "scopes", "all")
	scopesDSDir := filepath.Join(parsedDir, "scopes", "datasources")

	m := &Manager{
		root:         root,
		storePath:    storePath,
		models:       cfg.ModelResolver,
		state:        StoreState{Version: 1},
		parsedDir:    parsedDir,
		scopesAllDir: scopesAllDir,
		scopesDSDir:  scopesDSDir,
	}

	if err := m.loadLocked(); err != nil {
		return nil, err
	}
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}
	_ = m.writeIndexesLocked()
	return m, nil
}

func (m *Manager) List(ctx context.Context) (ViewState, error) {
	_ = ctx
	m.mu.RLock()
	defer m.mu.RUnlock()

	ready, msg := m.checkProviderLocked(ctx, "")
	return ViewState{
		State:             cloneState(m.state),
		AIProviderReady:   ready,
		AIProviderMessage: msg,
	}, nil
}

func (m *Manager) CreateCategory(ctx context.Context, input CategoryCreateInput) (ViewState, error) {
	_ = ctx
	if m == nil {
		return ViewState{}, errors.New("manager is nil")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ViewState{}, errors.New("name is required")
	}
	scope := normalizeScope(input.Scope)
	if scope == "" {
		scope = ScopeAll
	}

	now := time.Now().UTC().UnixNano()
	category := Category{
		ID:            newID("kbcat"),
		Name:          name,
		Description:   strings.TrimSpace(input.Description),
		Scope:         scope,
		DatasourceIDs: normalizeDatasourceIDs(input.DatasourceIDs),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if category.Scope == ScopeDatasource && len(category.DatasourceIDs) == 0 {
		return ViewState{}, errors.New("datasourceIds is required for scope=datasource")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.Categories = append(m.state.Categories, category)
	m.sortLocked()

	if err := m.saveLocked(); err != nil {
		return ViewState{}, err
	}
	_ = m.writeIndexesLocked()
	ready, msg := m.checkProviderLocked(ctx, "")
	return ViewState{State: cloneState(m.state), AIProviderReady: ready, AIProviderMessage: msg}, nil
}

func (m *Manager) UpdateCategory(ctx context.Context, id string, input CategoryUpdateInput) (ViewState, error) {
	_ = ctx
	if m == nil {
		return ViewState{}, errors.New("manager is nil")
	}
	categoryID := strings.TrimSpace(id)
	if categoryID == "" {
		return ViewState{}, errors.New("id is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ViewState{}, errors.New("name is required")
	}
	scope := normalizeScope(input.Scope)
	if scope == "" {
		scope = ScopeAll
	}
	dsIDs := normalizeDatasourceIDs(input.DatasourceIDs)
	if scope == ScopeDatasource && len(dsIDs) == 0 {
		return ViewState{}, errors.New("datasourceIds is required for scope=datasource")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := -1
	for i, cat := range m.state.Categories {
		if cat.ID == categoryID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ViewState{}, errors.New("category not found")
	}

	existing := m.state.Categories[idx]
	existing.Name = name
	existing.Description = strings.TrimSpace(input.Description)
	existing.Scope = scope
	existing.DatasourceIDs = dsIDs
	existing.UpdatedAt = time.Now().UTC().UnixNano()
	m.state.Categories[idx] = existing
	m.sortLocked()

	if err := m.saveLocked(); err != nil {
		return ViewState{}, err
	}
	_ = m.writeIndexesLocked()
	ready, msg := m.checkProviderLocked(ctx, "")
	return ViewState{State: cloneState(m.state), AIProviderReady: ready, AIProviderMessage: msg}, nil
}

func (m *Manager) DeleteCategory(ctx context.Context, id string) (ViewState, error) {
	_ = ctx
	if m == nil {
		return ViewState{}, errors.New("manager is nil")
	}
	categoryID := strings.TrimSpace(id)
	if categoryID == "" {
		return ViewState{}, errors.New("id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cats := m.state.Categories
	newCats := make([]Category, 0, len(cats))
	removed := false
	for _, c := range cats {
		if c.ID == categoryID {
			removed = true
			continue
		}
		newCats = append(newCats, c)
	}
	if !removed {
		return ViewState{}, errors.New("category not found")
	}
	m.state.Categories = newCats

	files := m.state.Files
	newFiles := make([]File, 0, len(files))
	for _, f := range files {
		if f.CategoryID == categoryID {
			_ = m.deleteFileArtifactsLocked(f)
			continue
		}
		newFiles = append(newFiles, f)
	}
	m.state.Files = newFiles
	m.sortLocked()

	if err := m.saveLocked(); err != nil {
		return ViewState{}, err
	}
	_ = m.writeIndexesLocked()
	ready, msg := m.checkProviderLocked(ctx, "")
	return ViewState{State: cloneState(m.state), AIProviderReady: ready, AIProviderMessage: msg}, nil
}

func (m *Manager) UploadFiles(ctx context.Context, categoryID string, files []UploadFileInput, aiConfigID string) (ViewState, error) {
	if m == nil {
		return ViewState{}, errors.New("manager is nil")
	}
	catID := strings.TrimSpace(categoryID)
	if catID == "" {
		return ViewState{}, errors.New("categoryId is required")
	}
	if len(files) == 0 {
		return ViewState{}, errors.New("files is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cat, ok := m.findCategoryLocked(catID)
	if !ok {
		return ViewState{}, errors.New("category not found")
	}

	ready, readyMsg := m.checkProviderLocked(ctx, aiConfigID)
	now := time.Now().UTC().UnixNano()

	added := make([]File, 0, len(files))
	for _, input := range files {
		originalName := strings.TrimSpace(input.Name)
		if originalName == "" {
			return ViewState{}, errors.New("file name is required")
		}
		dataB64 := strings.TrimSpace(input.Base64)
		if dataB64 == "" {
			return ViewState{}, errors.New("file base64 is required")
		}
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return ViewState{}, errors.New("invalid base64")
		}
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(originalName)))
		if !isAllowedExt(ext) {
			return ViewState{}, fmt.Errorf("unsupported file type %q", ext)
		}

		fileID := newID("kbfile")
		safeName := sanitizeFilename(originalName)
		uploadRel := filepath.ToSlash(filepath.Join("uploads", fileID, safeName))
		parsedRel := filepath.ToSlash(filepath.Join("parsed", "files", fileID+".txt"))
		uploadAbs := filepath.Join(m.root, filepath.FromSlash(uploadRel))
		parsedAbs := filepath.Join(m.root, filepath.FromSlash(parsedRel))

		if err := os.MkdirAll(filepath.Dir(uploadAbs), 0o755); err != nil {
			return ViewState{}, err
		}
		if err := securefile.WriteFile(uploadAbs, data, 0o644); err != nil {
			return ViewState{}, err
		}
		if err := os.MkdirAll(filepath.Dir(parsedAbs), 0o755); err != nil {
			return ViewState{}, err
		}

		record := File{
			ID:            fileID,
			CategoryID:    catID,
			OriginalName:  safeName,
			Ext:           ext,
			Size:          int64(len(data)),
			UploadPath:    uploadRel,
			ParsedPath:    parsedRel,
			ParseStatus:   ParseQueued,
			SummaryStatus: SummaryQueued,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		parseErr := m.parseToTextFile(ctx, ext, uploadAbs, parsedAbs)
		if parseErr != nil {
			record.ParseStatus = ParseFailed
			record.ParseError = parseErr.Error()
			record.SummaryStatus = SummarySkipped
			record.SummaryError = "parse failed"
		} else {
			record.ParseStatus = ParseOK
			_ = m.publishParsedToScopesLocked(cat, parsedAbs, fileID)

			if !ready {
				record.SummaryStatus = SummaryNeedsProvider
				record.SummaryError = readyMsg
			} else {
				summary, keywords, err := m.summarizeFileLocked(ctx, aiConfigID, safeName, parsedAbs)
				if err != nil {
					record.SummaryStatus = SummaryFailed
					record.SummaryError = err.Error()
				} else {
					record.SummaryStatus = SummaryOK
					record.AISummary = summary
					record.Keywords = keywords
				}
			}
		}

		added = append(added, record)
	}

	m.state.Files = append(m.state.Files, added...)
	m.sortLocked()
	if err := m.saveLocked(); err != nil {
		return ViewState{}, err
	}
	_ = m.writeIndexesLocked()

	ready, msg := m.checkProviderLocked(ctx, aiConfigID)
	return ViewState{State: cloneState(m.state), AIProviderReady: ready, AIProviderMessage: msg}, nil
}

func (m *Manager) DeleteFile(ctx context.Context, fileID string) (ViewState, error) {
	_ = ctx
	if m == nil {
		return ViewState{}, errors.New("manager is nil")
	}
	id := strings.TrimSpace(fileID)
	if id == "" {
		return ViewState{}, errors.New("fileId is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := -1
	var target File
	for i, f := range m.state.Files {
		if f.ID == id {
			idx = i
			target = f
			break
		}
	}
	if idx == -1 {
		return ViewState{}, errors.New("file not found")
	}

	_ = m.deleteFileArtifactsLocked(target)

	next := make([]File, 0, len(m.state.Files)-1)
	next = append(next, m.state.Files[:idx]...)
	next = append(next, m.state.Files[idx+1:]...)
	m.state.Files = next
	m.sortLocked()

	if err := m.saveLocked(); err != nil {
		return ViewState{}, err
	}
	_ = m.writeIndexesLocked()

	ready, msg := m.checkProviderLocked(ctx, "")
	return ViewState{State: cloneState(m.state), AIProviderReady: ready, AIProviderMessage: msg}, nil
}

func (m *Manager) ensureDirs() error {
	dirs := []string{
		filepath.Join(m.root, "uploads"),
		filepath.Join(m.parsedDir, "files"),
		m.scopesAllDir,
		m.scopesDSDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) loadLocked() error {
	content, err := securefile.ReadFile(m.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(content) == 0 {
		return nil
	}
	var state StoreState
	if err := json.Unmarshal(content, &state); err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	m.state = state
	m.sortLocked()
	return nil
}

func (m *Manager) saveLocked() error {
	payload, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.storePath + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.storePath)
}

func (m *Manager) sortLocked() {
	sort.SliceStable(m.state.Categories, func(i, j int) bool {
		return m.state.Categories[i].CreatedAt < m.state.Categories[j].CreatedAt
	})
	sort.SliceStable(m.state.Files, func(i, j int) bool {
		return m.state.Files[i].CreatedAt < m.state.Files[j].CreatedAt
	})
}

func cloneState(state StoreState) StoreState {
	out := StoreState{Version: state.Version}
	if len(state.Categories) > 0 {
		out.Categories = append([]Category(nil), state.Categories...)
	}
	if len(state.Files) > 0 {
		out.Files = append([]File(nil), state.Files...)
	}
	return out
}

func normalizeScope(scope CategoryScope) CategoryScope {
	switch strings.ToLower(strings.TrimSpace(string(scope))) {
	case "all", "":
		return ScopeAll
	case "datasource", "datasources":
		return ScopeDatasource
	default:
		return ""
	}
}

func normalizeDatasourceIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func newID(prefix string) string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("%s_%x", strings.TrimSpace(prefix), now)
}

func (m *Manager) checkProviderLocked(ctx context.Context, aiConfigID string) (bool, string) {
	if m == nil || m.models == nil {
		return false, "AI provider is not configured."
	}
	_, err := m.models.Resolve(strings.TrimSpace(aiConfigID))
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if msg == "" {
			msg = "AI provider is not configured."
		}
		return false, msg
	}
	return true, ""
}

func (m *Manager) deleteFileArtifactsLocked(f File) error {
	paths := []string{}
	add := func(p string) {
		if strings.TrimSpace(p) != "" {
			paths = append(paths, filepath.Join(m.root, filepath.FromSlash(p)))
		}
	}
	add(f.UploadPath)
	add(f.ParsedPath)
	for _, dsID := range m.categoryDatasourceIDsLocked(f.CategoryID) {
		paths = append(paths, filepath.Join(m.scopesDSDir, dsID, f.CategoryID, f.ID+".txt"))
	}
	paths = append(paths, filepath.Join(m.scopesAllDir, f.CategoryID, f.ID+".txt"))

	for _, p := range paths {
		_ = os.Remove(p)
	}
	return nil
}

func (m *Manager) categoryDatasourceIDsLocked(categoryID string) []string {
	for _, c := range m.state.Categories {
		if c.ID == categoryID {
			return append([]string(nil), c.DatasourceIDs...)
		}
	}
	return nil
}

func (m *Manager) writeIndexesLocked() error {
	_ = m.rebuildScopesLocked()
	// Minimal index generation for progressive retrieval: keep it readable and bounded.
	if err := os.MkdirAll(m.scopesAllDir, 0o755); err != nil {
		return err
	}

	globalTop := filepath.Join(m.scopesAllDir, "data_structure.md")
	if err := securefile.WriteFile(globalTop, []byte(m.renderGlobalIndexLocked()), 0o644); err != nil {
		return err
	}

	for _, cat := range m.state.Categories {
		if cat.Scope != ScopeAll {
			continue
		}
		catDir := filepath.Join(m.scopesAllDir, cat.ID)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}
		if err := securefile.WriteFile(filepath.Join(catDir, "data_structure.md"), []byte(m.renderCategoryIndexLocked(cat, "")), 0o644); err != nil {
			return err
		}
	}

	// Datasource-scoped indexes.
	dsIDs := m.allDatasourceIDsLocked()
	for _, dsID := range dsIDs {
		dsDir := filepath.Join(m.scopesDSDir, dsID)
		if err := os.MkdirAll(dsDir, 0o755); err != nil {
			return err
		}
		if err := securefile.WriteFile(filepath.Join(dsDir, "data_structure.md"), []byte(m.renderDatasourceIndexLocked(dsID)), 0o644); err != nil {
			return err
		}
		for _, cat := range m.state.Categories {
			if cat.Scope != ScopeDatasource {
				continue
			}
			if !containsString(cat.DatasourceIDs, dsID) {
				continue
			}
			catDir := filepath.Join(dsDir, cat.ID)
			if err := os.MkdirAll(catDir, 0o755); err != nil {
				return err
			}
			if err := securefile.WriteFile(filepath.Join(catDir, "data_structure.md"), []byte(m.renderCategoryIndexLocked(cat, dsID)), 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Manager) rebuildScopesLocked() error {
	scopesRoot := filepath.Join(m.parsedDir, "scopes")
	// All content under parsed/scopes is derived from store.json + parsed/files. Safe to rebuild.
	_ = os.RemoveAll(scopesRoot)
	if err := os.MkdirAll(m.scopesAllDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(m.scopesDSDir, 0o755); err != nil {
		return err
	}

	cats := map[string]Category{}
	for _, c := range m.state.Categories {
		cats[c.ID] = c
	}

	for _, f := range m.state.Files {
		if f.ParseStatus != ParseOK {
			continue
		}
		cat, ok := cats[f.CategoryID]
		if !ok {
			continue
		}
		parsedAbs := filepath.Join(m.root, filepath.FromSlash(f.ParsedPath))
		if _, err := os.Stat(parsedAbs); err != nil {
			continue
		}
		_ = m.publishParsedToScopesLocked(cat, parsedAbs, f.ID)
	}
	return nil
}

func (m *Manager) renderGlobalIndexLocked() string {
	var b strings.Builder
	b.WriteString("# User Knowledge Base Index (Global)\n\n")
	b.WriteString("This file is auto-generated.\n\n")
	b.WriteString("## Categories\n")
	for _, cat := range m.state.Categories {
		if cat.Scope != ScopeAll {
			continue
		}
		desc := strings.TrimSpace(cat.Description)
		if desc == "" {
			desc = "-"
		}
		b.WriteString(fmt.Sprintf("- %s (`%s`): %s\n", sanitizeInline(cat.Name), cat.ID, sanitizeInline(desc)))
		for _, f := range m.filesForCategoryLocked(cat.ID, "") {
			b.WriteString(fmt.Sprintf("  - %s (`%s`): %s\n", sanitizeInline(f.OriginalName), f.ID, sanitizeInline(fileSummaryLine(f))))
		}
	}
	b.WriteString("\n## Datasource-scoped categories\n")
	b.WriteString("Use `search_knowledge` with the current datasource id to include datasource-scoped entries.\n")
	return b.String()
}

func (m *Manager) renderDatasourceIndexLocked(datasourceID string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# User Knowledge Base Index (Datasource: %s)\n\n", sanitizeInline(datasourceID)))
	b.WriteString("This file is auto-generated. It includes global categories plus categories bound to this datasource.\n\n")

	b.WriteString("## Global categories\n")
	for _, cat := range m.state.Categories {
		if cat.Scope != ScopeAll {
			continue
		}
		desc := strings.TrimSpace(cat.Description)
		if desc == "" {
			desc = "-"
		}
		b.WriteString(fmt.Sprintf("- %s (`%s`): %s\n", sanitizeInline(cat.Name), cat.ID, sanitizeInline(desc)))
	}

	b.WriteString("\n## Datasource categories\n")
	for _, cat := range m.state.Categories {
		if cat.Scope != ScopeDatasource {
			continue
		}
		if !containsString(cat.DatasourceIDs, datasourceID) {
			continue
		}
		desc := strings.TrimSpace(cat.Description)
		if desc == "" {
			desc = "-"
		}
		b.WriteString(fmt.Sprintf("- %s (`%s`): %s\n", sanitizeInline(cat.Name), cat.ID, sanitizeInline(desc)))
		for _, f := range m.filesForCategoryLocked(cat.ID, datasourceID) {
			b.WriteString(fmt.Sprintf("  - %s (`%s`): %s\n", sanitizeInline(f.OriginalName), f.ID, sanitizeInline(fileSummaryLine(f))))
		}
	}
	return b.String()
}

func (m *Manager) renderCategoryIndexLocked(cat Category, datasourceID string) string {
	var b strings.Builder
	title := cat.Name
	if strings.TrimSpace(title) == "" {
		title = cat.ID
	}
	b.WriteString(fmt.Sprintf("# %s\n\n", sanitizeInline(title)))
	if desc := strings.TrimSpace(cat.Description); desc != "" {
		b.WriteString(sanitizeInline(desc) + "\n\n")
	}
	if cat.Scope == ScopeDatasource && strings.TrimSpace(datasourceID) != "" {
		b.WriteString(fmt.Sprintf("- Scope: datasource (`%s`)\n", sanitizeInline(datasourceID)))
	} else {
		b.WriteString("- Scope: all\n")
	}
	b.WriteString("\n## Files\n")
	for _, f := range m.filesForCategoryLocked(cat.ID, datasourceID) {
		b.WriteString(fmt.Sprintf("- %s (`%s`): %s\n", sanitizeInline(f.OriginalName), f.ID, sanitizeInline(fileSummaryLine(f))))
	}
	return b.String()
}

func (m *Manager) filesForCategoryLocked(categoryID string, datasourceID string) []File {
	files := make([]File, 0, 8)
	for _, f := range m.state.Files {
		if f.CategoryID != categoryID {
			continue
		}
		files = append(files, f)
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].CreatedAt < files[j].CreatedAt
	})
	return files
}

func (m *Manager) allDatasourceIDsLocked() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, cat := range m.state.Categories {
		if cat.Scope != ScopeDatasource {
			continue
		}
		for _, ds := range cat.DatasourceIDs {
			if strings.TrimSpace(ds) == "" {
				continue
			}
			if _, ok := seen[ds]; ok {
				continue
			}
			seen[ds] = struct{}{}
			out = append(out, ds)
		}
	}
	sort.Strings(out)
	return out
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func fileSummaryLine(f File) string {
	if strings.TrimSpace(f.Note) != "" {
		return f.Note
	}
	if strings.TrimSpace(f.AISummary) != "" {
		return f.AISummary
	}
	switch f.SummaryStatus {
	case SummaryNeedsProvider:
		return "AI summary pending (configure AI provider)."
	case SummaryQueued:
		return "AI summary queued."
	case SummaryFailed:
		if strings.TrimSpace(f.SummaryError) != "" {
			return "AI summary failed: " + f.SummaryError
		}
		return "AI summary failed."
	default:
		return "-"
	}
}

func sanitizeInline(text string) string {
	s := strings.TrimSpace(text)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

func (m *Manager) findCategoryLocked(id string) (Category, bool) {
	for _, cat := range m.state.Categories {
		if cat.ID == id {
			return cat, true
		}
	}
	return Category{}, false
}

func isAllowedExt(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".txt", ".md", ".markdown", ".docx", ".pdf":
		return true
	default:
		return false
	}
}

func sanitizeFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	base = strings.ReplaceAll(base, string(filepath.Separator), "_")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.TrimSpace(base)
	if base == "" {
		return "file"
	}
	return base
}

func (m *Manager) parseToTextFile(ctx context.Context, ext string, srcPath string, dstPath string) error {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".txt", ".md", ".markdown":
		data, err := securefile.ReadFile(srcPath)
		if err != nil {
			return err
		}
		return securefile.WriteFile(dstPath, data, 0o644)
	case ".docx":
		text, err := extractDocxText(srcPath)
		if err != nil {
			return err
		}
		return securefile.WriteFile(dstPath, []byte(text), 0o644)
	case ".pdf":
		return extractPDFText(ctx, srcPath, dstPath)
	default:
		return fmt.Errorf("unsupported file type %q", ext)
	}
}

func (m *Manager) publishParsedToScopesLocked(cat Category, parsedAbs string, fileID string) error {
	if cat.Scope == ScopeAll {
		dst := filepath.Join(m.scopesAllDir, cat.ID, fileID+".txt")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return linkOrCopy(parsedAbs, dst)
	}
	if cat.Scope == ScopeDatasource {
		for _, dsID := range cat.DatasourceIDs {
			dst := filepath.Join(m.scopesDSDir, dsID, cat.ID, fileID+".txt")
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := linkOrCopy(parsedAbs, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

func linkOrCopy(src string, dst string) error {
	_ = os.Remove(dst)
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

type summaryEnvelope struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
}

func (m *Manager) summarizeFileLocked(ctx context.Context, aiConfigID string, filename string, parsedAbs string) (string, []string, error) {
	if m.models == nil {
		return "", nil, errors.New("ai provider not configured")
	}
	model, err := m.models.Resolve(strings.TrimSpace(aiConfigID))
	if err != nil {
		return "", nil, err
	}
	data, err := securefile.ReadFile(parsedAbs)
	if err != nil {
		return "", nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", nil, errors.New("parsed text is empty")
	}
	// Keep input bounded.
	maxChars := 12000
	if len(text) > maxChars {
		text = text[:maxChars] + "\n\n[truncated]\n"
	}

	systemPrompt := "You summarize knowledge base files. Return ONLY JSON: {\"summary\": string, \"keywords\": string[]}. Summary should be 1-3 concise sentences describing what this file is useful for (schemas, relationships, pitfalls). Keywords up to 8."
	userPrompt := fmt.Sprintf("Filename: %s\n\nContent:\n%s\n", filename, text)

	resp, err := model.Chat(ctx, systemPrompt, []Message{{Role: "user", Content: userPrompt}})
	if err != nil {
		return "", nil, err
	}
	summary, keywords, err := parseSummaryJSON(resp)
	if err != nil {
		return "", nil, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", nil, errors.New("empty summary")
	}
	keywords = normalizeKeywords(keywords)
	return summary, keywords, nil
}

func parseSummaryJSON(content string) (string, []string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", nil, errors.New("empty ai response")
	}
	// Strip code fences.
	if strings.HasPrefix(trimmed, "```") {
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
		if end := strings.LastIndex(trimmed, "```"); end != -1 {
			trimmed = trimmed[:end]
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end <= start {
		return "", nil, errors.New("json payload not found")
	}
	payload := trimmed[start : end+1]
	var env summaryEnvelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		return "", nil, err
	}
	return env.Summary, env.Keywords, nil
}

func normalizeKeywords(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(values), 8))
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if len(out) >= 8 {
			break
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractDocxText(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()

	var document *zip.File
	for _, f := range r.File {
		if filepath.ToSlash(f.Name) == "word/document.xml" {
			document = f
			break
		}
	}
	if document == nil {
		return "", errors.New("docx missing word/document.xml")
	}
	rc, err := document.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var b strings.Builder
	inText := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Local == "p" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
			}
			if v.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			if v.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				b.WriteString(string(v))
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n", nil
}

func extractPDFText(ctx context.Context, srcPath string, dstPath string) error {
	tool, err := exec.LookPath("pdftotext")
	if err != nil {
		return errors.New("pdftotext not found (install poppler) to parse PDF")
	}
	cmd := exec.CommandContext(ctx, tool, "-layout", "-enc", "UTF-8", srcPath, dstPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("pdftotext failed: %s", msg)
	}
	return nil
}
