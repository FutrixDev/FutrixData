package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/auth"
	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/toolexec"
)

// TestAuditedCall_EmptyKeyHumanPath asserts the gate is a no-op when the user
// runs the CLI without --agent-access-key. This is the path a human operator
// uses from a terminal and must not produce audit rows.
func TestAuditedCall_EmptyKeyHumanPath(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	calls := 0
	svc := &fakeService{
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			calls++
			return []datasource.DataSource{{ID: "ds_1"}}, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--json", "datasource", "list"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}
	if calls != 1 {
		t.Fatalf("expected 1 service call, got %d", calls)
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected no audit rows for human path; got %s", readAuditFile(t, dataPath))
	}
}

// TestAuditedCall_ValidKeyAuditsSuccess asserts a valid agent key both runs
// the call AND records a Source=cli audit row pointing at the tool registry
// name (not the CLI subcommand verb).
func TestAuditedCall_ValidKeyAuditsSuccess(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	svc := &fakeService{
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			return []datasource.DataSource{{ID: "ds_1"}}, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d: stderr=%s", code, stderr.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Protocol != string(toolexec.SourceCLI) {
		t.Fatalf("expected protocol=cli, got %q", got.Protocol)
	}
	if got.ToolName != "list_datasources" {
		t.Fatalf("expected toolName=list_datasources, got %q", got.ToolName)
	}
	if got.Status != agentaudit.StatusSuccess {
		t.Fatalf("expected status=success, got %q", got.Status)
	}
	if got.AccessKey != accessKey {
		t.Fatalf("expected accessKey=%q, got %q", accessKey, got.AccessKey)
	}
}

// TestAuditedCall_RevokedKeyRejects asserts a revoked key short-circuits
// before the service runs and writes the rate-limited revocation row that
// makes mid-flight revocations forensically auditable.
func TestAuditedCall_RevokedKeyRejects(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	calls := 0
	svc := &fakeService{
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			calls++
			return nil, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for revoked key, got 0; stdout=%s", stdout.String())
	}
	if calls != 0 {
		t.Fatalf("expected service NOT to be called for revoked key, got %d calls", calls)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 revocation audit row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected revocation row status=error, got %q", entries[0].Status)
	}
	if entries[0].Protocol != string(toolexec.SourceCLI) {
		t.Fatalf("expected revocation row protocol=cli, got %q", entries[0].Protocol)
	}
}

// TestAuditedCall_UnknownKeyRejects asserts a key that does not match any
// stored identity is rejected without running the call. Unlike revocation,
// no audit row is written — the row would have to reference an identity we
// don't have, and AppendToolCall refuses unknown keys by design.
func TestAuditedCall_UnknownKeyRejects(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	calls := 0
	svc := &fakeService{
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			calls++
			return nil, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", "ak_does_not_exist_zzzz", "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for unknown key, got 0; stdout=%s", stdout.String())
	}
	if calls != 0 {
		t.Fatalf("expected service NOT to be called for unknown key, got %d calls", calls)
	}
}

func TestAuditedCall_DatasourceAllowlistRejectsBeforeOperation(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.SetDatasourceScope(accessKey, agentaudit.DatasourceScopeAllowList, []string{"ds_allowed"}); err != nil {
		t.Fatalf("SetDatasourceScope: %v", err)
	}

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	calls := 0
	_, err := auditedCall(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_denied", "statement": "SELECT 1"}, func(context.Context) (any, error) {
		calls++
		return map[string]any{"ok": true}, nil
	})
	if err == nil {
		t.Fatal("expected out-of-scope datasource to reject")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("operation ran despite out-of-scope datasource, calls=%d", calls)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 forbidden audit row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", entries[0].Status)
	}
	if entries[0].DatasourceID != "ds_denied" {
		t.Fatalf("expected datasourceId=ds_denied, got %q", entries[0].DatasourceID)
	}
	if entries[0].DatasourceName != "" || entries[0].DatasourceType != "" {
		t.Fatalf("forbidden row should not enrich datasource metadata, got name=%q type=%q", entries[0].DatasourceName, entries[0].DatasourceType)
	}
}

func TestAuditedCall_DatasourceAllowlistRejectsInventoryBeforeOperation(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.SetDatasourceScope(accessKey, agentaudit.DatasourceScopeAllowList, []string{"ds_allowed"}); err != nil {
		t.Fatalf("SetDatasourceScope: %v", err)
	}

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	calls := 0
	_, err := auditedCall(context.Background(), opts, &fakeService{}, "list_datasources", nil, func(context.Context) (any, error) {
		calls++
		return []datasource.DataSource{{ID: "ds_allowed"}}, nil
	})
	if err == nil {
		t.Fatal("expected allowlisted datasource inventory to reject")
	}
	if !strings.Contains(err.Error(), "full datasource inventory") {
		t.Fatalf("expected inventory scope error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("operation ran despite inventory scope rejection, calls=%d", calls)
	}
}

func TestAuditedCall_KeyExpiredDuringExecutionReturnsError(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCall(context.Background(), opts, &fakeService{}, "list_datasources", nil, func(context.Context) (any, error) {
		if _, expiryErr := store.SetExpiresAt(accessKey, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); expiryErr != nil {
			t.Fatalf("SetExpiresAt: %v", expiryErr)
		}
		return []datasource.DataSource{{ID: "ds_1"}}, nil
	})
	if !errors.Is(err, agentaudit.ErrAccessExpired) {
		t.Fatalf("expected ErrAccessExpired, got %v", err)
	}
}

// TestAuditedCall_ErrorPathRecordsFailure asserts that when the service call
// returns an error, we still emit an audit row with status=error so the
// forensic trail captures attempted-but-failed calls — not just successes.
func TestAuditedCall_ErrorPathRecordsFailure(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	svc := &fakeService{
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			return nil, errSimulated{}
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for upstream error, got 0; stdout=%s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit row for error path, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", entries[0].Status)
	}
	if !strings.Contains(entries[0].Message, "simulated") {
		t.Fatalf("expected error message to surface, got %q", entries[0].Message)
	}
}

// errSimulated is a sentinel error used by error-path tests to assert the
// helper plumbs the message into the audit row's Message field.
type errSimulated struct{}

func (errSimulated) Error() string { return "simulated upstream failure" }

// errWithRiskInfo lets a test simulate the console adapter signaling
// "static check passed but Guard EXPLAIN says approval required". The CLI
// converts this into an approvalRequired prompt rather than a hard error;
// auditedCall must NOT log it as StatusError.
type errWithRiskInfo struct{}

func (errWithRiskInfo) Error() string { return "approval required: dangerous statement" }
func (errWithRiskInfo) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return console.ExecuteRiskInfo{Action: "require_approval", Level: "high"}
}

// TestApprovalGate_EmptyKeyHumanPath asserts the approval prompt path is
// untouched when no agent key is present — the human ops contract.
func TestApprovalGate_EmptyKeyHumanPath(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	// In --json mode an approval-required prompt prints the envelope and exits
	// 0; the envelope's ok=false carries the failure signal.
	code := runner.Run([]string{"--data-path", dataPath, "--json", "datasource", "delete", "--id", "ds_x"})
	if code != 0 {
		t.Fatalf("expected exit 0 (approval prompt printed), got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("expected approvalRequired envelope in stdout, got %s", stdout.String())
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected no audit rows for human approval prompt; got %s", readAuditFile(t, dataPath))
	}
}

func TestApprovalGate_EmptyKeyHumanTextKeepsApproveGuidance(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "datasource", "delete", "--id", "ds_x"})
	if code != 1 {
		t.Fatalf("expected non-json human approval prompt to exit 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires --approve") {
		t.Fatalf("expected human CLI guidance to preserve --approve retry, got stderr=%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "waiting for user approval in FutrixData") {
		t.Fatalf("human CLI path must not imply an external pending approval event, got stderr=%s", stderr.String())
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected no audit rows for human approval prompt; got %s", readAuditFile(t, dataPath))
	}
}

// TestApprovalGate_ValidKeyWritesRejectedRow asserts a valid agent key is
// rejected when the operation requires approval, mirroring toolexec.Dispatch's
// behavior so the CLI direct path is forensically symmetric with `tool call`.
func TestApprovalGate_ValidKeyWritesRejectedRow(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "delete", "--id", "ds_x"})
	if code != 1 {
		t.Fatalf("expected approval rejection exit 1, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("agent approval rejection must not render approvalRequired, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection message, got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.ToolName != "delete_datasource" {
		t.Fatalf("expected toolName=delete_datasource, got %q", got.ToolName)
	}
	if got.Protocol != string(toolexec.SourceCLI) {
		t.Fatalf("expected protocol=cli, got %q", got.Protocol)
	}
}

// TestApprovalGate_RevokedKeyRejectsBeforePrompt asserts a revoked agent key
// is rejected at the approval-required path before the prompt envelope is
// rendered, and writes a revocation audit row. This is the core fix from
// codex's [P1]: without it, revoked agents got the prompt back, which leaks
// the registered tool name + parameters.
func TestApprovalGate_RevokedKeyRejectsBeforePrompt(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "delete", "--id", "ds_x"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for revoked key, got 0; stdout=%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("revoked key must NOT receive approvalRequired envelope; got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 revocation audit row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected revocation row status=error, got %q", entries[0].Status)
	}
}

// TestApprovalGate_UnknownKeyRejectsBeforePrompt asserts an unknown agent key
// short-circuits the approval-required path without rendering the prompt and
// without producing a row (AppendToolCall refuses unknown keys by design).
func TestApprovalGate_UnknownKeyRejectsBeforePrompt(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", "ak_does_not_exist_zzzz", "--json", "datasource", "delete", "--id", "ds_x"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for unknown key, got 0; stdout=%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("unknown key must NOT receive approvalRequired envelope; got %s", stdout.String())
	}
}

// TestAuditedCall_RiskInfoErrorStillRecordsFailure asserts that auditedCall
// audits ExecuteRiskInfo-bearing errors as StatusError. This guards
// console execute --approve where a riskengine.BlockedError satisfies the
// RiskInfo interface but represents a hard block — the audit trail must
// keep that record. The narrower auditedCallApprovalRedirect helper is the
// right tool for the approval-redirect path.
func TestAuditedCall_RiskInfoErrorStillRecordsFailure(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCall(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_x", "statement": "DROP TABLE foo"}, func(context.Context) (any, error) {
		return nil, errWithRiskInfo{}
	})
	if err == nil {
		t.Fatal("expected RiskInfo error to surface")
	}
	if !errors.As(err, new(console.ExecuteRiskInfoProvider)) {
		t.Fatalf("expected RiskInfo error to pass through, got %T: %v", err, err)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 StatusError row for RiskInfo err under auditedCall, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", entries[0].Status)
	}
}

// TestAuditedCallApprovalRedirect_RiskInfoSkipsErrorRow asserts the
// narrower approval-redirect helper drops the StatusError write when
// the err carries ExecuteRiskInfo. The caller is expected to convert the
// err into an approval prompt and let validateAgentAccessForApproval log
// the canonical StatusApprovalRequired row.
func TestAuditedCallApprovalRedirect_RiskInfoSkipsErrorRow(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCallApprovalRedirect(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_x", "statement": "DROP TABLE foo"}, func(context.Context) (any, error) {
		return nil, errWithRiskInfo{}
	})
	if err == nil {
		t.Fatal("expected RiskInfo error to surface")
	}
	if !errors.As(err, new(console.ExecuteRiskInfoProvider)) {
		t.Fatalf("expected RiskInfo error to pass through, got %T: %v", err, err)
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected NO row for RiskInfo err under auditedCallApprovalRedirect, got %s", readAuditFile(t, dataPath))
	}
}

// TestAuditedCallApprovalRedirect_NonRiskInfoErrorRecordsFailure asserts
// the approval-redirect variant still audits non-RiskInfo errors so a
// genuine adapter failure does not silently disappear from the trail.
func TestAuditedCallApprovalRedirect_NonRiskInfoErrorRecordsFailure(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCallApprovalRedirect(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_x"}, func(context.Context) (any, error) {
		return nil, errSimulated{}
	})
	if err == nil {
		t.Fatal("expected error to surface")
	}
	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 StatusError row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", entries[0].Status)
	}
}

// errWithBlockedRiskInfo simulates a riskengine.BlockedError that satisfies
// the ExecuteRiskInfoProvider interface but represents a hard block, not an
// approval-required ask. Mirrors what toolexec.Dispatch sees when a "block"
// rule fires inside the adapter Guard at execute time.
type errWithBlockedRiskInfo struct{}

func (errWithBlockedRiskInfo) Error() string { return "blocked by policy: dangerous statement" }
func (errWithBlockedRiskInfo) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return console.ExecuteRiskInfo{
		Action:          "block",
		Level:           "high",
		RuleID:          "sql-block-truncate",
		RuleCode:        "RR-TRUNC-1",
		RuleDescription: "Block TRUNCATE without explicit policy",
	}
}

// TestAuditedCallApprovalRedirect_BlockedRiskInfoRecordsFailure pins the
// codex pass-7 [P2] fix. auditedCallApprovalRedirect is the "drop the
// StatusError row only when caller will redirect to approval" variant — so
// the row-skipping branch must be Action=require_approval-only. A
// BlockedError satisfies the same RiskInfo interface but represents a hard
// rejection; suppressing its row would erase the forensic trail of the
// blocked execution and the caller (commands_console.go execute path) would
// incorrectly route it into the approval prompt. The fix asserts a
// StatusError row IS written and the err passes through unchanged.
func TestAuditedCallApprovalRedirect_BlockedRiskInfoRecordsFailure(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCallApprovalRedirect(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_x", "statement": "TRUNCATE foo"}, func(context.Context) (any, error) {
		return nil, errWithBlockedRiskInfo{}
	})
	if err == nil {
		t.Fatal("expected blocked RiskInfo error to surface")
	}
	if !errors.As(err, new(console.ExecuteRiskInfoProvider)) {
		t.Fatalf("expected RiskInfo error to pass through, got %T: %v", err, err)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 StatusError row for blocked RiskInfo, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error for blocked action, got %q", got.Status)
	}
	if got.RiskAttribution == nil {
		t.Fatal("expected risk attribution preserved on block row, got nil")
	}
	if got.RiskAttribution.RuleID != "sql-block-truncate" {
		t.Fatalf("expected ruleId=sql-block-truncate, got %q", got.RiskAttribution.RuleID)
	}
}

// errWithWarnRiskInfo simulates a warn-action rule firing inside the
// adapter Guard during the optimistic ExecuteStatement call. The daemon
// path doesn't fail-closed on warn (it sets WithUserApproved which
// suppresses the Guard); the CLI direct path doesn't set that flag, so
// the Guard can still surface a warn-action RiskInfo. The CLI must NOT
// turn that into a hard StatusError — it must route into the approval
// prompt, mirroring the legacy "any RiskInfo → approval prompt" behavior
// for everything except block.
type errWithWarnRiskInfo struct{}

func (errWithWarnRiskInfo) Error() string {
	return "warn: medium-risk statement on cautious datasource"
}
func (errWithWarnRiskInfo) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return console.ExecuteRiskInfo{
		Action:          "warn",
		Level:           "medium",
		RuleID:          "sql-warn-update",
		RuleCode:        "RR-UPDATE-1",
		RuleDescription: "Warn on UPDATE without LIMIT",
	}
}

// TestAuditedCallApprovalRedirect_WarnRiskInfoSkipsErrorRow pins the codex
// pass-9 [P2] fix at the helper layer. A warn-action RiskInfo must NOT
// trigger a StatusError row — the caller will route it into
// validateAgentAccessForApproval, which writes the canonical
// StatusApprovalRequired row. Treating warn as StatusError would regress
// the legacy CLI flow that prompted for approval on any non-allow
// adapter-Guard signal, and would diverge from the daemon path which
// doesn't surface warn as a hard error at all.
func TestAuditedCallApprovalRedirect_WarnRiskInfoSkipsErrorRow(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	opts := Options{DataPath: dataPath, AgentAccessKey: accessKey}
	_, err := auditedCallApprovalRedirect(context.Background(), opts, &fakeService{}, "execute_statement", map[string]any{"datasourceId": "ds_x", "statement": "UPDATE foo SET x=1"}, func(context.Context) (any, error) {
		return nil, errWithWarnRiskInfo{}
	})
	if err == nil {
		t.Fatal("expected warn RiskInfo error to surface")
	}
	if !errors.As(err, new(console.ExecuteRiskInfoProvider)) {
		t.Fatalf("expected RiskInfo error to pass through, got %T: %v", err, err)
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected NO StatusError row for warn RiskInfo (caller writes the approval row), got %s", readAuditFile(t, dataPath))
	}
}

// TestApprovalGate_ConsoleExecuteWarnRiskInfoRejectsWithAttribution pins the
// direct-CLI agent path: when an adapter Guard escalates to warn, the agent
// cannot approve it, so the call is rejected and the matched rule remains
// visible in the audit row.
func TestApprovalGate_ConsoleExecuteWarnRiskInfoRejectsWithAttribution(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	svc := &fakeService{
		// Static check allows; adapter Guard escalates to warn at execute.
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
		},
		executeStatementFn: func(_ context.Context, _, _, _, _ string, _ int, _ string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
			return console.QueryResult{}, errWithWarnRiskInfo{}
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_x", "--statement", "UPDATE foo SET x=1"})
	if code != 1 {
		t.Fatalf("expected approval rejection exit 1, got %d; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("agent approval rejection must not render approvalRequired, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection message, got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 rejected row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error for warn approval rejection, got %q", got.Status)
	}
	if got.RiskAttribution == nil || got.RiskAttribution.RuleID != "sql-warn-update" {
		t.Fatalf("expected warn rule attribution preserved, got %+v", got.RiskAttribution)
	}
}

// TestApprovalGate_ConsoleExecuteBlockedRiskInfoNoApprovalRedirect pins the
// caller-side half of the pass-7 fix. When the adapter Guard returns a
// blocked RiskInfo error, the execute handler must NOT route it through
// validateAgentAccessForApproval / approvalRequired — the action is `block`,
// not `require_approval`. The forensic trail should show a single
// StatusError row from auditedCallApprovalRedirect, the agent should see the
// block error, and no approvalRequired envelope should reach stdout.
func TestApprovalGate_ConsoleExecuteBlockedRiskInfoNoApprovalRedirect(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	svc := &fakeService{
		// Static check allows; adapter Guard escalates to a hard block.
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
		},
		executeStatementFn: func(_ context.Context, _, _, _, _ string, _ int, _ string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
			return console.QueryResult{}, errWithBlockedRiskInfo{}
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_x", "--statement", "TRUNCATE foo"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for blocked statement, got 0; stdout=%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("blocked action must NOT receive approvalRequired envelope; got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 StatusError row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.ToolName != "execute_statement" {
		t.Fatalf("expected toolName=execute_statement, got %q", got.ToolName)
	}
	if got.RiskAttribution == nil || got.RiskAttribution.RuleID != "sql-block-truncate" {
		t.Fatalf("expected blocked rule attribution preserved, got %+v", got.RiskAttribution)
	}
}

// TestApprovalGate_ConsoleExecuteRevokedKey asserts the console execute
// approval path also gates on agent-key validation, mirroring the datasource
// and d1 fixes. The codex review's second pass caught that the execute
// pathway returned approvalRequired without validateAgentAccessForApproval.
//
// This test additionally pins the pass-3 [P2] fix: a revoked key must
// short-circuit BEFORE the service is touched. Before the pre-flight gate,
// AssessStatement (which reads the datasource and runs the riskengine) was
// invoked first and only then was the access key validated — leaking service
// access to invalid keys. The assessCalls counter asserts the gate fires
// before the service is reached.
func TestApprovalGate_ConsoleExecuteRevokedKey(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	assessCalls := 0
	// assessStatementFn would force the approval branch if reached. With the
	// pre-flight gate it must NEVER fire for a revoked key.
	svc := &fakeService{
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			assessCalls++
			return riskengine.RiskAssessment{Level: riskengine.RiskHigh, Action: riskengine.ActionRequireApproval}, nil
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_x", "--statement", "DROP TABLE foo"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for revoked key, got 0; stdout=%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("revoked key must NOT receive approvalRequired envelope; got %s", stdout.String())
	}
	if assessCalls != 0 {
		t.Fatalf("expected AssessStatement to be skipped for revoked key, got %d call(s)", assessCalls)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 revocation audit row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected revocation row status=error, got %q", entries[0].Status)
	}
	if entries[0].ToolName != "execute_statement" {
		t.Fatalf("expected toolName=execute_statement, got %q", entries[0].ToolName)
	}
}

// TestApprovalGate_ConsoleExecuteApprovalRejectionCarriesMatchedRule asserts
// that when approval is driven by a riskengine rule match, the rejected audit
// row preserves the matched rule's id/code/description.
func TestApprovalGate_ConsoleExecuteApprovalRejectionCarriesMatchedRule(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	svc := &fakeService{
		// Real rule match — non-empty RuleID + Action != Allow is what
		// AssessStatementApproval surfaces as a Decision.Assessment.
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{
				Level:           riskengine.RiskHigh,
				Action:          riskengine.ActionRequireApproval,
				RuleID:          "sql-block-drop",
				RuleCode:        "RR-DROP-1",
				RuleDescription: "Block DROP TABLE without approval",
			}, nil
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_x", "--statement", "DROP TABLE foo"})
	if code != 1 {
		t.Fatalf("expected approval rejection exit 1, got %d; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("agent approval rejection must not render approvalRequired, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"riskAttribution"`) {
		t.Fatalf("expected approval rejection output to include riskAttribution, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ruleCode": "RR-DROP-1"`) {
		t.Fatalf("expected approvalRequired output to include ruleCode, got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 rejected row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.RiskAttribution == nil {
		t.Fatal("expected risk attribution on approval row, got nil")
	}
	if got.RiskAttribution.RuleID != "sql-block-drop" {
		t.Fatalf("expected ruleId=sql-block-drop, got %q", got.RiskAttribution.RuleID)
	}
	if got.RiskAttribution.RuleCode != "RR-DROP-1" {
		t.Fatalf("expected ruleCode=RR-DROP-1, got %q", got.RiskAttribution.RuleCode)
	}
}

func TestApprovalGate_ConsoleExecuteHumanErrorIncludesWritePreview(t *testing.T) {
	dataPath, _ := setupToolCallIdentity(t)
	svc := &fakeService{
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{
				Level:           riskengine.RiskMedium,
				Action:          riskengine.ActionWarn,
				RuleID:          "sql-warn-delete",
				RuleCode:        "SQL-007",
				RuleDescription: "Warn on DELETE with WHERE",
			}, nil
		},
		previewWriteFn: func(_ context.Context, _, _, _, _ string) (console.WritePreview, error) {
			return console.WritePreview{
				Operation:                "delete",
				TargetEntity:             "rooms",
				EstimatedAffectedRows:    250,
				RequiresElevatedApproval: true,
				ThresholdRows:            100,
			}, nil
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "console", "execute", "--datasource", "ds_x", "--statement", "DELETE FROM rooms WHERE user_id = 'u1'"})
	if code == 0 {
		t.Fatalf("expected non-json approval prompt to exit non-zero; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "estimated affected rows: 250") {
		t.Fatalf("expected human approval error to include estimated affected rows, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "elevated approval required above 100") {
		t.Fatalf("expected human approval error to include elevated approval threshold, got %q", stderr.String())
	}
}

// TestApprovalGate_PolicyAttributionFallback asserts that approval rejections
// on categorical approval paths carry PolicyAttribution(require_approval) so
// the row records *why* approval was required, even when no risk rule was
// evaluated.
func TestApprovalGate_PolicyAttributionFallback(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "delete", "--id", "ds_x"})
	if code != 1 {
		t.Fatalf("expected approval rejection exit 1, got %d; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"source": "policy"`) {
		t.Fatalf("expected rejection output to include policy attribution, got %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"ruleCode"`) {
		t.Fatalf("policy approval output must not include a ruleCode, got %s", stdout.String())
	}
	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 rejected row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.RiskAttribution == nil {
		t.Fatal("expected policy attribution on categorical approval row, got nil")
	}
	if got.RiskAttribution.Source != agentaudit.AttributionSourcePolicy {
		t.Fatalf("expected attribution source=policy, got %q", got.RiskAttribution.Source)
	}
	if got.RiskAttribution.Action != string(riskengine.ActionRequireApproval) {
		t.Fatalf("expected attribution action=require_approval, got %q", got.RiskAttribution.Action)
	}
}

// errWithRiskInfoRule simulates the adapter Guard's EXPLAIN probe escalating
// a statically-allowed statement at execute time, carrying a matched
// riskengine rule. The CLI must preserve this attribution on the approval
// audit row so the daemon and CLI direct paths agree on which rule fired.
type errWithRiskInfoRule struct{}

func (errWithRiskInfoRule) Error() string { return "approval required: dangerous statement" }
func (errWithRiskInfoRule) ExecuteRiskInfo() console.ExecuteRiskInfo {
	return console.ExecuteRiskInfo{
		Action:          "require_approval",
		Level:           "high",
		RuleID:          "sql-explain-large-scan",
		RuleCode:        "RR-SCAN-1",
		RuleDescription: "Block large-scan statements without approval",
	}
}

// TestApprovalGate_AdapterGuardEscalationRejectsAndPreservesAttribution pins the codex
// pass-5 [P2] fix: when the static AssessStatement says "allow" but the
// adapter Guard's EXPLAIN probe escalates at execute time, the rejected audit
// row must carry the Guard's matched-rule attribution — not the generic
// PolicyAttribution(require_approval) fallback. Without this fix, the daemon
// `tool call` path and the CLI direct path would record different
// attributions for the same blocked statement, and the Agent Audit UI would
// lose the rule link for direct-CLI executions.
func TestApprovalGate_AdapterGuardEscalationRejectsAndPreservesAttribution(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	svc := &fakeService{
		// Static check allows; the adapter Guard does the escalation.
		assessStatementFn: func(_ context.Context, _, _, _, _ string) (riskengine.RiskAssessment, error) {
			return riskengine.RiskAssessment{Level: riskengine.RiskLow, Action: riskengine.ActionAllow}, nil
		},
		executeStatementFn: func(_ context.Context, _, _, _, _ string, _ int, _ string, _ ...console.ExecuteBounds) (console.QueryResult, error) {
			return console.QueryResult{}, errWithRiskInfoRule{}
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "console", "execute", "--datasource", "ds_x", "--statement", "SELECT * FROM huge"})
	if code != 1 {
		t.Fatalf("expected approval rejection exit 1, got %d; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "approvalRequired") {
		t.Fatalf("agent approval rejection must not render approvalRequired, got %s", stdout.String())
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 rejected row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.RiskAttribution == nil {
		t.Fatal("expected risk attribution from adapter Guard, got nil")
	}
	if got.RiskAttribution.Source != agentaudit.AttributionSourceRiskEngine {
		t.Fatalf("expected attribution source=risk_engine, got %q", got.RiskAttribution.Source)
	}
	if got.RiskAttribution.RuleID != "sql-explain-large-scan" {
		t.Fatalf("expected ruleId=sql-explain-large-scan, got %q", got.RiskAttribution.RuleID)
	}
	if got.RiskAttribution.RuleCode != "RR-SCAN-1" {
		t.Fatalf("expected ruleCode=RR-SCAN-1, got %q", got.RiskAttribution.RuleCode)
	}
}

// TestApprovalGate_RevokedKeyBeatsLoginRequired pins the codex pass-4 [P2]
// fix: a revoked agent key combined with absent user-login state must surface
// the access-revoked error, not "login required". Previously the dispatch
// called EnsureAuthenticated *before* CheckAccess, so an agent with a bad key
// got "login required" first — masking the real failure and making it look
// like the key was fine but the user just needed to log in. The Runner-level
// gate now bypasses EnsureAuthenticated when the agent key is invalid so the
// handler's preflightAgentAccess can write the canonical revocation row and
// return the access error.
func TestApprovalGate_RevokedKeyBeatsLoginRequired(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	listCalls := 0
	svc := &fakeService{
		// EnsureAuthenticated would mask the access-key error if it ran first.
		ensureAuthFn: func(context.Context) (auth.State, error) {
			return auth.State{}, auth.ErrLoginRequired
		},
		listDatasourcesFn: func(context.Context) ([]datasource.DataSource, error) {
			listCalls++
			return nil, nil
		},
	}
	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for revoked key, got 0; stdout=%s", stdout.String())
	}
	if listCalls != 0 {
		t.Fatalf("expected service NOT to be called, got %d", listCalls)
	}

	body := stdout.String() + stderr.String()
	if strings.Contains(strings.ToLower(body), "login required") {
		t.Fatalf("expected access-revoked error to surface, got login-required: %s", body)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 revocation audit row, got %d (%v)", len(entries), entries)
	}
	if entries[0].Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", entries[0].Status)
	}
	// Canonical toolName preserved — the handler's preflightAgentAccess wrote
	// the row, not a coarse Runner-level fallback.
	if entries[0].ToolName != "list_datasources" {
		t.Fatalf("expected toolName=list_datasources (canonical), got %q", entries[0].ToolName)
	}
}

// newDirectRunner builds a Runner whose serviceFactory always returns svc and
// whose desktop-app validator is a no-op. The CLI direct subcommands run in
// the CLI process, so a test can drive them with just a Service fake — no
// daemon, no IPC.
func newDirectRunner(svc Service) *Runner {
	r := NewRunner(nil, nil)
	r.serviceFactory = func(Options) (Service, error) { return svc, nil }
	r.desktopAppValidator = func() error { return nil }
	return r
}

// errFactoryFailure is a sentinel for the "serviceFactory blew up before any
// subcommand handler ran" path — e.g., corrupted datasources.json, unreadable
// auth store, missing keyring entry.
type errFactoryFailure struct{}

func (errFactoryFailure) Error() string { return "simulated datastore corruption" }

// TestDirectSubcommand_BadKeyMaskedByFactoryFailure pins the codex pass-8 [P2]
// fix. When `serviceFactory` errors (e.g., corrupted datastore) AND
// `--agent-access-key` is invalid, the handler is unreachable so its
// canonical-row write never happens. Before the fix, the user-facing error
// was the factory error ("simulated datastore corruption") and no revocation
// row was written — masking the access rejection from both the agent and the
// audit log. The Runner-level directSubcommand gate now surfaces the
// access-key error instead and writes a coarse revocation row tagged with the
// subcommand verb, restoring "validate first, audit always" symmetry with
// toolexec.Dispatch.
func TestDirectSubcommand_BadKeyMaskedByFactoryFailure(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	runner := NewRunner(nil, nil)
	runner.serviceFactory = func(Options) (Service, error) { return nil, errFactoryFailure{} }
	runner.desktopAppValidator = func() error { return nil }
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	body := stdout.String() + stderr.String()
	if strings.Contains(body, "simulated datastore corruption") {
		t.Fatalf("factory error must NOT mask the access-key error; got: %s", body)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 revocation row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	if got.Protocol != string(toolexec.SourceCLI) {
		t.Fatalf("expected protocol=cli, got %q", got.Protocol)
	}
	// Coarse-fallback toolName is the subcommand verb because the handler
	// never ran and the specific sub-action wasn't parsed at the Runner layer.
	if got.ToolName != "datasource" {
		t.Fatalf("expected toolName=datasource (coarse fallback), got %q", got.ToolName)
	}
}

// TestPayloadCommand_RevokedKeyRejectsBeforeFileRead pins the codex pass-10
// [P2] fix. `datasource test-payload / create / update --file <path>` used to
// open the payload file BEFORE the audit gate fired. A revoked or unknown
// key would surface "file not found" or a JSON parse error first — and a
// failed read left no revocation row at all. The fix adds a Runner-level
// preflightAgentAccess between flag parse and readJSONInput. This test
// uses a path that doesn't exist: with the fix, the access-revoked error
// surfaces (not the missing-file error) and a revocation row is written.
func TestPayloadCommand_RevokedKeyRejectsBeforeFileRead(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	store := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	if _, err := store.Revoke(accessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	runner := newDirectRunner(&fakeService{})
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	missingPath := "/tmp/futrixdata-pass10-missing-payload-DOES-NOT-EXIST.json"
	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "create", "--file", missingPath})
	if code == 0 {
		t.Fatalf("expected non-zero exit; stdout=%s", stdout.String())
	}
	body := stdout.String() + stderr.String()
	if strings.Contains(body, missingPath) {
		t.Fatalf("file path leaked before access-key check; got: %s", body)
	}
	if strings.Contains(strings.ToLower(body), "no such file") || strings.Contains(strings.ToLower(body), "json") {
		t.Fatalf("expected access-revoked error to surface BEFORE file read, got: %s", body)
	}

	entries := listAudit(t, dataPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 revocation row, got %d (%v)", len(entries), entries)
	}
	got := entries[0]
	if got.Status != agentaudit.StatusError {
		t.Fatalf("expected status=error, got %q", got.Status)
	}
	// Canonical tool name preserved: the preflight uses
	// payloadPreflightToolName so the row aligns with what auditedCall would
	// have written had the gate not short-circuited.
	if got.ToolName != "create_datasource" {
		t.Fatalf("expected toolName=create_datasource (canonical), got %q", got.ToolName)
	}
}

// TestDirectSubcommand_GoodKeyFactoryFailureSurfacesFactoryError asserts the
// dual case: when the agent key is valid but serviceFactory fails, the user
// sees the factory error (not the access path) and no audit row is written.
// Without this assertion the pass-8 fix could regress to "always show access
// error" which would itself mask legitimate datastore errors.
func TestDirectSubcommand_GoodKeyFactoryFailureSurfacesFactoryError(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)

	runner := NewRunner(nil, nil)
	runner.serviceFactory = func(Options) (Service, error) { return nil, errFactoryFailure{} }
	runner.desktopAppValidator = func() error { return nil }
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{"--data-path", dataPath, "--agent-access-key", accessKey, "--json", "datasource", "list"})
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}

	body := stdout.String() + stderr.String()
	if !strings.Contains(body, "simulated datastore corruption") {
		t.Fatalf("expected factory error to surface, got: %s", body)
	}
	if !auditFileMissingOrEmpty(t, dataPath) {
		t.Fatalf("expected NO audit row when key is valid + factory fails, got %s", readAuditFile(t, dataPath))
	}
}

// listAudit reads agent-tool-calls.json for assertions. Returns an empty slice
// when the file doesn't exist — the human-path case should leave it absent.
func listAudit(t *testing.T, dataPath string) []agentaudit.AuditEntry {
	t.Helper()
	path := bootstrap.AgentAuditPath(dataPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	entries, err := agentaudit.NewAuditStore(path).List(agentaudit.AuditFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	return entries
}

// TestAuditedCall_SchemaEgressDeniesUnsetConsent pins the codex P1 finding:
// the direct CLI surface for schema-emitting tools (list_entities,
// describe_entity, get_schema_knowledge, get_er_knowledge) used to bypass the
// schemaprivacy consent check that toolexec.Dispatch enforces. With the gate
// shared via toolexec.SchemaEgressPreflight, both surfaces now reject calls
// against a datasource whose consent is unset/denied — even when the agent
// has a valid access key.
func TestAuditedCall_SchemaEgressDeniesUnsetConsent(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{ID: id, Type: datasource.TypeMySQL}, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{
		"--data-path", dataPath,
		"--agent-access-key", accessKey,
		"--json",
		"console", "entities",
		"--datasource", "ds-unset",
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit (gate must deny), got 0: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	entries := listAudit(t, dataPath)
	// The denial path writes an agent-audit StatusError row tagged with the
	// schema_egress_denied policy attribution.
	found := false
	for _, e := range entries {
		if e.ToolName == "list_entities" && e.Status == agentaudit.StatusError {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected list_entities StatusError audit row, got: %v", entries)
	}
}

// TestAuditedCall_SchemaEgressAllowsConsented confirms the gate is permissive
// when the datasource has explicit Allowed consent — same policy as the
// daemon path.
func TestAuditedCall_SchemaEgressAllowsConsented(t *testing.T) {
	dataPath, accessKey := setupToolCallIdentity(t)
	svc := &fakeService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			return datasource.DataSource{
				ID:   id,
				Type: datasource.TypeMySQL,
				Options: map[string]any{
					"schemaToLLM": "allowed",
				},
			}, nil
		},
	}

	runner := newDirectRunner(svc)
	var stdout, stderr bytes.Buffer
	runner.stdout = &stdout
	runner.stderr = &stderr

	code := runner.Run([]string{
		"--data-path", dataPath,
		"--agent-access-key", accessKey,
		"--json",
		"console", "entities",
		"--datasource", "ds-allow",
	})
	if code != 0 {
		t.Fatalf("expected exit 0 with consent allowed, got %d: stderr=%s", code, stderr.String())
	}
	entries := listAudit(t, dataPath)
	if len(entries) == 0 {
		t.Fatalf("expected at least one audit row")
	}
	last := entries[len(entries)-1]
	if last.ToolName != "list_entities" || last.Status != agentaudit.StatusSuccess {
		t.Fatalf("expected list_entities/success audit row, got %+v", last)
	}
}

func auditFileMissingOrEmpty(t *testing.T, dataPath string) bool {
	t.Helper()
	return len(listAudit(t, dataPath)) == 0
}

func readAuditFile(t *testing.T, dataPath string) string {
	t.Helper()
	path := bootstrap.AgentAuditPath(dataPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
