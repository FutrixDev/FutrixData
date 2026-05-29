package datasource

import (
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"futrixdata/platform/internal/secrets"
)

var dsnUserInfoPattern = regexp.MustCompile(`^([^:@/\s]+):([^@/\s]+)@`)

func RedactValue(value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		return redactSlice(typed)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	switch typed := decoded.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		return redactSlice(typed)
	default:
		return decoded
	}
}

func RedactDatasource(ds DataSource) DataSource {
	next := ds
	if strings.TrimSpace(next.Password) != "" {
		next.Password = "[REDACTED]"
	} else if ref, ok := next.SecretRefs["password"]; ok && !ref.Empty() {
		// Only mark the password as externally backed for a real reference. An
		// empty ref is "no reference" (PruneSecretRefs should drop it before save,
		// but stay defensive for legacy records); redacting it would falsely show
		// existing-secret mode in the UI while resolution skips it.
		next.Password = "[REDACTED]"
	}
	if len(next.Options) > 0 {
		if redacted, ok := RedactValue(next.Options).(map[string]any); ok {
			next.Options = redacted
		}
	}
	return next
}

func RestoreRedactedDatasource(next, existing DataSource) DataSource {
	restored := next
	passwordRedacted := strings.TrimSpace(next.Password) == "[REDACTED]"
	// A non-empty, non-"[REDACTED]" password is a deliberate manual entry. It clears
	// a leftover empty (placeholder) password ref so the typed value is honored; an
	// empty password with a supplied ref is the existing-secret mode and must keep the
	// incoming reference. A real incoming ref always wins over inline plaintext below.
	manualPassword := !passwordRedacted && strings.TrimSpace(next.Password) != ""
	// The "[REDACTED]" marker means the client round-tripped the stored record
	// without changing the secret, so we restore the password secret ref it could
	// not see — per key, so a partial update that carries other refs does not
	// silently drop the password reference.
	nextPasswordRef, nextHasPasswordRef := restored.SecretRefs["password"]
	nextHasRealPasswordRef := nextHasPasswordRef && !nextPasswordRef.Empty()
	existingPasswordRef, existingHasPasswordRef := existing.SecretRefs["password"]
	if passwordRedacted && !nextHasPasswordRef && existingHasPasswordRef {
		restored.SecretRefs = cloneSecretRefs(restored.SecretRefs)
		restored.SecretRefs["password"] = existingPasswordRef
	} else if manualPassword && nextHasPasswordRef && !nextHasRealPasswordRef {
		// A manual password supplied next to only an empty (placeholder) ref means
		// "switch to inline": drop the no-op ref so the typed value is honored. A real
		// incoming ref is NOT dropped — the reference always wins (handled below),
		// matching the create/externalize contract so update behaves identically.
		restored.SecretRefs = cloneSecretRefs(restored.SecretRefs)
		delete(restored.SecretRefs, "password")
	}
	switch {
	case nextHasRealPasswordRef:
		// A real password ref governs the credential regardless of whether the inline
		// value was the "[REDACTED]" marker or a freshly typed password. Never persist
		// plaintext beside the reference — leaving both would keep a stale local secret
		// that shadows (and later overwrites) the external one. This mirrors the create
		// path, where ExternalizeDatasourceSecrets clears the inline value for any
		// supplied ref, so the same payload behaves identically on create and update.
		restored.Password = ""
	case passwordRedacted && strings.TrimSpace(existing.Password) != "":
		restored.Password = existing.Password
	case passwordRedacted:
		if _, ok := restored.SecretRefs["password"]; ok {
			restored.Password = ""
		}
	default:
		if value, ok := restoreRedactedValue(next.Password, existing.Password).(string); ok {
			restored.Password = value
		}
	}
	if len(next.Options) > 0 || len(existing.Options) > 0 {
		if value, ok := restoreRedactedValue(next.Options, existing.Options).(map[string]any); ok {
			restored.Options = value
		}
	}
	return restored
}

func cloneSecretRefs(input map[string]secrets.SecretRef) map[string]secrets.SecretRef {
	out := make(map[string]secrets.SecretRef, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func redactMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		if sensitiveField(key) {
			out[key] = "[REDACTED]"
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			out[key] = redactMap(typed)
		case []any:
			out[key] = redactSlice(typed)
		case string:
			out[key] = redactStringValue(key, typed)
		default:
			out[key] = typed
		}
	}
	return out
}

func redactSlice(input []any) []any {
	out := make([]any, len(input))
	for i, value := range input {
		switch typed := value.(type) {
		case map[string]any:
			out[i] = redactMap(typed)
		case []any:
			out[i] = redactSlice(typed)
		default:
			out[i] = typed
		}
	}
	return out
}

// restoreRedactedField reverses redactMap's key-contextual redaction for a single
// map entry. redactMap wholesale-replaces sensitive scalar fields (api_token,
// secret_access_key, ...) with the "[REDACTED]" marker and partially rewrites
// connection-string fields (uri/dsn/...). The generic restoreRedactedValue only
// compares against RedactValue, which does not re-apply that key context, so once
// a sibling key changes (breaking the whole-map fast path) a redacted scalar would
// be persisted as the literal marker — silently destroying the stored credential.
// Restoring per key keeps option secrets intact while honoring deliberate edits.
func restoreRedactedField(key string, next, existing any) any {
	if sensitiveField(key) {
		if s, ok := next.(string); ok && s == "[REDACTED]" && existing != nil {
			return existing
		}
		return next
	}
	if ns, ok := next.(string); ok && connectionStringField(key) {
		es, _ := existing.(string)
		// Unchanged redacted value: the client round-tripped the stored secret
		// without touching it, so restore the real connection string verbatim.
		if es != "" && ns == redactStringValue(key, es) {
			return es
		}
		// A partial edit (e.g. the host or database changed) leaves the "[REDACTED]"
		// marker sitting in the credential position. Persisting it would overwrite
		// the real secret with the literal marker and break the datasource, so splice
		// the stored secret back into the edited string instead.
		if strings.Contains(ns, redactionMarker) {
			return mergeRedactedConnectionString(ns, es)
		}
		return next
	}
	return restoreRedactedValue(next, existing)
}

const (
	redactionMarker = "[REDACTED]"
	// redactionToken is a parse-safe stand-in for redactionMarker. url.Parse rejects
	// the bracketed marker in userinfo/query, so we swap it before re-parsing an
	// edited connection string, then splice the stored secret over the token.
	redactionToken = "FUTRIXREDACTEDSECRET"
)

// mergeRedactedConnectionString re-injects the stored secret into an edited
// connection string whose secret portions still carry the "[REDACTED]" marker.
// It honors the user's non-secret edits (host, database, options) while keeping the
// credential intact. When the secret cannot be restored (no usable stored value),
// it falls back to the last known-good value rather than persisting the marker.
func mergeRedactedConnectionString(next, existing string) string {
	if strings.TrimSpace(existing) == "" {
		return next
	}
	if merged, ok := mergeRedactedURI(next, existing); ok {
		return merged
	}
	if merged, ok := mergeRedactedDSN(next, existing); ok {
		return merged
	}
	return existing
}

func mergeRedactedURI(next, existing string) (string, bool) {
	nextParsed, err := url.Parse(strings.TrimSpace(strings.ReplaceAll(next, redactionMarker, redactionToken)))
	if err != nil || strings.TrimSpace(nextParsed.Scheme) == "" {
		return "", false
	}
	existingParsed, err := url.Parse(strings.TrimSpace(existing))
	if err != nil {
		return "", false
	}
	if nextParsed.User != nil {
		if pw, ok := nextParsed.User.Password(); ok && pw == redactionToken {
			if existingParsed.User != nil {
				if epw, eok := existingParsed.User.Password(); eok {
					nextParsed.User = url.UserPassword(nextParsed.User.Username(), epw)
				}
			}
		}
	}
	nextQuery := nextParsed.Query()
	existingQuery := existingParsed.Query()
	for key, values := range nextQuery {
		if !sensitiveField(key) {
			continue
		}
		existingValues := existingQuery[key]
		for i := range values {
			if values[i] == redactionToken && i < len(existingValues) {
				values[i] = existingValues[i]
			}
		}
		nextQuery[key] = values
	}
	nextParsed.RawQuery = nextQuery.Encode()
	out := nextParsed.String()
	// A leftover token means the stored value lacked the corresponding secret, so we
	// could not fully restore — signal failure so the caller keeps the stored value.
	if strings.Contains(out, redactionToken) || strings.Contains(out, redactionMarker) {
		return "", false
	}
	return out, true
}

func mergeRedactedDSN(next, existing string) (string, bool) {
	if strings.Contains(next, "://") {
		return "", false
	}
	merged := next
	if dsnUserInfoPattern.MatchString(existing) {
		existingPassword := dsnUserInfoPattern.FindStringSubmatch(existing)[2]
		merged = dsnUserInfoPattern.ReplaceAllStringFunc(merged, func(match string) string {
			sub := dsnUserInfoPattern.FindStringSubmatch(match)
			if sub == nil || sub[2] != redactionMarker {
				return match
			}
			return sub[1] + ":" + existingPassword + "@"
		})
	}
	if base, rawQuery, hasQuery := strings.Cut(merged, "?"); hasQuery {
		if restored, ok := mergeRedactedDSNQuery(rawQuery, existing); ok {
			merged = base + "?" + restored
		}
	}
	if strings.Contains(merged, redactionMarker) {
		return "", false
	}
	return merged, true
}

func mergeRedactedDSNQuery(rawQuery, existing string) (string, bool) {
	nextQuery, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false
	}
	_, existingRaw, hasExisting := strings.Cut(existing, "?")
	if !hasExisting {
		return "", false
	}
	existingQuery, err := url.ParseQuery(existingRaw)
	if err != nil {
		return "", false
	}
	for key, values := range nextQuery {
		if !sensitiveField(key) {
			continue
		}
		existingValues := existingQuery[key]
		for i := range values {
			if values[i] == redactionMarker && i < len(existingValues) {
				values[i] = existingValues[i]
			}
		}
		nextQuery[key] = values
	}
	return nextQuery.Encode(), true
}

func restoreRedactedValue(next, existing any) any {
	if next == nil || existing == nil {
		return next
	}
	if reflect.DeepEqual(next, RedactValue(existing)) {
		return existing
	}
	switch typed := next.(type) {
	case map[string]any:
		existingMap, ok := existing.(map[string]any)
		if !ok {
			return next
		}
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = restoreRedactedField(key, value, existingMap[key])
		}
		return out
	case []any:
		existingSlice, ok := existing.([]any)
		if !ok || len(typed) != len(existingSlice) {
			return next
		}
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = restoreRedactedValue(typed[i], existingSlice[i])
		}
		return out
	default:
		return next
	}
}

func sensitiveField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "passwd", "api_token", "apitoken", "token", "access_token", "accesstoken", "session_token", "sessiontoken", "secret_access_key", "secretaccesskey":
		return true
	default:
		return false
	}
}

func redactStringValue(key, value string) string {
	if !connectionStringField(key) {
		return value
	}
	if redacted, ok := redactURIString(value); ok {
		return redacted
	}
	if redacted, ok := redactDSNString(value); ok {
		return redacted
	}
	return value
}

func connectionStringField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "uri", "url", "dsn", "databaseurl", "database_url", "connectionstring", "connection_string":
		return true
	default:
		return false
	}
}

func redactURIString(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" {
		return value, false
	}
	changed := false
	if parsed.User != nil {
		username := parsed.User.Username()
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(username, "[REDACTED]")
			changed = true
		}
	}
	query := parsed.Query()
	queryChanged := false
	for key, values := range query {
		if !sensitiveField(key) {
			continue
		}
		for i := range values {
			values[i] = "[REDACTED]"
		}
		query[key] = values
		queryChanged = true
	}
	if queryChanged {
		parsed.RawQuery = query.Encode()
		changed = true
	}
	if !changed {
		return value, false
	}
	redacted := parsed.String()
	redacted = strings.ReplaceAll(redacted, "%5BREDACTED%5D", "[REDACTED]")
	return redacted, true
}

func redactDSNString(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "://") {
		return value, false
	}
	base, rawQuery, hasQuery := strings.Cut(trimmed, "?")
	changed := false
	if dsnUserInfoPattern.MatchString(base) {
		base = dsnUserInfoPattern.ReplaceAllString(base, `$1:[REDACTED]@`)
		changed = true
	}
	if hasQuery {
		query, err := url.ParseQuery(rawQuery)
		if err == nil {
			queryChanged := false
			for key, values := range query {
				if !sensitiveField(key) {
					continue
				}
				for i := range values {
					values[i] = "[REDACTED]"
				}
				query[key] = values
				queryChanged = true
			}
			if queryChanged {
				rawQuery = strings.ReplaceAll(query.Encode(), "%5BREDACTED%5D", "[REDACTED]")
				changed = true
			}
		}
	}
	if !changed {
		return value, false
	}
	if hasQuery {
		return base + "?" + rawQuery, true
	}
	return base, true
}
