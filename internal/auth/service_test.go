package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/securefile"
)

func TestServiceStartLoginPersistsPendingLoginAndOpensBrowser(t *testing.T) {
	var openedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}

	service := NewService(ServiceConfig{
		BaseURL:    srv.URL,
		Store:      store,
		OpenURL:    func(rawURL string) error { openedURL = rawURL; return nil },
		HTTPClient: srv.Client(),
	})

	started, err := service.StartLogin(context.Background(), StartLoginInput{})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	if started.LoginURL == "" {
		t.Fatalf("expected login url")
	}
	if openedURL != started.LoginURL {
		t.Fatalf("expected browser open url %q, got %q", started.LoginURL, openedURL)
	}

	current := store.Current()
	if current.PendingLogin == nil {
		t.Fatalf("expected pending login to be saved")
	}
	if current.PendingLogin.SessionID == "" {
		t.Fatalf("expected pending session id to be saved")
	}
	if current.PendingLogin.CodeVerifier == "" {
		t.Fatalf("expected code verifier to be saved")
	}
	if !strings.Contains(started.LoginURL, "session_id=") {
		t.Fatalf("expected session_id in login url, got %q", started.LoginURL)
	}
	if !strings.Contains(started.LoginURL, "code_challenge=") {
		t.Fatalf("expected code_challenge in login url, got %q", started.LoginURL)
	}
}

func TestServiceCompleteAuthLoginExchangesCodeAndPersistsSession(t *testing.T) {
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/exchange" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access_2",
			"refresh_token": "refresh_2",
			"expires_in":    900,
			"user": map[string]any{
				"id":           "user_123",
				"email":        "user@example.com",
				"display_name": "Auth User",
				"avatar_url":   "https://example.com/avatar.png",
			},
			"license": map[string]any{
				"plan":       "pro",
				"status":     "active",
				"expires_at": 1800000000,
			},
		})
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	state := store.Current()
	state.PendingLogin = &PendingLogin{
		SessionID:    "session_123",
		CodeVerifier: "verifier_123",
		LoginURL:     srv.URL + "/app?session_id=session_123",
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save pending login: %v", err)
	}

	service := NewService(ServiceConfig{
		BaseURL:    srv.URL,
		Store:      store,
		HTTPClient: srv.Client(),
	})

	next, err := service.CompleteAuthLogin(context.Background(), "A3F-K9M")
	if err != nil {
		t.Fatalf("complete login: %v", err)
	}
	if requestBody["code"] != "A3FK9M" {
		t.Fatalf("expected manual code in exchange payload, got %#v", requestBody["code"])
	}
	if requestBody["code_verifier"] != "verifier_123" {
		t.Fatalf("expected saved verifier in exchange payload, got %#v", requestBody["code_verifier"])
	}
	if next.Session == nil {
		t.Fatalf("expected session to be saved after exchange")
	}
	if next.Session.User.Email != "user@example.com" {
		t.Fatalf("expected exchanged user email, got %q", next.Session.User.Email)
	}
	if next.PendingLogin != nil {
		t.Fatalf("expected pending login to be cleared after exchange")
	}
}

func TestServiceCompleteAuthLoginPreservesOneTimeCodeFormat(t *testing.T) {
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/exchange" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access_2",
			"refresh_token": "refresh_2",
			"expires_in":    900,
			"user": map[string]any{
				"id":    "user_123",
				"email": "user@example.com",
			},
			"license": map[string]any{
				"plan":   "free",
				"status": "active",
			},
		})
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	state := store.Current()
	state.PendingLogin = &PendingLogin{
		SessionID:    "session_123",
		CodeVerifier: "verifier_123",
		LoginURL:     srv.URL + "/app?session_id=session_123",
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save pending login: %v", err)
	}

	service := NewService(ServiceConfig{
		BaseURL:    srv.URL,
		Store:      store,
		HTTPClient: srv.Client(),
	})

	if _, err := service.CompleteAuthLogin(context.Background(), "raw-one-time-code"); err != nil {
		t.Fatalf("complete login: %v", err)
	}
	if requestBody["code"] != "raw-one-time-code" {
		t.Fatalf("expected one-time code to keep separators, got %#v", requestBody["code"])
	}
}

func TestSaveEncryptionKeyDoesNotReplaceLocalRootKey(t *testing.T) {
	securefile.ResetForTest()
	t.Cleanup(securefile.ResetForTest)

	secrets := map[string]string{}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			value, ok := secrets[service+"/"+account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return value, nil
		},
		func(service, account, secret string) error {
			secrets[service+"/"+account] = secret
			return nil
		},
	)
	t.Cleanup(restore)

	localRoot := bytes.Repeat([]byte{4}, 32)
	serverKey := bytes.Repeat([]byte{5}, 32)
	securefile.SetKeys(localRoot)

	saveEncryptionKey(base64.RawURLEncoding.EncodeToString(serverKey))

	if got := securefile.Key(); !bytes.Equal(got, localRoot) {
		t.Fatalf("expected local root key to remain primary")
	}

	path := filepath.Join(t.TempDir(), "legacy.json")
	securefile.SetKey(serverKey)
	if err := securefile.WriteFile(path, []byte(`{"legacy":true}`), 0o600); err != nil {
		t.Fatalf("write legacy encrypted file: %v", err)
	}
	securefile.SetKeys(localRoot)
	saveEncryptionKey(base64.RawURLEncoding.EncodeToString(serverKey))
	if _, err := securefile.ReadFile(path); err != nil {
		t.Fatalf("expected saved server key to remain usable as fallback: %v", err)
	}
}

func TestServiceEnsureAuthenticatedRefreshesExpiredSession(t *testing.T) {
	var refreshBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client/refresh" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&refreshBody); err != nil {
			t.Fatalf("decode refresh request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access_refreshed",
			"refresh_token": "refresh_rotated",
			"expires_in":    900,
			"license": map[string]any{
				"plan":       "free",
				"status":     "expired",
				"expires_at": 1800000001,
			},
		})
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	current := store.Current()
	current.Session = &Session{
		AccessToken:  "access_old",
		RefreshToken: "refresh_old",
		ExpiresAt:    time.Now().Unix() - 60,
		User: User{
			ID:          "user_123",
			Email:       "user@example.com",
			DisplayName: "Auth User",
		},
		License: License{
			Plan:   "pro",
			Status: "active",
		},
	}
	if err := store.Save(current); err != nil {
		t.Fatalf("save current session: %v", err)
	}

	service := NewService(ServiceConfig{
		BaseURL:    srv.URL,
		Store:      store,
		HTTPClient: srv.Client(),
		Now:        time.Now,
	})

	next, err := service.EnsureAuthenticated(context.Background())
	if err != nil {
		t.Fatalf("ensure authenticated: %v", err)
	}
	if refreshBody["refresh_token"] != "refresh_old" {
		t.Fatalf("expected refresh token in refresh payload, got %#v", refreshBody["refresh_token"])
	}
	if refreshBody["device_id"] != current.DeviceID {
		t.Fatalf("expected device id %q in refresh payload, got %#v", current.DeviceID, refreshBody["device_id"])
	}
	if next.Session == nil {
		t.Fatalf("expected refreshed session")
	}
	if next.Session.AccessToken != "access_refreshed" {
		t.Fatalf("expected refreshed access token, got %q", next.Session.AccessToken)
	}
	if next.Session.RefreshToken != "refresh_rotated" {
		t.Fatalf("expected rotated refresh token, got %q", next.Session.RefreshToken)
	}
	if next.Session.License.Plan != "free" {
		t.Fatalf("expected refreshed license plan, got %q", next.Session.License.Plan)
	}
}

func TestAPIErrorError_DeviceLimitExceededReturnsStablePlanLimitCode(t *testing.T) {
	err := (&APIError{
		Code:  "device_limit_exceeded",
		Limit: 1,
	}).Error()

	if err != "plan_limit_exceeded:devices:free:1" {
		t.Fatalf("expected stable device limit error, got %q", err)
	}
}
