package agentaudit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/toolreg"
)

var auditStoreCache sync.Map

// MaskAccessKey returns a display-safe form of an access key suitable for
// error messages and logs. A full key should never leak into surfaces that
// can be copied or screenshotted by untrusted readers; masking preserves
// enough entropy to correlate entries without exposing the secret.
func MaskAccessKey(accessKey string) string {
	trimmed := strings.TrimSpace(accessKey)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 10 {
		return "***"
	}
	return trimmed[:6] + "..." + trimmed[len(trimmed)-4:]
}

var (
	revokedAuditMu       sync.Mutex
	revokedAuditLastLog  = make(map[string]time.Time)
	revokedAuditThrottle = 60 * time.Second
)

// LogRevokedAccess appends an audit entry for a revoked-key attempt, rate
// limited per access key so a runaway agent cannot flood the audit log. The
// first attempt always writes; subsequent attempts within the throttle window
// are dropped. Callers should invoke this whenever CheckAccess returns
// ErrAccessRevoked (or after a post-execution recheck detects a mid-flight
// revoke).
//
// svc is intentionally dropped before reaching AppendToolCall: that path
// fetches the datasource by ID for row enrichment when svc != nil, and a
// revoked-key row must never trigger a service-side read. The audit entry
// loses datasource-name / -type enrichment as a result, but the rejection
// trail is preserved (access_key + tool + params still land). Callers keep
// the svc parameter in the signature for parity with the success-path
// helpers, and so future enrichment that doesn't require a service hit can
// be added without churning every call site.
func LogRevokedAccess(dataPath string, _ toolreg.Service, protocol, accessKey, toolName string, params map[string]any, message string) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return
	}

	revokedAuditMu.Lock()
	last, seen := revokedAuditLastLog[trimmedKey]
	now := time.Now()
	if seen && now.Sub(last) < revokedAuditThrottle {
		revokedAuditMu.Unlock()
		return
	}
	revokedAuditLastLog[trimmedKey] = now
	revokedAuditMu.Unlock()

	_ = AppendToolCall(dataPath, nil, protocol, accessKey, toolName, params, StatusError, message)
}

// ErrAccessRevoked is returned from CheckAccess when an identity has been
// revoked by the user. Callers should short-circuit tool execution and surface
// the revocation to the agent rather than running the call.
var ErrAccessRevoked = errors.New("agent access revoked")

// CheckAccess validates that the access key resolves to an active agent
// identity. It returns (identity, nil) when the key is valid, or an error when
// the key is unknown, empty, or revoked. Callers should invoke this before
// executing a tool on behalf of an agent; a revocation-triggered audit entry
// should be appended separately via AppendToolCall with StatusError.
func CheckAccess(dataPath, accessKey string) (AgentIdentity, error) {
	return CheckAccessAt(dataPath, accessKey, time.Now())
}

func CheckAccessAt(dataPath, accessKey string, now time.Time) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	if strings.TrimSpace(dataPath) == "" {
		return AgentIdentity{}, fmt.Errorf("data path is required")
	}
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, ok, err := identityStore.Get(trimmedKey)
	if err != nil {
		return AgentIdentity{}, err
	}
	if !ok {
		return AgentIdentity{}, fmt.Errorf("%w for access key %s", errIdentityNotFound, MaskAccessKey(trimmedKey))
	}
	if strings.TrimSpace(identity.RevokedAt) != "" {
		return identity, ErrAccessRevoked
	}
	if err := CheckAccessExpiry(identity, now); err != nil {
		return identity, err
	}
	return identity, nil
}

// AppendToolCall is a thin wrapper over AppendToolCallWithAttribution for the
// success path and other call sites that have no risk attribution to record.
// New code paths that *do* have a matched rule (approval-required gate, block
// error) should call AppendToolCallWithAttribution directly so the rule is
// preserved in the audit log.
func AppendToolCall(dataPath string, svc toolreg.Service, protocol, accessKey, toolName string, params map[string]any, status, message string) error {
	return AppendToolCallWithAttribution(dataPath, svc, protocol, accessKey, toolName, params, status, message, nil)
}

// AppendToolCallWithAttribution writes an audit entry that may carry a
// structured RiskAttribution. The attribution argument is preserved verbatim
// (only TrimSpace on the string fields would risk discarding intentional
// formatting) and is omitted from the JSON when nil.
func AppendToolCallWithAttribution(
	dataPath string,
	svc toolreg.Service,
	protocol, accessKey, toolName string,
	params map[string]any,
	status, message string,
	attribution *RiskAttribution,
) error {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" || strings.TrimSpace(dataPath) == "" {
		return nil
	}
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, ok, err := identityStore.Get(trimmedKey); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%w for access key %s", errIdentityNotFound, MaskAccessKey(trimmedKey))
	}

	entry := AuditEntry{
		AccessKey:       trimmedKey,
		Protocol:        strings.TrimSpace(protocol),
		ToolName:        strings.TrimSpace(toolName),
		Summary:         BuildSummary(toolName, params),
		Statement:       BuildStatement(params),
		Status:          strings.TrimSpace(status),
		Message:         strings.TrimSpace(message),
		Target:          PrimaryTarget(toolName, params),
		RiskAttribution: attribution,
	}
	if dsID := toolreg.DatasourceIDFromParams(params); dsID != "" {
		entry.DatasourceID = dsID
		if svc != nil {
			if ds, err := svc.GetDatasource(context.Background(), dsID); err == nil {
				applyDatasourceMeta(&entry, ds)
			}
		}
	}
	return sharedAuditStore(bootstrap.AgentAuditPath(dataPath)).Append(entry)
}

func sharedAuditStore(path string) *AuditStore {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return NewAuditStore(path)
	}
	if store, ok := auditStoreCache.Load(trimmedPath); ok {
		if auditStore, ok := store.(*AuditStore); ok {
			return auditStore
		}
	}
	store := NewAuditStore(trimmedPath)
	actual, _ := auditStoreCache.LoadOrStore(trimmedPath, store)
	auditStore, _ := actual.(*AuditStore)
	if auditStore == nil {
		return store
	}
	return auditStore
}

func applyDatasourceMeta(entry *AuditEntry, ds datasource.DataSource) {
	if entry == nil {
		return
	}
	entry.DatasourceName = strings.TrimSpace(ds.Name)
	entry.DatasourceType = string(ds.Type)
}

func BuildStatement(params map[string]any) string {
	return stringValue(params, "statement")
}

func BuildSummary(toolName string, params map[string]any) string {
	switch strings.TrimSpace(toolName) {
	case "execute_statement":
		statement := firstLine(BuildStatement(params))
		if statement != "" {
			return statement
		}
	case "execute_redis_batch":
		if batchID := stringValue(params, "batchId"); batchID != "" {
			return "redis batch " + batchID
		}
		statement := firstLine(BuildStatement(params))
		if statement != "" {
			return statement
		}
	case "describe_entity":
		if name := stringValue(params, "name", "entity"); name != "" {
			return "describe " + name
		}
	case "list_entities":
		if database := stringValue(params, "database"); database != "" {
			return "list entities in " + database
		}
		return "list entities"
	case "get_schema_knowledge":
		if entity := stringValue(params, "entity", "name"); entity != "" {
			return "schema knowledge for " + entity
		}
	case "get_er_knowledge":
		if database := stringValue(params, "database"); database != "" {
			return "ER knowledge for " + database
		}
	case "save_sensitivity_report":
		return "save sensitivity report"
	case "get_sensitivity_report":
		return "get sensitivity report"
	}
	return strings.TrimSpace(toolName)
}

func PrimaryTarget(toolName string, params map[string]any) string {
	switch strings.TrimSpace(toolName) {
	case "describe_entity", "get_schema_knowledge":
		return stringValue(params, "entity", "name")
	case "list_entities", "get_er_knowledge":
		return stringValue(params, "database")
	case "execute_statement":
		return firstTargetFromStatement(BuildStatement(params))
	case "execute_redis_batch":
		if batchID := stringValue(params, "batchId"); batchID != "" {
			return batchID
		}
		return firstTargetFromStatement(BuildStatement(params))
	default:
		return stringValue(params, "datasourceId", "id", "name")
	}
}

func firstTargetFromStatement(statement string) string {
	line := firstLine(statement)
	if line == "" {
		return ""
	}
	if len(line) > 120 {
		return line[:120] + "..."
	}
	return line
}

func firstLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func stringValue(p map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := p[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}
