package agentaudit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"futrixdata/platform/internal/securefile"
)

const (
	SourceDetected = "detected"
	SourceManual   = "manual"
)

type IdentityStore struct {
	path string
	mu   sync.Mutex
}

func NewIdentityStore(path string) *IdentityStore {
	return &IdentityStore{path: path}
}

func (s *IdentityStore) CreateManual(name string) (AgentIdentity, error) {
	var identity AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		identity = newIdentity(strings.TrimSpace(name), "manual", SourceManual)
		return append(items, identity), nil
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return identity, nil
}

// EnsureManual returns the existing active manual identity, or creates one
// if none exists yet. Use this for idempotent "show me my manual access key"
// flows where the caller opens a settings panel repeatedly and we must not
// mint a new key on every visit. Revoked manual identities are skipped: the
// user deliberately killed those keys, so handing one back would emit
// install snippets with a dead key and no visible way to reinstate it. For
// deliberate "create a new manual agent" flows, use CreateManual instead.
func (s *IdentityStore) EnsureManual(name string) (AgentIdentity, error) {
	var identity AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for _, item := range items {
			if item.Source != SourceManual {
				continue
			}
			if strings.TrimSpace(item.RevokedAt) != "" {
				continue
			}
			identity = item
			return items, nil
		}
		identity = newIdentity(strings.TrimSpace(name), "manual", SourceManual)
		return append(items, identity), nil
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return identity, nil
}

func (s *IdentityStore) EnsureDetected(agentType, name string) (AgentIdentity, error) {
	var identity AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		normalizedType := strings.TrimSpace(agentType)
		for _, item := range items {
			if item.Source == SourceDetected && item.AgentType == normalizedType {
				identity = item
				return items, nil
			}
		}
		identity = newIdentity(strings.TrimSpace(name), normalizedType, SourceDetected)
		return append(items, identity), nil
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return identity, nil
}

func (s *IdentityStore) EnsureBound(accessKey, agentType, name string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}

	var identity AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for _, item := range items {
			if item.AccessKey == trimmedKey {
				identity = item
				return items, nil
			}
		}

		identity = newIdentityWithKey(trimmedKey, strings.TrimSpace(name), strings.TrimSpace(agentType), SourceDetected)
		return append(items, identity), nil
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return identity, nil
}

// normalizeInstallPath canonicalizes an install path for equality comparisons.
// Without this, macOS case-insensitivity, symlinks (think `/var` → `/private/var`),
// trailing separators, and relative/absolute variants all produce different
// identity rows for the same physical file — meaning one revoke touches only
// one of them and the other keeps working.
//
// EvalSymlinks fails silently when the path does not exist yet (first install),
// so we fall back to Abs+Clean which still gives us a deterministic form.
//
// On platforms whose default filesystem is case-insensitive (macOS HFS+/APFS
// in the default configuration, Windows NTFS), we also lowercase the result.
// EvalSymlinks does not canonicalize case when the path is not yet present on
// disk, so without this step `/Users/A/...` and `/users/a/...` produce two
// identities for the same file. Linux filesystems are case-sensitive, so we
// leave them alone.
func normalizeInstallPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if abs, err := filepath.Abs(trimmed); err == nil {
		trimmed = abs
	}
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil && strings.TrimSpace(resolved) != "" {
		trimmed = resolved
	}
	cleaned := filepath.Clean(trimmed)
	if caseInsensitiveFS(cleaned) {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}

// caseInsensitiveFS reports whether the filesystem hosting `path` treats
// filenames case-insensitively. macOS and Windows support both case-sensitive
// and case-insensitive volumes (APFS, NTFS with POSIX flag), so `runtime.GOOS`
// alone is the wrong signal: on a case-sensitive APFS home, `/Users/A/x` and
// `/Users/a/x` are distinct installs and must not share an identity row.
//
// We probe: find the nearest existing ancestor of `path`, then stat both a
// lower-cased and upper-cased variant of its final component. If both stats
// succeed and return the same inode, the filesystem folds case. Any probe
// failure falls back to the OS default (insensitive on darwin/windows), which
// preserves historical behavior for unreachable paths.
func caseInsensitiveFS(path string) bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return probeCaseInsensitive(path)
	default:
		return false
	}
}

func probeCaseInsensitive(path string) bool {
	dir := path
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "" {
			return osDefaultCaseInsensitive()
		}
		dir = parent
	}
	parent := filepath.Dir(dir)
	base := filepath.Base(dir)
	idx := -1
	var original rune
	for i, r := range base {
		if unicode.IsLetter(r) {
			idx = i
			original = r
			break
		}
	}
	if idx < 0 {
		return osDefaultCaseInsensitive()
	}
	var toggled rune
	if unicode.IsUpper(original) {
		toggled = unicode.ToLower(original)
	} else {
		toggled = unicode.ToUpper(original)
	}
	alt := base[:idx] + string(toggled) + base[idx+utf8.RuneLen(original):]
	origInfo, err := os.Stat(dir)
	if err != nil {
		return osDefaultCaseInsensitive()
	}
	altInfo, err := os.Stat(filepath.Join(parent, alt))
	if err != nil {
		return false
	}
	return os.SameFile(origInfo, altInfo)
}

func osDefaultCaseInsensitive() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	default:
		return false
	}
}

// EnsureForInstall returns an identity bound to a specific (agentType, installPath)
// pair. This is the install-time path for Skill/MCP flows: every unique install
// location gets its own stable access key so users can rename and revoke per
// install, while uninstall+reinstall to the same path reuses the same key.
//
// Resolution order:
//  1. Match an existing identity on (agentType, installPath) — reuse, refresh name if missing.
//  2. Backfill a legacy `detected` identity that has no installPath yet but matches
//     agentType — assign installPath so migration is transparent.
//  3. Mint a new identity.
func (s *IdentityStore) EnsureForInstall(agentType, installPath, name string) (AgentIdentity, error) {
	normalizedType := strings.TrimSpace(agentType)
	normalizedPath := normalizeInstallPath(installPath)

	var identity AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		now := time.Now().UTC().Format(time.RFC3339)

		if normalizedPath != "" {
			for idx, item := range items {
				if item.AgentType == normalizedType && item.InstallPath == normalizedPath {
					if item.Name == "" {
						item.Name = normalizeName(name, item.AccessKey)
						item.UpdatedAt = now
						items[idx] = item
					}
					identity = item
					return items, nil
				}
			}

			// Backfill: reuse a legacy detected identity for this agentType if
			// it has no installPath recorded yet. This preserves the access key
			// and audit history from the previous one-identity-per-type model.
			for idx, item := range items {
				if item.Source != SourceDetected || item.AgentType != normalizedType || item.InstallPath != "" {
					continue
				}
				item.InstallPath = normalizedPath
				if item.Name == "" {
					item.Name = normalizeName(name, item.AccessKey)
				}
				item.UpdatedAt = now
				items[idx] = item
				identity = item
				return items, nil
			}
		}

		identity = newIdentity(strings.TrimSpace(name), normalizedType, SourceDetected)
		identity.InstallPath = normalizedPath
		return append(items, identity), nil
	})
	if err != nil {
		return AgentIdentity{}, err
	}
	return identity, nil
}

// BindInstallPath attaches an installPath to the identity with the given
// access key. Unlike EnsureForInstall, this never mints a new identity and
// never backfills a sibling row — if the access key does not exist, it
// returns an error. Use this when the caller already holds an access key
// (e.g. startup refresh reading a config file) and needs to make sure that
// specific identity is bound to a specific install location.
func (s *IdentityStore) BindInstallPath(accessKey, installPath string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	normalizedPath := normalizeInstallPath(installPath)

	var bound AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			if item.InstallPath != normalizedPath {
				item.InstallPath = normalizedPath
				item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				items[idx] = item
			}
			bound = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return bound, nil
}

// Revoke marks an identity as revoked so runtime access checks can reject it
// without deleting audit history.
func (s *IdentityStore) Revoke(accessKey string) (AgentIdentity, error) {
	return s.setRevocation(accessKey, time.Now().UTC().Format(time.RFC3339))
}

// Unrevoke clears the revocation marker.
func (s *IdentityStore) Unrevoke(accessKey string) (AgentIdentity, error) {
	return s.setRevocation(accessKey, "")
}

func (s *IdentityStore) setRevocation(accessKey, revokedAt string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.RevokedAt = revokedAt
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

// SetSensitivityGrant flips the per-identity grant that controls access to
// the sensitivity-policy write tools (see AgentIdentity for the field
// rationale). Mirrors the Rename / setRevocation pattern: locked write,
// updates UpdatedAt, returns the post-write identity. Errors when the
// access key is missing or unknown.
func (s *IdentityStore) SetSensitivityGrant(accessKey string, grant bool) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.SensitivityClassificationGrant = grant
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

// SetRiskRuleManagementGrant flips the per-identity grant that lets an
// identity call the risk-rule write tools (set_risk_rule, delete_risk_rule)
// without an interactive approval prompt. Mirrors SetSensitivityGrant:
// locked write, updates UpdatedAt, returns the post-write identity. Errors
// when the access key is missing or unknown.
func (s *IdentityStore) SetRiskRuleManagementGrant(accessKey string, grant bool) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.RiskRuleManagementGrant = grant
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

// SetDatasourceManagementGrant flips the per-identity grant that lets an
// identity create new datasources through the Skill/MCP tool surface without
// an interactive approval prompt. Mirrors SetSensitivityGrant:
// locked write, updates UpdatedAt, returns the post-write identity. Errors
// when the access key is missing or unknown.
func (s *IdentityStore) SetDatasourceManagementGrant(accessKey string, grant bool) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.DatasourceManagementGrant = grant
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

// SetDatasourceScope persists the datasource access model for one agent
// identity. Empty scope is normalized to the compatibility default
// inherit_user. The allowlist is normalized and de-duplicated, but an empty
// allowlist remains meaningful: it denies all datasource-scoped tool calls.
func (s *IdentityStore) SetDatasourceScope(accessKey, scope string, allowedDatasourceIDs []string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	nextScope := NormalizeDatasourceScope(scope)
	if nextScope != DatasourceScopeInheritUser && nextScope != DatasourceScopeAllowList {
		return AgentIdentity{}, fmt.Errorf("unsupported datasource scope: %s", nextScope)
	}
	allowed := NormalizeAllowedDatasourceIDs(allowedDatasourceIDs)
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.DatasourceScope = nextScope
			if nextScope == DatasourceScopeAllowList {
				item.AllowedDatasourceIDs = allowed
			} else {
				item.AllowedDatasourceIDs = nil
			}
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

// SetExpiresAt stores the optional expiry timestamp for one agent identity.
// Empty string means no automatic expiry; non-empty values must be RFC3339.
func (s *IdentityStore) SetExpiresAt(accessKey, expiresAt string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	trimmedExpiry := strings.TrimSpace(expiresAt)
	if trimmedExpiry != "" {
		if _, err := time.Parse(time.RFC3339, trimmedExpiry); err != nil {
			return AgentIdentity{}, fmt.Errorf("invalid expiresAt: %w", err)
		}
	}
	var updated AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.ExpiresAt = trimmedExpiry
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			updated = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return updated, nil
}

func (s *IdentityStore) Rename(accessKey, name string) (AgentIdentity, error) {
	trimmedKey := strings.TrimSpace(accessKey)
	if trimmedKey == "" {
		return AgentIdentity{}, fmt.Errorf("access key is required")
	}
	var renamed AgentIdentity
	err := s.withWrite(func(items []AgentIdentity) ([]AgentIdentity, error) {
		nextName := normalizeName(name, trimmedKey)
		for idx, item := range items {
			if item.AccessKey != trimmedKey {
				continue
			}
			item.Name = nextName
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			items[idx] = item
			renamed = item
			return items, nil
		}
		return nil, errIdentityNotFound
	})
	if err != nil {
		if errors.Is(err, errIdentityNotFound) {
			return AgentIdentity{}, fmt.Errorf("agent identity not found")
		}
		return AgentIdentity{}, err
	}
	return renamed, nil
}

func (s *IdentityStore) Get(accessKey string) (AgentIdentity, bool, error) {
	var identity AgentIdentity
	var found bool
	err := s.withRead(func(items []AgentIdentity) error {
		trimmedKey := strings.TrimSpace(accessKey)
		for _, item := range items {
			if item.AccessKey == trimmedKey {
				identity = item
				found = true
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return AgentIdentity{}, false, err
	}
	return identity, found, nil
}

func (s *IdentityStore) ListAll() ([]AgentIdentity, error) {
	var items []AgentIdentity
	err := s.withRead(func(loaded []AgentIdentity) error {
		items = append([]AgentIdentity(nil), loaded...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *IdentityStore) loadAll() ([]AgentIdentity, error) {
	data, err := securefile.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var items []AgentIdentity
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *IdentityStore) saveAll(items []AgentIdentity) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return securefile.WriteFile(s.path, payload, 0o644)
}

func (s *IdentityStore) withRead(fn func([]AgentIdentity) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return securefile.WithPathLock(s.path, func() error {
		items, err := s.loadAll()
		if err != nil {
			return err
		}
		return fn(items)
	})
}

func (s *IdentityStore) withWrite(fn func([]AgentIdentity) ([]AgentIdentity, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return securefile.WithPathLock(s.path, func() error {
		items, err := s.loadAll()
		if err != nil {
			return err
		}
		nextItems, err := fn(items)
		if err != nil {
			return err
		}
		return s.saveAll(nextItems)
	})
}

var errIdentityNotFound = errors.New("agent identity not found")

func newIdentity(name, agentType, source string) AgentIdentity {
	key := generateAccessKey()
	return newIdentityWithKey(key, name, agentType, source)
}

func newIdentityWithKey(accessKey, name, agentType, source string) AgentIdentity {
	key := strings.TrimSpace(accessKey)
	if key == "" {
		key = generateAccessKey()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return AgentIdentity{
		AccessKey:       key,
		Name:            normalizeName(name, key),
		AgentType:       strings.TrimSpace(agentType),
		Source:          strings.TrimSpace(source),
		DatasourceScope: DatasourceScopeInheritUser,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func normalizeName(name, accessKey string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	suffix := strings.TrimSpace(accessKey)
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	if suffix == "" {
		suffix = "0000"
	}
	return "agent-" + suffix
}

func generateAccessKey() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("agent_%x", now)
	}
	return "agent_" + hex.EncodeToString(buf)
}
