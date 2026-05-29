package securefile

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/version"
)

var (
	mu                 sync.RWMutex
	globalKey          []byte
	globalFallbackKeys [][]byte
	requireEncryption  bool
	metadataOverride   *EnvelopeMetadata
	readerVersion      string
)

var (
	magicPrefix = []byte("FXENC")
	magicHeader = []byte("FXENC\x01") // legacy format: magic(6) + nonce(12) + ciphertext
	magicV2     = []byte("FXENC\x02") // metadata format: magic(6) + metadata length(4) + metadata + nonce + ciphertext
)

var ErrKeyUnavailable = errors.New("securefile: local encryption key unavailable")
var ErrAppVersionTooOld = errors.New("securefile: app version too old for encrypted file")
var ErrDataCorrupt = errors.New("securefile: encrypted file is corrupt")
var ErrDecryptFailed = errors.New("securefile: encrypted file could not be decrypted")

type EnvelopeMetadata struct {
	FormatVersion       int    `json:"formatVersion"`
	WriterAppVersion    string `json:"writerAppVersion,omitempty"`
	MinReaderAppVersion string `json:"minReaderAppVersion,omitempty"`
	MigrationSource     string `json:"migrationSource,omitempty"`
	MigratedAt          string `json:"migratedAt,omitempty"`
}

type EnvelopeVersionError struct {
	CurrentVersion string
	Metadata       EnvelopeMetadata
}

func (e *EnvelopeVersionError) Error() string {
	return fmt.Sprintf("securefile: encrypted file requires FutrixData %s or newer (current %s)", e.Metadata.MinReaderAppVersion, e.CurrentVersion)
}

func (e *EnvelopeVersionError) Unwrap() error {
	return ErrAppVersionTooOld
}

func UseEnvelopeMetadataForTest(metadata EnvelopeMetadata) func() {
	mu.Lock()
	original := metadataOverride
	cp := normalizeMetadata(metadata)
	metadataOverride = &cp
	mu.Unlock()
	return func() {
		mu.Lock()
		defer mu.Unlock()
		metadataOverride = original
	}
}

func UseReaderAppVersionForTest(value string) func() {
	mu.Lock()
	original := readerVersion
	readerVersion = strings.TrimSpace(value)
	mu.Unlock()
	return func() {
		mu.Lock()
		defer mu.Unlock()
		readerVersion = original
	}
}

// SetKey sets the global encryption key. Pass nil to disable encryption.
func SetKey(key []byte) {
	SetKeys(key)
}

// SetKeys sets the primary encryption key and optional legacy fallback keys.
// Writes always use the primary key. Reads try the primary key first, then
// fallback keys so old encrypted files can be migrated without data loss.
func SetKeys(key []byte, fallbackKeys ...[]byte) {
	mu.Lock()
	defer mu.Unlock()
	globalKey = copyKey(key)
	globalFallbackKeys = copyDistinctFallbackKeys(globalKey, fallbackKeys)
}

// Key returns a copy of the current global key, or nil.
func Key() []byte {
	mu.RLock()
	defer mu.RUnlock()
	if len(globalKey) == 0 {
		return nil
	}
	cp := make([]byte, len(globalKey))
	copy(cp, globalKey)
	return cp
}

// AddFallbackKey adds a legacy read-only key. Writes continue to use Key().
func AddFallbackKey(key []byte) {
	if len(key) == 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if bytes.Equal(globalKey, key) {
		return
	}
	for _, existing := range globalFallbackKeys {
		if bytes.Equal(existing, key) {
			return
		}
	}
	globalFallbackKeys = append(globalFallbackKeys, append([]byte(nil), key...))
}

// RequireEncryption controls whether WriteFile may fall back to plaintext when
// no primary key is available. App, daemon, and CLI startup enable this after
// attempting local root key initialization.
func RequireEncryption(required bool) {
	mu.Lock()
	defer mu.Unlock()
	requireEncryption = required
}

func EncryptionRequired() bool {
	mu.RLock()
	defer mu.RUnlock()
	return requireEncryption
}

func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	globalKey = nil
	globalFallbackKeys = nil
	requireEncryption = false
	metadataOverride = nil
	readerVersion = ""
}

// ReadFile reads and decrypts a file. Plain files are returned unchanged so
// startup migration can load older installs. Encrypted files must decrypt with
// the primary key or a configured legacy fallback key.
func ReadFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !looksEncrypted(raw) {
		return raw, nil
	}
	if _, _, err := parseEnvelope(raw); err != nil {
		return nil, err
	}
	if err := enforceReaderVersion(raw); err != nil {
		return nil, err
	}
	plaintext, err := decryptWithConfiguredKeys(raw)
	if err != nil {
		return nil, fmt.Errorf("securefile: decrypt file: %w", err)
	}
	return plaintext, nil
}

func ReadEnvelopeMetadata(path string) (EnvelopeMetadata, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return EnvelopeMetadata{}, false, err
	}
	if !looksEncrypted(raw) {
		return EnvelopeMetadata{}, false, nil
	}
	metadata, _, err := parseEnvelope(raw)
	if err != nil {
		return EnvelopeMetadata{}, true, err
	}
	return metadata, true, nil
}

// WriteFile encrypts and writes data to a file. If no key is set,
// writes plaintext only when encryption is not required.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	key, _, required := keySnapshot()
	if key == nil {
		if required {
			return fmt.Errorf("%w; refusing to write plaintext", ErrKeyUnavailable)
		}
		return os.WriteFile(path, data, perm)
	}
	encrypted, err := encryptWithMetadata(key, data, metadataForWrite(""))
	if err != nil {
		return err
	}
	return os.WriteFile(path, encrypted, perm)
}

// MigrateFile rewrites an existing plaintext or legacy-encrypted file with the
// current primary key. It returns false when the file does not exist or already
// decrypts with the primary key.
func MigrateFile(path string) (bool, error) {
	return migrateFile(path)
}

// AppendFile reads an existing file (decrypting if needed), appends data,
// then encrypts and writes back. Used for append-only files like events.jsonl.
func AppendFile(path string, data []byte, perm os.FileMode) error {
	return withPathLock(path, func() error {
		existing, err := ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		combined := append(existing, data...)
		return WriteFile(path, combined, perm)
	})
}

// WithPathLock runs fn while holding the per-path file lock used by securefile
// read-modify-write helpers. Callers should wrap the entire critical section.
func WithPathLock(path string, fn func() error) error {
	return withPathLock(path, fn)
}

func looksEncrypted(data []byte) bool {
	return len(data) >= len(magicPrefix) && bytes.Equal(data[:len(magicPrefix)], magicPrefix)
}

func hasHeader(data []byte) bool {
	if len(data) < len(magicPrefix)+1 {
		return false
	}
	return bytes.Equal(data[:len(magicHeader)], magicHeader) || bytes.Equal(data[:len(magicV2)], magicV2)
}

func migrateFile(path string) (bool, error) {
	var migrated bool
	err := withPathLock(path, func() error {
		stat, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if stat.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		key, _, _ := keySnapshot()
		if key == nil {
			return fmt.Errorf("%w; cannot migrate local file", ErrKeyUnavailable)
		}
		if hasHeader(raw) && bytes.Equal(raw[:len(magicV2)], magicV2) {
			if err := enforceReaderVersion(raw); err != nil {
				return err
			}
			if _, err := decrypt(key, raw); err == nil {
				return nil
			}
		}
		plaintext, err := ReadFile(path)
		if err != nil {
			return err
		}
		source := migrationSource(raw)
		if err := writeFileAtomic(path, plaintext, raw, stat.Mode().Perm(), source); err != nil {
			return err
		}
		migrated = true
		return nil
	})
	return migrated, err
}

func writeFileAtomic(path string, data []byte, original []byte, perm os.FileMode, source string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Close(); err != nil {
		return err
	}
	metadata := metadataForWrite(source)
	encrypted, err := encryptWithMetadata(mustPrimaryKey(), data, metadata)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, encrypted, perm); err != nil {
		return err
	}
	readBack, err := ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("securefile: verify migrated temp file: %w", err)
	}
	if !bytes.Equal(readBack, data) {
		return errors.New("securefile: verify migrated temp file: plaintext mismatch")
	}
	backupPath, err := nextBackupPath(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(backupPath, original, perm); err != nil {
		return fmt.Errorf("securefile: write migration backup: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

func mustPrimaryKey() []byte {
	key, _, _ := keySnapshot()
	return key
}

func decryptWithConfiguredKeys(data []byte) ([]byte, error) {
	key, fallbacks, _ := keySnapshot()
	if key == nil && len(fallbacks) == 0 {
		return nil, ErrKeyUnavailable
	}
	var lastErr error
	if key != nil {
		plaintext, err := decrypt(key, data)
		if err == nil {
			return plaintext, nil
		}
		lastErr = err
	}
	for _, fallback := range fallbacks {
		plaintext, err := decrypt(fallback, data)
		if err == nil {
			return plaintext, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrKeyUnavailable
	}
	return nil, fmt.Errorf("%w: %v", ErrDecryptFailed, lastErr)
}

func keySnapshot() ([]byte, [][]byte, bool) {
	mu.RLock()
	defer mu.RUnlock()
	var key []byte
	if len(globalKey) > 0 {
		key = append([]byte(nil), globalKey...)
	}
	fallbacks := make([][]byte, 0, len(globalFallbackKeys))
	for _, fallback := range globalFallbackKeys {
		if len(fallback) == 0 {
			continue
		}
		fallbacks = append(fallbacks, append([]byte(nil), fallback...))
	}
	return key, fallbacks, requireEncryption
}

func copyKey(key []byte) []byte {
	if len(key) == 0 {
		return nil
	}
	return append([]byte(nil), key...)
}

func copyDistinctFallbackKeys(primary []byte, keys [][]byte) [][]byte {
	out := make([][]byte, 0, len(keys))
	for _, key := range keys {
		if len(key) == 0 || bytes.Equal(primary, key) {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if bytes.Equal(existing, key) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, append([]byte(nil), key...))
	}
	return out
}

func encrypt(key, plaintext []byte) ([]byte, error) {
	return encryptWithMetadata(key, plaintext, metadataForWrite(""))
}

func encryptWithMetadata(key, plaintext []byte, metadata EnvelopeMetadata) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	metadata = normalizeMetadata(metadata)
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	// Format: magic(6) + metadata length(4) + metadata JSON + nonce(12) + ciphertext(n)
	out := make([]byte, 0, len(magicV2)+4+len(metadataRaw)+len(nonce)+len(ciphertext))
	out = append(out, magicV2...)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(metadataRaw)))
	out = append(out, length[:]...)
	out = append(out, metadataRaw...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func decrypt(key, data []byte) ([]byte, error) {
	_, payload, err := parseEnvelope(data)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, payload.nonce, payload.ciphertext, nil)
}

type encryptedPayload struct {
	nonce      []byte
	ciphertext []byte
}

func parseEnvelope(data []byte) (EnvelopeMetadata, encryptedPayload, error) {
	if len(data) < len(magicHeader) || !looksEncrypted(data) {
		return EnvelopeMetadata{}, encryptedPayload{}, ErrDataCorrupt
	}
	switch {
	case bytes.Equal(data[:len(magicHeader)], magicHeader):
		if len(data) < len(magicHeader)+12 {
			return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: data too short", ErrDataCorrupt)
		}
		return EnvelopeMetadata{FormatVersion: 1}, encryptedPayload{
			nonce:      data[len(magicHeader) : len(magicHeader)+12],
			ciphertext: data[len(magicHeader)+12:],
		}, nil
	case bytes.Equal(data[:len(magicV2)], magicV2):
		if len(data) < len(magicV2)+4 {
			return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: missing metadata length", ErrDataCorrupt)
		}
		metadataLen := int(binary.BigEndian.Uint32(data[len(magicV2) : len(magicV2)+4]))
		metadataStart := len(magicV2) + 4
		metadataEnd := metadataStart + metadataLen
		if metadataLen <= 0 || metadataEnd > len(data) {
			return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: invalid metadata length", ErrDataCorrupt)
		}
		if len(data) < metadataEnd+12 {
			return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: data too short", ErrDataCorrupt)
		}
		var metadata EnvelopeMetadata
		if err := json.Unmarshal(data[metadataStart:metadataEnd], &metadata); err != nil {
			return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: decode metadata: %v", ErrDataCorrupt, err)
		}
		metadata = normalizeMetadata(metadata)
		return metadata, encryptedPayload{
			nonce:      data[metadataEnd : metadataEnd+12],
			ciphertext: data[metadataEnd+12:],
		}, nil
	default:
		return EnvelopeMetadata{}, encryptedPayload{}, fmt.Errorf("%w: unsupported encrypted format", ErrDataCorrupt)
	}
}

func enforceReaderVersion(data []byte) error {
	metadata, _, err := parseEnvelope(data)
	if err != nil {
		return err
	}
	if metadata.FormatVersion < 2 || strings.TrimSpace(metadata.MinReaderAppVersion) == "" {
		return nil
	}
	current := currentReaderVersion()
	if versionAtLeast(current, metadata.MinReaderAppVersion) {
		return nil
	}
	return &EnvelopeVersionError{CurrentVersion: current, Metadata: metadata}
}

func currentReaderVersion() string {
	mu.RLock()
	defer mu.RUnlock()
	if strings.TrimSpace(readerVersion) != "" {
		return strings.TrimSpace(readerVersion)
	}
	return strings.TrimSpace(version.Version)
}

func metadataForWrite(source string) EnvelopeMetadata {
	mu.RLock()
	override := metadataOverride
	var metadata EnvelopeMetadata
	if override != nil {
		metadata = *override
	}
	mu.RUnlock()
	if metadata.WriterAppVersion == "" {
		metadata.WriterAppVersion = version.Version
	}
	if metadata.MinReaderAppVersion == "" {
		metadata.MinReaderAppVersion = "1.0.14"
	}
	if strings.TrimSpace(source) != "" && metadata.MigrationSource == "" {
		metadata.MigrationSource = strings.TrimSpace(source)
	}
	if metadata.MigrationSource != "" && metadata.MigratedAt == "" {
		metadata.MigratedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return normalizeMetadata(metadata)
}

func normalizeMetadata(metadata EnvelopeMetadata) EnvelopeMetadata {
	if metadata.FormatVersion == 0 {
		metadata.FormatVersion = 2
	}
	metadata.WriterAppVersion = strings.TrimSpace(metadata.WriterAppVersion)
	metadata.MinReaderAppVersion = strings.TrimSpace(metadata.MinReaderAppVersion)
	metadata.MigrationSource = strings.TrimSpace(metadata.MigrationSource)
	metadata.MigratedAt = strings.TrimSpace(metadata.MigratedAt)
	return metadata
}

func migrationSource(raw []byte) string {
	if !looksEncrypted(raw) {
		return "plaintext"
	}
	if len(raw) >= len(magicHeader) && bytes.Equal(raw[:len(magicHeader)], magicHeader) {
		return "legacy-encrypted"
	}
	return "encrypted"
}

func nextBackupPath(path string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	base := fmt.Sprintf("%s.backup-%s", path, stamp)
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d", base, i)
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("securefile: could not allocate backup path for %s", path)
}

func versionAtLeast(current, required string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	required = strings.TrimPrefix(strings.TrimSpace(required), "v")
	if current == "" || current == "dev" || required == "" {
		return true
	}
	currentParts, currentOK := parseVersionParts(current)
	requiredParts, requiredOK := parseVersionParts(required)
	if !currentOK || !requiredOK {
		return true
	}
	for i := 0; i < len(requiredParts); i++ {
		if currentParts[i] > requiredParts[i] {
			return true
		}
		if currentParts[i] < requiredParts[i] {
			return false
		}
	}
	return true
}

func parseVersionParts(value string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return out, false
	}
	for i := 0; i < len(out); i++ {
		if i >= len(parts) {
			break
		}
		part := parts[i]
		if idx := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); idx >= 0 {
			part = part[:idx]
		}
		if part == "" {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
