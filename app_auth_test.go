package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/securefile"
)

func TestHandleOpenURLCompletesPendingLoginFromCallback(t *testing.T) {
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
			},
			"license": map[string]any{
				"plan":   "pro",
				"status": "active",
			},
		})
	}))
	defer srv.Close()

	store := auth.NewStore(filepath.Join(t.TempDir(), "auth-session.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	state := store.Current()
	state.PendingLogin = &auth.PendingLogin{
		SessionID:    "session_123",
		CodeVerifier: "verifier_123",
		LoginURL:     srv.URL + "/app?session_id=session_123",
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var emittedName string
	var emittedState auth.State
	app := &App{
		authStore: store,
		authService: auth.NewService(auth.ServiceConfig{
			BaseURL:    srv.URL,
			Store:      store,
			HTTPClient: srv.Client(),
		}),
		emitEvent: func(ctx context.Context, eventName string, data ...interface{}) {
			emittedName = eventName
			if len(data) == 1 {
				if next, ok := data[0].(auth.State); ok {
					emittedState = next
				}
			}
		},
	}

	app.handleOpenURL("futrix://callback?code=one-time-authorization-code")

	var current auth.State
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current = store.Current()
		if current.Session != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if current.Session == nil {
		t.Fatalf("expected callback to complete login")
	}
	if current.PendingLogin != nil {
		t.Fatalf("expected pending login to be cleared")
	}
	if requestBody["code"] != "one-time-authorization-code" {
		t.Fatalf("expected callback authorization code to be exchanged as-is, got %#v", requestBody["code"])
	}
	if emittedName != "auth:state" {
		t.Fatalf("expected auth:state event, got %q", emittedName)
	}
	if emittedState.Session == nil || emittedState.Session.User.Email != "user@example.com" {
		t.Fatalf("expected emitted state to include session, got %#v", emittedState)
	}
}

func TestHandleOpenURLEmitsCodexConnectRequestBeforeAuthorization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	key := bytes.Repeat([]byte{7}, 32)
	securefile.SetKey(key)
	t.Cleanup(securefile.ResetForTest)

	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	var emittedName string
	var emittedPayload map[string]any
	app := &App{
		cfg: Config{DataPath: dataPath},
		ctx: context.Background(),
		emitEvent: func(ctx context.Context, eventName string, data ...interface{}) {
			emittedName = eventName
			if len(data) == 1 {
				if payload, ok := data[0].(map[string]any); ok {
					emittedPayload = payload
				}
			}
		},
	}

	app.handleOpenURL("futrixdata://codex/connect?source=codex-plugin")

	bridgePath := filepath.Join(home, ".futrixdata", "codex-plugin.json")
	if _, err := os.Stat(bridgePath); !os.IsNotExist(err) {
		t.Fatalf("expected deep link to wait for user confirmation before writing bridge, stat err=%v", err)
	}
	if emittedName != "codex:connect-request" {
		t.Fatalf("expected codex:connect-request event, got %q", emittedName)
	}
	if emittedPayload["source"] != "codex-plugin" {
		t.Fatalf("expected source to round-trip, got %#v", emittedPayload["source"])
	}

	result := app.AuthorizeCodexPlugin()
	if len(result.Installed) != 1 || !result.Installed[0].Success {
		t.Fatalf("expected confirmed authorization to succeed, got %#v", result.Installed)
	}
	bridge, err := os.ReadFile(bridgePath)
	if err != nil {
		t.Fatalf("read codex plugin bridge: %v", err)
	}
	content := string(bridge)
	for _, token := range []string{
		"futrixdata-cli",
		"accessKey",
		"agent_",
	} {
		if !strings.Contains(content, token) {
			t.Fatalf("expected bridge to contain %q, got %s", token, content)
		}
	}

	identities, err := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath)).ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("expected one codex identity, got %#v", identities)
	}
	if identities[0].AgentType != "codex" {
		t.Fatalf("identity agent type = %q, want codex", identities[0].AgentType)
	}
}

func TestHandleLaunchArgsForwardsFutrixDataCodexDeepLink(t *testing.T) {
	var emittedName string
	var emittedPayload map[string]any
	app := &App{
		cfg: Config{DataPath: filepath.Join(t.TempDir(), "datasources.json")},
		ctx: context.Background(),
		emitEvent: func(ctx context.Context, eventName string, data ...interface{}) {
			emittedName = eventName
			if len(data) == 1 {
				if payload, ok := data[0].(map[string]any); ok {
					emittedPayload = payload
				}
			}
		},
	}

	app.handleLaunchArgs([]string{
		"--already-running",
		"futrixdata://codex/connect?source=codex-plugin",
	})

	if emittedName != "codex:connect-request" {
		t.Fatalf("expected second-instance futrixdata deep link to emit codex:connect-request, got %q", emittedName)
	}
	if emittedPayload["source"] != "codex-plugin" {
		t.Fatalf("expected source to round-trip, got %#v", emittedPayload["source"])
	}
}
