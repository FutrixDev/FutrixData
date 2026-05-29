package schemaprivacy

import (
	"errors"
	"fmt"
	"strings"

	"futrixdata/platform/internal/datasource"
)

// NormalizeConsent coerces a raw string into one of the three Consent
// constants. Anything not recognized as "allowed" or "denied" — including
// empty, whitespace, or legacy true/false — falls back to ConsentUnset, which
// is the default-deny posture for this feature.
func NormalizeConsent(value string) Consent {
	switch Consent(strings.ToLower(strings.TrimSpace(value))) {
	case ConsentAllowed:
		return ConsentAllowed
	case ConsentDenied:
		return ConsentDenied
	default:
		return ConsentUnset
	}
}

// ConsentFromOptions reads the per-datasource consent from a raw options map.
// Exported so callers that only have an options map (update payloads, tests)
// can check without rebuilding a DataSource.
func ConsentFromOptions(opts map[string]any) Consent {
	if opts == nil {
		return ConsentUnset
	}
	raw, ok := opts[OptionKey]
	if !ok || raw == nil {
		return ConsentUnset
	}
	return NormalizeConsent(fmt.Sprint(raw))
}

// ConsentOf is the convenience wrapper used by gating code: "given this
// datasource value, may schema metadata be sent?". Callers typically check
// `ConsentOf(ds) == ConsentAllowed` and refuse otherwise.
func ConsentOf(ds datasource.DataSource) Consent {
	return ConsentFromOptions(ds.Options)
}

// ApplyConsent writes the consent value into an options map, returning the
// possibly-newly-allocated map and a boolean indicating whether anything
// changed. A nil options map is allocated when the consent is non-default;
// when consent is ConsentUnset on a nil map, we leave it nil to keep stored
// JSON minimal.
func ApplyConsent(opts map[string]any, consent Consent) (map[string]any, bool) {
	normalized := NormalizeConsent(string(consent))
	if opts == nil {
		if normalized == ConsentUnset {
			return nil, false
		}
		return map[string]any{OptionKey: string(normalized)}, true
	}
	if normalized == ConsentUnset {
		if _, ok := opts[OptionKey]; !ok {
			return opts, false
		}
		delete(opts, OptionKey)
		return opts, true
	}
	current, _ := opts[OptionKey].(string)
	if strings.ToLower(strings.TrimSpace(current)) == string(normalized) {
		return opts, false
	}
	opts[OptionKey] = string(normalized)
	return opts, true
}

// ErrNotAllowed is the canonical error returned when a controlled path is
// invoked while the datasource consent is not ConsentAllowed. Callers wrap or
// translate it; the message is human-readable in both Chinese and English so
// it can surface in tool results without further i18n.
var ErrNotAllowed = notAllowedError{}

type notAllowedError struct{}

func (notAllowedError) Error() string {
	return "schema_to_llm_not_allowed: this datasource has not been authorized to share schema metadata; open Data Sensitivity → AI Schema Access to allow it"
}

// IsNotAllowed reports whether an error is the schema-egress refusal. Useful
// at call sites that want to distinguish refusal (user surface) from genuine
// runtime errors (logging). Uses errors.As so the classification survives the
// fmt.Errorf("...: %w", ...) wrapping that callers do when translating the
// refusal into tool-result messaging.
func IsNotAllowed(err error) bool {
	if err == nil {
		return false
	}
	var target notAllowedError
	return errors.As(err, &target)
}
