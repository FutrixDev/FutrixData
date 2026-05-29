package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Handler implements one daemon-side op. It receives the parsed Request and
// returns either a result payload (any JSON-marshalable value) or a structured
// *Error. Returning a non-nil err that is *not* an *Error wraps it as
// CodeInternal so we never leak Go strings into the wire protocol unprotected.
type Handler func(ctx context.Context, req Request, conn net.Conn) (any, *Error)

// AuthGate runs before agent-routed ops (tool.call). Daemon implementations
// plug in agentaudit.CheckAccess here. Returning a non-nil *Error short-
// circuits dispatch; nil means the access key is valid.
//
// Required ops: anything where the design requires "agent has no path into
// any tool without an access key". For user ops (datasource CRUD, console
// exec) the daemon trusts socket peer permissions and skips the gate.
type AuthGate func(ctx context.Context, req Request) *Error

// ServerConfig wires the server to its dependencies. All fields are required
// except Logger (defaults to log.Default).
type ServerConfig struct {
	Listener net.Listener
	// Handlers maps op name → handler. An unknown op returns CodeUnknownOp.
	Handlers map[string]Handler
	// AgentOps is the set of op names that require an access-key gate. The
	// gate runs before the handler. ops outside this set are user ops and
	// dispatch directly.
	AgentOps map[string]bool
	// Auth is the access-key validator. Must be non-nil if AgentOps is
	// non-empty.
	Auth AuthGate
	// ReadTimeout caps how long a single connection waits for a frame. 0
	// means no timeout (use cautiously — agents may pause between frames).
	ReadTimeout time.Duration
	// Logger is used for non-fatal connection errors. Defaults to
	// log.Default if nil.
	Logger *log.Logger
}

// Server is a goroutine-per-connection IPC dispatcher. The caller owns the
// listener; Server.Serve blocks until ctx cancels or the listener fails.
type Server struct {
	cfg    ServerConfig
	wg     sync.WaitGroup
	closed chan struct{}
}

// NewServer validates cfg and returns a Server ready to be Serve()'d.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Listener == nil {
		return nil, errors.New("ipc server: Listener is required")
	}
	if cfg.Handlers == nil {
		return nil, errors.New("ipc server: Handlers is required")
	}
	if len(cfg.AgentOps) > 0 && cfg.Auth == nil {
		return nil, errors.New("ipc server: AgentOps non-empty but Auth is nil")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Server{cfg: cfg, closed: make(chan struct{})}, nil
}

// Serve runs the accept loop. Each accepted connection runs in its own
// goroutine. Returns when ctx is cancelled or the listener returns a fatal
// error. Pending connections finish in flight.
func (s *Server) Serve(ctx context.Context) error {
	defer close(s.closed)

	// Cancel ctx → close listener so Accept returns immediately.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = s.cfg.Listener.Close()
	}()

	for {
		conn, err := s.cfg.Listener.Accept()
		if err != nil {
			if ctx.Err() != nil || isUseOfClosedConnection(err) {
				s.wg.Wait()
				return nil
			}
			return fmt.Errorf("ipc server: accept: %w", err)
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(ctx, c)
		}(conn)
	}
}

// Wait blocks until Serve has returned and all connection goroutines drained.
func (s *Server) Wait() {
	<-s.closed
	s.wg.Wait()
}

// handleConn services one peer connection. The wire protocol is half-duplex
// per request, so we loop: read one Request → dispatch → write one Response,
// repeating until the peer closes or we hit a framing error.
//
// Framing errors (errFrameTooLarge / errShortFrame) and io.EOF terminate the
// connection silently — the wire protocol guarantees no partial state.
// Handler-level errors are surfaced as wire Error responses.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// When the server's ctx cancels, force the conn closed so a blocking
	// ReadFrame returns immediately. Otherwise a quiet client would keep us
	// in s.wg forever and Serve() would never return.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		if s.cfg.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
		}
		var req Request
		if err := ReadFrame(conn, &req); err != nil {
			if errors.Is(err, io.EOF) || IsConnDropError(err) {
				return
			}
			s.cfg.Logger.Printf("ipc server: read frame: %v", err)
			return
		}
		// Reset deadline before handler runs; long ops are normal.
		_ = conn.SetReadDeadline(time.Time{})
		resp := s.dispatch(ctx, req, conn)
		if err := WriteFrame(conn, resp); err != nil {
			if !IsConnDropError(err) {
				s.cfg.Logger.Printf("ipc server: write frame: %v", err)
			}
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, req Request, conn net.Conn) Response {
	resp := Response{V: ProtocolVersion, ID: req.ID}
	if req.V != ProtocolVersion {
		resp.Error = NewError(CodeBadRequest, fmt.Sprintf("unsupported protocol version %d (want %d)", req.V, ProtocolVersion))
		return resp
	}
	if req.Op == "" {
		resp.Error = NewError(CodeBadRequest, "missing op")
		return resp
	}
	handler, ok := s.cfg.Handlers[req.Op]
	if !ok {
		resp.Error = NewError(CodeUnknownOp, fmt.Sprintf("unknown op: %s", req.Op))
		return resp
	}
	if s.cfg.AgentOps[req.Op] {
		if req.Auth == nil || req.Auth.AccessKey == "" {
			resp.Error = NewError(CodeAccessKeyRequired, "agent op requires auth.accessKey")
			return resp
		}
		if e := s.cfg.Auth(ctx, req); e != nil {
			resp.Error = e
			return resp
		}
	}
	result, e := handler(ctx, req, conn)
	if e != nil {
		resp.Error = e
		return resp
	}
	if result != nil {
		body, err := json.Marshal(result)
		if err != nil {
			resp.Error = NewError(CodeInternal, fmt.Sprintf("marshal result: %v", err))
			return resp
		}
		resp.Result = body
	}
	resp.OK = true
	return resp
}

// isUseOfClosedConnection mirrors net's internal sentinel used when a listener
// is closed under us (the cancel-via-Close trick above). We can't `errors.Is`
// it because net.errClosed is unexported, so we fall back to message match.
func isUseOfClosedConnection(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed)
}
