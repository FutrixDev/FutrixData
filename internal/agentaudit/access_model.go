package agentaudit

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// DatasourceScopeInheritUser is the default compatibility model: the
	// agent key is a sub-principal of the local user and can target the same
	// configured datasources, while still passing trust, risk, approval,
	// schema-egress, masking, and per-tool grant gates.
	DatasourceScopeInheritUser = "inherit_user"
	// DatasourceScopeAllowList restricts an agent identity to the listed
	// datasource ids. Runtime callers must enforce this before reading schema
	// or data from the target datasource.
	DatasourceScopeAllowList = "allowlist"
)

var (
	ErrAccessExpired       = errors.New("agent access expired")
	ErrDatasourceForbidden = errors.New("agent datasource forbidden")
)

func effectiveDatasourceScope(identity AgentIdentity) string {
	scope := strings.TrimSpace(identity.DatasourceScope)
	if scope == "" {
		return DatasourceScopeInheritUser
	}
	return scope
}

func NormalizeDatasourceScope(scope string) string {
	trimmed := strings.TrimSpace(scope)
	if trimmed == "" {
		return DatasourceScopeInheritUser
	}
	return trimmed
}

func NormalizeAllowedDatasourceIDs(ids []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

func UsesDatasourceAllowList(identity AgentIdentity) bool {
	return effectiveDatasourceScope(identity) == DatasourceScopeAllowList
}

func CheckDatasourceInventoryScope(identity AgentIdentity) error {
	if UsesDatasourceAllowList(identity) {
		return fmt.Errorf("%w: full datasource inventory is outside this agent key allowlist", ErrDatasourceForbidden)
	}
	return nil
}

func CheckDatasourceScope(identity AgentIdentity, datasourceID string) error {
	dsID := strings.TrimSpace(datasourceID)
	if dsID == "" {
		return nil
	}
	switch scope := effectiveDatasourceScope(identity); scope {
	case DatasourceScopeInheritUser:
		return nil
	case DatasourceScopeAllowList:
		for _, allowed := range identity.AllowedDatasourceIDs {
			if strings.TrimSpace(allowed) == dsID {
				return nil
			}
		}
		return fmt.Errorf("%w: datasource %s is outside this agent key allowlist", ErrDatasourceForbidden, dsID)
	default:
		return fmt.Errorf("%w: unsupported datasource scope %q", ErrDatasourceForbidden, scope)
	}
}

func CheckAccessExpiry(identity AgentIdentity, now time.Time) error {
	expiresAt := strings.TrimSpace(identity.ExpiresAt)
	if expiresAt == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return fmt.Errorf("agent access expiry is invalid: %w", err)
	}
	if !now.UTC().Before(parsed.UTC()) {
		return ErrAccessExpired
	}
	return nil
}
