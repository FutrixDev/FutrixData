package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func newThreadEvent(kind string, payload map[string]any) threadEvent {
	return threadEvent{
		ID:        newThreadEventID(),
		Kind:      strings.TrimSpace(kind),
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}

func newThreadEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UTC().UnixNano())
}

func summarizeThreadApproval(approval *Approval) map[string]any {
	if approval == nil {
		return nil
	}
	payload := map[string]any{
		"id":      strings.TrimSpace(approval.ID),
		"kind":    strings.TrimSpace(string(approval.Kind)),
		"summary": strings.TrimSpace(approval.Summary),
	}
	if approval.Payload != nil {
		raw, err := json.Marshal(approval.Payload)
		if err == nil {
			var compact map[string]any
			if json.Unmarshal(raw, &compact) == nil {
				delete(compact, "rows")
				payload["payload"] = compact
			}
		}
	}
	return payload
}

func summarizeThreadConsoleResult(effect *ConsoleResultEffect) map[string]any {
	if effect == nil {
		return nil
	}
	payload := map[string]any{
		"toolName":     "execute_statement",
		"datasourceId": strings.TrimSpace(effect.DatasourceID),
		"database":     strings.TrimSpace(effect.Database),
		"statement":    strings.TrimSpace(effect.Statement),
		"rowCount":     effect.Result.RowCount,
		"hasMore":      effect.Result.HasMore,
		"elapsedMs":    effect.Result.ElapsedMs,
	}
	if strings.TrimSpace(effect.DatasourceType) != "" {
		payload["datasourceType"] = strings.TrimSpace(effect.DatasourceType)
	}
	if effect.Result.NextToken != "" {
		payload["nextTokenAvailable"] = true
	}
	if effect.Result.PrevToken != "" {
		payload["prevTokenAvailable"] = true
	}
	return payload
}

func summarizeThreadEventRecord(record threadEventRecord) string {
	switch record.Kind {
	case "user_message", "assistant_message":
		return strings.TrimSpace(stringPayload(record.Payload, "content", "text"))
	case "approval_pending":
		return strings.TrimSpace(stringPayload(record.Payload, "summary"))
	case "approval_decision":
		decision := strings.TrimSpace(stringPayload(record.Payload, "decision"))
		if decision == "" {
			return "approval decision"
		}
		return "approval " + decision
	case "tool_result_summary":
		statement := strings.TrimSpace(stringPayload(record.Payload, "statement"))
		rowCount := intPayload(record.Payload, "rowCount")
		if statement == "" {
			return fmt.Sprintf("tool result rows=%d", rowCount)
		}
		return fmt.Sprintf("%s (rows=%d)", statement, rowCount)
	case "working_context_updated":
		parts := []string{}
		if datasourceID := strings.TrimSpace(stringPayload(record.Payload, "datasourceId")); datasourceID != "" {
			parts = append(parts, "datasource="+datasourceID)
		}
		if entity := strings.TrimSpace(stringPayload(record.Payload, "entity", "name")); entity != "" {
			parts = append(parts, "entity="+entity)
		}
		if len(parts) == 0 {
			return "working context updated"
		}
		return "working context updated: " + strings.Join(parts, " | ")
	case "memory_saved":
		problem := strings.TrimSpace(stringPayload(record.Payload, "problem"))
		if problem == "" {
			return "memory saved"
		}
		return fmt.Sprintf("memory saved: %s", problem)
	case "memory_snapshot_seeded":
		summary := strings.TrimSpace(stringPayload(record.Payload, "summary", "content", "text"))
		if summary != "" {
			return summary
		}
		return "pinned memory carryover seeded"
	default:
		return strings.TrimSpace(stringPayload(record.Payload, "summary", "content", "text"))
	}
}

func stringPayload(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(stringArg(payload, key))
		if value != "" {
			return value
		}
	}
	return ""
}

func intPayload(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func boolPayload(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	value, ok := payload[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	if !ok {
		return false
	}
	return typed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
