package riskengine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var safeRuleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

const builtinOverrideFilePrefix = "builtin-override-"
const builtinOverrideDirName = ".builtin-overrides"

// Store manages user-defined risk rules persisted as YAML files.
type Store struct {
	mu               sync.RWMutex
	dirPath          string
	rules            map[string]Rule
	paths            map[string]string
	builtinLookup    map[string]Rule
	probeLookup      map[string]Rule
	builtinOverrides map[string]Rule
	builtinPaths     map[string]string
}

// NewStore creates a new rule store at the given directory path.
func NewStore(dirPath string) *Store {
	return &Store{
		dirPath:          dirPath,
		rules:            make(map[string]Rule),
		paths:            make(map[string]string),
		builtinLookup:    builtinRuleMap(),
		probeLookup:      probeCatalogRuleMap(),
		builtinOverrides: make(map[string]Rule),
		builtinPaths:     make(map[string]string),
	}
}

// Load reads all *.yaml files from the store directory.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dirPath, 0o755); err != nil {
		return fmt.Errorf("create risk-rules dir: %w", err)
	}

	entries, err := os.ReadDir(s.dirPath)
	if err != nil {
		return fmt.Errorf("read risk-rules dir: %w", err)
	}

	rules := make(map[string]Rule)
	paths := make(map[string]string)
	builtinOverrides := make(map[string]Rule)
	builtinPaths := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(s.dirPath, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rule Rule
		if err := yaml.Unmarshal(content, &rule); err != nil {
			continue
		}
		if rule.ID == "" {
			rule.ID = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}
		if err := validateRuleID(rule.ID); err != nil {
			continue
		}
		rules[rule.ID] = rule
		paths[rule.ID] = path
	}
	overrideDir := filepath.Join(s.dirPath, builtinOverrideDirName)
	overrideEntries, err := os.ReadDir(overrideDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read builtin override dir: %w", err)
	}
	for _, entry := range overrideEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(overrideDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rule Rule
		if err := yaml.Unmarshal(content, &rule); err != nil {
			continue
		}
		if rule.ID == "" {
			rule.ID = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}
		if err := validateRuleID(rule.ID); err != nil {
			continue
		}
		if _, builtin := s.builtinLookup[rule.ID]; !builtin {
			if _, probe := s.probeLookup[rule.ID]; !probe {
				continue
			}
		}
		if _, probe := s.probeLookup[rule.ID]; probe {
			sanitized, err := sanitizeProbeCatalogThresholds(rule.ID, rule.Thresholds)
			if err != nil {
				continue
			}
			rule.Thresholds = sanitized
		}
		if _, builtin := s.builtinLookup[rule.ID]; builtin {
			rule.Thresholds = RuleThresholds{}
		}
		builtinOverrides[rule.ID] = Rule{ID: rule.ID, Enabled: rule.Enabled, Thresholds: rule.Thresholds}
		builtinPaths[rule.ID] = path
	}
	assignMissingUserRuleCodes(rules)

	s.rules = rules
	s.paths = paths
	s.builtinOverrides = builtinOverrides
	s.builtinPaths = builtinPaths
	return nil
}

// List returns all user-defined rules.
func (s *Store) List() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Rule, 0, len(s.rules))
	for _, rule := range s.rules {
		result = append(result, rule)
	}
	slices.SortFunc(result, func(a, b Rule) int {
		return strings.Compare(a.ID, b.ID)
	})
	return result
}

// BuiltinRules returns built-in rules with any persisted enabled-state overrides applied.
func (s *Store) BuiltinRules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := AllBuiltinRules()
	for i := range result {
		override, ok := s.builtinOverrides[result[i].ID]
		if !ok {
			continue
		}
		result[i].Enabled = override.Enabled
		result[i].Thresholds = overlayRuleThresholds(result[i].Thresholds, override.Thresholds)
	}
	return result
}

// ProbeRules returns probe catalog rules with persisted threshold overrides applied.
func (s *Store) ProbeRules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := ProbeCatalogRules()
	for i := range result {
		override, ok := s.builtinOverrides[result[i].ID]
		if !ok {
			continue
		}
		result[i].Enabled = override.Enabled
		result[i].Thresholds = overlayRuleThresholds(result[i].Thresholds, override.Thresholds)
	}
	return result
}

// Get returns a rule by ID.
func (s *Store) Get(id string) (Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.rules[id]
	return rule, ok
}

// Create adds a new rule and persists it.
func (s *Store) Create(rule Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}
	if err := validateRuleID(rule.ID); err != nil {
		return err
	}
	if _, exists := s.rules[rule.ID]; exists {
		return fmt.Errorf("rule %q already exists", rule.ID)
	}
	if strings.HasPrefix(rule.ID, builtinOverrideFilePrefix) {
		return fmt.Errorf("rule %q uses a reserved internal prefix", rule.ID)
	}
	if _, builtin := s.builtinLookup[rule.ID]; builtin {
		return fmt.Errorf("rule %q is reserved by a built-in rule", rule.ID)
	}
	if strings.TrimSpace(rule.Code) == "" {
		rule.Code = nextUserRuleCode(s.rules)
	}
	if err := s.writeRule(rule); err != nil {
		return err
	}
	s.rules[rule.ID] = rule
	return nil
}

// Update modifies an existing rule.
func (s *Store) Update(id string, rule Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rules[id]; !exists {
		return fmt.Errorf("rule %q not found", id)
	}
	if err := validateRuleID(id); err != nil {
		return err
	}
	rule.ID = id
	if strings.TrimSpace(rule.Code) == "" {
		rule.Code = s.rules[id].Code
	}
	if err := s.writeRule(rule); err != nil {
		return err
	}
	s.rules[id] = rule
	return nil
}

// Delete removes a rule.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rules[id]; !exists {
		return fmt.Errorf("rule %q not found", id)
	}
	if err := validateRuleID(id); err != nil {
		return err
	}
	for _, path := range s.rulePaths(id) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete rule file: %w", err)
		}
	}
	delete(s.rules, id)
	delete(s.paths, id)
	return nil
}

// SetEnabled enables or disables a rule.
func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateRuleID(id); err != nil {
		return err
	}
	if rule, exists := s.rules[id]; exists {
		rule.Enabled = enabled
		if err := s.writeRule(rule); err != nil {
			return err
		}
		s.rules[id] = rule
		return nil
	}
	return fmt.Errorf("rule %q not found", id)
}

// SetBuiltinEnabled enables or disables a built-in rule override.
func (s *Store) SetBuiltinEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateRuleID(id); err != nil {
		return err
	}
	base, builtin := s.builtinLookup[id]
	if !builtin {
		return fmt.Errorf("rule %q not found", id)
	}
	override := s.builtinOverrides[id]
	override.ID = id
	override.Enabled = enabled
	if enabled == !base.defaultDisabled && override.Thresholds.empty() {
		return s.deleteBuiltinOverride(id)
	}
	if err := s.writeBuiltinOverride(override); err != nil {
		return err
	}
	s.builtinOverrides[id] = override
	return nil
}

// UpdateBuiltinProbeRuleThresholds persists editable threshold overrides for probe catalog rules.
func (s *Store) UpdateBuiltinProbeRuleThresholds(id string, thresholds RuleThresholds) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateRuleID(id); err != nil {
		return err
	}
	base, probe := s.probeLookup[id]
	if !probe {
		return fmt.Errorf("rule %q not found", id)
	}
	sanitized, err := sanitizeProbeCatalogThresholds(id, thresholds)
	if err != nil {
		return err
	}
	override := s.builtinOverrides[id]
	override.ID = id
	override.Enabled = base.Enabled
	override.Thresholds = sanitized
	if override.Thresholds.empty() {
		return s.deleteBuiltinOverride(id)
	}
	if err := s.writeBuiltinOverride(override); err != nil {
		return err
	}
	s.builtinOverrides[id] = override
	return nil
}

func (s *Store) writeRule(rule Rule) error {
	if err := validateRuleID(rule.ID); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dirPath, 0o755); err != nil {
		return fmt.Errorf("create risk-rules dir: %w", err)
	}
	content, err := yaml.Marshal(rule)
	if err != nil {
		return fmt.Errorf("marshal rule: %w", err)
	}
	path := s.ruleFilePath(rule.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("write rule file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	for _, altPath := range s.rulePaths(rule.ID) {
		if altPath == path {
			continue
		}
		if err := os.Remove(altPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cleanup stale rule file: %w", err)
		}
	}
	s.paths[rule.ID] = path
	return nil
}

func (s *Store) ruleFilePath(id string) string {
	if path, ok := s.paths[id]; ok && strings.TrimSpace(path) != "" {
		return path
	}
	return filepath.Join(s.dirPath, id+".yaml")
}

func (s *Store) rulePaths(id string) []string {
	paths := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	add(s.ruleFilePath(id))
	add(filepath.Join(s.dirPath, id+".yaml"))
	add(filepath.Join(s.dirPath, id+".yml"))
	return paths
}

func (s *Store) builtinOverrideFilePath(id string) string {
	if path, ok := s.builtinPaths[id]; ok && strings.TrimSpace(path) != "" {
		return path
	}
	return filepath.Join(s.dirPath, builtinOverrideDirName, id+".yaml")
}

func (s *Store) builtinOverridePaths(id string) []string {
	paths := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	add(s.builtinOverrideFilePath(id))
	overrideDir := filepath.Join(s.dirPath, builtinOverrideDirName)
	add(filepath.Join(overrideDir, id+".yaml"))
	add(filepath.Join(overrideDir, id+".yml"))
	return paths
}

func (s *Store) writeBuiltinOverride(rule Rule) error {
	if err := validateRuleID(rule.ID); err != nil {
		return err
	}
	content, err := yaml.Marshal(rule)
	if err != nil {
		return fmt.Errorf("marshal rule: %w", err)
	}
	path := s.builtinOverrideFilePath(rule.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create builtin override dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("write rule file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	for _, altPath := range s.builtinOverridePaths(rule.ID) {
		if altPath == path {
			continue
		}
		if err := os.Remove(altPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cleanup stale builtin rule file: %w", err)
		}
	}
	s.builtinPaths[rule.ID] = path
	return nil
}

func (s *Store) deleteBuiltinOverride(id string) error {
	for _, path := range s.builtinOverridePaths(id) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete builtin override file: %w", err)
		}
	}
	delete(s.builtinOverrides, id)
	delete(s.builtinPaths, id)
	return nil
}

func builtinRuleMap() map[string]Rule {
	rules := AllBuiltinRules()
	lookup := make(map[string]Rule, len(rules))
	for _, rule := range rules {
		lookup[rule.ID] = rule
	}
	return lookup
}

func validateRuleID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return fmt.Errorf("rule ID is required")
	}
	if !safeRuleIDPattern.MatchString(trimmed) {
		return fmt.Errorf("rule ID %q contains unsupported characters", id)
	}
	return nil
}

func assignMissingUserRuleCodes(rules map[string]Rule) {
	if len(rules) == 0 {
		return
	}
	used := make(map[string]struct{}, len(rules))
	ids := make([]string, 0, len(rules))
	for id, rule := range rules {
		ids = append(ids, id)
		if code := strings.TrimSpace(rule.Code); code != "" {
			used[code] = struct{}{}
		}
	}
	slices.Sort(ids)
	next := 1
	for _, id := range ids {
		rule := rules[id]
		if strings.TrimSpace(rule.Code) != "" {
			continue
		}
		for {
			code := fmt.Sprintf("USR-%03d", next)
			next++
			if _, exists := used[code]; exists {
				continue
			}
			rule.Code = code
			rules[id] = rule
			used[code] = struct{}{}
			break
		}
	}
}

func nextUserRuleCode(rules map[string]Rule) string {
	maxValue := 0
	for _, rule := range rules {
		code := strings.TrimSpace(rule.Code)
		if !strings.HasPrefix(code, "USR-") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimPrefix(code, "USR-"))
		if err != nil {
			continue
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return fmt.Sprintf("USR-%03d", maxValue+1)
}
