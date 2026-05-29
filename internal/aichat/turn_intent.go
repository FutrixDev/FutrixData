package aichat

import (
	"encoding/json"
	"strings"
)

const (
	turnIntentFocusAuto          = "auto"
	turnIntentFocusPreferCurrent = "prefer_current"
	turnIntentFocusAvoidCurrent  = "avoid_current"
)

func normalizeTurnIntentCurrentFocus(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.Join(strings.Fields(normalized), "_")

	switch normalized {
	case "":
		return ""
	case turnIntentFocusAuto:
		return turnIntentFocusAuto
	case turnIntentFocusPreferCurrent:
		return turnIntentFocusPreferCurrent
	case turnIntentFocusAvoidCurrent:
		return turnIntentFocusAvoidCurrent
	default:
		return turnIntentFocusAuto
	}
}

func cloneTurnIntent(value *TurnIntent) *TurnIntent {
	if value == nil {
		return nil
	}
	out := *value
	out.CurrentFocus = normalizeTurnIntentCurrentFocus(out.CurrentFocus)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.CurrentFocus == "" && out.Reason == "" && out.Confidence == 0 {
		return nil
	}
	return &out
}

func decodeTurnIntentFromAny(value any) *TurnIntent {
	if value == nil {
		return nil
	}
	if typed, ok := value.(*TurnIntent); ok && typed != nil {
		return cloneTurnIntent(typed)
	}
	if typed, ok := value.(TurnIntent); ok {
		return cloneTurnIntent(&typed)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out TurnIntent
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return cloneTurnIntent(&out)
}

func turnIntentPrefersCurrentFocus(req TurnRequest) bool {
	intent := cloneTurnIntent(req.Intent)
	return intent != nil && intent.CurrentFocus == turnIntentFocusPreferCurrent
}

func turnIntentAvoidsCurrentFocus(req TurnRequest) bool {
	intent := cloneTurnIntent(req.Intent)
	return intent != nil && intent.CurrentFocus == turnIntentFocusAvoidCurrent
}
