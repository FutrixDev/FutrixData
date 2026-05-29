package secrets

import (
	"context"
	"errors"
	"strings"
)

const (
	ProviderVaultKVV2 = "vault-kv-v2"
)

var (
	ErrProviderNotFound    = errors.New("secret provider not found")
	ErrProviderUnavailable = errors.New("secret provider unavailable")
	ErrUnauthorized        = errors.New("secret provider unauthorized")
	ErrSecretNotFound      = errors.New("secret not found")
	ErrInvalidSecretRef    = errors.New("invalid secret reference")
	ErrUnsupported         = errors.New("secret provider operation unsupported")
)

type SecretRef struct {
	ProviderConfigID string `json:"providerConfigId"`
	Scope            string `json:"scope,omitempty"`
	ResourceID       string `json:"resourceId,omitempty"`
	Field            string `json:"field"`
	Key              string `json:"key"`
	Version          string `json:"version,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
}

// Empty reports a blank reference that callers may ignore. A ref carrying any of
// providerConfigId, key, or field is a deliberate (if partial) reference: it must
// fall through to Resolvable validation and be rejected when incomplete, not
// silently skipped. Omitting field here let a field-only ref (e.g. from a non-UI
// caller) pass validation and persist as an unusable record that redacts as an
// external secret yet never resolves.
func (r SecretRef) Empty() bool {
	return strings.TrimSpace(r.ProviderConfigID) == "" &&
		strings.TrimSpace(r.Key) == "" &&
		strings.TrimSpace(r.Field) == ""
}

// Resolvable reports whether the ref carries every field resolution needs: a
// provider to route to, a key to look up, and the field within that secret.
// Empty() (neither provider nor key) only screens out blank entries; validation
// that bypasses plaintext host/URI requirements must use Resolvable so a partial
// ref (e.g. provider but no key) is not accepted into an unusable datasource
// record that later fails at resolve time.
func (r SecretRef) Resolvable() bool {
	return strings.TrimSpace(r.ProviderConfigID) != "" &&
		strings.TrimSpace(r.Key) != "" &&
		strings.TrimSpace(r.Field) != ""
}

type SecretValue struct {
	Plaintext string
}

type ProviderCapabilities struct {
	Type              string `json:"type"`
	Versioning        bool   `json:"versioning"`
	Labels            bool   `json:"labels"`
	Rotation          bool   `json:"rotation"`
	BinaryPayloads    bool   `json:"binaryPayloads"`
	Metadata          bool   `json:"metadata"`
	ProviderSideAudit bool   `json:"providerSideAudit"`
	ValueSizeLimit    int64  `json:"valueSizeLimit,omitempty"`
	RequiresAgent     bool   `json:"requiresAgent"`
}

type Provider interface {
	Put(ctx context.Context, ref SecretRef, value SecretValue) (SecretRef, error)
	Resolve(ctx context.Context, ref SecretRef) (SecretValue, error)
	Delete(ctx context.Context, ref SecretRef) error
	Rotate(ctx context.Context, ref SecretRef) (SecretRef, error)
	HealthCheck(ctx context.Context) error
	Capabilities() ProviderCapabilities
}
