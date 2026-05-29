package main

import (
	"context"
	"fmt"
	"strings"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/datasource"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type appAIChatProgressDiagnostics struct {
	emitCtx context.Context
	next    aichat.Diagnostics
	store   *datasource.Store
}

func newAppAIChatProgressDiagnostics(emitCtx context.Context, next aichat.Diagnostics, store *datasource.Store) aichat.Diagnostics {
	return &appAIChatProgressDiagnostics{
		emitCtx: emitCtx,
		next:    next,
		store:   store,
	}
}

func (d *appAIChatProgressDiagnostics) Log(event string, fields map[string]any) {
	if d == nil {
		return
	}
	if d.next != nil {
		d.next.Log(event, fields)
	}
	if d.emitCtx == nil {
		return
	}
	if fields == nil {
		return
	}

	streamID, _ := fields["streamId"].(string)
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return
	}
	conversationID, _ := fields["conversationId"].(string)

	message := d.progressMessageForEvent(event, fields)
	if message == "" {
		return
	}

	if d.next != nil {
		d.next.Log("ui_progress_emit", map[string]any{
			"streamId":       streamID,
			"conversationId": conversationID,
			"sourceEvent":    event,
			"message":        message,
		})
	}

	payload := map[string]any{
		"streamId":       streamID,
		"conversationId": conversationID,
		"message":        message,
		"event":          event,
	}
	if ms, ok := fields["durationMs"]; ok {
		payload["durationMs"] = ms
	}
	runtime.EventsEmit(d.emitCtx, "aichat:progress", payload)
}

func (d *appAIChatProgressDiagnostics) progressMessageForEvent(event string, fields map[string]any) string {
	fieldString := func(key string) string {
		if fields == nil {
			return ""
		}
		if v, ok := fields[key].(string); ok {
			return strings.TrimSpace(v)
		}
		return strings.TrimSpace(fmt.Sprint(fields[key]))
	}
	fieldInt := func(key string) int {
		if fields == nil {
			return 0
		}
		switch v := fields[key].(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		default:
			var parsed int
			_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(v)), "%d", &parsed)
			return parsed
		}
	}

	datasourceLabel := func(id string) string {
		id = strings.TrimSpace(id)
		if id == "" {
			return ""
		}
		if d == nil || d.store == nil {
			return id
		}
		ds, ok := d.store.Get(id)
		if !ok {
			return id
		}
		name := strings.TrimSpace(ds.Name)
		if name == "" {
			return id
		}
		return name
	}

	switch event {
	case "turn_start":
		routeName := fieldString("routeName")
		currentDatasource := datasourceLabel(fieldString("currentDatasource"))
		messageCount := fieldInt("messageCount")
		parts := make([]string, 0, 3)
		if routeName != "" {
			parts = append(parts, "route: "+routeName)
		}
		if currentDatasource != "" {
			parts = append(parts, "datasource: "+currentDatasource)
		}
		if messageCount > 0 {
			parts = append(parts, fmt.Sprintf("messages: %d", messageCount))
		}
		if len(parts) > 0 {
			return fmt.Sprintf("Thinking (%s)…", strings.Join(parts, ", "))
		}
		return "Thinking…"
	case "model_stream_first_delta":
		return "Model is responding…"
	case "model_output_parsed":
		toolCallsCount := fieldInt("toolCallsCount")
		assistantLen := fieldInt("assistantLen")
		if toolCallsCount > 0 && assistantLen == 0 {
			preview := fieldString("toolCallsPreview")
			if preview != "" && preview != "<nil>" {
				return fmt.Sprintf("Planning next actions (%s)…", preview)
			}
			return fmt.Sprintf("Planning next actions (%d tools)…", toolCallsCount)
		}
		return ""
	case "tool_protocol_force_toolcalls_triggered":
		return "Refining plan…"

	case "tool_list_entities_start":
		database := fieldString("database")
		datasourceID := datasourceLabel(fieldString("datasourceId"))
		if database != "" && datasourceID != "" {
			return fmt.Sprintf("Listing entities (db: %s, datasource: %s)…", database, datasourceID)
		}
		if database != "" {
			return fmt.Sprintf("Listing entities (db: %s)…", database)
		}
		if datasourceID != "" {
			return fmt.Sprintf("Listing entities (datasource: %s)…", datasourceID)
		}
		return "Listing entities…"
	case "tool_describe_entity_start":
		name := fieldString("name")
		database := fieldString("database")
		datasourceID := datasourceLabel(fieldString("datasourceId"))
		parts := make([]string, 0, 3)
		if name != "" {
			parts = append(parts, name)
		}
		if database != "" {
			parts = append(parts, "db: "+database)
		}
		if datasourceID != "" {
			parts = append(parts, "datasource: "+datasourceID)
		}
		if len(parts) > 0 {
			return fmt.Sprintf("Describing entity (%s)…", strings.Join(parts, ", "))
		}
		return "Describing entity…"
	case "tool_explain_statement_start":
		database := fieldString("database")
		datasourceID := datasourceLabel(fieldString("datasourceId"))
		statementLen := fieldInt("statementLen")
		parts := make([]string, 0, 3)
		if statementLen > 0 {
			parts = append(parts, fmt.Sprintf("len: %d", statementLen))
		}
		if database != "" {
			parts = append(parts, "db: "+database)
		}
		if datasourceID != "" {
			parts = append(parts, "datasource: "+datasourceID)
		}
		if len(parts) > 0 {
			return fmt.Sprintf("Explaining statement (%s)…", strings.Join(parts, ", "))
		}
		return "Explaining statement…"
	case "tool_execute_statement_start":
		database := fieldString("database")
		datasourceID := datasourceLabel(fieldString("datasourceId"))
		statementLen := fieldInt("statementLen")
		parts := make([]string, 0, 3)
		if statementLen > 0 {
			parts = append(parts, fmt.Sprintf("len: %d", statementLen))
		}
		if database != "" {
			parts = append(parts, "db: "+database)
		}
		if datasourceID != "" {
			parts = append(parts, "datasource: "+datasourceID)
		}
		if len(parts) > 0 {
			return fmt.Sprintf("Executing statement (%s)…", strings.Join(parts, ", "))
		}
		return "Executing statement…"
	case "approval_pending":
		kind := fieldString("kind")
		if kind != "" {
			return fmt.Sprintf("Waiting for approval (%s)…", kind)
		}
		return "Waiting for approval…"
	}

	return ""
}
