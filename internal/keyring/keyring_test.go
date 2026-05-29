package keyring

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

type fakeBackend struct {
	mu      sync.Mutex
	secrets map[string]string
	setErr  error
}

func newFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	backend := &fakeBackend{secrets: map[string]string{}}
	restore := UseBackendForTest(backend.get, backend.set)
	t.Cleanup(restore)
	return backend
}

func (b *fakeBackend) get(service, account string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	value, ok := b.secrets[service+"/"+account]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (b *fakeBackend) set(service, account, secret string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.setErr != nil {
		return b.setErr
	}
	b.secrets[service+"/"+account] = secret
	return nil
}

func TestEnsureLocalRootKeyCreatesAndPersists(t *testing.T) {
	newFakeBackend(t)

	first, created, err := EnsureLocalRootKey()
	if err != nil {
		t.Fatalf("ensure local root key: %v", err)
	}
	if !created {
		t.Fatalf("expected first call to create local root key")
	}
	if len(first) != 32 {
		t.Fatalf("expected 32-byte local root key, got %d bytes", len(first))
	}

	second, created, err := EnsureLocalRootKey()
	if err != nil {
		t.Fatalf("ensure existing local root key: %v", err)
	}
	if created {
		t.Fatalf("expected second call to reuse existing local root key")
	}
	if !bytes.Equal(second, first) {
		t.Fatalf("expected persisted local root key to be reused")
	}
}

func TestEnsureLocalRootKeyReturnsSetFailure(t *testing.T) {
	backend := newFakeBackend(t)
	backend.setErr = errors.New("keychain locked")

	_, _, err := EnsureLocalRootKey()
	if !errors.Is(err, backend.setErr) {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestEnsureLocalRootKeyConcurrentFirstRunReturnsSingleSecret(t *testing.T) {
	newFakeBackend(t)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make([][]byte, workers)
	created := make([]bool, workers)
	errs := make([]error, workers)
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			results[i], created[i], errs[i] = EnsureLocalRootKey()
		}()
	}
	close(start)
	wg.Wait()

	createdCount := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureLocalRootKey worker %d: %v", i, err)
		}
		if created[i] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly one creator, got %d", createdCount)
	}
	first := results[0]
	if len(first) != 32 {
		t.Fatalf("expected 32-byte local root key, got %d", len(first))
	}
	for i, result := range results[1:] {
		if !bytes.Equal(first, result) {
			t.Fatalf("worker %d got divergent local root key", i+1)
		}
	}
}

func TestEnsureMaskingSecretGeneratesAndReusesKeyringSecret(t *testing.T) {
	newFakeBackend(t)

	first, err := EnsureMaskingSecret()
	if err != nil {
		t.Fatalf("EnsureMaskingSecret first call: %v", err)
	}
	if len(first) != 32 {
		t.Fatalf("expected 32-byte masking secret, got %d", len(first))
	}

	second, err := EnsureMaskingSecret()
	if err != nil {
		t.Fatalf("EnsureMaskingSecret second call: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("expected masking secret to be reused from keyring")
	}
}

func TestGetMaskingSecretReturnsNilWhenMissing(t *testing.T) {
	newFakeBackend(t)

	secret, err := GetMaskingSecret()
	if err != nil {
		t.Fatalf("GetMaskingSecret: %v", err)
	}
	if secret != nil {
		t.Fatalf("expected missing masking secret to return nil, got %d bytes", len(secret))
	}
}

func TestEnsureMaskingSecretConcurrentFirstRunReturnsSingleSecret(t *testing.T) {
	newFakeBackend(t)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make([][]byte, workers)
	errs := make([]error, workers)
	start := make(chan struct{})

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = EnsureMaskingSecret()
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureMaskingSecret worker %d: %v", i, err)
		}
	}
	first := results[0]
	if len(first) != 32 {
		t.Fatalf("expected 32-byte masking secret, got %d", len(first))
	}
	for i, result := range results[1:] {
		if !bytes.Equal(first, result) {
			t.Fatalf("worker %d got divergent masking secret", i+1)
		}
	}
}
