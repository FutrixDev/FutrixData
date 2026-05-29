package sensitivity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/securefile"
)

// Store persists sensitivity classifications to a JSON file.
type Store struct {
	mu    sync.RWMutex
	path  string
	state StoreState
}

func NewStore(path string) *Store {
	defaults := DefaultLevelConfig()
	return &Store{
		path: path,
		state: StoreState{
			Version:     2,
			Mode:        ModeWhitelist,
			LevelConfig: &defaults,
			Datasources: make(map[string]DatasourceClassification),
		},
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := securefile.ReadFile(s.path)
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
	if state.Datasources == nil {
		state.Datasources = make(map[string]DatasourceClassification)
	}
	if state.Mode == "" {
		state.Mode = ModeWhitelist
	}
	if state.Version < 2 {
		state = migrateV1ToV2(state)
	}
	if state.LevelConfig == nil {
		defaults := DefaultLevelConfig()
		state.LevelConfig = &defaults
	} else {
		// Check whether we need to migrate from legacy agentThreshold to new range fields.
		// We distinguish legacy from new-format by checking if the JSON contains the new keys.
		var rawCfg struct {
			LevelConfig *json.RawMessage `json:"levelConfig"`
		}
		needsMigrate := false
		if json.Unmarshal(content, &rawCfg) == nil && rawCfg.LevelConfig != nil {
			var fields map[string]json.RawMessage
			if json.Unmarshal(*rawCfg.LevelConfig, &fields) == nil {
				_, hasFrom := fields["agentAccessFrom"]
				_, hasTo := fields["agentAccessTo"]
				if !hasFrom && !hasTo {
					needsMigrate = true
				}
			}
		}
		if needsMigrate {
			var legacy struct {
				LevelConfig *struct {
					AgentThreshold *int `json:"agentThreshold"`
				} `json:"levelConfig"`
			}
			if json.Unmarshal(content, &legacy) == nil && legacy.LevelConfig != nil && legacy.LevelConfig.AgentThreshold != nil && *legacy.LevelConfig.AgentThreshold > 0 {
				state.LevelConfig.AgentAccessFrom = 1
				state.LevelConfig.AgentAccessTo = *legacy.LevelConfig.AgentThreshold
			}
			// else: agentThreshold was 0 or absent → keep 0,0 (no restriction)
		}
	}
	s.state = state
	return nil
}

// migrateV1ToV2 converts old level strings (critical/high/medium/low) to L1-L5 keys.
func migrateV1ToV2(state StoreState) StoreState {
	levelMap := map[SensitivityLevel]SensitivityLevel{
		LevelCritical: "L5",
		LevelHigh:     "L4",
		LevelMedium:   "L3",
		LevelLow:      "L2",
	}
	for dsID, dc := range state.Datasources {
		for eName, ec := range dc.Entities {
			for fName, fc := range ec.Fields {
				if newLevel, ok := levelMap[fc.Level]; ok {
					fc.Level = newLevel
					ec.Fields[fName] = fc
				}
			}
			dc.Entities[eName] = ec
		}
		state.Datasources[dsID] = dc
	}
	defaults := DefaultLevelConfig()
	state.LevelConfig = &defaults
	state.Version = 2
	return state
}

func (s *Store) save() error {
	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) GetMode() ModeType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Mode
}

func (s *Store) SetMode(mode ModeType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Mode = mode
	return s.save()
}

func (s *Store) GetCustomRules() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.CustomRules
}

func (s *Store) SetCustomRules(rules string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.CustomRules = rules
	return s.save()
}

func (s *Store) GetDatasource(datasourceID string) (DatasourceClassification, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dc, ok := s.state.Datasources[datasourceID]
	if !ok {
		return dc, false
	}
	// Deep copy to avoid callers holding references to store-owned maps.
	cp := dc
	cp.Entities = make(map[string]EntityClassification, len(dc.Entities))
	for eName, ec := range dc.Entities {
		ecCopy := EntityClassification{Fields: make(map[string]FieldClassification, len(ec.Fields))}
		for fName, fc := range ec.Fields {
			ecCopy.Fields[fName] = fc
		}
		cp.Entities[eName] = ecCopy
	}
	return cp, true
}

func (s *Store) SetDatasource(dc DatasourceClassification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Datasources[dc.DatasourceID] = dc
	return s.save()
}

func (s *Store) DeleteDatasource(datasourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Datasources, datasourceID)
	return s.save()
}

// SaveAgentReport persists a full datasource classification report produced by a local agent.
func (s *Store) SaveAgentReport(input SaveAgentReportInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	classification, err := s.buildAgentDatasourceClassification(input)
	if err != nil {
		return err
	}
	return s.saveAgentReportStateLocked(classification, "", false)
}

// SaveAgentReportWithCustomRules persists a local-agent report and matching rules in one save.
func (s *Store) SaveAgentReportWithCustomRules(input SaveAgentReportInput, customRules string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	classification, err := s.buildAgentDatasourceClassification(input)
	if err != nil {
		return err
	}
	return s.saveAgentReportStateLocked(classification, customRules, true)
}

func (s *Store) saveAgentReportStateLocked(classification DatasourceClassification, customRules string, updateRules bool) error {
	previousClassification, hadPreviousClassification := s.state.Datasources[classification.DatasourceID]
	previousRules := s.state.CustomRules

	s.state.Datasources[classification.DatasourceID] = classification
	if updateRules {
		s.state.CustomRules = customRules
	}
	if err := s.save(); err != nil {
		if hadPreviousClassification {
			s.state.Datasources[classification.DatasourceID] = previousClassification
		} else {
			delete(s.state.Datasources, classification.DatasourceID)
		}
		if updateRules {
			s.state.CustomRules = previousRules
		}
		return err
	}
	return nil
}

func (s *Store) buildAgentDatasourceClassification(input SaveAgentReportInput) (DatasourceClassification, error) {
	datasourceID := strings.TrimSpace(input.DatasourceID)
	if datasourceID == "" {
		return DatasourceClassification{}, fmt.Errorf("datasource ID is required")
	}
	if len(input.Entities) == 0 {
		return DatasourceClassification{}, fmt.Errorf("at least one entity is required")
	}

	levelCfg := s.state.LevelConfig
	if levelCfg == nil {
		defaults := DefaultLevelConfig()
		levelCfg = &defaults
	}
	validLevels := make(map[string]bool, len(levelCfg.Levels))
	for _, l := range levelCfg.Levels {
		validLevels[l.Key] = true
	}
	validCategories := map[string]Category{
		"pii":        CategoryPII,
		"credential": CategoryCredential,
		"financial":  CategoryFinancial,
		"behavioral": CategoryBehavioral,
		"medical":    CategoryMedical,
		"location":   CategoryLocation,
		"contact":    CategoryContact,
		"identifier": CategoryIdentifier,
		"none":       CategoryNone,
	}

	entities := make(map[string]EntityClassification, len(input.Entities))
	for _, entityInput := range input.Entities {
		entityName := strings.TrimSpace(entityInput.Entity)
		if entityName == "" {
			return DatasourceClassification{}, fmt.Errorf("entity name is required")
		}
		if _, exists := entities[entityName]; exists {
			return DatasourceClassification{}, fmt.Errorf("duplicate entity %q", entityName)
		}
		if len(entityInput.Fields) == 0 {
			return DatasourceClassification{}, fmt.Errorf("entity %q must include at least one field", entityName)
		}

		fields := make(map[string]FieldClassification, len(entityInput.Fields))
		for _, fieldInput := range entityInput.Fields {
			fieldName := strings.TrimSpace(fieldInput.Name)
			if fieldName == "" {
				return DatasourceClassification{}, fmt.Errorf("field name is required")
			}
			if _, exists := fields[fieldName]; exists {
				return DatasourceClassification{}, fmt.Errorf("duplicate field %q in entity %q", fieldName, entityName)
			}

			level := normalizeSensitivityLevel(fieldInput.Level, validLevels)
			if level == LevelUnconfirmed {
				return DatasourceClassification{}, fmt.Errorf("invalid level %q", strings.TrimSpace(fieldInput.Level))
			}
			categoryKey := strings.ToLower(strings.TrimSpace(fieldInput.Category))
			category, ok := validCategories[categoryKey]
			if !ok {
				return DatasourceClassification{}, fmt.Errorf("invalid category %q", fieldInput.Category)
			}

			fields[fieldName] = FieldClassification{
				Level:    level,
				Category: category,
				Reason:   fieldInput.Reason,
				Source:   SourceAgent,
			}
		}
		entities[entityName] = EntityClassification{Fields: fields}
	}

	return DatasourceClassification{
		DatasourceID:   datasourceID,
		DatasourceName: strings.TrimSpace(input.DatasourceName),
		DatasourceType: strings.TrimSpace(input.DatasourceType),
		Database:       strings.TrimSpace(input.Database),
		SchemaHash:     strings.TrimSpace(input.SchemaHash),
		ScannedAt:      time.Now().UTC().Unix(),
		Entities:       entities,
	}, nil
}

func (s *Store) UpdateField(datasourceID, entityName, fieldName string, fc FieldClassification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dc, ok := s.state.Datasources[datasourceID]
	if !ok {
		return fmt.Errorf("datasource %q not found", datasourceID)
	}
	ec, ok := dc.Entities[entityName]
	if !ok {
		ec = EntityClassification{Fields: make(map[string]FieldClassification)}
	}
	ec.Fields[fieldName] = fc
	if dc.Entities == nil {
		dc.Entities = make(map[string]EntityClassification)
	}
	dc.Entities[entityName] = ec
	s.state.Datasources[datasourceID] = dc
	return s.save()
}

// ConfirmField allows a user to manually confirm/override a field's classification.
func (s *Store) ConfirmField(datasourceID, entityName, fieldName string, level SensitivityLevel, category Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dc, ok := s.state.Datasources[datasourceID]
	if !ok {
		return fmt.Errorf("datasource %q not found", datasourceID)
	}
	ec, ok := dc.Entities[entityName]
	if !ok {
		return fmt.Errorf("entity %q not found in datasource %q", entityName, datasourceID)
	}
	fc, ok := ec.Fields[fieldName]
	if !ok {
		return fmt.Errorf("field %q not found in entity %q", fieldName, entityName)
	}
	fc.Level = level
	fc.Category = category
	fc.Source = SourceManual
	fc.ConfirmedBy = "user"
	fc.ConfirmedAt = time.Now().UTC().Unix()
	ec.Fields[fieldName] = fc
	dc.Entities[entityName] = ec
	s.state.Datasources[datasourceID] = dc
	return s.save()
}

// GetLevelConfig returns a copy of the current level configuration.
func (s *Store) GetLevelConfig() LevelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.LevelConfig == nil {
		return DefaultLevelConfig()
	}
	cfg := *s.state.LevelConfig
	cfg.Levels = make([]LevelDefinition, len(s.state.LevelConfig.Levels))
	copy(cfg.Levels, s.state.LevelConfig.Levels)
	return cfg
}

// SetLevelConfig validates and persists a new level configuration.
func (s *Store) SetLevelConfig(cfg LevelConfig) error {
	if len(cfg.Levels) == 0 {
		return fmt.Errorf("at least one level is required")
	}
	seen := make(map[string]bool, len(cfg.Levels))
	for _, l := range cfg.Levels {
		if l.Key == "" {
			return fmt.Errorf("level key cannot be empty")
		}
		if l.Key == string(LevelUnconfirmed) {
			return fmt.Errorf("level key %q is reserved", l.Key)
		}
		if seen[l.Key] {
			return fmt.Errorf("duplicate level key %q", l.Key)
		}
		seen[l.Key] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LevelConfig = &cfg
	return s.save()
}

// ListDatasources returns all datasource IDs with classifications.
func (s *Store) ListDatasources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.state.Datasources))
	for id := range s.state.Datasources {
		ids = append(ids, id)
	}
	return ids
}

// GetState returns a deep copy of the full store state (for export/debugging).
func (s *Store) GetState() StoreState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := StoreState{
		Version:     s.state.Version,
		Mode:        s.state.Mode,
		CustomRules: s.state.CustomRules,
		Datasources: make(map[string]DatasourceClassification, len(s.state.Datasources)),
	}
	if s.state.LevelConfig != nil {
		cfg := *s.state.LevelConfig
		cfg.Levels = make([]LevelDefinition, len(s.state.LevelConfig.Levels))
		copy(cfg.Levels, s.state.LevelConfig.Levels)
		cp.LevelConfig = &cfg
	}
	for dsID, dc := range s.state.Datasources {
		dcCopy := dc
		dcCopy.Entities = make(map[string]EntityClassification, len(dc.Entities))
		for eName, ec := range dc.Entities {
			ecCopy := EntityClassification{Fields: make(map[string]FieldClassification, len(ec.Fields))}
			for fName, fc := range ec.Fields {
				ecCopy.Fields[fName] = fc
			}
			dcCopy.Entities[eName] = ecCopy
		}
		cp.Datasources[dsID] = dcCopy
	}
	return cp
}
