package planlimits

import (
	"strings"
	"time"
)

// Effective status codes returned by EvaluateLicense. These are stable strings
// the frontend matches on to render plan/status copy.
const (
	StatusActive     = "active"
	StatusFree       = "free"
	StatusProExpired = "pro_expired"
	StatusTrial      = "trial"
)

// Entitlement represents the user's effective plan/status after reconciling
// the raw stored license fields with the current time. The "Raw" fields keep
// the historical/server values; the "Effective" fields are the source of truth
// for product behavior and UI display.
type Entitlement struct {
	RawPlan         string
	RawStatus       string
	ExpiresAt       int64
	TrialExpiresAt  int64
	EffectivePlan   string
	EffectiveStatus string
}

// proExpiredRawStatus enumerates the raw status strings the backend session
// may carry to mean "Pro entitlement no longer active". Must stay in sync
// with frontend/src/modules/plan/limits.ts PRO_EXPIRED_RAW_STATUS — both
// layers need to recognize the same set or backend gates and the UI will
// disagree (e.g. status=pro_expired with future expiresAt would let backend
// allow Pro-only operations while the UI shows expired).
var proExpiredRawStatus = map[string]struct{}{
	"expired":     {},
	"pro_expired": {},
}

// EvaluateLicense converts a stored license tuple plus the current time into
// an Entitlement. The rules are:
//
//   - rawPlan == "pro" and rawStatus in {"expired","pro_expired"} → effective Free, status pro_expired
//   - rawPlan == "pro" and expiresAt > 0 and expiresAt <= now → effective Free, status pro_expired
//   - rawPlan == "pro" otherwise → effective Pro, status active
//   - anything else → effective Free, status free
//
// Passing the zero time falls back to time.Now() so callers without a clock
// can still resolve expiry deterministically.
func EvaluateLicense(plan, status string, expiresAt int64, now time.Time) Entitlement {
	if now.IsZero() {
		now = time.Now()
	}
	rawPlan := strings.ToLower(strings.TrimSpace(plan))
	rawStatus := strings.ToLower(strings.TrimSpace(status))

	ent := Entitlement{
		RawPlan:         rawPlan,
		RawStatus:       rawStatus,
		ExpiresAt:       expiresAt,
		EffectivePlan:   PlanFree,
		EffectiveStatus: StatusFree,
	}

	if rawPlan != PlanPro {
		return ent
	}

	_, expired := proExpiredRawStatus[rawStatus]
	if !expired && expiresAt > 0 && expiresAt <= now.Unix() {
		expired = true
	}
	if expired {
		ent.EffectivePlan = PlanFree
		ent.EffectiveStatus = StatusProExpired
		return ent
	}
	ent.EffectivePlan = PlanPro
	ent.EffectiveStatus = StatusActive
	return ent
}

func EvaluateLicenseWithTrial(plan, status string, expiresAt, trialExpiresAt int64, now time.Time) Entitlement {
	ent := EvaluateLicense(plan, status, expiresAt, now)
	ent.TrialExpiresAt = trialExpiresAt
	if ent.EffectivePlan == PlanPro {
		return ent
	}
	if TrialActive(trialExpiresAt, now) {
		ent.EffectivePlan = PlanPro
		ent.EffectiveStatus = StatusTrial
	}
	return ent
}

func TrialActive(expiresAt int64, now time.Time) bool {
	if expiresAt <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	return expiresAt > now.Unix()
}

// EffectivePlan is a convenience wrapper that returns only the effective plan
// string. Use Evaluate for callers that also need expired-context.
func EffectivePlan(plan, status string, expiresAt int64, now time.Time) string {
	return EvaluateLicense(plan, status, expiresAt, now).EffectivePlan
}

func EffectivePlanWithTrial(plan, status string, expiresAt, trialExpiresAt int64, now time.Time) string {
	return EvaluateLicenseWithTrial(plan, status, expiresAt, trialExpiresAt, now).EffectivePlan
}
