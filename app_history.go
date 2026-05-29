package main

import (
	"errors"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/history"
)

type HistoryPayload struct {
	DatasourceID string `json:"datasourceId"`
	Statement    string `json:"statement"`
	Database     string `json:"database"`
}

type HistoryFilterPayload struct {
	DatasourceID string `json:"datasourceId"`
	Target       string `json:"target"`
	Database     string `json:"database"`
	Keyword      string `json:"keyword"`
	Limit        int    `json:"limit"`
}

type AgentAuditFilterPayload struct {
	AccessKey string `json:"accessKey"`
	Protocol  string `json:"protocol"`
	Keyword   string `json:"keyword"`
	Limit     int    `json:"limit"`
}

type AgentAuditEntryView struct {
	ID              string                       `json:"id"`
	AccessKey       string                       `json:"accessKey"`
	AgentName       string                       `json:"agentName"`
	AgentType       string                       `json:"agentType"`
	Protocol        string                       `json:"protocol"`
	ToolName        string                       `json:"toolName"`
	Summary         string                       `json:"summary"`
	Statement       string                       `json:"statement,omitempty"`
	DatasourceID    string                       `json:"datasourceId,omitempty"`
	DatasourceName  string                       `json:"datasourceName,omitempty"`
	DatasourceType  string                       `json:"datasourceType,omitempty"`
	Target          string                       `json:"target,omitempty"`
	Status          string                       `json:"status"`
	Message         string                       `json:"message,omitempty"`
	RiskAttribution *agentaudit.RiskAttribution  `json:"riskAttribution,omitempty"`
	ExecutedAt      string                       `json:"executedAt"`
}

func (a *App) AppendHistory(payload HistoryPayload) (history.Entry, error) {
	ds, ok := a.store.Get(payload.DatasourceID)
	if !ok {
		return history.Entry{}, errors.New("datasource not found")
	}
	stmt := strings.TrimSpace(payload.Statement)
	if stmt == "" {
		return history.Entry{}, nil
	}
	database := strings.TrimSpace(payload.Database)
	if ds.Type == datasource.TypeElasticsearch {
		database = ""
	} else if database == "" {
		database = ds.Database
	}
	if ds.Type == datasource.TypeRedis && database == "" {
		database = "0"
	}
	var targets []string
	switch ds.Type {
	case datasource.TypeMySQL, datasource.TypePostgreSQL, datasource.TypeD1:
		targets = history.ExtractSQLTargets(stmt)
	case datasource.TypeMongoDB:
		collection, _, err := console.ParseMongoTarget(stmt)
		if err == nil && collection != "" {
			targets = []string{collection}
		}
	case datasource.TypeElasticsearch:
		indices, err := console.ParseElasticsearchTargets(stmt)
		if err == nil && len(indices) > 0 {
			targets = indices
		}
	}
	return a.historyStore.Append(history.AppendInput{
		DatasourceID:   ds.ID,
		DatasourceName: ds.Name,
		DatasourceType: string(ds.Type),
		Database:       database,
		Statement:      stmt,
		Targets:        targets,
	})
}

func (a *App) ListHistory(filter HistoryFilterPayload) ([]history.Entry, error) {
	return a.historyStore.List(history.Filter{
		DatasourceID: filter.DatasourceID,
		Target:       filter.Target,
		Database:     filter.Database,
		Keyword:      filter.Keyword,
		Limit:        filter.Limit,
	}), nil
}

func (a *App) GetHistory(id string) (history.Entry, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return history.Entry{}, errors.New("history id is required")
	}
	entry, ok := a.historyStore.GetByID(trimmed)
	if !ok {
		return history.Entry{}, errors.New("history entry not found")
	}
	return entry, nil
}

func (a *App) DeleteHistory(id string) (bool, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return false, errors.New("history id is required")
	}
	return a.historyStore.DeleteByID(trimmed), nil
}

func (a *App) ClearHistory(filter HistoryFilterPayload) (int, error) {
	count := a.historyStore.Clear(history.Filter{
		DatasourceID: filter.DatasourceID,
		Target:       filter.Target,
		Database:     filter.Database,
		Keyword:      filter.Keyword,
	})
	return count, nil
}

func (a *App) ListAgentAudit(filter AgentAuditFilterPayload) ([]AgentAuditEntryView, error) {
	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(a.cfg.DataPath))
	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(a.cfg.DataPath))
	keyword := strings.TrimSpace(filter.Keyword)
	listLimit := filter.Limit
	if keyword != "" {
		listLimit = 0
	}
	items, err := auditStore.List(agentaudit.AuditFilter{
		AccessKey: strings.TrimSpace(filter.AccessKey),
		Protocol:  strings.TrimSpace(filter.Protocol),
		Keyword:   "",
		Limit:     listLimit,
	})
	if err != nil {
		return nil, err
	}
	identities, err := identityStore.ListAll()
	if err != nil {
		return nil, err
	}
	identityByAccessKey := make(map[string]agentaudit.AgentIdentity, len(identities))
	for _, identity := range identities {
		identityByAccessKey[identity.AccessKey] = identity
	}
	out := make([]AgentAuditEntryView, 0, len(items))
	for _, item := range items {
		view := AgentAuditEntryView{
			ID:              item.ID,
			AccessKey:       item.AccessKey,
			Protocol:        item.Protocol,
			ToolName:        item.ToolName,
			Summary:         item.Summary,
			Statement:       item.Statement,
			DatasourceID:    item.DatasourceID,
			DatasourceName:  item.DatasourceName,
			DatasourceType:  item.DatasourceType,
			Target:          item.Target,
			Status:          item.Status,
			Message:         item.Message,
			RiskAttribution: item.RiskAttribution,
			ExecutedAt:      item.ExecutedAt,
		}
		if identity, ok := identityByAccessKey[item.AccessKey]; ok {
			view.AgentName = identity.Name
			view.AgentType = identity.AgentType
		}
		if keyword != "" && !matchesAgentAuditKeyword(view, keyword) {
			continue
		}
		out = append(out, view)
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func matchesAgentAuditKeyword(entry AgentAuditEntryView, keyword string) bool {
	parts := []string{
		entry.AccessKey,
		entry.AgentName,
		entry.AgentType,
		entry.Protocol,
		entry.ToolName,
		entry.Summary,
		entry.Statement,
		entry.DatasourceName,
		entry.DatasourceType,
		entry.Target,
		entry.Status,
		entry.Message,
	}
	// The audit card now surfaces the matched-rule attribution (rule id, code,
	// description, action, level, source, and reasons). Keyword search has to
	// cover the same fields; otherwise users searching for visible text like
	// `delete_full_table`, the source bucket (`risk_engine` / `policy`), or a
	// reason fragment silently get empty results.
	if attr := entry.RiskAttribution; attr != nil {
		parts = append(parts,
			attr.RuleID,
			attr.RuleCode,
			attr.RuleDescription,
			attr.Action,
			attr.Level,
			attr.Source,
		)
		parts = append(parts, attr.Reasons...)
	}
	haystack := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(haystack, strings.ToLower(strings.TrimSpace(keyword)))
}
