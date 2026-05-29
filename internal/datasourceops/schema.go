package datasourceops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"futrixdata/platform/internal/datasource"
)

type schemaKnowledgeSnapshot struct {
	DatasourceID   string                  `json:"datasourceId"`
	DatasourceName string                  `json:"datasourceName"`
	DatasourceType string                  `json:"datasourceType"`
	Database       string                  `json:"database,omitempty"`
	CacheKey       string                  `json:"cacheKey"`
	UpdatedAt      int64                   `json:"updatedAt"`
	SchemaHash     string                  `json:"schemaHash"`
	Entities       []schemaKnowledgeEntity `json:"entities"`
}

type schemaKnowledgeERDocument struct {
	DatasourceID   string `json:"datasourceId"`
	DatasourceName string `json:"datasourceName"`
	DatasourceType string `json:"datasourceType"`
	SchemaHash     string `json:"schemaHash"`
	GeneratedAt    int64  `json:"generatedAt"`
	Content        string `json:"content"`
}

func (s *Service) GetSchemaKnowledge(ctx context.Context, datasourceID, entity, database string) (map[string]any, error) {
	_ = ctx
	ds, err := s.requireDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	snapshot, err := s.readSchemaSnapshot(ds)
	if err != nil {
		return nil, err
	}
	filtered := snapshot.Entities
	needle := strings.ToLower(strings.TrimSpace(entity))
	if needle != "" {
		filtered = make([]schemaKnowledgeEntity, 0, len(snapshot.Entities))
		for _, item := range snapshot.Entities {
			if strings.Contains(strings.ToLower(strings.TrimSpace(item.Name)), needle) {
				filtered = append(filtered, item)
			}
		}
	}
	return map[string]any{
		"datasourceId":   snapshot.DatasourceID,
		"datasourceName": snapshot.DatasourceName,
		"datasourceType": snapshot.DatasourceType,
		"database":       snapshot.Database,
		"cacheKey":       snapshot.CacheKey,
		"updatedAt":      snapshot.UpdatedAt,
		"schemaHash":     snapshot.SchemaHash,
		"entityCount":    len(filtered),
		"entities":       filtered,
	}, nil
}

func (s *Service) GetERKnowledge(ctx context.Context, datasourceID, database string) (map[string]any, error) {
	_ = ctx
	ds, err := s.requireDatasource(datasourceID)
	if err != nil {
		return nil, err
	}
	ds = datasourceWithDatabaseOverride(ds, database)
	doc, err := s.readERDoc(ds)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"datasourceId":   doc.DatasourceID,
		"datasourceName": doc.DatasourceName,
		"datasourceType": doc.DatasourceType,
		"schemaHash":     doc.SchemaHash,
		"generatedAt":    doc.GeneratedAt,
		"content":        doc.Content,
	}, nil
}

func (s *Service) readSchemaSnapshot(ds datasource.DataSource) (schemaKnowledgeSnapshot, error) {
	path := filepath.Join(s.schemaKnowledgeDatasourceDir(ds), "schema.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return schemaKnowledgeSnapshot{}, errors.New("schema knowledge not found")
		}
		return schemaKnowledgeSnapshot{}, err
	}
	var out schemaKnowledgeSnapshot
	if err := json.Unmarshal(content, &out); err != nil {
		return schemaKnowledgeSnapshot{}, err
	}
	return out, nil
}

func (s *Service) readERDoc(ds datasource.DataSource) (schemaKnowledgeERDocument, error) {
	path := filepath.Join(s.schemaKnowledgeDatasourceDir(ds), "er.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return schemaKnowledgeERDocument{}, errors.New("ER knowledge not found")
		}
		return schemaKnowledgeERDocument{}, err
	}
	var out schemaKnowledgeERDocument
	if err := json.Unmarshal(content, &out); err != nil {
		return schemaKnowledgeERDocument{}, err
	}
	if strings.TrimSpace(out.Content) == "" {
		return schemaKnowledgeERDocument{}, errors.New("ER knowledge is empty")
	}
	return out, nil
}

func (s *Service) schemaKnowledgeDatasourceDir(ds datasource.DataSource) string {
	root := strings.TrimSpace(s.schemaKnowledgeRoot)
	name := sanitizeKnowledgePathComponent(ds.Name)
	if name == "" {
		name = sanitizeKnowledgePathComponent(ds.ID)
	}
	if name == "" {
		name = "datasource"
	}
	id := sanitizeKnowledgePathComponent(ds.ID)
	if id == "" {
		id = "default"
	}
	return filepath.Join(root, name, id)
}

func sanitizeKnowledgePathComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		case r == ' ' || r == '/':
			return '_'
		default:
			return '_'
		}
	}, trimmed)
	return strings.Trim(out, "._")
}
