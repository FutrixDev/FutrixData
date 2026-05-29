package schemaprivacy

import (
	"strings"

	"futrixdata/platform/internal/datasource"
)

// SendSummary describes what is about to be sent. Callers populate it before
// invoking Gate so that — whether the call is allowed or denied — the audit
// log can record the actual scope of the egress.
type SendSummary struct {
	EntityCount      int
	FieldCount       int
	IncludesComments bool
	ProviderType     string
	Model            string
	AIConfigID       string
}

// Gate is the single decision/audit helper used by every controlled path.
// It centralizes:
//
//  1. reading the current per-datasource consent
//  2. recording an audit entry (allowed or denied)
//  3. returning ErrNotAllowed when the consent is not Allowed
//
// Concentrating the policy here means the gating story is "find the call to
// schemaprivacy.Gate" — adding a new schema-to-LLM path means adding one such
// call, not re-implementing the policy.
//
// audit may be nil; callers that don't have an audit store wired (tests,
// headless setups) get the gating semantics without the persistence side
// effect. Returning audit errors silently as nil is intentional: failing the
// call because we couldn't write a log line would degrade the user experience
// further than the hazard warrants. Audit *write* errors are out of band of
// the gate's job, which is to enforce consent.
func Gate(
	audit *AuditStore,
	ds datasource.DataSource,
	trigger TriggerSource,
	summary SendSummary,
) error {
	consent := ConsentOf(ds)
	entry := AuditEntry{
		DatasourceID:     strings.TrimSpace(ds.ID),
		DatasourceName:   strings.TrimSpace(ds.Name),
		DatasourceType:   string(ds.Type),
		TriggerSource:    trigger,
		EntityCount:      summary.EntityCount,
		FieldCount:       summary.FieldCount,
		IncludesComments: summary.IncludesComments,
		ProviderType:     strings.TrimSpace(summary.ProviderType),
		Model:            strings.TrimSpace(summary.Model),
		AIConfigID:       strings.TrimSpace(summary.AIConfigID),
	}
	if consent != ConsentAllowed {
		entry.Status = StatusDenied
		// ConsentUnset is the empty string in storage, but recording an empty
		// reason collapses to no value at all because Reason is omitempty.
		// Persist a stable sentinel so default-deny refusals stay
		// distinguishable from rows where reason was simply not set, and
		// remain greppable after consent flips later.
		entry.Reason = string(consent)
		if consent == ConsentUnset {
			entry.Reason = "unset"
		}
		_ = audit.Append(entry)
		return ErrNotAllowed
	}
	entry.Status = StatusAllowed
	_ = audit.Append(entry)
	return nil
}
