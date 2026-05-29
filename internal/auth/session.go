package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/machineid"
	"futrixdata/platform/internal/securefile"
)

const (
	sessionFilename = "auth-session.json"
	trialFilename   = "auth-trial.json"
)

var ErrLoginRequired = errors.New("login required")

const LocalTrialDuration = 30 * 24 * time.Hour

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type License struct {
	Plan      string `json:"plan"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expiresAt"`
}

type Session struct {
	AccessToken  string  `json:"accessToken"`
	RefreshToken string  `json:"refreshToken"`
	ExpiresAt    int64   `json:"expiresAt"`
	User         User    `json:"user"`
	License      License `json:"license"`
}

type PendingLogin struct {
	SessionID    string `json:"sessionId"`
	CodeVerifier string `json:"codeVerifier"`
	LoginURL     string `json:"loginUrl"`
}

type Trial struct {
	StartedAt int64 `json:"startedAt"`
	ExpiresAt int64 `json:"expiresAt"`
}

type StartLoginInput struct {
	NoBrowser bool `json:"noBrowser"`
}

type LoginStart struct {
	LoginURL  string `json:"loginUrl"`
	SessionID string `json:"sessionId"`
}

type LoginPoll struct {
	Status string `json:"status"`
	Code   string `json:"code,omitempty"`
}

type DeviceInfo struct {
	DeviceID     string `json:"deviceId"`
	DeviceName   string `json:"deviceName"`
	Platform     string `json:"platform"`
	LastActiveAt int64  `json:"lastActiveAt"`
	CreatedAt    int64  `json:"createdAt"`
}

type DeviceList struct {
	Devices []DeviceInfo `json:"devices"`
	Limit   int          `json:"limit"`
	Plan    string       `json:"plan"`
	// License is the freshly-resolved license for the active session, or nil
	// when there is no session. It lets clients reconcile the local copy of
	// auth.Session.License after EnsureAuthenticated refreshed it, instead of
	// trusting whatever stale Plan/Status they last cached.
	License *License `json:"license,omitempty"`
}

type State struct {
	DeviceID     string        `json:"deviceId"`
	PendingLogin *PendingLogin `json:"pendingLogin,omitempty"`
	Session      *Session      `json:"session,omitempty"`
	Trial        *Trial        `json:"trial,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	current State
}

func NewStore(path string) *Store {
	return &Store{path: strings.TrimSpace(path)}
}

func PathForDataPath(dataPath string) string {
	return filepath.Join(filepath.Dir(strings.TrimSpace(dataPath)), sessionFilename)
}

func TrialPathForDataPath(dataPath string) string {
	return filepath.Join(filepath.Dir(strings.TrimSpace(dataPath)), trialFilename)
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	content, err := securefile.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if s.current.DeviceID == "" {
			s.current.DeviceID = newDeviceID()
		}
		if _, err := s.ensureTrialUnlocked(time.Now(), false); err != nil {
			return err
		}
		return s.saveUnlocked()
	}
	if len(content) > 0 {
		var next State
		if err := json.Unmarshal(content, &next); err != nil {
			return err
		}
		s.current = next
	}
	changed, err := s.ensureTrialUnlocked(time.Now(), false)
	if err != nil {
		return err
	}
	if s.current.DeviceID == "" {
		s.current.DeviceID = newDeviceID()
		changed = true
	}
	if changed {
		return s.saveUnlocked()
	}
	return nil
}

func (s *Store) Current() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Save(next State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(next.DeviceID) == "" {
		next.DeviceID = s.current.DeviceID
	}
	if strings.TrimSpace(next.DeviceID) == "" {
		next.DeviceID = newDeviceID()
	}
	s.current = next
	if _, err := s.ensureTrialUnlocked(time.Now(), true); err != nil {
		return err
	}
	return s.saveUnlocked()
}

func (s *Store) ClearSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current.Session = nil
	s.current.PendingLogin = nil
	if strings.TrimSpace(s.current.DeviceID) == "" {
		s.current.DeviceID = newDeviceID()
	}
	if _, err := s.ensureTrialUnlocked(time.Now(), false); err != nil {
		return err
	}
	return s.saveUnlocked()
}

func (s *Store) ensureTrialUnlocked(now time.Time, preferCurrent bool) (bool, error) {
	if now.IsZero() {
		now = time.Now()
	}
	changed := false
	if !preferCurrent || s.current.Trial == nil {
		stored, err := s.loadTrialMarkerUnlocked()
		if err != nil {
			return false, err
		}
		if stored != nil {
			if s.current.Trial == nil || s.current.Trial.StartedAt != stored.StartedAt || s.current.Trial.ExpiresAt != stored.ExpiresAt {
				s.current.Trial = stored
				changed = true
			}
		}
	}
	if s.current.Trial == nil {
		startedAt := now.Unix()
		s.current.Trial = &Trial{
			StartedAt: startedAt,
			ExpiresAt: now.Add(LocalTrialDuration).Unix(),
		}
		changed = true
	}
	if s.current.Trial.StartedAt <= 0 && s.current.Trial.ExpiresAt > 0 {
		s.current.Trial.StartedAt = s.current.Trial.ExpiresAt - int64(LocalTrialDuration/time.Second)
		changed = true
	}
	if s.current.Trial.ExpiresAt <= 0 {
		startedAt := s.current.Trial.StartedAt
		if startedAt <= 0 {
			startedAt = now.Unix()
			s.current.Trial.StartedAt = startedAt
			changed = true
		}
		s.current.Trial.ExpiresAt = time.Unix(startedAt, 0).Add(LocalTrialDuration).Unix()
		changed = true
	}
	if err := s.saveTrialMarkerUnlocked(s.current.Trial); err != nil {
		return false, err
	}
	return changed, nil
}

func (s *Store) trialMarkerPathUnlocked() string {
	return filepath.Join(filepath.Dir(s.path), trialFilename)
}

func (s *Store) loadTrialMarkerUnlocked() (*Trial, error) {
	content, err := securefile.ReadFile(s.trialMarkerPathUnlocked())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(content) == 0 {
		return nil, nil
	}
	var trial Trial
	if err := json.Unmarshal(content, &trial); err != nil {
		return nil, err
	}
	return &trial, nil
}

func (s *Store) saveTrialMarkerUnlocked(trial *Trial) error {
	if trial == nil {
		return nil
	}
	payload, err := json.MarshalIndent(trial, "", "  ")
	if err != nil {
		return err
	}
	path := s.trialMarkerPathUnlocked()
	tmp := path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) saveUnlocked() error {
	payload, err := json.MarshalIndent(s.current, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := securefile.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newDeviceID() string {
	// Prefer stable machine-derived ID so the same device always gets
	// the same identifier, even if auth-session.json is deleted.
	if id := machineid.DeviceID(); id != "" {
		return id
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "device_fallback"
	}
	return "device_" + hex.EncodeToString(buf)
}
