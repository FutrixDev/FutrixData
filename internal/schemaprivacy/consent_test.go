package schemaprivacy

import (
	"errors"
	"fmt"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestNormalizeConsentDefaultsToUnset(t *testing.T) {
	cases := map[string]Consent{
		"":         ConsentUnset,
		"   ":      ConsentUnset,
		"true":     ConsentUnset,
		"false":    ConsentUnset,
		"unknown":  ConsentUnset,
		"allowed":  ConsentAllowed,
		"ALLOWED":  ConsentAllowed,
		" denied": ConsentDenied,
	}
	for input, want := range cases {
		if got := NormalizeConsent(input); got != want {
			t.Fatalf("NormalizeConsent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConsentFromOptionsUnsetWhenMissing(t *testing.T) {
	if got := ConsentFromOptions(nil); got != ConsentUnset {
		t.Fatalf("nil options should be Unset, got %q", got)
	}
	if got := ConsentFromOptions(map[string]any{}); got != ConsentUnset {
		t.Fatalf("empty options should be Unset, got %q", got)
	}
	if got := ConsentFromOptions(map[string]any{OptionKey: "allowed"}); got != ConsentAllowed {
		t.Fatalf("allowed mapping returned %q", got)
	}
	if got := ConsentFromOptions(map[string]any{OptionKey: "denied"}); got != ConsentDenied {
		t.Fatalf("denied mapping returned %q", got)
	}
	// Unrecognized strings fall back to Unset (default-deny)
	if got := ConsentFromOptions(map[string]any{OptionKey: "true"}); got != ConsentUnset {
		t.Fatalf("legacy truthy value should not be treated as allowed; got %q", got)
	}
}

func TestApplyConsentNilOptionsRoundTrip(t *testing.T) {
	out, changed := ApplyConsent(nil, ConsentUnset)
	if out != nil || changed {
		t.Fatalf("nil + Unset should stay nil; got %v changed=%v", out, changed)
	}

	out, changed = ApplyConsent(nil, ConsentAllowed)
	if !changed || out[OptionKey] != string(ConsentAllowed) {
		t.Fatalf("nil + Allowed should allocate map; got %v changed=%v", out, changed)
	}

	out, changed = ApplyConsent(out, ConsentAllowed)
	if changed {
		t.Fatalf("idempotent set should not report change")
	}

	out, changed = ApplyConsent(out, ConsentUnset)
	if !changed {
		t.Fatalf("clearing consent should report change")
	}
	if _, ok := out[OptionKey]; ok {
		t.Fatalf("clearing should remove key")
	}
}

func TestApplyConsentPreservesOtherOptions(t *testing.T) {
	opts := map[string]any{"trustLevel": "cautious", OptionKey: "allowed"}
	out, changed := ApplyConsent(opts, ConsentDenied)
	if !changed {
		t.Fatalf("change to denied should be detected")
	}
	if out["trustLevel"] != "cautious" {
		t.Fatalf("trust level should be preserved")
	}
	if out[OptionKey] != string(ConsentDenied) {
		t.Fatalf("consent not updated")
	}
}

func TestConsentOfReadsFromOptions(t *testing.T) {
	ds := datasource.DataSource{
		ID:      "ds1",
		Options: map[string]any{OptionKey: "allowed"},
	}
	if got := ConsentOf(ds); got != ConsentAllowed {
		t.Fatalf("ConsentOf returned %q", got)
	}
}

func TestIsNotAllowedDistinguishesError(t *testing.T) {
	if !IsNotAllowed(ErrNotAllowed) {
		t.Fatalf("ErrNotAllowed should match IsNotAllowed")
	}
	if IsNotAllowed(nil) {
		t.Fatalf("nil should not match")
	}
	// AI Chat tool callers wrap the refusal with %w when translating to a
	// user-facing tool result. Classification must survive that wrapping
	// so loggers don't promote refusals to "runtime error".
	wrapped := fmt.Errorf("describe_entity failed: %w", ErrNotAllowed)
	if !IsNotAllowed(wrapped) {
		t.Fatalf("wrapped ErrNotAllowed should still match IsNotAllowed")
	}
	doubleWrapped := fmt.Errorf("ai chat: %w", wrapped)
	if !IsNotAllowed(doubleWrapped) {
		t.Fatalf("double-wrapped ErrNotAllowed should still match IsNotAllowed")
	}
	if IsNotAllowed(errors.New("unrelated runtime failure")) {
		t.Fatalf("unrelated error must not be classified as a refusal")
	}
}
