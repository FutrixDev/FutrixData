package localcrypto

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/startuprecovery"
)

const authSessionFilename = "auth-session.json"

var ErrMoveAsideRequiresConfirmation = errors.New("localcrypto: moving encrypted data aside requires user confirmation")

type InitResult struct {
	DataPath          string
	CreatedLocalRoot  bool
	HasLegacyFallback bool
	MigratedPaths     []string
}

type InitOptions struct {
	AuxiliaryLoadMode bootstrap.AuxiliaryLoadMode
}

type MigrationOptions struct {
	AuxiliaryBestEffort bool
}

type MoveAsideResult struct {
	DataPath     string
	OriginalDir  string
	RetentionDir string
}

func (r InitResult) Migrated(path string) bool {
	clean := filepath.Clean(path)
	for _, migrated := range r.MigratedPaths {
		if filepath.Clean(migrated) == clean {
			return true
		}
	}
	return false
}

// Init installs the local root encryption key for this process and migrates
// known sensitive local files away from plaintext or legacy encrypted storage.
func Init(dataPath string) (InitResult, error) {
	return InitWithOptions(dataPath, InitOptions{})
}

// InitWithOptions installs the local root encryption key and lets callers keep
// migration failure handling consistent with their runtime load policy.
func InitWithOptions(dataPath string, opts InitOptions) (InitResult, error) {
	resolvedDataPath := bootstrap.ResolveDataPath(dataPath)
	result := InitResult{DataPath: resolvedDataPath}
	securefile.RequireEncryption(true)

	localRoot, created, err := keyring.EnsureLocalRootKey()
	if err != nil {
		securefile.SetKey(nil)
		wrapped := fmt.Errorf("local root encryption key unavailable: %w", err)
		return result, startuprecovery.Wrap(wrapped, startuprecovery.Classify(wrapped, resolvedDataPath))
	}
	result.CreatedLocalRoot = created

	var fallbacks [][]byte
	legacyKey, err := keyring.Get()
	if err == nil && len(legacyKey) > 0 && !bytes.Equal(localRoot, legacyKey) {
		fallbacks = append(fallbacks, legacyKey)
		result.HasLegacyFallback = true
	}
	securefile.SetKeys(localRoot, fallbacks...)
	securefile.RequireEncryption(true)

	migrated, err := MigrateKnownFilesWithOptions(resolvedDataPath, MigrationOptions{
		AuxiliaryBestEffort: opts.AuxiliaryLoadMode == bootstrap.AuxiliaryLoadBestEffort,
	})
	if err != nil {
		return result, startuprecovery.Wrap(err, startuprecovery.Classify(err, resolvedDataPath))
	}
	result.MigratedPaths = migrated
	return result, nil
}

func MigrateKnownFiles(dataPath string) ([]string, error) {
	return MigrateKnownFilesWithOptions(dataPath, MigrationOptions{})
}

func MigrateKnownFilesWithOptions(dataPath string, opts MigrationOptions) ([]string, error) {
	var migrated []string
	for _, entry := range sensitivePathEntries(dataPath) {
		changed, err := securefile.MigrateFile(entry.Path)
		if err != nil {
			if opts.AuxiliaryBestEffort && entry.Auxiliary {
				continue
			}
			return migrated, fmt.Errorf("migrate local encrypted file %s: %w", entry.Path, err)
		}
		if changed {
			migrated = append(migrated, entry.Path)
		}
	}
	return migrated, nil
}

func SensitivePaths(dataPath string) []string {
	entries := sensitivePathEntries(dataPath)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

type sensitivePathEntry struct {
	Path      string
	Auxiliary bool
}

func sensitivePathEntries(dataPath string) []sensitivePathEntry {
	resolvedDataPath := bootstrap.ResolveDataPath(dataPath)
	dataDir := filepath.Dir(resolvedDataPath)
	return []sensitivePathEntry{
		{Path: resolvedDataPath},
		{Path: bootstrap.AIConfigPath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.RedisCommandDocsPath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.EntitySchemaCachePath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.HistoryPath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.AgentIdentityPath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.AgentAuditPath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.SensitivityStorePath(resolvedDataPath), Auxiliary: true},
		{Path: bootstrap.SecretProviderConfigPath(resolvedDataPath), Auxiliary: true},
		{Path: filepath.Join(dataDir, authSessionFilename), Auxiliary: true},
	}
}

func MoveAsideUnrecoverableData(dataPath string, confirmed bool) (MoveAsideResult, error) {
	resolvedDataPath := bootstrap.ResolveDataPath(dataPath)
	dataDir := filepath.Dir(resolvedDataPath)
	result := MoveAsideResult{
		DataPath:    resolvedDataPath,
		OriginalDir: dataDir,
	}
	if !confirmed {
		return result, ErrMoveAsideRequiresConfirmation
	}
	if err := os.MkdirAll(filepath.Dir(dataDir), 0o700); err != nil {
		return result, err
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return result, err
		}
		return result, nil
	} else if err != nil {
		return result, err
	}
	retentionDir, err := nextRetentionDir(dataDir)
	if err != nil {
		return result, err
	}
	if err := os.Rename(dataDir, retentionDir); err != nil {
		return result, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		// Preserve the retained data and surface the failure. The caller stays
		// in recovery mode instead of pretending a fresh start succeeded.
		return MoveAsideResult{DataPath: resolvedDataPath, OriginalDir: dataDir, RetentionDir: retentionDir}, err
	}
	result.RetentionDir = retentionDir
	return result, nil
}

func nextRetentionDir(dataDir string) (string, error) {
	parent := filepath.Dir(dataDir)
	base := filepath.Base(dataDir)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	prefix := filepath.Join(parent, base+"-recovered-"+stamp)
	for i := 0; i < 100; i++ {
		candidate := prefix
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d", prefix, i)
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("localcrypto: could not allocate retention folder for %s", dataDir)
}
