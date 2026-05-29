package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"futrixdata/platform/internal/securefile"
)

func TestSessionStoreLoadCreatesStableDeviceID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-session.json")

	first := NewStore(path)
	if err := first.Load(); err != nil {
		t.Fatalf("first load: %v", err)
	}
	firstState := first.Current()
	if firstState.DeviceID == "" {
		t.Fatalf("expected device id to be generated on first load")
	}
	if firstState.Session != nil {
		t.Fatalf("expected empty auth session on first load")
	}
	if firstState.Trial == nil {
		t.Fatalf("expected local trial to be initialized on first load")
	}
	if firstState.Trial.StartedAt <= 0 || firstState.Trial.ExpiresAt <= firstState.Trial.StartedAt {
		t.Fatalf("expected valid trial window, got %#v", firstState.Trial)
	}
	gotDuration := time.Duration(firstState.Trial.ExpiresAt-firstState.Trial.StartedAt) * time.Second
	if gotDuration != LocalTrialDuration {
		t.Fatalf("expected trial duration %s, got %s", LocalTrialDuration, gotDuration)
	}

	second := NewStore(path)
	if err := second.Load(); err != nil {
		t.Fatalf("second load: %v", err)
	}
	secondState := second.Current()
	if secondState.DeviceID != firstState.DeviceID {
		t.Fatalf("expected stable device id %q, got %q", firstState.DeviceID, secondState.DeviceID)
	}
	if secondState.Trial == nil || secondState.Trial.StartedAt != firstState.Trial.StartedAt || secondState.Trial.ExpiresAt != firstState.Trial.ExpiresAt {
		t.Fatalf("expected stable trial %#v, got %#v", firstState.Trial, secondState.Trial)
	}
}

func TestSessionStoreLoadRestoresExpiredTrialAfterSessionFileDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-session.json")
	now := time.Now()
	expired := &Trial{
		StartedAt: now.Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state := store.Current()
	state.Trial = expired
	if err := store.Save(state); err != nil {
		t.Fatalf("save expired trial: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove session file: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload after session deletion: %v", err)
	}
	got := reloaded.Current().Trial
	if got == nil {
		t.Fatalf("expected trial marker to restore trial")
	}
	if got.StartedAt != expired.StartedAt || got.ExpiresAt != expired.ExpiresAt {
		t.Fatalf("expected expired trial marker %#v, got %#v", expired, got)
	}
	if got.ExpiresAt > time.Now().Unix() {
		t.Fatalf("expected restored trial to remain expired, got %#v", got)
	}
}

func TestSessionStoreLoadRestoresTrialWhenSessionStateDropsTrial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-session.json")
	now := time.Now()
	expired := &Trial{
		StartedAt: now.Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state := store.Current()
	state.Trial = expired
	if err := store.Save(state); err != nil {
		t.Fatalf("save expired trial: %v", err)
	}

	payload, err := json.MarshalIndent(State{DeviceID: state.DeviceID}, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	if err := securefile.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload legacy state: %v", err)
	}
	got := reloaded.Current().Trial
	if got == nil {
		t.Fatalf("expected trial marker to backfill legacy state")
	}
	if got.StartedAt != expired.StartedAt || got.ExpiresAt != expired.ExpiresAt {
		t.Fatalf("expected expired trial marker %#v, got %#v", expired, got)
	}
}

func TestSessionStoreLoadPrefersExpiredTrialMarkerOverOlderActiveSessionTrial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth-session.json")
	now := time.Now()
	expired := &Trial{
		StartedAt: now.Add(-31 * 24 * time.Hour).Unix(),
		ExpiresAt: now.Add(-24 * time.Hour).Unix(),
	}
	active := &Trial{
		StartedAt: now.Add(-time.Hour).Unix(),
		ExpiresAt: now.Add(LocalTrialDuration).Unix(),
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	state := store.Current()
	state.Trial = expired
	if err := store.Save(state); err != nil {
		t.Fatalf("save expired marker: %v", err)
	}

	legacy := State{
		DeviceID: state.DeviceID,
		Trial:    active,
	}
	payload, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy state: %v", err)
	}
	if err := securefile.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write older active session state: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload with older active session trial: %v", err)
	}
	got := reloaded.Current().Trial
	if got == nil {
		t.Fatalf("expected expired marker to win over active session trial")
	}
	if got.StartedAt != expired.StartedAt || got.ExpiresAt != expired.ExpiresAt {
		t.Fatalf("expected expired marker %#v to win over active session trial %#v, got %#v", expired, active, got)
	}
}

func TestSessionStoreSavePersistsSessionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-session.json")

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	current := store.Current()
	current.Session = &Session{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    1700000000,
		User: User{
			ID:          "user_123",
			Email:       "user@example.com",
			DisplayName: "Auth User",
			AvatarURL:   "https://example.com/avatar.png",
		},
		License: License{
			Plan:      "pro",
			Status:    "active",
			ExpiresAt: 1800000000,
		},
	}
	if err := store.Save(current); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	state := reloaded.Current()
	if state.DeviceID != current.DeviceID {
		t.Fatalf("expected device id %q, got %q", current.DeviceID, state.DeviceID)
	}
	if state.Session == nil {
		t.Fatalf("expected session to persist")
	}
	if state.Session.AccessToken != "access-token" {
		t.Fatalf("expected access token to persist, got %q", state.Session.AccessToken)
	}
	if state.Session.RefreshToken != "refresh-token" {
		t.Fatalf("expected refresh token to persist, got %q", state.Session.RefreshToken)
	}
	if state.Session.User.Email != "user@example.com" {
		t.Fatalf("expected user email to persist, got %q", state.Session.User.Email)
	}
	if state.Session.License.Plan != "pro" {
		t.Fatalf("expected license plan to persist, got %q", state.Session.License.Plan)
	}
}
