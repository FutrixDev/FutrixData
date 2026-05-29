package main

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"futrixdata/platform/internal/updater"

	"github.com/pkg/browser"
)

// CheckForUpdate asks FutrixServer for the latest released version and
// returns the comparison result. The frontend calls this on app start
// (after auth is ready) and on demand from the Settings page.
//
// When the user is not signed in the result is returned with
// Authenticated=false and a nil error so the frontend can render a
// "sign in to check" hint instead of an error banner.
func (a *App) CheckForUpdate() (updater.Result, error) {
	if a.updaterService == nil {
		return updater.Result{}, errors.New("updater service is not configured")
	}
	return a.updaterService.CheckLatest(context.Background())
}

// OpenUpdateDownload hands the download URL to the system browser. The URL
// is whatever CheckForUpdate returned for the running OS/arch — the
// frontend passes it back through this call so we keep one resolved
// platform-key path. We validate it points at the configured FutrixServer
// host before opening, to avoid being a generic open-anything sink.
func (a *App) OpenUpdateDownload(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("download url is required")
	}
	if !isAllowedUpdateURL(rawURL, a.cfg) {
		return errors.New("download url is not on an allowed host")
	}
	return browser.OpenURL(rawURL)
}

// isAllowedUpdateURL guards OpenUpdateDownload against being asked to open
// arbitrary URLs the frontend was tricked into sending. We only allow URLs
// served from the configured FutrixServer host (or its default).
func isAllowedUpdateURL(rawURL string, cfg Config) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	allowed := map[string]struct{}{}
	if base := strings.TrimSpace(resolveAuthBaseURL(cfg)); base != "" {
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			allowed[u.Host] = struct{}{}
		}
	}
	if u, err := url.Parse("https://futrixdata.com"); err == nil {
		allowed[u.Host] = struct{}{}
	}
	_, ok := allowed[parsed.Host]
	return ok
}
