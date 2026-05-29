package commandutil

import (
	"errors"
	"testing"
)

func TestResolveStableWorkingDir_UsesCurrentDirectoryWhenSafe(t *testing.T) {
	got := resolveStableWorkingDir(
		func() (string, error) { return "/Users/dylanwang/futrixdata/FutrixData", nil },
		func() (string, error) { return "/Users/dylanwang", nil },
		func() string { return "/tmp" },
	)
	if got != "/Users/dylanwang/futrixdata/FutrixData" {
		t.Fatalf("expected current working directory, got %q", got)
	}
}

func TestResolveStableWorkingDir_FallsBackFromRootToHome(t *testing.T) {
	got := resolveStableWorkingDir(
		func() (string, error) { return "/", nil },
		func() (string, error) { return "/Users/dylanwang", nil },
		func() string { return "/tmp" },
	)
	if got != "/Users/dylanwang" {
		t.Fatalf("expected user home directory, got %q", got)
	}
}

func TestResolveStableWorkingDir_FallsBackFromGetwdErrorToHome(t *testing.T) {
	got := resolveStableWorkingDir(
		func() (string, error) { return "", errors.New("boom") },
		func() (string, error) { return "/Users/dylanwang", nil },
		func() string { return "/tmp" },
	)
	if got != "/Users/dylanwang" {
		t.Fatalf("expected user home directory, got %q", got)
	}
}

func TestResolveStableWorkingDir_FallsBackToTempWhenHomeUnavailable(t *testing.T) {
	got := resolveStableWorkingDir(
		func() (string, error) { return "/", nil },
		func() (string, error) { return "", errors.New("boom") },
		func() string { return "/tmp" },
	)
	if got != "/tmp" {
		t.Fatalf("expected temp directory, got %q", got)
	}
}
