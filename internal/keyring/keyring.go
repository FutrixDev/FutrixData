package keyring

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"futrixdata/platform/internal/securefile"
	gokeyring "github.com/zalando/go-keyring"
)

const (
	serviceName          = "FutrixData"
	encryptionKeyAccount = "encryption-key"
	localRootKeyAccount  = "local-root-encryption-key"
	maskingSecretAccount = "masking-secret-v1"
	keySizeBytes         = 32
)

var (
	ErrNotFound = gokeyring.ErrNotFound

	backendMu  sync.RWMutex
	backendGet = gokeyring.Get
	backendSet = gokeyring.Set
)

// Get retrieves the legacy encryption key from the OS keychain.
// Returns nil if not found.
func Get() ([]byte, error) {
	return getAccountKey(encryptionKeyAccount)
}

// Set stores the legacy encryption key in the OS keychain.
func Set(key []byte) error {
	return setAccountKey(encryptionKeyAccount, key)
}

// EnsureLocalRootKey returns the local root encryption key, creating and
// storing a new one when this install has not yet initialized local storage
// encryption.
func EnsureLocalRootKey() ([]byte, bool, error) {
	var key []byte
	var created bool
	err := securefile.WithPathLock(accountLockPath(localRootKeyAccount), func() error {
		existing, err := getRequiredAccountKey(localRootKeyAccount)
		if err == nil {
			if len(existing) != keySizeBytes {
				return fmt.Errorf("keyring: local root key has invalid length %d", len(existing))
			}
			key = existing
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		generated := make([]byte, keySizeBytes)
		if _, err := io.ReadFull(rand.Reader, generated); err != nil {
			return err
		}
		if err := setAccountKey(localRootKeyAccount, generated); err != nil {
			return err
		}
		key = generated
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return key, created, nil
}

// GetMaskingSecret retrieves the local masking secret from the OS keychain.
// Returns nil if not found.
func GetMaskingSecret() ([]byte, error) {
	return getAccountKey(maskingSecretAccount)
}

// EnsureMaskingSecret returns the stored local masking secret, generating and
// storing one on first use.
func EnsureMaskingSecret() ([]byte, error) {
	var secret []byte
	err := securefile.WithPathLock(accountLockPath(maskingSecretAccount), func() error {
		existing, err := GetMaskingSecret()
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			secret = existing
			return nil
		}

		generated := make([]byte, keySizeBytes)
		if _, err := io.ReadFull(rand.Reader, generated); err != nil {
			return err
		}
		if err := setAccountKey(maskingSecretAccount, generated); err != nil {
			return err
		}
		secret = generated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func UseBackendForTest(
	get func(service, account string) (string, error),
	set func(service, account, secret string) error,
) func() {
	backendMu.Lock()
	originalGet := backendGet
	originalSet := backendSet
	backendGet = get
	backendSet = set
	backendMu.Unlock()
	return func() {
		backendMu.Lock()
		defer backendMu.Unlock()
		backendGet = originalGet
		backendSet = originalSet
	}
}

func getAccountKey(account string) ([]byte, error) {
	key, err := getRequiredAccountKey(account)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return key, err
}

func getRequiredAccountKey(account string) ([]byte, error) {
	encoded, err := getSecret(serviceName, account)
	if err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(encoded)
}

func setAccountKey(account string, key []byte) error {
	encoded := base64.RawURLEncoding.EncodeToString(key)
	return setSecret(serviceName, account, encoded)
}

func getSecret(service, account string) (string, error) {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return backendGet(service, account)
}

func setSecret(service, account, secret string) error {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return backendSet(service, account, secret)
}

func accountLockPath(account string) string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "FutrixData", account)
}
