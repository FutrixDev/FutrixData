// Package ipc carries CLI ↔ daemon traffic for FutrixData.
//
// Wire format: a single TCP-style stream of length-prefixed JSON frames.
//
//	┌─────────────────────────────┐
//	│ uint32 BE — payload bytes   │
//	├─────────────────────────────┤
//	│ N bytes UTF-8 JSON payload  │
//	└─────────────────────────────┘
//
// Each connection is half-duplex per request: client writes a Request frame,
// daemon writes a Response frame. The protocol does not multiplex in-flight
// requests on a single connection — concurrency is achieved by opening more
// connections.
//
// This package is the stable boundary between GUI, CLI, MCP, and daemon
// processes.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProtocolVersion is the IPC wire-protocol version. Bumped on incompatible
// changes; CLI and daemon ignore frames with mismatched protocol versions.
const ProtocolVersion = 1

// MaxFrameBytes caps a single frame at 16 MiB. Anything larger is treated as
// a corrupt stream — closing the connection is the only safe response.
const MaxFrameBytes = 16 * 1024 * 1024

// Request is the wire-shape sent from the CLI to the daemon.
type Request struct {
	V    int             `json:"v"`
	ID   string          `json:"id"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
	Auth *AuthEnvelope   `json:"auth,omitempty"`
}

// AuthEnvelope carries the agent identity. For tool.call this MUST be present
// (the design rule: agent has no path into any tool without an access key).
// For human-driven user ops it MAY be empty — daemon trusts the socket
// peer's filesystem permissions to enforce same-user isolation.
type AuthEnvelope struct {
	AccessKey string `json:"accessKey,omitempty"`
}

// Response is the wire-shape sent back from the daemon.
type Response struct {
	V      int             `json:"v"`
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error is structured so agents can switch on Code instead of regexing strings.
// Remediation is filled in for client-detected failures (DAEMON_NOT_RUNNING
// etc.) so the agent prompt can guide the user without further reasoning.
type Error struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Remediation string         `json:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

// Error codes. Two groups: ones the CLI-side client emits without ever talking
// to daemon (the DAEMON_*, VERSION_MISMATCH, INSTALL_CORRUPTED triple plus
// LOCATE_*), and ones the daemon emits in response to a malformed or
// unauthorized request.
const (
	CodeDaemonNotRunning  = "DAEMON_NOT_RUNNING"
	CodeDaemonUnreachable = "DAEMON_UNREACHABLE"
	CodeVersionMismatch   = "VERSION_MISMATCH"
	CodeInstallCorrupted  = "INSTALL_CORRUPTED"
	CodeLocateMainApp     = "LOCATE_MAIN_APP_FAILED"

	CodeBadRequest        = "BAD_REQUEST"
	CodeUnknownOp         = "UNKNOWN_OP"
	CodeAccessKeyRequired = "ACCESS_KEY_REQUIRED"
	CodeAccessKeyUnknown  = "ACCESS_KEY_UNKNOWN"
	CodeAccessKeyRevoked  = "ACCESS_KEY_REVOKED"
	CodeAccessKeyExpired  = "ACCESS_KEY_EXPIRED"
	// CodeAgentForbidden — the access key is valid but the identity lacks a
	// per-tool grant or datasource scope (currently used for the
	// sensitivity-policy write tools, risk-rule management, and optional
	// datasource allowlists).
	// Distinct from CodeAccessKeyRevoked because the identity is still active
	// for everything else; clients should surface this as a permissions
	// problem rather than retrying or treating the key as dead.
	CodeAgentForbidden   = "AGENT_FORBIDDEN"
	CodeApprovalRequired = "APPROVAL_REQUIRED"
	CodeToolError        = "TOOL_ERROR"
	CodeServiceError     = "SERVICE_ERROR"
	CodeStartupRecovery  = "STARTUP_RECOVERY"
	CodeInternal         = "INTERNAL"
)

// NewError builds an *Error with no details.
func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// errFrameTooLarge / errShortFrame surface as transport-level failures rather
// than wire Errors — a frame that fails framing checks means the stream is
// unusable, so the only sane response is to close the connection.
var (
	errFrameTooLarge = errors.New("ipc: frame exceeds MaxFrameBytes")
	errShortFrame    = errors.New("ipc: short frame header")
)

// WriteFrame encodes payload as a length-prefixed JSON frame on w.
func WriteFrame(w io.Writer, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ipc: marshal frame: %w", err)
	}
	if len(body) > MaxFrameBytes {
		return errFrameTooLarge
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return nil
}

// ReadFrame reads one length-prefixed JSON frame and unmarshals into out.
// Returns io.EOF cleanly when the peer closed the connection between frames.
func ReadFrame(r io.Reader, out any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return errShortFrame
		}
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		// Empty frame — caller sees {} unmarshaled. We unmarshal a literal
		// "{}" rather than nil because json.Unmarshal(nil, out) returns
		// "unexpected end of JSON input" and would force the server to tear
		// down the connection on a perfectly legal frame.
		return json.Unmarshal([]byte("{}"), out)
	}
	if n > MaxFrameBytes {
		return errFrameTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// MarshalArgs is a convenience for callers building Requests.
func MarshalArgs(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}
