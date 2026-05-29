package datasourcesecrets

import (
	"context"
	"fmt"
	"strings"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

const (
	ScopeDatasource = "datasource"
)

type Manager struct {
	registry *secrets.Registry
}

func NewManager(registry *secrets.Registry) *Manager {
	if registry == nil {
		return nil
	}
	return &Manager{registry: registry}
}

// ResolveDatasource fills in any field that the stored record delegates to an
// external secret provider via SecretRefs, returning a datasource with live
// credentials for the duration of an execution. Resolution is read-only: it
// never creates, rotates, or deletes provider secrets.
func (m *Manager) ResolveDatasource(ctx context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	if m == nil || m.registry == nil || len(ds.SecretRefs) == 0 {
		return ds, nil
	}
	next := ds
	next.Options = cloneOptions(ds.Options)
	for fieldPath, ref := range ds.SecretRefs {
		if ref.Empty() {
			continue
		}
		value, err := m.registry.Resolve(ctx, ref)
		if err != nil {
			return datasource.DataSource{}, fmt.Errorf("resolve datasource secret %s: %w", fieldPath, err)
		}
		if err := writeFieldPath(&next, fieldPath, value.Plaintext); err != nil {
			return datasource.DataSource{}, fmt.Errorf("resolve datasource secret %s: %w", fieldPath, err)
		}
	}
	return next, nil
}

// ExternalizeDatasourceSecrets enforces the reference-only contract on the
// create/update paths. FutrixData references externally owned secrets read-only;
// it never creates, rotates, or deletes provider material, so a manual password
// stays inline (stored locally, encrypted at rest like any non-referenced
// datasource) and user-supplied SecretRefs are persisted as-is for
// ResolveDatasource to read at execution time.
//
// The one thing it must not do is persist plaintext alongside a reference for the
// same field. The UI clears the inline value when a ref is supplied, but a non-UI
// caller (agent/MCP/CLI/HTTP) can submit both `password` and
// `secretRefs["password"]` (or `options.apiToken` and its ref); runtime resolution
// would prefer the ref while the stale plaintext lingered in the datasource store.
// It delegates to datasource.ClearInlineSecretsForRefs — shared with the direct
// HTTP handler — so every persistence surface strips inline secrets identically.
func (m *Manager) ExternalizeDatasourceSecrets(_ context.Context, ds datasource.DataSource) (datasource.DataSource, error) {
	return datasource.ClearInlineSecretsForRefs(ds), nil
}

func (m *Manager) HealthCheckDefault(ctx context.Context) error {
	if m == nil || m.registry == nil {
		return nil
	}
	defaultID := m.registry.DefaultProviderConfigID()
	if strings.TrimSpace(defaultID) == "" {
		return nil
	}
	return m.registry.HealthCheck(ctx, defaultID)
}

func writeFieldPath(ds *datasource.DataSource, fieldPath, value string) error {
	switch strings.TrimSpace(fieldPath) {
	case "password":
		ds.Password = value
		return nil
	}
	if !strings.HasPrefix(fieldPath, "options.") {
		return fmt.Errorf("unsupported field path %q", fieldPath)
	}
	if ds.Options == nil {
		ds.Options = map[string]any{}
	}
	writeMapPath(ds.Options, strings.Split(strings.TrimPrefix(fieldPath, "options."), "."), value)
	return nil
}

func writeMapPath(input map[string]any, parts []string, value string) {
	current := input
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func cloneOptions(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		switch typed := v.(type) {
		case map[string]any:
			out[k] = cloneOptions(typed)
		default:
			out[k] = typed
		}
	}
	return out
}
