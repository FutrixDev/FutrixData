package aichat

import "testing"

func TestCloneTurnIntent_ClonesAndNormalizes(t *testing.T) {
	input := &TurnIntent{
		CurrentFocus: " prefer_current ",
		Reason:       "  stay on the current datasource  ",
		Confidence:   0.91,
	}

	got := cloneTurnIntent(input)
	if got == nil {
		t.Fatalf("expected cloned intent")
	}
	if got == input {
		t.Fatalf("expected cloneTurnIntent to return a distinct pointer")
	}
	if got.CurrentFocus != turnIntentFocusPreferCurrent {
		t.Fatalf("expected normalized current focus, got %#v", got)
	}
	if got.Reason != "stay on the current datasource" {
		t.Fatalf("expected trimmed reason, got %#v", got)
	}
	if got.Confidence != 0.91 {
		t.Fatalf("expected confidence preserved, got %#v", got)
	}
}

func TestDecodeTurnIntentFromAny_DecodesMapPayload(t *testing.T) {
	got := decodeTurnIntentFromAny(map[string]any{
		"currentFocus": "avoid_current",
		"reason":       "target is outside current page focus",
		"confidence":   0.88,
	})
	if got == nil {
		t.Fatalf("expected decoded intent")
	}
	if got.CurrentFocus != turnIntentFocusAvoidCurrent {
		t.Fatalf("expected avoid_current, got %#v", got)
	}
	if got.Reason != "target is outside current page focus" {
		t.Fatalf("expected reason preserved, got %#v", got)
	}
	if got.Confidence != 0.88 {
		t.Fatalf("expected confidence preserved, got %#v", got)
	}
}

func TestTurnIntentFocusPredicates(t *testing.T) {
	avoidReq := TurnRequest{
		Intent: &TurnIntent{CurrentFocus: turnIntentFocusAvoidCurrent},
	}
	if !turnIntentAvoidsCurrentFocus(avoidReq) {
		t.Fatalf("expected avoid_current intent to avoid current focus")
	}
	if turnIntentPrefersCurrentFocus(avoidReq) {
		t.Fatalf("expected avoid_current intent to not prefer current focus")
	}

	preferReq := TurnRequest{
		Intent: &TurnIntent{CurrentFocus: turnIntentFocusPreferCurrent},
	}
	if !turnIntentPrefersCurrentFocus(preferReq) {
		t.Fatalf("expected prefer_current intent to prefer current focus")
	}
	if turnIntentAvoidsCurrentFocus(preferReq) {
		t.Fatalf("expected prefer_current intent to not avoid current focus")
	}
}

func TestRememberIntent_PreservesExplicitAutoReset(t *testing.T) {
	runtime := &einoTurnRuntime{
		req: TurnRequest{
			Intent: &TurnIntent{
				CurrentFocus: turnIntentFocusAvoidCurrent,
				Confidence:   0.93,
			},
		},
	}

	runtime.rememberIntent(&TurnIntent{CurrentFocus: turnIntentFocusAuto})

	if runtime.req.Intent == nil {
		t.Fatalf("expected explicit auto intent to be preserved")
	}
	if runtime.req.Intent.CurrentFocus != turnIntentFocusAuto {
		t.Fatalf("expected explicit auto reset, got %#v", runtime.req.Intent)
	}
}
