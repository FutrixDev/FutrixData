package cli

import (
	"errors"
	"io"
	"testing"

	"futrixdata/platform/internal/ipc"
)

// TestIsInfraErrorClassifiesConnDrop pins the Round 12 fix: raw transport
// errors returned by ipc.Client.Roundtrip when a previously-open connection
// dies during a daemon restart must classify as infra so the spawn-and-
// retry recovery runs. Without this, the first tool call after a daemon
// crash/restart surfaces user-visibly even though recovery is automatic.
//
// Wire-coded errors are exercised separately in ipc/client_test (the
// errorWithCode type is unexported there); this test focuses on the
// regression case — bare network errors with no wire code.
func TestIsInfraErrorClassifiesConnDrop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain-business", errors.New("dataSource not found"), false},
		{"permission-denied", errors.New("permission denied"), false},

		{"eof", io.EOF, true},
		{"peer-closed", ipc.ErrPeerClosed, true},
		{"broken-pipe", errors.New("write tcp: broken pipe"), true},
		{"connection-reset", errors.New("read: connection reset by peer"), true},
		{"closed-conn", errors.New("use of closed network connection"), true},
		{"file-already-closed", errors.New("file already closed"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isInfraError(c.err); got != c.want {
				t.Fatalf("isInfraError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
