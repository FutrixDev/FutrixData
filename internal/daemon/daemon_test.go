package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/keyring"
	"futrixdata/platform/internal/localcrypto"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/securefile"
	"futrixdata/platform/internal/sensitivity"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

// shortDataDir returns a /tmp-rooted file path the daemon treats as the
// datasources.json location. Bootstrap.NewRuntime expects a file path here,
// not a directory. Using /tmp dodges the 104-byte AF_UNIX path limit on
// macOS that t.TempDir() can blow past.
func shortDataDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("daemon_test uses ad-hoc UDS paths; pipe path is fixed on windows")
	}
	dir, err := os.MkdirTemp("/tmp", "daemon-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "datasources.json")
}

func TestHandleToolCall_DecodesArgs(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := store.EnsureManual("daemon-test")
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if _, ok := toolreg.ByName("list_datasources"); !ok {
		t.Fatal("list_datasources not in registry")
	}
	svc := &stubAuthService{}
	handler := handleToolCall(dir, svc, nil)

	args := ToolCallArgs{Tool: "list_datasources", Source: "skill"}
	rawArgs, _ := json.Marshal(args)
	resp, e := handler(context.Background(), ipc.Request{
		ID:   "1",
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	}, nil)
	if e != nil {
		t.Fatalf("handler returned error: %+v", e)
	}
	tcr, ok := resp.(ToolCallResult)
	if !ok {
		t.Fatalf("expected ToolCallResult, got %T", resp)
	}
	if !tcr.OK {
		t.Fatalf("expected OK=true, got %+v", tcr)
	}
	if tcr.Tool != "list_datasources" {
		t.Fatalf("Tool = %q, want list_datasources", tcr.Tool)
	}
}

func TestHandleToolCall_ApprovalRejectionIncludesRiskAttribution(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := store.EnsureManual("daemon-test")
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	handler := handleToolCall(dir, &riskApprovalAuthService{}, nil)

	args := ToolCallArgs{
		Tool:   "execute_statement",
		Source: "skill",
		Params: map[string]any{"datasourceId": "ds-mysql", "statement": "DROP TABLE users"},
	}
	rawArgs, _ := json.Marshal(args)
	resp, e := handler(context.Background(), ipc.Request{
		ID:   "1",
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	}, nil)
	if e == nil || e.Code != ipc.CodeToolError {
		t.Fatalf("expected TOOL_ERROR approval rejection, got resp=%T err=%+v", resp, e)
	}
	attr, ok := e.Details["riskAttribution"].(*agentaudit.RiskAttribution)
	if !ok || attr == nil {
		t.Fatalf("expected risk attribution in error details, got %#v", e.Details)
	}
	if attr.RuleCode != "SQL-007" {
		t.Fatalf("ruleCode = %q, want SQL-007", attr.RuleCode)
	}
	if attr.Source != agentaudit.AttributionSourceRiskEngine {
		t.Fatalf("source = %q, want risk_engine", attr.Source)
	}
}

func TestHandleToolCall_RejectsBadJSON(t *testing.T) {
	dir := shortDataDir(t)
	svc := &stubAuthService{}
	handler := handleToolCall(dir, svc, nil)
	_, e := handler(context.Background(), ipc.Request{
		ID:   "1",
		Op:   "tool.call",
		Args: json.RawMessage("not json"),
		Auth: &ipc.AuthEnvelope{AccessKey: "x"},
	}, nil)
	if e == nil || e.Code != ipc.CodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %+v", e)
	}
}

func TestMakeAuthGate_RejectsMissingKey(t *testing.T) {
	dir := shortDataDir(t)
	svc := &stubAuthService{}
	gate := makeAuthGate(dir, svc, nil)
	if e := gate(context.Background(), ipc.Request{}); e == nil || e.Code != ipc.CodeAccessKeyRequired {
		t.Fatalf("expected ACCESS_KEY_REQUIRED, got %+v", e)
	}
}

func TestMakeAuthGate_RejectsUnknownKey(t *testing.T) {
	dir := shortDataDir(t)
	svc := &stubAuthService{}
	gate := makeAuthGate(dir, svc, nil)
	e := gate(context.Background(), ipc.Request{Auth: &ipc.AuthEnvelope{AccessKey: "fake"}})
	if e == nil {
		t.Fatal("expected unknown-key error")
	}
	if e.Code != ipc.CodeAccessKeyUnknown && e.Code != ipc.CodeAccessKeyRevoked {
		t.Fatalf("expected ACCESS_KEY_UNKNOWN, got %+v", e)
	}
}

func TestMakeAuthGate_RejectsExpiredKeyWithExpiredCode(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := store.EnsureManual("expired-daemon")
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if _, err := store.SetExpiresAt(identity.AccessKey, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	args := ToolCallArgs{Tool: "list_datasources", Source: "mcp"}
	rawArgs, _ := json.Marshal(args)

	gate := makeAuthGate(dir, &stubAuthService{}, nil)
	e := gate(context.Background(), ipc.Request{
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	})
	if e == nil || e.Code != ipc.CodeAccessKeyExpired {
		t.Fatalf("expected ACCESS_KEY_EXPIRED, got %+v", e)
	}
	entries, err := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dir)).List(agentaudit.AuditFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entry count = %d, want 1", len(entries))
	}
	if entries[0].Protocol != string(toolexec.SourceMCP) || entries[0].ToolName != "list_datasources" {
		t.Fatalf("unexpected audit attribution: %+v", entries[0])
	}
}

func TestMakeAuthGate_RequiresLoginForPolicyMutationToolsEvenWithAgentGrants(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := store.EnsureManual("policy-mutation-daemon")
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if _, err := store.SetRiskRuleManagementGrant(identity.AccessKey, true); err != nil {
		t.Fatalf("set risk-rule grant: %v", err)
	}
	if _, err := store.SetSensitivityGrant(identity.AccessKey, true); err != nil {
		t.Fatalf("set sensitivity grant: %v", err)
	}

	gate := makeAuthGate(dir, &loginRequiredAuthService{}, nil)
	cases := []struct {
		tool   string
		params map[string]any
	}{
		{"set_risk_rule", map[string]any{"id": "URD-PROBE-001", "action": "warn"}},
		{"delete_risk_rule", map[string]any{"id": "URD-PROBE-001"}},
		{"set_builtin_risk_rule_enabled", map[string]any{"id": "sql-allow-insert", "enabled": true}},
		{"set_builtin_risk_rule_thresholds", map[string]any{"id": "probe-wide-scan", "thresholds": map[string]any{"maxExaminedRows": 1}}},
		{"set_sensitivity_custom_rules", map[string]any{"rules": "mask email"}},
		{"save_sensitivity_report", map[string]any{"datasourceId": "ds-1", "entities": []any{map[string]any{"entity": "users"}}}},
		{"delete_sensitivity_report", map[string]any{"datasourceId": "ds-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			rawArgs, _ := json.Marshal(ToolCallArgs{Tool: tc.tool, Source: "mcp", Params: tc.params})
			e := gate(context.Background(), ipc.Request{
				Op:   "tool.call",
				Args: rawArgs,
				Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
			})
			if e == nil || e.Code != ipc.CodeAgentForbidden {
				t.Fatalf("expected AGENT_FORBIDDEN login gate, got %+v", e)
			}
			if e.Message != auth.ErrLoginRequired.Error() {
				t.Fatalf("expected login-required message, got %q", e.Message)
			}
		})
	}

	entries, err := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dir)).List(agentaudit.AuditFilter{})
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if len(entries) != len(cases) {
		t.Fatalf("audit entry count = %d, want %d", len(entries), len(cases))
	}
	for _, entry := range entries {
		if entry.Protocol != string(toolexec.SourceMCP) {
			t.Fatalf("audit protocol = %q, want %q", entry.Protocol, toolexec.SourceMCP)
		}
		if entry.Status != agentaudit.StatusError || entry.Message != auth.ErrLoginRequired.Error() {
			t.Fatalf("unexpected audit entry: %+v", entry)
		}
	}
}

func TestMakeAuthGate_AllowsPolicyMutationToolsDuringActiveTrial(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := store.EnsureManual("policy-mutation-trial")
	if err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if _, err := store.SetRiskRuleManagementGrant(identity.AccessKey, true); err != nil {
		t.Fatalf("set risk-rule grant: %v", err)
	}

	gate := makeAuthGate(dir, &activeTrialAuthService{}, nil)
	rawArgs, _ := json.Marshal(ToolCallArgs{
		Tool:   "set_risk_rule",
		Source: "mcp",
		Params: map[string]any{"id": "URD-TRIAL-001", "action": "warn"},
	})
	if e := gate(context.Background(), ipc.Request{
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	}); e != nil {
		t.Fatalf("expected active trial to pass signed-in gate, got %+v", e)
	}
}

// TestDaemonE2E_PingAndStatus spins up Run() in a goroutine, dials it as an
// IPC client, and confirms the full Run → Listen → handshake → dispatch
// loop works end-to-end. Doesn't exercise process detachment / supervisor
// integration, but does exercise everything inside the daemon process.
func TestDaemonE2E_PingAndStatus(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	if _, err := store.EnsureManual("e2e"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	runErrCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		runErrCh <- Run(ctx, Config{DataPath: dir})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		select {
		case err := <-runErrCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("daemon Run returned: %v", err)
			}
		default:
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	var hs ipc.Handshake
	dataDir := filepath.Dir(dir)
	for {
		var rerr error
		hs, rerr = ipc.ReadHandshake(dataDir)
		if rerr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handshake never published: %v", rerr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := ipc.Dial(ctx, hs.Socket, time.Second)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	defer conn.Close()

	if err := ipc.WriteFrame(conn, ipc.Request{
		V:  ipc.ProtocolVersion,
		ID: "ping",
		Op: "daemon.ping",
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	var resp ipc.Response
	if err := ipc.ReadFrame(conn, &resp); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ping not OK: %+v", resp.Error)
	}
	var pong map[string]string
	_ = json.Unmarshal(resp.Result, &pong)
	if pong["pong"] != "ping" {
		t.Fatalf("ping echo: got %q want %q", pong["pong"], "ping")
	}
}

// TestDaemonShutdown_RemovesHandshake pins the contract that a clean
// daemon shutdown unlinks cli-handshake.json from the data directory.
// Regression guard: passing dataPath (a file) to RemoveHandshake instead
// of dataDir made cleanup a silent no-op, leaking the handshake across
// PID reuse and confusing reconnect logic.
func TestDaemonShutdown_RemovesHandshake(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	if _, err := store.EnsureManual("shutdown-test"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	runErrCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		runErrCh <- Run(ctx, Config{DataPath: dir})
	}()

	dataDir := filepath.Dir(dir)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := ipc.ReadHandshake(dataDir); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			wg.Wait()
			t.Fatalf("handshake never published")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	wg.Wait()
	select {
	case err := <-runErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("daemon Run returned: %v", err)
		}
	default:
	}

	if _, err := os.Stat(ipc.HandshakePath(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("handshake still present after shutdown: stat err = %v", err)
	}
}

// TestDaemonShutdownOp_GracefulHandoff pins the GUI handoff contract: a
// pre-existing --headless daemon must release the IPC socket when it
// receives a daemon.shutdown request, so a freshly-launched GUI can take
// over without leaving two Services racing on the same datasources.json.
//
// Regression guard: without time.AfterFunc deferring cancel, the listener
// would close before the response frame was flushed, and the caller would
// see a connection-reset rather than a clean ack.
func TestDaemonShutdownOp_GracefulHandoff(t *testing.T) {
	dir := shortDataDir(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	if _, err := store.EnsureManual("shutdown-op-test"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan error, 1)
	runErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErrCh <- Run(ctx, Config{DataPath: dir, Ready: ready})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("daemon ready error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon never signaled ready")
	}

	dataDir := filepath.Dir(dir)
	hs, err := ipc.ReadHandshake(dataDir)
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}

	conn, err := ipc.Dial(ctx, hs.Socket, time.Second)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	if err := ipc.WriteFrame(conn, ipc.Request{
		V:  ipc.ProtocolVersion,
		ID: "handoff",
		Op: "daemon.shutdown",
	}); err != nil {
		_ = conn.Close()
		t.Fatalf("write shutdown: %v", err)
	}
	var resp ipc.Response
	if err := ipc.ReadFrame(conn, &resp); err != nil {
		_ = conn.Close()
		t.Fatalf("read shutdown ack: %v", err)
	}
	_ = conn.Close()
	if !resp.OK {
		t.Fatalf("shutdown not OK: %+v", resp.Error)
	}

	select {
	case err := <-runErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("daemon Run returned non-cancel error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon Run did not exit after daemon.shutdown")
	}

	if _, err := os.Stat(ipc.HandshakePath(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("handshake still present after shutdown op: stat err = %v", err)
	}
}

func TestDaemonRunContinuesWhenLocalCryptoUnavailable(t *testing.T) {
	dir := shortDataDir(t)
	keyringErr := errors.New("keyring unavailable")
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			return "", keyringErr
		},
		func(service, account, secret string) error {
			return keyringErr
		},
	)
	t.Cleanup(restore)
	t.Cleanup(securefile.ResetForTest)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan error, 1)
	runErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErrCh <- Run(ctx, Config{DataPath: dir, Ready: ready})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("daemon should keep listening without keyring, got ready error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon never signaled ready")
	}
	if !securefile.EncryptionRequired() {
		t.Fatal("expected encryption to remain required when keyring is unavailable")
	}

	dataDir := filepath.Dir(dir)
	hs, err := ipc.ReadHandshake(dataDir)
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	conn, err := ipc.Dial(ctx, hs.Socket, time.Second)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	defer conn.Close()
	if err := ipc.WriteFrame(conn, ipc.Request{
		V:  ipc.ProtocolVersion,
		ID: "ping",
		Op: "daemon.ping",
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	var resp ipc.Response
	if err := ipc.ReadFrame(conn, &resp); err != nil {
		t.Fatalf("read ping response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ping not OK: %+v", resp.Error)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("daemon Run returned non-cancel error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon Run did not exit after cancel")
	}
}

func TestDaemonRetriesLocalCryptoAfterKeyringRecovers(t *testing.T) {
	dir := shortDataDir(t)
	keyringErr := errors.New("keyring unavailable")
	var keyringMu sync.Mutex
	keyringAvailable := true
	secrets := map[string]string{}
	secretKey := func(service, account string) string {
		return service + "\x00" + account
	}
	restore := keyring.UseBackendForTest(
		func(service, account string) (string, error) {
			keyringMu.Lock()
			defer keyringMu.Unlock()
			if !keyringAvailable {
				return "", keyringErr
			}
			secret, ok := secrets[secretKey(service, account)]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return secret, nil
		},
		func(service, account, secret string) error {
			keyringMu.Lock()
			defer keyringMu.Unlock()
			if !keyringAvailable {
				return keyringErr
			}
			secrets[secretKey(service, account)] = secret
			return nil
		},
	)
	t.Cleanup(restore)
	t.Cleanup(securefile.ResetForTest)

	if _, err := localcrypto.Init(dir); err != nil {
		t.Fatalf("seed local crypto: %v", err)
	}
	store := datasource.NewStore(dir)
	if _, err := store.Create(datasource.DataSource{
		ID:   "ds_retry",
		Name: "Recovered datasource",
		Type: datasource.TypePostgreSQL,
	}); err != nil {
		t.Fatalf("seed encrypted datasource: %v", err)
	}
	authStore := auth.NewStore(auth.PathForDataPath(dir))
	if err := authStore.Save(auth.State{
		DeviceID: "device_retry",
		Session: &auth.Session{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(time.Hour).Unix(),
			User: auth.User{
				ID:    "user_retry",
				Email: "user@example.com",
			},
			License: auth.License{
				Plan:   "pro",
				Status: "active",
			},
		},
	}); err != nil {
		t.Fatalf("seed encrypted auth session: %v", err)
	}
	sensStore := sensitivity.NewStore(bootstrap.SensitivityStorePath(dir))
	if err := sensStore.SetMode(sensitivity.ModeBlacklist); err != nil {
		t.Fatalf("seed encrypted sensitivity config: %v", err)
	}
	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dir))
	identity, err := identityStore.EnsureManual("daemon-retry")
	if err != nil {
		t.Fatalf("seed encrypted agent identity: %v", err)
	}
	securefile.ResetForTest()
	keyringMu.Lock()
	keyringAvailable = false
	keyringMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan error, 1)
	runErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErrCh <- Run(ctx, Config{DataPath: dir, Ready: ready})
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("daemon should keep listening without keyring, got ready error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon never signaled ready")
	}
	if securefile.Key() != nil {
		t.Fatal("expected no securefile key while keyring is unavailable")
	}
	if !securefile.EncryptionRequired() {
		t.Fatal("expected encryption to remain required while waiting for keyring")
	}

	keyringMu.Lock()
	keyringAvailable = true
	keyringMu.Unlock()

	dataDir := filepath.Dir(dir)
	hs, err := ipc.ReadHandshake(dataDir)
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	conn, err := ipc.Dial(ctx, hs.Socket, time.Second)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	defer conn.Close()
	if err := ipc.WriteFrame(conn, ipc.Request{
		V:  ipc.ProtocolVersion,
		ID: "retry",
		Op: "daemon.ping",
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	var resp ipc.Response
	if err := ipc.ReadFrame(conn, &resp); err != nil {
		t.Fatalf("read ping response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ping not OK: %+v", resp.Error)
	}
	if securefile.Key() == nil {
		t.Fatal("expected daemon request to retry local crypto init after keyring recovered")
	}
	if err := securefile.WriteFile(filepath.Join(dataDir, "retry-probe.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("expected securefile write to recover after retry: %v", err)
	}
	rawArgs, _ := json.Marshal(ToolCallArgs{Tool: "list_datasources", Source: "skill"})
	if err := ipc.WriteFrame(conn, ipc.Request{
		V:    ipc.ProtocolVersion,
		ID:   "list",
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	}); err != nil {
		t.Fatalf("write list_datasources: %v", err)
	}
	var listResp ipc.Response
	if err := ipc.ReadFrame(conn, &listResp); err != nil {
		t.Fatalf("read list_datasources response: %v", err)
	}
	if !listResp.OK {
		t.Fatalf("list_datasources not OK after retry: %+v", listResp.Error)
	}
	var toolResult struct {
		OK     bool                    `json:"ok"`
		Tool   string                  `json:"tool"`
		Result []datasource.DataSource `json:"result"`
	}
	if err := json.Unmarshal(listResp.Result, &toolResult); err != nil {
		t.Fatalf("decode list_datasources result: %v", err)
	}
	if len(toolResult.Result) != 1 || toolResult.Result[0].ID != "ds_retry" {
		t.Fatalf("datasources were not reloaded after retry: %+v", toolResult.Result)
	}
	rawArgs, _ = json.Marshal(ToolCallArgs{Tool: "get_sensitivity_config", Source: "skill"})
	if err := ipc.WriteFrame(conn, ipc.Request{
		V:    ipc.ProtocolVersion,
		ID:   "sensitivity",
		Op:   "tool.call",
		Args: rawArgs,
		Auth: &ipc.AuthEnvelope{AccessKey: identity.AccessKey},
	}); err != nil {
		t.Fatalf("write get_sensitivity_config: %v", err)
	}
	var sensitivityResp ipc.Response
	if err := ipc.ReadFrame(conn, &sensitivityResp); err != nil {
		t.Fatalf("read get_sensitivity_config response: %v", err)
	}
	if !sensitivityResp.OK {
		t.Fatalf("get_sensitivity_config not OK after retry: %+v", sensitivityResp.Error)
	}
	var sensitivityResult struct {
		OK     bool           `json:"ok"`
		Tool   string         `json:"tool"`
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(sensitivityResp.Result, &sensitivityResult); err != nil {
		t.Fatalf("decode get_sensitivity_config result: %v", err)
	}
	if sensitivityResult.Result["mode"] != string(sensitivity.ModeBlacklist) {
		t.Fatalf("sensitivity store was not reloaded after retry: %+v", sensitivityResult.Result)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("daemon Run returned non-cancel error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon Run did not exit after cancel")
	}
}

// stubAuthService implements toolreg.AuthService with mostly-no-op methods.
// The daemon's handlers only touch a few of them (the audit log and
// redaction paths reach into the Service for certain redactions).
type stubAuthService struct{}

type loginRequiredAuthService struct {
	stubAuthService
}

func (s *loginRequiredAuthService) EnsureAuthenticated(context.Context) (auth.State, error) {
	return auth.State{}, auth.ErrLoginRequired
}

type activeTrialAuthService struct {
	stubAuthService
}

func (s *activeTrialAuthService) EnsureAuthenticated(context.Context) (auth.State, error) {
	now := time.Now()
	return auth.State{
		Trial: &auth.Trial{
			StartedAt: now.Add(-time.Hour).Unix(),
			ExpiresAt: now.Add(30 * 24 * time.Hour).Unix(),
		},
	}, auth.ErrLoginRequired
}

type riskApprovalAuthService struct {
	stubAuthService
}

func (s *riskApprovalAuthService) GetDatasource(_ context.Context, id string) (datasource.DataSource, error) {
	return datasource.DataSource{
		ID:      id,
		Type:    datasource.TypeMySQL,
		Options: map[string]any{datasource.TrustLevelOptionKey: string(datasource.TrustCautious)},
	}, nil
}

func (s *riskApprovalAuthService) AssessStatement(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
	return riskengine.RiskAssessment{
		Level:           riskengine.RiskHigh,
		Action:          riskengine.ActionRequireApproval,
		RuleID:          "sql-require-approval-drop",
		RuleCode:        "SQL-007",
		RuleDescription: "DROP statements require approval",
		Builtin:         true,
		Reasons:         []string{"DROP TABLE can destroy data"},
	}, nil
}

func (s *stubAuthService) GetDatasource(_ context.Context, id string) (datasource.DataSource, error) {
	return datasource.DataSource{ID: id}, nil
}
func (s *stubAuthService) ListDatasources(context.Context) ([]datasource.DataSource, error) {
	return []datasource.DataSource{}, nil
}
func (s *stubAuthService) CreateDatasource(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *stubAuthService) UpdateDatasource(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *stubAuthService) DeleteDatasource(context.Context, string) (bool, error) { return false, nil }
func (s *stubAuthService) TestDatasource(context.Context, string) (bool, error)   { return false, nil }
func (s *stubAuthService) TestDatasourcePayload(context.Context, datasourceops.DataSourcePayload) (bool, error) {
	return false, nil
}
func (s *stubAuthService) ListDatabases(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (s *stubAuthService) ListEntities(context.Context, string, string, string, string, bool) ([]string, error) {
	return nil, nil
}
func (s *stubAuthService) DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}
func (s *stubAuthService) ListRiskRules(context.Context, bool) ([]riskengine.Rule, error) {
	return nil, nil
}
func (s *stubAuthService) SetRiskRule(context.Context, riskengine.Rule) (riskengine.Rule, error) {
	return riskengine.Rule{}, nil
}
func (s *stubAuthService) DeleteRiskRule(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubAuthService) SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
func (s *stubAuthService) SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	return riskengine.RuleThresholds{}, nil
}
func (s *stubAuthService) ExecuteStatement(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *stubAuthService) AssessStatement(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
	return riskengine.RiskAssessment{}, nil
}
func (s *stubAuthService) ExecuteRedisCommand(context.Context, string, []string, string, string) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *stubAuthService) AssessRedisCommand(context.Context, string, []string, string, string) (riskengine.RiskAssessment, error) {
	return riskengine.RiskAssessment{}, nil
}
func (s *stubAuthService) ExplainStatement(context.Context, string, string, bool, string, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}
func (s *stubAuthService) ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error) {
	return datasourceops.RedisKeyPage{}, nil
}
func (s *stubAuthService) GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *stubAuthService) GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *stubAuthService) GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error) {
	return console.RedisCommandDocsEntry{}, nil
}
func (s *stubAuthService) GetSchemaKnowledge(context.Context, string, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubAuthService) GetERKnowledge(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubAuthService) D1DeployMigrations(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubAuthService) D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *stubAuthService) D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *stubAuthService) D1IsWranglerInstalled(context.Context) (bool, error) { return false, nil }
func (s *stubAuthService) D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error) {
	return nil, nil
}
func (s *stubAuthService) D1CreateCloudDatabase(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error) {
	return datasourceops.D1CloudDatabase{}, nil
}
func (s *stubAuthService) DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error) {
	return nil, nil
}
func (s *stubAuthService) DynamoDBSSOLogin(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error) {
	return datasourceops.DynamoDBSSOLoginResult{}, nil
}
func (s *stubAuthService) DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error) {
	return datasourceops.DynamoDBSSOOAuthResult{}, nil
}
func (s *stubAuthService) DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error) {
	return nil, nil
}
func (s *stubAuthService) DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error) {
	return nil, nil
}
func (s *stubAuthService) DynamoDBSSOGetRoleCredentials(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error) {
	return datasourceops.DynamoDBSSORoleCredentials{}, nil
}
func (s *stubAuthService) GetSensitivityConfig(context.Context) (map[string]any, error) {
	return nil, nil
}
func (s *stubAuthService) SetSensitivityCustomRules(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubAuthService) GetSensitivityReport(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubAuthService) SaveSensitivityReport(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
	return nil, nil
}
func (s *stubAuthService) DeleteSensitivityReport(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubAuthService) EnsureAuthenticated(context.Context) (auth.State, error) {
	return auth.State{}, nil
}
func (s *stubAuthService) CurrentAuth(context.Context) (auth.State, error) { return auth.State{}, nil }

var _ toolreg.AuthService = (*stubAuthService)(nil)

// TestBuildService_WiresRiskDependencies pins that the daemon-side Service
// constructor (which the GUI also uses via injection) wires the risk
// engine's builtin rules. Previously this lived in the MCP package, which
// stood up its own Service; after the IPC refactor the MCP process is a
// thin client and the canonical Service builder lives here.
func TestBuildService_WiresRiskDependencies(t *testing.T) {
	dataPath := shortDataDir(t)
	svc, err := buildService(context.Background(), dataPath, "")
	if err != nil {
		t.Fatalf("buildService returned error: %v", err)
	}
	rules, err := svc.ListRiskRules(context.Background(), true)
	if err != nil {
		t.Fatalf("ListRiskRules(includeBuiltin=true) returned error: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected builtin risk rules to be available")
	}
	userRules, err := svc.ListRiskRules(context.Background(), false)
	if err != nil {
		t.Fatalf("ListRiskRules(includeBuiltin=false) returned error: %v", err)
	}
	if len(userRules) != 0 {
		t.Fatalf("expected no user rules in fresh temp store, got %d", len(userRules))
	}
}
