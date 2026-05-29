package datasourceops

import (
	"context"
	"errors"
	"strings"

	"futrixdata/platform/internal/sensitivity"
)

type SensitivityFieldInput struct {
	Name     string `json:"name"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

type SensitivityEntityInput struct {
	Database string                  `json:"database,omitempty"`
	Entity   string                  `json:"entity"`
	Fields   []SensitivityFieldInput `json:"fields"`
}

type SaveSensitivityReportInput struct {
	DatasourceID string                   `json:"datasourceId"`
	SchemaHash   string                   `json:"schemaHash,omitempty"`
	Database     string                   `json:"database,omitempty"`
	CustomRules  string                   `json:"customRules,omitempty"`
	Entities     []SensitivityEntityInput `json:"entities"`
}

func (s *Service) requireSensitivityStore() (*sensitivity.Store, error) {
	if s.sensitivityStore == nil {
		return nil, errors.New("sensitivity store is not configured")
	}
	return s.sensitivityStore, nil
}

func (s *Service) GetSensitivityConfig(ctx context.Context) (map[string]any, error) {
	_ = ctx
	store, err := s.requireSensitivityStore()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"mode":        string(store.GetMode()),
		"customRules": store.GetCustomRules(),
		"levelConfig": store.GetLevelConfig(),
	}, nil
}

func (s *Service) SetSensitivityCustomRules(ctx context.Context, rules string) (bool, error) {
	_ = ctx
	store, err := s.requireSensitivityStore()
	if err != nil {
		return false, err
	}
	if err := store.SetCustomRules(rules); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) GetSensitivityReport(ctx context.Context, datasourceID string) (map[string]any, error) {
	_ = ctx
	store, err := s.requireSensitivityStore()
	if err != nil {
		return nil, err
	}
	dc, ok := store.GetDatasource(strings.TrimSpace(datasourceID))
	if !ok {
		return map[string]any{"found": false}, nil
	}
	return map[string]any{
		"found":          true,
		"datasourceId":   dc.DatasourceID,
		"datasourceName": dc.DatasourceName,
		"datasourceType": dc.DatasourceType,
		"database":       dc.Database,
		"schemaHash":     dc.SchemaHash,
		"scannedAt":      dc.ScannedAt,
		"aiConfigId":     dc.AIConfigID,
		"entities":       dc.Entities,
	}, nil
}

func (s *Service) SaveSensitivityReport(ctx context.Context, input SaveSensitivityReportInput) (map[string]any, error) {
	_ = ctx
	store, err := s.requireSensitivityStore()
	if err != nil {
		return nil, err
	}
	ds, err := s.requireDatasource(input.DatasourceID)
	if err != nil {
		return nil, err
	}
	entities := make([]sensitivity.AgentEntityInput, 0, len(input.Entities))
	levelConfig := store.GetLevelConfig()
	validLevels := make(map[string]struct{}, len(levelConfig.Levels))
	levelKeys := make([]string, 0, len(levelConfig.Levels))
	for _, level := range levelConfig.Levels {
		validLevels[level.Key] = struct{}{}
		levelKeys = append(levelKeys, level.Key)
	}
	validCategories := []string{"pii", "credential", "financial", "behavioral", "medical", "location", "contact", "identifier", "none"}
	categorySet := make(map[string]struct{}, len(validCategories))
	for _, category := range validCategories {
		categorySet[category] = struct{}{}
	}

	entityDatabase := strings.TrimSpace(input.Database)
	for _, entityInput := range input.Entities {
		if db := strings.TrimSpace(entityInput.Database); db != "" {
			if entityDatabase == "" {
				entityDatabase = db
			} else if entityDatabase != db {
				return nil, errors.New("save_sensitivity_report currently supports one database value per report")
			}
		}
		fields := make([]sensitivity.AgentFieldInput, 0, len(entityInput.Fields))
		for _, fieldInput := range entityInput.Fields {
			level, ok := normalizeSensitivityLevelInput(fieldInput.Level, validLevels)
			if !ok {
				return nil, errors.New(`invalid level "` + fieldInput.Level + `"; valid: [` + strings.Join(levelKeys, ", ") + `]`)
			}
			category, ok := normalizeSensitivityCategoryInput(fieldInput.Category, categorySet)
			if !ok {
				return nil, errors.New(`invalid category "` + fieldInput.Category + `"; valid: [` + strings.Join(validCategories, ", ") + `]`)
			}
			fields = append(fields, sensitivity.AgentFieldInput{
				Name:     fieldInput.Name,
				Level:    level,
				Category: category,
				Reason:   fieldInput.Reason,
			})
		}
		entities = append(entities, sensitivity.AgentEntityInput{
			Entity: entityInput.Entity,
			Fields: fields,
		})
	}

	database := entityDatabase
	if database == "" {
		database = strings.TrimSpace(input.Database)
	}
	if database == "" {
		database = strings.TrimSpace(ds.Database)
	}

	reportInput := sensitivity.SaveAgentReportInput{
		DatasourceID:   ds.ID,
		DatasourceName: ds.Name,
		DatasourceType: string(ds.Type),
		Database:       database,
		SchemaHash:     strings.TrimSpace(input.SchemaHash),
		Entities:       entities,
	}
	var saveErr error
	if strings.TrimSpace(input.CustomRules) != "" {
		saveErr = store.SaveAgentReportWithCustomRules(reportInput, input.CustomRules)
	} else {
		saveErr = store.SaveAgentReport(reportInput)
	}
	if saveErr != nil {
		return nil, saveErr
	}

	return map[string]any{
		"ok":           true,
		"datasourceId": ds.ID,
		"entityCount":  len(entities),
	}, nil
}

func normalizeSensitivityLevelInput(raw string, validLevels map[string]struct{}) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if _, ok := validLevels[trimmed]; ok {
		return trimmed, true
	}
	upper := strings.ToUpper(trimmed)
	for level := range validLevels {
		if strings.ToUpper(level) == upper {
			return level, true
		}
	}
	legacyMap := map[string]string{
		"critical": "L5",
		"high":     "L4",
		"medium":   "L3",
		"low":      "L2",
	}
	if mapped, ok := legacyMap[strings.ToLower(trimmed)]; ok {
		if _, valid := validLevels[mapped]; valid {
			return mapped, true
		}
	}
	return "", false
}

func normalizeSensitivityCategoryInput(raw string, validCategories map[string]struct{}) (string, bool) {
	category := strings.ToLower(strings.TrimSpace(raw))
	if category == "" {
		return "", false
	}
	_, ok := validCategories[category]
	return category, ok
}

func (s *Service) DeleteSensitivityReport(ctx context.Context, datasourceID string) (bool, error) {
	_ = ctx
	store, err := s.requireSensitivityStore()
	if err != nil {
		return false, err
	}
	if err := store.DeleteDatasource(strings.TrimSpace(datasourceID)); err != nil {
		return false, err
	}
	return true, nil
}
