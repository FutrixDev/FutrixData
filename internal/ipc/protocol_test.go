package ipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRoundTripFrame(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	req := Request{V: ProtocolVersion, ID: "abc", Op: "tool.call"}
	if err := WriteFrame(&buf, req); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	var got Request
	if err := ReadFrame(&buf, &got); err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if got.ID != "abc" || got.Op != "tool.call" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestReadFrameRejectsOversize(t *testing.T) {
	t.Parallel()
	// Header claims a frame larger than MaxFrameBytes — caller must reject before
	// allocating, otherwise a malicious peer could OOM the process.
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], MaxFrameBytes+1)
	buf.Write(hdr[:])
	var dst Request
	err := ReadFrame(&buf, &dst)
	if !errors.Is(err, errFrameTooLarge) {
		t.Fatalf("expected errFrameTooLarge, got %v", err)
	}
}

func TestWriteFrameRejectsOversize(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	huge := strings.Repeat("x", MaxFrameBytes+1)
	err := WriteFrame(&buf, struct {
		X string `json:"x"`
	}{X: huge})
	if !errors.Is(err, errFrameTooLarge) {
		t.Fatalf("expected errFrameTooLarge, got %v", err)
	}
}

func TestReadFrameEOF(t *testing.T) {
	t.Parallel()
	// Clean EOF between frames is the normal "peer closed" signal — must not
	// turn into a corruption error.
	var dst Request
	err := ReadFrame(bytes.NewReader(nil), &dst)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestReadFrameShortHeader(t *testing.T) {
	t.Parallel()
	// Partial header = malformed peer; surface as errShortFrame so the
	// transport layer closes the connection rather than retrying.
	var dst Request
	err := ReadFrame(bytes.NewReader([]byte{0x00, 0x01}), &dst)
	if !errors.Is(err, errShortFrame) {
		t.Fatalf("expected errShortFrame, got %v", err)
	}
}

func TestErrorString(t *testing.T) {
	t.Parallel()
	e := NewError(CodeAccessKeyRequired, "missing key")
	if got := e.Error(); got != "ACCESS_KEY_REQUIRED: missing key" {
		t.Fatalf("unexpected: %q", got)
	}
	var nilErr *Error
	if got := nilErr.Error(); got != "" {
		t.Fatalf("nil Error should stringify empty, got %q", got)
	}
}
