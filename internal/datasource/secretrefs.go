package datasource

import (
	"fmt"
	"strings"

	"futrixdata/platform/internal/secrets"
)

// ValidateSecretRefs ensures every supplied secret reference is usable before a
// datasource is persisted: each non-empty ref must target a supported field path
// (the password, or an options.<path> key) and be fully resolvable
// (providerConfigId, key, and field all present). Empty entries are treated as
// "no reference" and ignored. Every create/update/test surface (Wails, HTTP,
// CLI) runs this so a partial ref can never be saved into a record that only
// fails later at connection time.
func ValidateSecretRefs(refs map[string]secrets.SecretRef) error {
	for fieldPath, ref := range refs {
		if ref.Empty() {
			continue
		}
		if !SupportedSecretFieldPath(fieldPath) {
			return fmt.Errorf("secret ref %q targets an unsupported field path", fieldPath)
		}
		if !ref.Resolvable() {
			return fmt.Errorf("secret ref %q is incomplete: providerConfigId, key, and field are required", fieldPath)
		}
	}
	return nil
}

// PruneSecretRefs drops empty (no-reference) entries so a payload such as
// {"password": {}} is never persisted. An empty ref passes ValidateSecretRefs
// (treated as "no reference") yet, if stored, makes RedactDatasource and the edit
// form behave as if an external secret exists while resolution skips it — leaving
// an unusable record. Returns nil when no real refs remain so the field stays
// absent rather than an empty map.
func PruneSecretRefs(refs map[string]secrets.SecretRef) map[string]secrets.SecretRef {
	if len(refs) == 0 {
		return refs
	}
	out := make(map[string]secrets.SecretRef, len(refs))
	for fieldPath, ref := range refs {
		if ref.Empty() {
			continue
		}
		out[fieldPath] = ref
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// supportedOptionSecretPaths is the closed set of options.<path> keys that may be
// delegated to a secret provider. It is intentionally narrow: a path qualifies only
// when it (a) carries actual secret material and (b) is recognized by every
// required-field validator and the edit form as ref-satisfiable. That second
// property is what makes a ref-backed datasource round-trip: ClearInlineSecretsForRefs
// strips the inline value on save, so any required field whose ref the validators do
// NOT honor would pass once and then fail every later normal save/test, leaving the
// record uneditable. Non-secret identifiers (accountId, databaseId, region, host, …)
// are deliberately excluded — they are not secrets and have no ref-aware validator.
// Keep this in lockstep with the HasResolvableOptionRef call sites in
// internal/datasource/handlers.go, internal/datasourceops/helpers.go, app_datasource.go,
// and the frontend datasource form's hasResolvableOptionRef checks.
var supportedOptionSecretPaths = map[string]struct{}{
	"options.uri":                         {},
	"options.apiToken":                    {},
	"options.credentials.accessKeyId":     {},
	"options.credentials.secretAccessKey": {},
	"options.credentials.sessionToken":    {},
}

// SupportedSecretFieldPath reports whether a datasource field can be backed by a
// secret reference: the literal "password", or one of the allowlisted
// options.<path> secret keys. The exact-match allowlist also rejects malformed
// variants for free — a padded or empty-segment path ("options.uri ",
// "options..uri") is simply absent from the set, so it can never pass validation
// only to silently misroute the secret to a key adapters never read at resolution.
func SupportedSecretFieldPath(fieldPath string) bool {
	// Require the exact "password" key. RedactDatasource, the edit-form fill, and the
	// manual-switch restore all key off the literal "password"; a padded variant like
	// " password " would resolve at execution time yet never be shown or restored as
	// an existing-secret password, leaving manual edits silently shadowed.
	if fieldPath == "password" {
		return true
	}
	_, ok := supportedOptionSecretPaths[fieldPath]
	return ok
}

// HasRealSecretRefs reports whether refs holds at least one non-empty reference.
// Empty placeholder entries are "no reference" and do not count. Resolution-binding
// callers use this to decide whether a connection would touch an external secret
// (and therefore must be bound to the stored datasource that owns the ref) rather
// than resolved against a caller-supplied target.
func HasRealSecretRefs(refs map[string]secrets.SecretRef) bool {
	for _, ref := range refs {
		if !ref.Empty() {
			return true
		}
	}
	return false
}

// InlineOptionURIWillBeStripped reports whether ClearInlineSecretsForRefs will
// delete an inline options.uri on save because a password reference shadows it.
// When true, required-field validation must NOT let an inline options.uri count
// as connection addressing: the externalization step removes that URI on save, so
// the persisted record would carry neither a URI nor host/port and could never
// connect. Mirror the strip condition in ClearInlineSecretsForRefs exactly (a
// non-empty password ref), so the validator and the persistence step agree.
func InlineOptionURIWillBeStripped(refs map[string]secrets.SecretRef) bool {
	ref, ok := refs["password"]
	return ok && !ref.Empty()
}

// HasResolvableOptionURIRef reports whether the connection URI is delegated to a
// secret provider via a complete options.uri reference. When it is, the
// plaintext URI/host/port is absent by design (resolved read-only at execution
// time), so required-field validation must treat the connection shape as
// satisfied rather than rejecting the direct-URL secret flow.
func HasResolvableOptionURIRef(refs map[string]secrets.SecretRef) bool {
	return HasResolvableOptionRef(refs, "options.uri")
}

// HasResolvableOptionRef reports whether the given options field path is
// delegated to a secret provider via a complete reference. Like the URI case,
// the plaintext value is absent by design (resolved read-only at execution
// time), so required-field validation must treat it as satisfied.
func HasResolvableOptionRef(refs map[string]secrets.SecretRef, fieldPath string) bool {
	ref, ok := refs[fieldPath]
	return ok && ref.Resolvable()
}

// ClearInlineSecretsForRefs enforces the reference-only contract on persisted
// records: for every non-empty SecretRef the record carries, the matching inline
// value is stripped so only the reference is saved, never stale plaintext beside
// it (which runtime resolution would shadow while leaving a usable local secret).
// It mirrors the resolver's field addressing — the password, or an options.<path>
// key — and never mutates the input (Options is cloned before any deletion).
//
// This is the shared implementation behind datasourcesecrets.Manager.
// ExternalizeDatasourceSecrets (Wails/service) and the direct HTTP datasource
// handler, which persists records without going through the secrets manager. Both
// surfaces must clear inline secrets identically so a payload carrying both a
// field and its ref behaves the same on every create/update path.
func ClearInlineSecretsForRefs(ds DataSource) DataSource {
	if len(ds.SecretRefs) == 0 {
		return ds
	}
	next := ds
	next.Options = cloneOptionsMap(ds.Options)
	for fieldPath, ref := range ds.SecretRefs {
		if ref.Empty() {
			continue
		}
		clearInlineFieldPath(&next, fieldPath)
	}
	// A password reference and an inline connection URI both carry the credential, and
	// SQL/Mongo adapters prefer options.uri over DataSource.Password. So an inline URI
	// left beside a password ref would shadow it: the Vault password is never used and
	// the URI's stale plaintext (often with embedded credentials) stays in the stored
	// record. Clearing only the password field is not enough — strip the conflicting URI
	// too so the ref-assembled credential is the only one. A URI that is itself delegated
	// to a provider uses secretRefs["options.uri"] and is cleared by the loop above.
	if ref, ok := ds.SecretRefs["password"]; ok && !ref.Empty() && next.Options != nil {
		delete(next.Options, "uri")
	}
	return next
}

func clearInlineFieldPath(ds *DataSource, fieldPath string) {
	switch strings.TrimSpace(fieldPath) {
	case "password":
		ds.Password = ""
		return
	}
	if !strings.HasPrefix(fieldPath, "options.") || ds.Options == nil {
		return
	}
	deleteOptionsPath(ds.Options, strings.Split(strings.TrimPrefix(fieldPath, "options."), "."))
}

func deleteOptionsPath(input map[string]any, parts []string) {
	current := input
	for i, part := range parts {
		if i == len(parts)-1 {
			delete(current, part)
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
}

func cloneOptionsMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		switch typed := v.(type) {
		case map[string]any:
			out[k] = cloneOptionsMap(typed)
		default:
			out[k] = typed
		}
	}
	return out
}
