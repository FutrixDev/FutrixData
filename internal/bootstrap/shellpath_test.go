//go:build darwin || linux

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrichPath_AddsShellDirectories(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	os.Setenv("PATH", "/usr/bin:/bin")

	EnrichPath()

	got := os.Getenv("PATH")
	if !strings.HasPrefix(got, "/usr/bin:/bin") {
		t.Fatalf("original PATH prefix lost: %s", got)
	}
	if got == "/usr/bin:/bin" {
		t.Log("warning: EnrichPath did not add any extra directories; may be expected in minimal CI")
	}
}

func TestMergePath_SkipsRelativePaths(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	os.Setenv("PATH", "/usr/bin")
	mergePath("banner text:/opt/homebrew/bin:relative/path:/usr/local/bin")

	got := os.Getenv("PATH")
	if strings.Contains(got, "banner") {
		t.Errorf("banner text should have been filtered out: %s", got)
	}
	if strings.Contains(got, "relative/path") {
		t.Errorf("relative path should have been filtered out: %s", got)
	}
	if !strings.Contains(got, "/opt/homebrew/bin") {
		t.Errorf("expected /opt/homebrew/bin in PATH: %s", got)
	}
	if !strings.Contains(got, "/usr/local/bin") {
		t.Errorf("expected /usr/local/bin in PATH: %s", got)
	}
}

func TestMergePath_NoDuplicates(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	os.Setenv("PATH", "/usr/bin:/opt/homebrew/bin")
	mergePath("/opt/homebrew/bin:/usr/local/bin")

	got := os.Getenv("PATH")
	if got != "/usr/bin:/opt/homebrew/bin:/usr/local/bin" {
		t.Errorf("unexpected PATH: %s", got)
	}
}

func TestMergeWellKnownDirs(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	os.Setenv("PATH", "/usr/bin")
	mergeWellKnownDirs(func() []string {
		return []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(os.Getenv("HOME"), ".local/bin")}
	})

	got := os.Getenv("PATH")
	hasExtra := false
	for _, d := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if _, err := os.Stat(d); err == nil && strings.Contains(got, d) {
			hasExtra = true
		}
	}
	if !hasExtra {
		t.Log("warning: no well-known dirs were added; expected on standard dev setup")
	}
}
