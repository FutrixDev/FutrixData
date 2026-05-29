// Package updater compares the running FutrixData build against the latest
// release advertised by FutrixServer. The check is deliberately read-only:
// when an update is available we surface the platform-specific download URL
// and let the frontend hand it to the system browser. We do not download or
// replace the installed binary in this layer.
package updater

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"futrixdata/platform/internal/auth"
)

// LatestVersionPath is the FutrixServer endpoint that returns the latest
// shipped release. It lives under /api/client/* so it shares the existing
// Bearer-token / session-cookie auth path used by the rest of the desktop
// client API.
const LatestVersionPath = "/api/client/latest-version"

// Result is what the Wails facade returns to the frontend.
type Result struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	HasUpdate       bool   `json:"hasUpdate"`
	DownloadURL     string `json:"downloadUrl"`
	PlatformKey     string `json:"platformKey"`
	PlatformLabel   string `json:"platformLabel"`
	ReleaseNotesURL string `json:"releaseNotesUrl"`
	// Authenticated is false when the user is not signed in. The frontend
	// uses this to swap "Update now" for a "Sign in to check for updates"
	// hint instead of showing an error.
	Authenticated bool  `json:"authenticated"`
	LastCheckedAt int64 `json:"lastCheckedAt"`
}

type latestVersionResponse struct {
	Version         string                            `json:"version"`
	ReleasedAt      *int64                            `json:"releasedAt"`
	Platforms       map[string]latestVersionPlatform  `json:"platforms"`
	ReleaseNotesURL string                            `json:"releaseNotesUrl"`
}

type latestVersionPlatform struct {
	DownloadURL string `json:"downloadUrl"`
	Label       string `json:"label"`
}

// Service performs the version check against FutrixServer.
type Service struct {
	auth         *auth.Service
	currentVer   string
	platformKey  func() string
	now          func() time.Time
}

// NewService wires the updater to the existing auth service so it reuses
// the Bearer-token plumbing. currentVersion is typically the build-time
// injected futrixdata/platform/internal/version.Version (defaults to
// "dev" for source builds — see semver handling below).
func NewService(authSvc *auth.Service, currentVersion string) *Service {
	return &Service{
		auth:        authSvc,
		currentVer:  strings.TrimSpace(currentVersion),
		platformKey: defaultPlatformKey,
		now:         time.Now,
	}
}

// CheckLatest hits the server and returns a populated Result. When the user
// is not signed in (or the session has expired and refresh failed), Result
// is returned with Authenticated=false and a nil error — callers treat
// "not signed in" as "skip the update prompt", not "show an error".
func (s *Service) CheckLatest(ctx context.Context) (Result, error) {
	now := s.now().Unix()
	base := Result{
		Current:       s.currentVer,
		PlatformKey:   s.platformKey(),
		LastCheckedAt: now,
	}
	if s.auth == nil {
		base.Authenticated = false
		return base, nil
	}

	var resp latestVersionResponse
	if err := s.auth.GetJSON(ctx, LatestVersionPath, &resp); err != nil {
		if errors.Is(err, auth.ErrLoginRequired) {
			base.Authenticated = false
			return base, nil
		}
		return base, fmt.Errorf("check latest version: %w", err)
	}

	base.Authenticated = true
	base.Latest = strings.TrimSpace(resp.Version)
	base.ReleaseNotesURL = strings.TrimSpace(resp.ReleaseNotesURL)
	base.HasUpdate = IsNewer(base.Latest, base.Current)

	if plat, ok := resp.Platforms[base.PlatformKey]; ok {
		base.DownloadURL = strings.TrimSpace(plat.DownloadURL)
		base.PlatformLabel = strings.TrimSpace(plat.Label)
	}
	return base, nil
}

// IsNewer reports whether `latest` is strictly newer than `current`.
// "dev" or any unparseable current version is treated as "older than any
// concrete semver" — source builds always see updates as available, which
// matches what we want during in-house testing. Conversely an unparseable
// `latest` is treated as not-newer (we never falsely promote bad input).
func IsNewer(latest, current string) bool {
	latest = strings.TrimSpace(latest)
	current = strings.TrimSpace(current)
	if latest == "" {
		return false
	}
	lv, ok := parseSemver(latest)
	if !ok {
		return false
	}
	cv, cok := parseSemver(current)
	if !cok {
		return true
	}
	for i := 0; i < 3; i++ {
		if lv[i] > cv[i] {
			return true
		}
		if lv[i] < cv[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return [3]int{}, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// defaultPlatformKey maps the running runtime to a key the FutrixServer
// /api/client/latest-version response uses (and that /api/download/:platform
// also accepts). Unsupported combinations return "" so the frontend can
// fall back to the release notes link.
func defaultPlatformKey() string {
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return "macos-arm64"
		case "amd64":
			return "macos-amd64"
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "windows-amd64"
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "linux-amd64"
		}
	}
	return ""
}
