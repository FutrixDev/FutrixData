package secrets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type VaultKVV2Provider struct {
	config ProviderConfig
	vault  VaultKVV2Config
	client *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewVaultKVV2Provider(cfg ProviderConfig) (*VaultKVV2Provider, error) {
	cfg = normalizeProviderConfig(cfg)
	vault := cfg.VaultKVV2
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, errors.New("provider config id is required")
	}
	if strings.TrimSpace(vault.Address) == "" {
		return nil, errors.New("vault address is required")
	}
	if vault.Mount == "" {
		vault.Mount = "secret"
	}
	if vault.AuthMethod == "" {
		vault.AuthMethod = "token"
	}
	timeout := time.Duration(vault.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if vault.TLSSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &VaultKVV2Provider{
		config: cfg,
		vault:  vault,
		client: &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

func (p *VaultKVV2Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Type:              ProviderVaultKVV2,
		Versioning:        true,
		Labels:            false,
		Rotation:          true,
		BinaryPayloads:    false,
		Metadata:          true,
		ProviderSideAudit: true,
		RequiresAgent:     p.vault.AuthMethod == "agent-token-file",
	}
}

func (p *VaultKVV2Provider) HealthCheck(ctx context.Context) error {
	if err := p.do(ctx, http.MethodGet, "sys/health", nil, nil, nil, requestOptions{AllowUnauthenticated: true, AllowHealthCodes: true}); err != nil {
		return err
	}
	// Verify the configured KV v2 mount is actually reachable. Its config endpoint
	// returns 200 for a valid KV v2 engine even when no secrets are stored, so it
	// distinguishes a missing/misconfigured mount (404) from a merely empty path
	// prefix. A LIST on the metadata path 404s in both cases, so it cannot tell a
	// broken mount from an empty one and would report a dead provider as healthy.
	configPath := joinVaultPath(p.vault.Mount, "config")
	return p.do(ctx, http.MethodGet, configPath, nil, nil, nil, requestOptions{})
}

func (p *VaultKVV2Provider) Put(ctx context.Context, ref SecretRef, value SecretValue) (SecretRef, error) {
	ref = p.normalizeRef(ref)
	if err := validateVaultRef(ref); err != nil {
		return SecretRef{}, err
	}
	body := map[string]any{
		"data": map[string]any{
			ref.Field: value.Plaintext,
		},
	}
	var resp struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err := p.do(ctx, http.MethodPost, p.dataPath(ref.Key), nil, body, &resp, requestOptions{}); err != nil {
		return SecretRef{}, err
	}
	if resp.Data.Version > 0 {
		ref.Version = strconv.Itoa(resp.Data.Version)
	}
	ref.Fingerprint = fingerprintRef(ref)
	return ref, nil
}

func (p *VaultKVV2Provider) Resolve(ctx context.Context, ref SecretRef) (SecretValue, error) {
	ref = p.normalizeRef(ref)
	if err := validateVaultRef(ref); err != nil {
		return SecretValue{}, err
	}
	query := url.Values{}
	if strings.TrimSpace(ref.Version) != "" {
		query.Set("version", strings.TrimSpace(ref.Version))
	}
	var resp struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := p.do(ctx, http.MethodGet, p.dataPath(ref.Key), query, nil, &resp, requestOptions{}); err != nil {
		return SecretValue{}, err
	}
	raw, ok := resp.Data.Data[ref.Field]
	if !ok {
		return SecretValue{}, fmt.Errorf("%w: %s", ErrSecretNotFound, ref.Field)
	}
	switch typed := raw.(type) {
	case string:
		return SecretValue{Plaintext: typed}, nil
	default:
		return SecretValue{Plaintext: fmt.Sprint(typed)}, nil
	}
}

func (p *VaultKVV2Provider) Delete(ctx context.Context, ref SecretRef) error {
	ref = p.normalizeRef(ref)
	if err := validateVaultRef(ref); err != nil {
		return err
	}
	return p.do(ctx, http.MethodDelete, p.metadataPath(ref.Key), nil, nil, nil, requestOptions{})
}

func (p *VaultKVV2Provider) Rotate(ctx context.Context, ref SecretRef) (SecretRef, error) {
	value, err := p.Resolve(ctx, ref)
	if err != nil {
		return SecretRef{}, err
	}
	ref.Version = ""
	return p.Put(ctx, ref, value)
}

func (p *VaultKVV2Provider) normalizeRef(ref SecretRef) SecretRef {
	ref.ProviderConfigID = firstNonEmpty(strings.TrimSpace(ref.ProviderConfigID), p.config.ID)
	ref.Scope = strings.TrimSpace(ref.Scope)
	ref.ResourceID = strings.TrimSpace(ref.ResourceID)
	ref.Field = strings.TrimSpace(ref.Field)
	if ref.Field == "" {
		ref.Field = "value"
	}
	ref.Key = strings.Trim(strings.TrimSpace(ref.Key), "/")
	ref.Version = strings.TrimSpace(ref.Version)
	return ref
}

func validateVaultRef(ref SecretRef) error {
	if strings.TrimSpace(ref.ProviderConfigID) == "" {
		return fmt.Errorf("%w: providerConfigId is required", ErrInvalidSecretRef)
	}
	if strings.TrimSpace(ref.Key) == "" {
		return fmt.Errorf("%w: key is required", ErrInvalidSecretRef)
	}
	// The key is user-controlled and is later joined under the configured
	// mount/path prefix. path.Join (in joinVaultPath) collapses "." and ".."
	// segments, so a key like "../../other/data/prod" would escape the prefix and
	// let the provider token reach an unrelated Vault path. Reject relative
	// segments outright rather than relying on path cleaning.
	if hasRelativePathSegment(ref.Key) {
		return fmt.Errorf("%w: key must not contain '.' or '..' path segments", ErrInvalidSecretRef)
	}
	if strings.TrimSpace(ref.Field) == "" {
		return fmt.Errorf("%w: field is required", ErrInvalidSecretRef)
	}
	return nil
}

func hasRelativePathSegment(key string) bool {
	for _, segment := range strings.Split(key, "/") {
		switch strings.TrimSpace(segment) {
		case ".", "..":
			return true
		}
	}
	return false
}

func (p *VaultKVV2Provider) dataPath(key string) string {
	return joinVaultPath(p.vault.Mount, "data", p.vault.PathPrefix, key)
}

func (p *VaultKVV2Provider) metadataPath(key string) string {
	return joinVaultPath(p.vault.Mount, "metadata", p.vault.PathPrefix, key)
}

type requestOptions struct {
	AllowUnauthenticated bool
	AllowHealthCodes     bool
}

func (p *VaultKVV2Provider) do(ctx context.Context, method, apiPath string, query url.Values, body any, out any, opts requestOptions) error {
	rawURL, err := url.JoinPath(strings.TrimRight(p.vault.Address, "/"), "v1", apiPath)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(p.vault.Namespace) != "" {
		req.Header.Set("X-Vault-Namespace", strings.TrimSpace(p.vault.Namespace))
	}
	if !opts.AllowUnauthenticated {
		token, err := p.token(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()
	if opts.AllowHealthCodes && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 472 || resp.StatusCode == 473) {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return vaultHTTPError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode vault response: %v", ErrProviderUnavailable, err)
	}
	return nil
}

func vaultHTTPError(resp *http.Response) error {
	var payload struct {
		Errors []string `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	message := strings.Join(payload.Errors, "; ")
	if message == "" {
		message = resp.Status
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrSecretNotFound, message)
	default:
		return fmt.Errorf("%w: vault http %d: %s", ErrProviderUnavailable, resp.StatusCode, message)
	}
}

func (p *VaultKVV2Provider) token(ctx context.Context) (string, error) {
	switch strings.ToLower(strings.TrimSpace(p.vault.AuthMethod)) {
	case "", "token", "env":
		return p.tokenFromEnvOrFile()
	case "agent-token-file":
		return readSecretFile(firstNonEmpty(p.vault.AgentTokenSink, p.vault.TokenFile))
	case "approle":
		return p.appRoleToken(ctx)
	default:
		return "", fmt.Errorf("%w: unsupported vault auth method %q", ErrUnsupported, p.vault.AuthMethod)
	}
}

func (p *VaultKVV2Provider) tokenFromEnvOrFile() (string, error) {
	envName := strings.TrimSpace(p.vault.TokenEnv)
	tokenFile := strings.TrimSpace(p.vault.TokenFile)

	// An explicitly configured token env var wins: the operator named it deliberately.
	if envName != "" {
		if token := strings.TrimSpace(os.Getenv(envName)); token != "" {
			return token, nil
		}
	}
	// A configured token file outranks the ambient default VAULT_TOKEN so a
	// file-backed provider authenticates with its own configured identity rather
	// than an unrelated token that merely happens to sit in the process environment.
	if tokenFile != "" {
		return readSecretFile(tokenFile)
	}
	// If a token source was configured explicitly (tokenEnv) but yielded nothing,
	// do NOT silently fall back to an ambient process-wide token — that could
	// authenticate to the configured Vault with an unrelated identity. Require the
	// configured source to be satisfied instead.
	if envName != "" {
		return "", fmt.Errorf("%w: configured vault token env %q is empty", ErrProviderUnavailable, envName)
	}
	// Nothing configured: fall back to the conventional ambient sources.
	if token := strings.TrimSpace(os.Getenv("VAULT_TOKEN")); token != "" {
		return token, nil
	}
	if ambientFile := strings.TrimSpace(os.Getenv("VAULT_TOKEN_FILE")); ambientFile != "" {
		return readSecretFile(ambientFile)
	}
	return "", fmt.Errorf("%w: vault token is not configured", ErrProviderUnavailable)
}

func (p *VaultKVV2Provider) appRoleToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.cachedToken != "" && time.Until(p.tokenExpiry) > time.Minute {
		token := p.cachedToken
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()

	roleID, err := readEnvOrFile(p.vault.RoleIDEnv, p.vault.RoleIDFile)
	if err != nil {
		return "", fmt.Errorf("%w: approle role_id: %v", ErrProviderUnavailable, err)
	}
	secretID, err := readEnvOrFile(p.vault.SecretIDEnv, p.vault.SecretIDFile)
	if err != nil {
		return "", fmt.Errorf("%w: approle secret_id: %v", ErrProviderUnavailable, err)
	}
	mount := firstNonEmpty(p.vault.AppRoleMount, "approle")
	var resp struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	body := map[string]any{"role_id": roleID, "secret_id": secretID}
	if err := p.do(ctx, http.MethodPost, joinVaultPath("auth", mount, "login"), nil, body, &resp, requestOptions{AllowUnauthenticated: true}); err != nil {
		return "", err
	}
	token := strings.TrimSpace(resp.Auth.ClientToken)
	if token == "" {
		return "", fmt.Errorf("%w: approle login returned no client token", ErrProviderUnavailable)
	}
	ttl := time.Duration(resp.Auth.LeaseDuration) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	p.mu.Lock()
	p.cachedToken = token
	p.tokenExpiry = time.Now().Add(ttl)
	p.mu.Unlock()
	return token, nil
}

func readEnvOrFile(envName, filePath string) (string, error) {
	if strings.TrimSpace(envName) != "" {
		if value := strings.TrimSpace(os.Getenv(strings.TrimSpace(envName))); value != "" {
			return value, nil
		}
	}
	if strings.TrimSpace(filePath) != "" {
		return readSecretFile(filePath)
	}
	return "", errors.New("missing env/file")
}

func readSecretFile(filePath string) (string, error) {
	content, err := os.ReadFile(strings.TrimSpace(filePath))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", errors.New("secret file is empty")
	}
	return value, nil
}

func joinVaultPath(parts ...string) string {
	var out []string
	for _, part := range parts {
		for _, segment := range strings.Split(strings.Trim(part, "/"), "/") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			out = append(out, url.PathEscape(segment))
		}
	}
	return path.Join(out...)
}

func fingerprintRef(ref SecretRef) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		ref.ProviderConfigID,
		ref.Scope,
		ref.ResourceID,
		ref.Key,
		ref.Field,
		ref.Version,
	}, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
