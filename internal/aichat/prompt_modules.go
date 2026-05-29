package aichat

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PromptModules holds optional prompt/knowledge extensions that can be appended
// to the base system prompt depending on the active datasource.
type PromptModules struct {
	TypePrompts         map[string]string
	DatasourcePrompts   map[string]string
	TypeKnowledge       map[string]string
	DatasourceKnowledge map[string]string
}

func (m PromptModules) empty() bool {
	return len(m.TypePrompts) == 0 && len(m.DatasourcePrompts) == 0 && len(m.TypeKnowledge) == 0 && len(m.DatasourceKnowledge) == 0
}

func (m PromptModules) merge(over PromptModules) PromptModules {
	out := PromptModules{
		TypePrompts:         cloneStringMap(m.TypePrompts),
		DatasourcePrompts:   cloneStringMap(m.DatasourcePrompts),
		TypeKnowledge:       cloneStringMap(m.TypeKnowledge),
		DatasourceKnowledge: cloneStringMap(m.DatasourceKnowledge),
	}
	mergeStringMap(out.TypePrompts, over.TypePrompts)
	mergeStringMap(out.DatasourcePrompts, over.DatasourcePrompts)
	mergeStringMap(out.TypeKnowledge, over.TypeKnowledge)
	mergeStringMap(out.DatasourceKnowledge, over.DatasourceKnowledge)
	return out
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mergeStringMap(dst map[string]string, src map[string]string) {
	if len(src) == 0 {
		return
	}
	for k, v := range src {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		dst[key] = val
	}
}

func DefaultPromptModules() PromptModules {
	return PromptModules{
		TypePrompts: map[string]string{
			"mysql":         normalizePromptModule(mysqlPromptModule),
			"postgresql":    normalizePromptModule(postgresqlPromptModule),
			"mongodb":       normalizePromptModule(mongodbPromptModule),
			"redis":         normalizePromptModule(redisPromptModule),
			"redis_cluster": normalizePromptModule(redisPromptModule),
			"elasticsearch": normalizePromptModule(elasticsearchPromptModule),
			"dynamodb":      normalizePromptModule(dynamodbPromptModule),
		},
	}
}

type PromptModulesLoadConfig struct {
	// PromptsDir is expected to contain:
	// - types/<type>.md
	// - datasources/<datasourceId>.md
	PromptsDir string

	// KnowledgeDir is expected to contain:
	// - types/<type>.md OR types/<type>/*.md
	// - datasources/<datasourceId>.md OR datasources/<datasourceId>/*.md
	KnowledgeDir string

	// MaxBytes caps each loaded module to avoid prompt bloat.
	// 0 means no cap.
	MaxBytes int
}

func LoadPromptModules(cfg PromptModulesLoadConfig) (PromptModules, error) {
	var out PromptModules
	if dir := strings.TrimSpace(cfg.PromptsDir); dir != "" {
		loaded, err := loadModulesRoot(dir, cfg.MaxBytes)
		if err != nil {
			return PromptModules{}, err
		}
		out = out.merge(PromptModules{
			TypePrompts:       loaded.TypePrompts,
			DatasourcePrompts: loaded.DatasourcePrompts,
		})
	}
	if dir := strings.TrimSpace(cfg.KnowledgeDir); dir != "" {
		loaded, err := loadModulesRoot(dir, cfg.MaxBytes)
		if err != nil {
			return PromptModules{}, err
		}
		out = out.merge(PromptModules{
			TypeKnowledge:       loaded.TypePrompts,
			DatasourceKnowledge: loaded.DatasourcePrompts,
		})
	}
	return out, nil
}

type loadedModules struct {
	TypePrompts       map[string]string
	DatasourcePrompts map[string]string
}

func loadModulesRoot(baseDir string, maxBytes int) (loadedModules, error) {
	info, err := os.Stat(baseDir)
	if err != nil || !info.IsDir() {
		return loadedModules{}, nil
	}

	typeMods := map[string]string{}
	dsMods := map[string]string{}

	if err := loadModulesDir(filepath.Join(baseDir, "types"), maxBytes, typeMods); err != nil {
		return loadedModules{}, err
	}
	if err := loadModulesDir(filepath.Join(baseDir, "datasources"), maxBytes, dsMods); err != nil {
		return loadedModules{}, err
	}

	return loadedModules{
		TypePrompts:       typeMods,
		DatasourcePrompts: dsMods,
	}, nil
}

func loadModulesDir(dir string, maxBytes int, dst map[string]string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Allow both:
	// - <key>.md
	// - <key>/*.md
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}

		key := strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
		if key == "" {
			continue
		}

		fullPath := filepath.Join(dir, name)
		content, ok, err := readMarkdownModule(fullPath, maxBytes)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		dst[key] = content
	}
	return nil
}

func readMarkdownModule(path string, maxBytes int) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, nil
	}

	if info.IsDir() {
		return readMarkdownModuleDir(path, maxBytes)
	}

	if strings.ToLower(filepath.Ext(path)) != ".md" {
		return "", false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false, nil
	}
	return clampModuleBytes(content, maxBytes), true, nil
}

func readMarkdownModuleDir(dir string, maxBytes int) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, err
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		if strings.ToLower(filepath.Ext(name)) != ".md" {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)

	if len(paths) == 0 {
		return "", false, nil
	}

	var b strings.Builder
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", false, err
		}
		part := strings.TrimSpace(string(data))
		if part == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if i == 0 && strings.HasPrefix(part, "#") {
			// Keep natural headings as-is.
			b.WriteString(part)
			continue
		}
		b.WriteString(part)
	}

	combined := strings.TrimSpace(b.String())
	if combined == "" {
		return "", false, nil
	}
	return clampModuleBytes(combined, maxBytes), true, nil
}

func clampModuleBytes(content string, maxBytes int) string {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return content
	}
	cut := strings.TrimSpace(content[:maxBytes])
	return cut + "\n\n(…truncated)"
}
