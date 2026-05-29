package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/planlimits"
	"futrixdata/platform/internal/securefile"
)

type ServiceConfig struct {
	BaseURL    string
	Store      *Store
	OpenURL    func(rawURL string) error
	HTTPClient *http.Client
	Now        func() time.Time
	DeviceName string
	Platform   string
}

const DefaultBaseURL = "https://futrixdata.com"

type Service struct {
	baseURL    string
	store      *Store
	openURL    func(rawURL string) error
	httpClient *http.Client
	now        func() time.Time
	deviceName string
	platform   string
}

type APIError struct {
	StatusCode int          `json:"-"`
	Code       string       `json:"error,omitempty"`
	Message    string       `json:"message,omitempty"`
	Limit      int          `json:"limit,omitempty"`
	Devices    []DeviceInfo `json:"devices,omitempty"`
}

type pollResponse struct {
	Status string `json:"status"`
	Code   string `json:"code"`
}

type tokenResponse struct {
	AccessToken   string       `json:"access_token"`
	RefreshToken  string       `json:"refresh_token"`
	ExpiresIn     int64        `json:"expires_in"`
	EncryptionKey string       `json:"encryption_key"`
	User          tokenUser    `json:"user"`
	License       tokenLicense `json:"license"`
}

type tokenUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type tokenLicense struct {
	Plan      string `json:"plan"`
	Status    string `json:"status"`
	ExpiresAt *int64 `json:"expires_at"`
}

type apiErrorPayload struct {
	Error   string       `json:"error"`
	Message string       `json:"message"`
	Limit   int          `json:"limit"`
	Devices []DeviceInfo `json:"devices"`
}

type logoutResponse struct {
	Success bool `json:"success"`
}

type removeDeviceResponse struct {
	Success bool `json:"success"`
}

func NewService(cfg ServiceConfig) *Service {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		store:      cfg.Store,
		openURL:    cfg.OpenURL,
		httpClient: httpClient,
		now:        now,
		deviceName: defaultDeviceName(cfg.DeviceName),
		platform:   defaultPlatform(cfg.Platform),
	}
}

func (s *Service) Current(ctx context.Context) (State, error) {
	_ = ctx
	if s.store == nil {
		return State{}, nil
	}
	return s.store.Current(), nil
}

func (s *Service) StartLogin(ctx context.Context, input StartLoginInput) (LoginStart, error) {
	if s.store == nil {
		return LoginStart{}, errors.New("auth store is not configured")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return LoginStart{}, errors.New("auth service base url is not configured")
	}
	state := s.store.Current()
	pending := &PendingLogin{
		SessionID:    randomBase64URL(24),
		CodeVerifier: randomBase64URL(32),
	}
	challenge := sha256Base64URL(pending.CodeVerifier)
	loginURL, err := buildLoginURL(s.baseURL, pending.SessionID, challenge)
	if err != nil {
		return LoginStart{}, err
	}
	pending.LoginURL = loginURL
	state.PendingLogin = pending
	if err := s.store.Save(state); err != nil {
		return LoginStart{}, err
	}
	if !input.NoBrowser && s.openURL != nil {
		if err := s.openURL(loginURL); err != nil {
			return LoginStart{}, err
		}
	}
	return LoginStart{
		LoginURL:  loginURL,
		SessionID: pending.SessionID,
	}, nil
}

func (s *Service) PollLogin(ctx context.Context) (LoginPoll, error) {
	if s.store == nil {
		return LoginPoll{}, errors.New("auth store is not configured")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return LoginPoll{}, errors.New("auth service base url is not configured")
	}
	state := s.store.Current()
	if state.PendingLogin == nil || strings.TrimSpace(state.PendingLogin.SessionID) == "" {
		return LoginPoll{}, errors.New("login is not pending")
	}
	endpoint, err := url.Parse(s.baseURL)
	if err != nil {
		return LoginPoll{}, err
	}
	endpoint.Path = path.Join(endpoint.Path, "/api/client/poll")
	query := endpoint.Query()
	query.Set("session_id", state.PendingLogin.SessionID)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return LoginPoll{}, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return LoginPoll{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return LoginPoll{}, decodeAPIError(resp)
	}
	var payload pollResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return LoginPoll{}, err
	}
	return LoginPoll{
		Status: strings.TrimSpace(payload.Status),
		Code:   strings.TrimSpace(payload.Code),
	}, nil
}

func (s *Service) CompleteAuthLogin(ctx context.Context, code string) (State, error) {
	if s.store == nil {
		return State{}, errors.New("auth store is not configured")
	}
	state := s.store.Current()
	if state.PendingLogin == nil {
		return State{}, errors.New("login is not pending")
	}
	next, err := s.exchangeCode(ctx, state, normalizeExchangeCode(code))
	if err != nil {
		return State{}, err
	}
	if err := s.store.Save(next); err != nil {
		return State{}, err
	}
	return next, nil
}

func (s *Service) EnsureAuthenticated(ctx context.Context) (State, error) {
	if s.store == nil {
		return State{}, nil
	}
	state := s.store.Current()
	if state.Session == nil {
		return state, ErrLoginRequired
	}
	if state.Session.ExpiresAt <= 0 || s.now().Unix() < state.Session.ExpiresAt {
		return state, nil
	}
	refreshed, err := s.refreshSession(ctx, state)
	if err != nil {
		if errors.Is(err, ErrLoginRequired) {
			cleared := state
			cleared.Session = nil
			cleared.PendingLogin = nil
			_ = s.store.Save(cleared)
			return cleared, err
		}
		return state, err
	}
	if err := s.store.Save(refreshed); err != nil {
		return State{}, err
	}
	return refreshed, nil
}

func (s *Service) Logout(ctx context.Context) (State, error) {
	if s.store == nil {
		return State{}, nil
	}
	state := s.store.Current()
	if state.Session != nil && strings.TrimSpace(state.Session.RefreshToken) != "" && strings.TrimSpace(s.baseURL) != "" {
		payload := map[string]any{
			"refresh_token": state.Session.RefreshToken,
			"device_id":     state.DeviceID,
		}
		var out logoutResponse
		err := s.doJSON(ctx, http.MethodPost, "/api/client/logout", payload, "", &out)
		if err != nil && !isIgnorableLogoutError(err) {
			return state, err
		}
	}
	state.Session = nil
	state.PendingLogin = nil
	if err := s.store.Save(state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Service) ListDevices(ctx context.Context) (DeviceList, error) {
	state, err := s.EnsureAuthenticated(ctx)
	if err != nil {
		return DeviceList{}, err
	}
	var result DeviceList
	err = s.doJSON(ctx, http.MethodGet, "/api/client/devices", nil, state.Session.AccessToken, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			_, _ = s.Logout(context.Background())
			return DeviceList{}, ErrLoginRequired
		}
		return DeviceList{}, err
	}
	result.License = s.cloneCurrentLicense()
	return result, nil
}

// cloneCurrentLicense returns a pointer to a copy of the active session's
// license, or nil when no session is loaded. Callers expose this on
// DeviceList so the frontend can reconcile its stale local plan/status copy
// after EnsureAuthenticated refreshes the backend session.
func (s *Service) cloneCurrentLicense() *License {
	if s == nil || s.store == nil {
		return nil
	}
	state := s.store.Current()
	if state.Session == nil {
		return nil
	}
	clone := state.Session.License
	return &clone
}

func (s *Service) RemoveDevice(ctx context.Context, deviceID string) (DeviceList, error) {
	state, err := s.EnsureAuthenticated(ctx)
	if err != nil {
		return DeviceList{}, err
	}
	target := strings.TrimSpace(deviceID)
	if target == "" {
		return DeviceList{}, errors.New("device id is required")
	}
	var out removeDeviceResponse
	if err := s.doJSON(ctx, http.MethodDelete, "/api/client/devices/"+url.PathEscape(target), nil, state.Session.AccessToken, &out); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			_, _ = s.Logout(context.Background())
			return DeviceList{}, ErrLoginRequired
		}
		return DeviceList{}, err
	}
	if target == state.DeviceID {
		if _, err := s.Logout(ctx); err != nil {
			return DeviceList{}, err
		}
		return DeviceList{}, nil
	}
	return s.ListDevices(ctx)
}

// GetJSON performs an authenticated GET against the configured FutrixServer
// base URL and decodes the response body into out. The active session's
// access token is sent as a Bearer credential. On 401 the local session is
// cleared and ErrLoginRequired is returned (matching ListDevices behavior),
// so callers can prompt the user to sign in again.
func (s *Service) GetJSON(ctx context.Context, requestPath string, out any) error {
	state, err := s.EnsureAuthenticated(ctx)
	if err != nil {
		return err
	}
	if err := s.doJSON(ctx, http.MethodGet, requestPath, nil, state.Session.AccessToken, out); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			_, _ = s.Logout(context.Background())
			return ErrLoginRequired
		}
		return err
	}
	return nil
}

func (s *Service) exchangeCode(ctx context.Context, state State, code string) (State, error) {
	if strings.TrimSpace(s.baseURL) == "" {
		return State{}, errors.New("auth service base url is not configured")
	}
	payload := map[string]any{
		"code":          code,
		"code_verifier": state.PendingLogin.CodeVerifier,
		"device_id":     state.DeviceID,
		"device_name":   s.deviceName,
		"platform":      s.platform,
	}
	var result tokenResponse
	if err := s.doJSON(ctx, http.MethodPost, "/api/client/exchange", payload, "", &result); err != nil {
		return State{}, err
	}
	saveEncryptionKey(result.EncryptionKey)
	state.Session = tokenResponseToSession(result, s.now())
	state.PendingLogin = nil
	return state, nil
}

func (s *Service) refreshSession(ctx context.Context, state State) (State, error) {
	if strings.TrimSpace(s.baseURL) == "" {
		return state, errors.New("auth service base url is not configured")
	}
	payload := map[string]any{
		"refresh_token": state.Session.RefreshToken,
		"device_id":     state.DeviceID,
	}
	var result tokenResponse
	if err := s.doJSON(ctx, http.MethodPost, "/api/client/refresh", payload, "", &result); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			switch apiErr.Code {
			case "invalid_refresh_token", "device_revoked", "device_limit_exceeded":
				return state, ErrLoginRequired
			}
		}
		return state, err
	}
	saveEncryptionKey(result.EncryptionKey)
	state.Session.AccessToken = result.AccessToken
	state.Session.RefreshToken = result.RefreshToken
	if result.ExpiresIn > 0 {
		state.Session.ExpiresAt = s.now().Unix() + result.ExpiresIn
	}
	state.Session.License = tokenResponseToSession(result, s.now()).License
	return state, nil
}

func (s *Service) doJSON(ctx context.Context, method, requestPath string, payload any, bearerToken string, out any) error {
	if strings.TrimSpace(s.baseURL) == "" {
		return errors.New("auth service base url is not configured")
	}
	endpoint, err := url.Parse(s.baseURL)
	if err != nil {
		return err
	}
	endpoint.Path = path.Join(endpoint.Path, requestPath)
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return decodeAPIError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func decodeAPIError(resp *http.Response) error {
	var payload apiErrorPayload
	body, _ := io.ReadAll(resp.Body)
	if len(body) > 0 {
		_ = json.Unmarshal(body, &payload)
	}
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Code:       strings.TrimSpace(payload.Error),
		Message:    strings.TrimSpace(payload.Message),
		Limit:      payload.Limit,
		Devices:    payload.Devices,
	}
	if apiErr.Code == "" && apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(body))
	}
	if apiErr.Code == "" && apiErr.Message == "" {
		apiErr.Message = resp.Status
	}
	return apiErr
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Code) == "device_limit_exceeded" {
		return planlimits.DeviceLimitError(planlimits.PlanForDeviceLimit(e.Limit), e.Limit)
	}
	if strings.TrimSpace(e.Message) != "" {
		return strings.TrimSpace(e.Message)
	}
	if strings.TrimSpace(e.Code) != "" {
		return strings.TrimSpace(e.Code)
	}
	return "auth request failed"
}

func buildLoginURL(baseURL, sessionID, codeChallenge string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	target.Path = path.Join(target.Path, "/app")
	query := target.Query()
	query.Set("session_id", strings.TrimSpace(sessionID))
	query.Set("code_challenge", strings.TrimSpace(codeChallenge))
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func tokenResponseToSession(payload tokenResponse, now time.Time) *Session {
	expiresAt := int64(0)
	if payload.ExpiresIn > 0 {
		expiresAt = now.Unix() + payload.ExpiresIn
	}
	licenseExpiresAt := int64(0)
	if payload.License.ExpiresAt != nil {
		licenseExpiresAt = *payload.License.ExpiresAt
	}
	return &Session{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    expiresAt,
		User: User{
			ID:          strings.TrimSpace(payload.User.ID),
			Email:       strings.TrimSpace(payload.User.Email),
			DisplayName: strings.TrimSpace(payload.User.DisplayName),
			AvatarURL:   strings.TrimSpace(payload.User.AvatarURL),
		},
		License: License{
			Plan:      strings.TrimSpace(payload.License.Plan),
			Status:    strings.TrimSpace(payload.License.Status),
			ExpiresAt: licenseExpiresAt,
		},
	}
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomBase64URL(byteCount int) string {
	if byteCount <= 0 {
		byteCount = 16
	}
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func defaultDeviceName(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	host, err := os.Hostname()
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return "FutrixData Device"
}

func defaultPlatform(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	default:
		return strings.TrimSpace(runtime.GOOS)
	}
}

func normalizeExchangeCode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	var compact strings.Builder
	compact.Grow(len(trimmed))
	for _, r := range trimmed {
		if r == '-' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		compact.WriteRune(r)
	}
	shortCode := compact.String()
	if len(shortCode) == 6 {
		return strings.ToUpper(shortCode)
	}
	return trimmed
}

func saveEncryptionKey(encKeyB64 string) {
	trimmed := strings.TrimSpace(encKeyB64)
	if trimmed == "" {
		return
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return
	}
	if len(keyBytes) != 32 {
		return
	}
	if existing := securefile.Key(); bytes.Equal(existing, keyBytes) {
		return
	}
	_ = keyring.Set(keyBytes)
	securefile.AddFallbackKey(keyBytes)
}

func isIgnorableLogoutError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == "invalid_refresh_token"
}
