package aichat

import (
	"context"
	"strings"
)

func cloneWorkingContext(value *WorkingContext) *WorkingContext {
	if value == nil {
		return nil
	}
	copyValue := *value
	copyValue.DatasourceID = strings.TrimSpace(copyValue.DatasourceID)
	copyValue.DatasourceType = normalizeDatasourceType(copyValue.DatasourceType)
	copyValue.Database = strings.TrimSpace(copyValue.Database)
	copyValue.Entity = strings.TrimSpace(copyValue.Entity)
	copyValue.Source = strings.TrimSpace(copyValue.Source)
	copyValue.ToolName = strings.TrimSpace(copyValue.ToolName)
	if copyValue.DatasourceID == "" && copyValue.DatasourceType == "" && copyValue.Database == "" && copyValue.Entity == "" {
		return nil
	}
	return &copyValue
}

func workingContextFromPageContext(page PageContext) *WorkingContext {
	if strings.TrimSpace(page.CurrentDatasourceID) == "" &&
		strings.TrimSpace(page.CurrentDatasourceType) == "" &&
		strings.TrimSpace(page.CurrentDatabase) == "" &&
		strings.TrimSpace(page.CurrentEntity) == "" {
		return nil
	}
	return cloneWorkingContext(&WorkingContext{
		DatasourceID:   strings.TrimSpace(page.CurrentDatasourceID),
		DatasourceType: strings.TrimSpace(page.CurrentDatasourceType),
		Database:       strings.TrimSpace(page.CurrentDatabase),
		Entity:         strings.TrimSpace(page.CurrentEntity),
		Source:         "focus",
		Confidence:     0.55,
	})
}

func workingContextFromContextChips(chips []ContextChip) *WorkingContext {
	for _, chip := range chips {
		if strings.TrimSpace(chip.DatasourceID) == "" {
			continue
		}
		return cloneWorkingContext(&WorkingContext{
			DatasourceID: strings.TrimSpace(chip.DatasourceID),
			Source:       "explicit",
			Confidence:   0.95,
		})
	}
	return nil
}

func mergeWorkingContext(primary *WorkingContext, fallback *WorkingContext) *WorkingContext {
	primary = cloneWorkingContext(primary)
	fallback = cloneWorkingContext(fallback)
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	sameDatasource := primary.DatasourceID == "" || fallback.DatasourceID == "" || primary.DatasourceID == fallback.DatasourceID
	if primary.DatasourceID == "" {
		primary.DatasourceID = fallback.DatasourceID
	}
	if primary.DatasourceType == "" && sameDatasource {
		primary.DatasourceType = fallback.DatasourceType
	}
	if primary.Database == "" && sameDatasource {
		primary.Database = fallback.Database
	}
	if primary.Entity == "" && sameDatasource {
		primary.Entity = fallback.Entity
	}
	if primary.Source == "" {
		primary.Source = fallback.Source
	}
	if primary.ToolName == "" {
		primary.ToolName = fallback.ToolName
	}
	if primary.Confidence <= 0 {
		primary.Confidence = fallback.Confidence
	}
	return cloneWorkingContext(primary)
}

func workingContextFromPayload(payload map[string]any) *WorkingContext {
	if payload == nil {
		return nil
	}
	return cloneWorkingContext(&WorkingContext{
		DatasourceID:   stringPayload(payload, "datasourceId"),
		DatasourceType: stringPayload(payload, "datasourceType"),
		Database:       stringPayload(payload, "database"),
		Entity:         stringPayload(payload, "entity", "name"),
		Source:         stringPayload(payload, "source"),
		ToolName:       stringPayload(payload, "toolName"),
		Confidence:     floatArg(payload, "confidence", 0.8),
	})
}

func collectWorkingContext(events []threadEventRecord) *WorkingContext {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "working_context_updated" {
			continue
		}
		if resolved := workingContextFromPayload(events[i].Payload); resolved != nil {
			return resolved
		}
	}
	return nil
}

func (s *Service) resolveWorkingContext(ctx context.Context, req TurnRequest, ws WorkingSet) *WorkingContext {
	focus := workingContextFromPageContext(req.PageContext)
	if turnIntentPrefersCurrentFocus(req) && focus != nil {
		return s.fillWorkingContextDatasourceType(ctx, focus)
	}

	if explicit := workingContextFromContextChips(req.ContextChips); explicit != nil {
		return s.fillWorkingContextDatasourceType(ctx, mergeWorkingContext(explicit, focus))
	}
	if direct := cloneWorkingContext(req.WorkingContext); direct != nil {
		if turnIntentAvoidsCurrentFocus(req) {
			return s.fillWorkingContextDatasourceType(ctx, direct)
		}
		return s.fillWorkingContextDatasourceType(ctx, mergeWorkingContext(direct, focus))
	}
	if sticky := cloneWorkingContext(ws.WorkingContext); sticky != nil {
		resolved := sticky
		if !turnIntentAvoidsCurrentFocus(req) {
			resolved = mergeWorkingContext(sticky, focus)
		}
		resolved.Source = "sticky"
		return s.fillWorkingContextDatasourceType(ctx, resolved)
	}
	if turnIntentAvoidsCurrentFocus(req) {
		return nil
	}
	return s.fillWorkingContextDatasourceType(ctx, focus)
}

func (s *Service) fillWorkingContextDatasourceType(ctx context.Context, value *WorkingContext) *WorkingContext {
	value = cloneWorkingContext(value)
	if value == nil {
		return nil
	}
	if value.DatasourceType == "" && value.DatasourceID != "" && s != nil {
		if ds, err := s.getDatasourceCached(ctx, value.DatasourceID); err == nil {
			value.DatasourceType = normalizeDatasourceType(ds.Type)
		}
	}
	return cloneWorkingContext(value)
}

func (s *Service) attachResolvedWorkingContext(ctx context.Context, req TurnRequest, ws *workingSetBuildResult) TurnRequest {
	if ws != nil {
		req.WorkingContext = s.resolveWorkingContext(ctx, req, ws.WorkingSet)
		ws.WorkingSet.WorkingContext = mergeWorkingContext(req.WorkingContext, ws.WorkingSet.WorkingContext)
		return req
	}
	req.WorkingContext = s.resolveWorkingContext(ctx, req, WorkingSet{})
	return req
}

func establishedWorkingContext(req TurnRequest) *WorkingContext {
	if !hasEstablishedWorkingTarget(req) {
		return nil
	}
	return cloneWorkingContext(req.WorkingContext)
}
