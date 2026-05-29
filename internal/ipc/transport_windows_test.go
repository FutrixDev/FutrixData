//go:build windows

package ipc

import (
	"strings"
	"testing"
)

// TestWindowsPipeNamePerDataDir pins the contract that two installs with
// different data paths get different pipe names. Without this scoping, a
// daemon already bound for one profile/user would block any second daemon
// from starting on the same machine — even though their handshake files
// (and Service state) are completely separate.
func TestWindowsPipeNamePerDataDir(t *testing.T) {
	t.Parallel()
	a := windowsPipeName(`C:\Users\alice\AppData\Roaming\FutrixData`)
	b := windowsPipeName(`C:\Users\bob\AppData\Roaming\FutrixData`)
	if a == b {
		t.Fatalf("expected distinct pipe names for distinct dataDirs, both got %s", a)
	}
	if !strings.HasPrefix(a, pipeNamePrefix) || !strings.HasPrefix(b, pipeNamePrefix) {
		t.Fatalf("pipe names lost the canonical prefix: a=%s b=%s", a, b)
	}
}

// TestWindowsPipeNameStable pins that the same dataDir produces the same
// pipe name across calls (and across processes — the GUI daemon and a
// later CLI dial must agree on the address).
func TestWindowsPipeNameStable(t *testing.T) {
	t.Parallel()
	dataDir := `C:\Users\alice\AppData\Roaming\FutrixData`
	first := windowsPipeName(dataDir)
	second := windowsPipeName(dataDir)
	if first != second {
		t.Fatalf("windowsPipeName not deterministic: %s vs %s", first, second)
	}
}

// TestWindowsPipeNameCaseInsensitive pins case-folding: Windows is case-
// insensitive at the filesystem layer, so two paths that differ only in
// case must map to the same pipe — otherwise the GUI and CLI could
// compute different names from the same install.
func TestWindowsPipeNameCaseInsensitive(t *testing.T) {
	t.Parallel()
	lower := windowsPipeName(`c:\users\alice\appdata\roaming\futrixdata`)
	mixed := windowsPipeName(`C:\Users\Alice\AppData\Roaming\FutrixData`)
	if lower != mixed {
		t.Fatalf("case sensitivity leaked: %s vs %s", lower, mixed)
	}
}
