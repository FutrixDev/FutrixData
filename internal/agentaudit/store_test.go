package agentaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"futrixdata/platform/internal/bootstrap"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/securefile"
)

func TestIdentityStoreCreateManualAndRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if identity.AccessKey == "" {
		t.Fatal("expected access key")
	}
	if identity.Name == "" {
		t.Fatal("expected default name")
	}

	renamed, err := store.Rename(identity.AccessKey, "analytics-agent")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name != "analytics-agent" {
		t.Fatalf("renamed.Name = %q, want analytics-agent", renamed.Name)
	}
}

func TestIdentityStoreEnsureBoundRecreatesMissingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.EnsureBound("agent_bound_1234", "claude", "Claude Code")
	if err != nil {
		t.Fatalf("EnsureBound: %v", err)
	}
	if identity.AccessKey != "agent_bound_1234" {
		t.Fatalf("access key = %q, want %q", identity.AccessKey, "agent_bound_1234")
	}
	if identity.AgentType != "claude" {
		t.Fatalf("agent type = %q, want claude", identity.AgentType)
	}
	if identity.Source != SourceDetected {
		t.Fatalf("source = %q, want detected", identity.Source)
	}

	loaded, found, err := store.Get("agent_bound_1234")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected recreated identity to exist")
	}
	if loaded.Name != identity.Name {
		t.Fatalf("loaded name = %q, want %q", loaded.Name, identity.Name)
	}
}

func TestAppendToolCallWritesAuditEntry(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("agent-1234")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	svc := &stubService{
		ds: datasource.DataSource{ID: "ds_1", Name: "Primary", Type: datasource.TypeMySQL},
	}
	err = AppendToolCall(dataPath, svc, "skill", identity.AccessKey, "execute_statement", map[string]any{
		"datasourceId": "ds_1",
		"statement":    "SELECT * FROM users\nLIMIT 50",
	}, StatusSuccess, "")
	if err != nil {
		t.Fatalf("AppendToolCall: %v", err)
	}

	items, err := NewAuditStore(bootstrap.AgentAuditPath(dataPath)).List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	if items[0].Protocol != "skill" {
		t.Fatalf("protocol = %q, want skill", items[0].Protocol)
	}
	if items[0].DatasourceName != "Primary" {
		t.Fatalf("datasourceName = %q, want Primary", items[0].DatasourceName)
	}
	if items[0].Summary != "SELECT * FROM users" {
		t.Fatalf("summary = %q, want first statement line", items[0].Summary)
	}
	if items[0].Statement != "SELECT * FROM users\nLIMIT 50" {
		t.Fatalf("statement = %q, want full statement", items[0].Statement)
	}
	if items[0].Seq != 1 {
		t.Fatalf("seq = %d, want 1", items[0].Seq)
	}
	if items[0].ChainVersion != AuditChainVersion {
		t.Fatalf("chainVersion = %q, want %q", items[0].ChainVersion, AuditChainVersion)
	}
	if items[0].PayloadHash == "" || items[0].ChainHash == "" {
		t.Fatalf("expected hash-chain fields, got payloadHash=%q chainHash=%q", items[0].PayloadHash, items[0].ChainHash)
	}
}

func TestAuditStoreVerifyPassesForHashChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)

	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 2")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 3")

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if !result.Pass {
		t.Fatalf("expected chain to pass, got %+v", result)
	}
	if result.VerifiedRecords != 3 {
		t.Fatalf("verifiedRecords = %d, want 3", result.VerifiedRecords)
	}
	if result.Path != path || result.Source != "file" {
		t.Fatalf("unexpected source fields: %+v", result)
	}
}

func TestAuditStoreVerifyIdentifiesLegacyRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	legacy := AuditEntry{
		ID:         "audit_legacy",
		AccessKey:  "agent_legacy",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT legacy",
		Status:     StatusSuccess,
		ExecutedAt: "2026-04-29T00:00:00Z",
	}
	writeAuditLines(t, path, legacy)

	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT new")

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if !result.Pass {
		t.Fatalf("expected legacy prefix plus new chain to pass, got %+v", result)
	}
	if result.LegacyRecords != 1 || result.VerifiedRecords != 1 {
		t.Fatalf("legacy=%d verified=%d, want legacy=1 verified=1", result.LegacyRecords, result.VerifiedRecords)
	}
	entries := readAuditLines(t, path)
	if entries[1].Seq != 2 {
		t.Fatalf("first chained row after legacy seq = %d, want physical row number 2", entries[1].Seq)
	}
	if entries[1].PrevHash != AuditChainGenesisHash {
		t.Fatalf("first chained row after legacy prevHash = %q, want genesis hash", entries[1].PrevHash)
	}
}

func TestAuditStoreVerifyDetectsModifiedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 2")

	lines := readAuditLines(t, path)
	lines[1].Summary = "SELECT 999"
	writeAuditLines(t, path, lines...)

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if result.Pass {
		t.Fatalf("expected modified row to fail verification")
	}
	if result.FirstBrokenPosition != 2 {
		t.Fatalf("firstBrokenPosition = %d, want 2 (%+v)", result.FirstBrokenPosition, result)
	}
	if result.ExpectedHash == "" || result.ActualHash == "" || result.ExpectedHash == result.ActualHash {
		t.Fatalf("expected different hash details, got %+v", result)
	}
}

func TestAuditStoreVerifyDetectsDeletedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 2")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 3")

	lines := readAuditLines(t, path)
	writeAuditLines(t, path, lines[0], lines[2])

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if result.Pass {
		t.Fatalf("expected deleted row to fail verification")
	}
	if result.FirstBrokenPosition != 2 {
		t.Fatalf("firstBrokenPosition = %d, want 2 (%+v)", result.FirstBrokenPosition, result)
	}
}

func TestAuditStoreVerifyDetectsInsertedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 2")

	lines := readAuditLines(t, path)
	inserted := AuditEntry{
		ID:         "audit_inserted",
		AccessKey:  "agent_legacy",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT inserted",
		Status:     StatusSuccess,
		ExecutedAt: "2026-04-29T00:00:00Z",
	}
	writeAuditLines(t, path, lines[0], inserted, lines[1])

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if result.Pass {
		t.Fatalf("expected inserted row to fail verification")
	}
	if result.FirstBrokenPosition != 2 {
		t.Fatalf("firstBrokenPosition = %d, want 2 (%+v)", result.FirstBrokenPosition, result)
	}
}

func TestAuditStoreVerifyDetectsReorderedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 2")
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 3")

	lines := readAuditLines(t, path)
	writeAuditLines(t, path, lines[0], lines[2], lines[1])

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if result.Pass {
		t.Fatalf("expected reordered rows to fail verification")
	}
	if result.FirstBrokenPosition != 2 {
		t.Fatalf("firstBrokenPosition = %d, want 2 (%+v)", result.FirstBrokenPosition, result)
	}
}

func TestAuditStoreVerifyDetectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	appendTestAuditEntry(t, store, "agent_chain", "SELECT 1")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	raw = bytes.Replace(raw, []byte(`"summary":"SELECT 1"`), []byte(`"summary":"SELECT 1","summary":"SELECT changed"`), 1)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write duplicate-key audit file: %v", err)
	}

	result, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if result.Pass {
		t.Fatalf("expected duplicate-key row to fail verification")
	}
	if result.FirstBrokenPosition != 1 {
		t.Fatalf("firstBrokenPosition = %d, want 1 (%+v)", result.FirstBrokenPosition, result)
	}
	if !strings.Contains(result.Reason, "duplicate") {
		t.Fatalf("reason = %q, want duplicate-key failure", result.Reason)
	}
}

func appendTestAuditEntry(t *testing.T, store *AuditStore, accessKey, statement string) {
	t.Helper()
	if err := store.Append(AuditEntry{
		AccessKey:  accessKey,
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    statement,
		Statement:  statement,
		Status:     StatusSuccess,
		ExecutedAt: "2026-04-29T00:00:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func readAuditLines(t *testing.T, path string) []AuditEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	entries := make([]AuditEntry, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("unmarshal audit line %q: %v", string(line), err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func writeAuditLines(t *testing.T, path string, entries ...AuditEntry) {
	t.Helper()
	var buf bytes.Buffer
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal audit entry: %v", err)
		}
		buf.Write(payload)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir audit dir: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write audit file: %v", err)
	}
}

func TestAppendToolCallWithAttributionRoundTripsRiskRule(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("agent-attr")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	svc := &stubService{ds: datasource.DataSource{ID: "ds_1", Name: "Primary", Type: datasource.TypeMySQL}}

	attribution := AttributionFromAssessment(riskengine.RiskAssessment{
		RuleID:          "rule_delete",
		RuleCode:        "delete_full_table",
		RuleDescription: "Delete without WHERE",
		Action:          riskengine.ActionRequireApproval,
		Level:           riskengine.RiskHigh,
		Reasons:         []string{"DELETE without WHERE on users"},
	})
	if attribution == nil {
		t.Fatal("AttributionFromAssessment returned nil for non-empty assessment")
	}

	if err := AppendToolCallWithAttribution(dataPath, svc, "skill", identity.AccessKey, "execute_statement", map[string]any{
		"datasourceId": "ds_1",
		"statement":    "DELETE FROM users",
	}, StatusApprovalRequired, "", attribution); err != nil {
		t.Fatalf("AppendToolCallWithAttribution: %v", err)
	}

	items, err := NewAuditStore(bootstrap.AgentAuditPath(dataPath)).List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	got := items[0].RiskAttribution
	if got == nil {
		t.Fatal("expected RiskAttribution to round-trip, got nil")
	}
	if got.Source != AttributionSourceRiskEngine {
		t.Fatalf("Source = %q, want %q", got.Source, AttributionSourceRiskEngine)
	}
	if got.RuleID != "rule_delete" {
		t.Fatalf("RuleID = %q, want rule_delete", got.RuleID)
	}
	if got.RuleCode != "delete_full_table" {
		t.Fatalf("RuleCode = %q, want delete_full_table", got.RuleCode)
	}
	if got.RuleDescription != "Delete without WHERE" {
		t.Fatalf("RuleDescription = %q, want Delete without WHERE", got.RuleDescription)
	}
	if got.Action != string(riskengine.ActionRequireApproval) {
		t.Fatalf("Action = %q, want %q", got.Action, riskengine.ActionRequireApproval)
	}
	if got.Level != string(riskengine.RiskHigh) {
		t.Fatalf("Level = %q, want %q", got.Level, riskengine.RiskHigh)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "DELETE without WHERE on users" {
		t.Fatalf("Reasons = %v, want one reason", got.Reasons)
	}
}

func TestAppendToolCallWithAttributionPolicySource(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("agent-pol")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	svc := &stubService{ds: datasource.DataSource{ID: "ds_1", Name: "Primary", Type: datasource.TypeMySQL}}

	attribution := PolicyAttribution(string(riskengine.ActionRequireApproval))
	if err := AppendToolCallWithAttribution(dataPath, svc, "mcp", identity.AccessKey, "create_datasource", map[string]any{
		"name": "new-db",
	}, StatusApprovalRequired, "", attribution); err != nil {
		t.Fatalf("AppendToolCallWithAttribution: %v", err)
	}

	items, err := NewAuditStore(bootstrap.AgentAuditPath(dataPath)).List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	got := items[0].RiskAttribution
	if got == nil {
		t.Fatal("expected RiskAttribution for policy source")
	}
	if got.Source != AttributionSourcePolicy {
		t.Fatalf("Source = %q, want %q", got.Source, AttributionSourcePolicy)
	}
	if got.RuleID != "" {
		t.Fatalf("policy attribution should not carry RuleID, got %q", got.RuleID)
	}
}

func TestAppendToolCallLeavesRiskAttributionNil(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("agent-noattr")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	svc := &stubService{ds: datasource.DataSource{ID: "ds_1", Name: "Primary", Type: datasource.TypeMySQL}}

	if err := AppendToolCall(dataPath, svc, "skill", identity.AccessKey, "execute_statement", map[string]any{
		"datasourceId": "ds_1",
		"statement":    "SELECT 1",
	}, StatusSuccess, ""); err != nil {
		t.Fatalf("AppendToolCall: %v", err)
	}

	items, err := NewAuditStore(bootstrap.AgentAuditPath(dataPath)).List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(items))
	}
	if items[0].RiskAttribution != nil {
		t.Fatalf("AppendToolCall should not synthesize attribution, got %+v", items[0].RiskAttribution)
	}
}

func TestAppendToolCallReturnsErrorWhenIdentityMissing(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")

	err := AppendToolCall(dataPath, nil, "mcp", "agent_missing_1234", "execute_statement", map[string]any{
		"statement": "SELECT 1",
	}, StatusSuccess, "")
	if err == nil {
		t.Fatal("expected missing identity error")
	}
	if !errors.Is(err, errIdentityNotFound) {
		t.Fatalf("expected errIdentityNotFound, got %v", err)
	}
}

// TestLogRevokedAccessSkipsServiceEnrichment pins the security boundary
// behind the gate refactor: when a revoked key triggers an audit row, the
// caller's toolreg.Service must NOT be invoked to enrich the row with the
// datasource name/type. Otherwise an invalid-key path still surfaces a
// service-side state read, defeating the "validate first, then act"
// contract that the CLI gate (and toolexec.Dispatch) rely on.
func TestLogRevokedAccessSkipsServiceEnrichment(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("agent-revoked")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if _, err := identityStore.Revoke(identity.AccessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Reset the per-key throttle so this test is independent of any earlier
	// LogRevokedAccess calls in the same test binary.
	revokedAuditMu.Lock()
	delete(revokedAuditLastLog, identity.AccessKey)
	revokedAuditMu.Unlock()

	svc := &countingDatasourceService{stubService: stubService{ds: datasource.DataSource{ID: "ds_1", Name: "Primary", Type: datasource.TypeMySQL}}}
	LogRevokedAccess(dataPath, svc, "cli", identity.AccessKey, "execute_statement", map[string]any{
		"datasourceId": "ds_1",
		"statement":    "SELECT 1",
	}, "agent access revoked")

	if svc.getCalls != 0 {
		t.Fatalf("revoked-key path called GetDatasource %d times, want 0 (service must not be touched after access rejection)", svc.getCalls)
	}

	items, err := NewAuditStore(bootstrap.AgentAuditPath(dataPath)).List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 revocation row, got %d", len(items))
	}
	if items[0].Status != StatusError {
		t.Fatalf("status = %q, want StatusError", items[0].Status)
	}
	if items[0].DatasourceName != "" || items[0].DatasourceType != "" {
		t.Fatalf("expected revocation row to skip datasource enrichment, got name=%q type=%q", items[0].DatasourceName, items[0].DatasourceType)
	}
	// The datasource ID still rides along from params so the row is
	// correlatable; only the service-side name/type lookup is skipped.
	if items[0].DatasourceID != "ds_1" {
		t.Fatalf("expected datasourceID=ds_1 (passthrough from params), got %q", items[0].DatasourceID)
	}
}

type countingDatasourceService struct {
	stubService
	getCalls int
}

func (c *countingDatasourceService) GetDatasource(ctx context.Context, id string) (datasource.DataSource, error) {
	c.getCalls++
	return c.stubService.GetDatasource(ctx, id)
}

func TestAuditStoreListHandlesLargeEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)

	largeSummary := strings.Repeat("select 1 ", 20000)
	if err := store.Append(AuditEntry{
		AccessKey:  "agent_large",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    largeSummary,
		Status:     StatusSuccess,
		ExecutedAt: "2026-04-22T15:10:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	items, err := store.List(AuditFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(items))
	}
	if items[0].Summary != largeSummary {
		t.Fatal("expected large summary to round-trip")
	}
}

func TestAuditStoreListWaitsForPathLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-audit.jsonl")
	store := NewAuditStore(path)
	if err := store.Append(AuditEntry{
		AccessKey:  "agent_locked",
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT 1",
		Status:     StatusSuccess,
		ExecutedAt: "2026-04-22T15:10:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	lockReady := make(chan struct{})
	releaseLock := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- securefile.WithPathLock(path, func() error {
			close(lockReady)
			<-releaseLock
			return nil
		})
	}()
	<-lockReady

	listDone := make(chan error, 1)
	go func() {
		_, err := store.List(AuditFilter{})
		listDone <- err
	}()

	select {
	case err := <-listDone:
		t.Fatalf("List returned before lock release: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseLock)
	if err := <-lockDone; err != nil {
		t.Fatalf("lock release: %v", err)
	}
	if err := <-listDone; err != nil {
		t.Fatalf("List after release: %v", err)
	}
}

func TestIdentityStoreEnsureForInstallReusesBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	const inputPath = "/home/user/.claude/skills/futrixdata/SKILL.md"
	first, err := store.EnsureForInstall("claude", inputPath, "Claude Code")
	if err != nil {
		t.Fatalf("EnsureForInstall first: %v", err)
	}
	// On case-insensitive filesystems (macOS/Windows) the stored path is
	// lowercased so /Users/A and /users/a collapse to one identity. Compare
	// against the normalized form rather than the raw input.
	if first.InstallPath != normalizeInstallPath(inputPath) {
		t.Fatalf("installPath = %q, want %q", first.InstallPath, normalizeInstallPath(inputPath))
	}

	second, err := store.EnsureForInstall("claude", "/home/user/.claude/skills/futrixdata/SKILL.md", "Claude Code")
	if err != nil {
		t.Fatalf("EnsureForInstall reuse: %v", err)
	}
	if second.AccessKey != first.AccessKey {
		t.Fatalf("reuse minted new key %q vs %q", second.AccessKey, first.AccessKey)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 identity after reuse, got %d", len(items))
	}
}

func TestIdentityStoreEnsureForInstallDifferentPathsCreateDistinctIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	a, err := store.EnsureForInstall("claude", "/home/u1/.claude/skills/futrixdata/SKILL.md", "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := store.EnsureForInstall("claude", "/home/u2/.claude/skills/futrixdata/SKILL.md", "")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.AccessKey == b.AccessKey {
		t.Fatal("expected distinct access keys for distinct install paths")
	}
}

func TestIdentityStoreEnsureForInstallBackfillsLegacyDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	legacy, err := store.EnsureDetected("claude", "Claude Code")
	if err != nil {
		t.Fatalf("EnsureDetected: %v", err)
	}
	if legacy.InstallPath != "" {
		t.Fatalf("expected legacy identity to lack installPath, got %q", legacy.InstallPath)
	}

	upgraded, err := store.EnsureForInstall("claude", "/home/user/.claude/skills/futrixdata/SKILL.md", "Claude Code")
	if err != nil {
		t.Fatalf("EnsureForInstall after legacy: %v", err)
	}
	if upgraded.AccessKey != legacy.AccessKey {
		t.Fatalf("backfill minted new key %q vs legacy %q", upgraded.AccessKey, legacy.AccessKey)
	}
	if upgraded.InstallPath == "" {
		t.Fatal("expected installPath to be backfilled")
	}
}

func TestIdentityStoreSetSensitivityGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("grant-test")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	// Default must be false — opt-in semantics are what protect users who
	// already created agents before the grant existed.
	if identity.SensitivityClassificationGrant {
		t.Fatal("expected new identity to default to no grant")
	}

	granted, err := store.SetSensitivityGrant(identity.AccessKey, true)
	if err != nil {
		t.Fatalf("SetSensitivityGrant on: %v", err)
	}
	if !granted.SensitivityClassificationGrant {
		t.Fatal("SetSensitivityGrant(true) did not flip flag in returned identity")
	}
	reloaded, ok, err := store.Get(identity.AccessKey)
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if !reloaded.SensitivityClassificationGrant {
		t.Fatal("grant did not persist to disk")
	}

	cleared, err := store.SetSensitivityGrant(identity.AccessKey, false)
	if err != nil {
		t.Fatalf("SetSensitivityGrant off: %v", err)
	}
	if cleared.SensitivityClassificationGrant {
		t.Fatal("SetSensitivityGrant(false) did not clear flag")
	}
}

func TestIdentityStoreSetSensitivityGrantUnknownKeyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	if _, err := store.SetSensitivityGrant("agent_does_not_exist", true); err == nil {
		t.Fatal("expected error for unknown access key")
	}
}

func TestIdentityStoreSetDatasourceManagementGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("datasource-grant-test")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if identity.DatasourceManagementGrant {
		t.Fatal("expected new identity to default to no datasource-management grant")
	}

	granted, err := store.SetDatasourceManagementGrant(identity.AccessKey, true)
	if err != nil {
		t.Fatalf("SetDatasourceManagementGrant on: %v", err)
	}
	if !granted.DatasourceManagementGrant {
		t.Fatal("SetDatasourceManagementGrant(true) did not flip flag in returned identity")
	}
	reloaded, ok, err := store.Get(identity.AccessKey)
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if !reloaded.DatasourceManagementGrant {
		t.Fatal("datasource-management grant did not persist to disk")
	}

	cleared, err := store.SetDatasourceManagementGrant(identity.AccessKey, false)
	if err != nil {
		t.Fatalf("SetDatasourceManagementGrant off: %v", err)
	}
	if cleared.DatasourceManagementGrant {
		t.Fatal("SetDatasourceManagementGrant(false) did not clear flag")
	}
}

func TestIdentityStoreSetDatasourceManagementGrantUnknownKeyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	if _, err := store.SetDatasourceManagementGrant("agent_does_not_exist", true); err == nil {
		t.Fatal("expected error for unknown access key")
	}
}

func TestIdentityStoreRevokeSetsAndClearsMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("rev-test")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	revoked, err := store.Revoke(identity.AccessKey)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revoked.RevokedAt == "" {
		t.Fatal("expected revokedAt to be set")
	}
	reloaded, ok, err := store.Get(identity.AccessKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || reloaded.RevokedAt == "" {
		t.Fatalf("expected revocation to persist, got %+v", reloaded)
	}
	restored, err := store.Unrevoke(identity.AccessKey)
	if err != nil {
		t.Fatalf("Unrevoke: %v", err)
	}
	if restored.RevokedAt != "" {
		t.Fatalf("expected Unrevoke to clear marker, got %q", restored.RevokedAt)
	}
}

func TestIdentityStoreRecordsDefaultAccessModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("model-test")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if identity.DatasourceScope != DatasourceScopeInheritUser {
		t.Fatalf("DatasourceScope = %q, want %q", identity.DatasourceScope, DatasourceScopeInheritUser)
	}
	if len(identity.AllowedDatasourceIDs) != 0 {
		t.Fatalf("AllowedDatasourceIDs = %#v, want empty for inherit_user", identity.AllowedDatasourceIDs)
	}
	if identity.ExpiresAt != "" {
		t.Fatalf("ExpiresAt = %q, want no default expiry", identity.ExpiresAt)
	}
}

func TestIdentityStoreDatasourceAllowlistScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	identity, err := store.CreateManual("scope-test")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	updated, err := store.SetDatasourceScope(identity.AccessKey, DatasourceScopeAllowList, []string{" ds_allowed ", "", "ds_allowed"})
	if err != nil {
		t.Fatalf("SetDatasourceScope: %v", err)
	}
	if updated.DatasourceScope != DatasourceScopeAllowList {
		t.Fatalf("DatasourceScope = %q, want %q", updated.DatasourceScope, DatasourceScopeAllowList)
	}
	if got := updated.AllowedDatasourceIDs; len(got) != 1 || got[0] != "ds_allowed" {
		t.Fatalf("AllowedDatasourceIDs = %#v, want normalized singleton", got)
	}
	if err := CheckDatasourceScope(updated, "ds_allowed"); err != nil {
		t.Fatalf("allowed datasource rejected: %v", err)
	}
	if err := CheckDatasourceScope(updated, "ds_denied"); !errors.Is(err, ErrDatasourceForbidden) {
		t.Fatalf("denied datasource error = %v, want ErrDatasourceForbidden", err)
	}
}

func TestCheckAccessRejectsRevokedIdentity(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))

	identity, err := identityStore.CreateManual("revoked-agent")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if _, err := identityStore.Revoke(identity.AccessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err = CheckAccess(dataPath, identity.AccessKey)
	if err == nil {
		t.Fatal("expected revoked CheckAccess to error")
	}
	if !errors.Is(err, ErrAccessRevoked) {
		t.Fatalf("expected ErrAccessRevoked, got %v", err)
	}
}

func TestCheckAccessRejectsExpiredIdentity(t *testing.T) {
	root := t.TempDir()
	dataPath := filepath.Join(root, "datasources.json")
	identityStore := NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))

	identity, err := identityStore.CreateManual("expired-agent")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if _, err := identityStore.SetExpiresAt(identity.AccessKey, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("SetExpiresAt: %v", err)
	}

	_, err = CheckAccess(dataPath, identity.AccessKey)
	if err == nil {
		t.Fatal("expected expired CheckAccess to error")
	}
	if !errors.Is(err, ErrAccessExpired) {
		t.Fatalf("expected ErrAccessExpired, got %v", err)
	}
}

func TestIdentityStoreListAllReturnsStoredItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	manual, err := store.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	detected, err := store.EnsureDetected("cursor", "Cursor")
	if err != nil {
		t.Fatalf("EnsureDetected: %v", err)
	}

	items, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(items))
	}

	got := map[string]AgentIdentity{}
	for _, item := range items {
		got[item.AccessKey] = item
	}
	if got[manual.AccessKey].Name != "warehouse-bot" {
		t.Fatalf("manual identity missing from ListAll")
	}
	if got[detected.AccessKey].AgentType != "cursor" {
		t.Fatalf("detected identity missing from ListAll")
	}
}

func TestIdentityStoreCreateManualAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")

	const total = 24
	var wg sync.WaitGroup
	for idx := 0; idx < total; idx++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := NewIdentityStore(path)
			if _, err := store.CreateManual(fmt.Sprintf("agent-%02d", i)); err != nil {
				t.Errorf("CreateManual(%d): %v", i, err)
			}
		}(idx)
	}
	wg.Wait()

	items, err := NewIdentityStore(path).ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != total {
		t.Fatalf("expected %d identities, got %d", total, len(items))
	}

	names := map[string]struct{}{}
	for _, item := range items {
		names[item.Name] = struct{}{}
	}
	for idx := 0; idx < total; idx++ {
		name := fmt.Sprintf("agent-%02d", idx)
		if _, ok := names[name]; !ok {
			t.Fatalf("missing %s after concurrent writes", name)
		}
	}
}

func TestIdentityStoreBindInstallPathTargetsExactKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	// Pre-seed a legacy detected row for the same agent type with no install
	// path (the shape EnsureForInstall's backfill would otherwise claim).
	legacy, err := store.EnsureDetected("claude", "Claude Code")
	if err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if legacy.InstallPath != "" {
		t.Fatalf("seed legacy.InstallPath = %q, want empty", legacy.InstallPath)
	}

	// Mint a second identity for the same agent type with a known key.
	bound, err := store.EnsureBound("agent_bind_xyz", "claude", "Claude Code 2")
	if err != nil {
		t.Fatalf("EnsureBound: %v", err)
	}

	// Bind the install path to the second key; the legacy row must remain untouched.
	updated, err := store.BindInstallPath(bound.AccessKey, "/tmp/agent/bound")
	if err != nil {
		t.Fatalf("BindInstallPath: %v", err)
	}
	if updated.AccessKey != bound.AccessKey {
		t.Fatalf("BindInstallPath returned wrong key: got %q want %q", updated.AccessKey, bound.AccessKey)
	}
	if strings.TrimSpace(updated.InstallPath) == "" {
		t.Fatal("BindInstallPath did not set InstallPath")
	}

	items, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	for _, item := range items {
		if item.AccessKey == legacy.AccessKey && item.InstallPath != "" {
			t.Fatalf("legacy identity got install path %q, should be unchanged", item.InstallPath)
		}
	}
}

func TestIdentityStoreBindInstallPathReturnsErrorForMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	if _, err := store.BindInstallPath("agent_nope_9999", "/tmp/a"); err == nil {
		t.Fatal("expected error for missing access key")
	}
}

func TestIdentityStoreEnsureManualIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	first, err := store.EnsureManual("")
	if err != nil {
		t.Fatalf("EnsureManual first: %v", err)
	}
	second, err := store.EnsureManual("")
	if err != nil {
		t.Fatalf("EnsureManual second: %v", err)
	}
	if first.AccessKey != second.AccessKey {
		t.Fatalf("EnsureManual minted a new key on second call: first=%q second=%q", first.AccessKey, second.AccessKey)
	}

	items, err := store.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	manualCount := 0
	for _, item := range items {
		if item.Source == SourceManual {
			manualCount++
		}
	}
	if manualCount != 1 {
		t.Fatalf("expected exactly 1 manual identity, got %d", manualCount)
	}
}

func TestIdentityStoreEnsureManualSkipsRevoked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	first, err := store.EnsureManual("")
	if err != nil {
		t.Fatalf("EnsureManual seed: %v", err)
	}
	if _, err := store.Revoke(first.AccessKey); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	second, err := store.EnsureManual("")
	if err != nil {
		t.Fatalf("EnsureManual after revoke: %v", err)
	}
	if second.AccessKey == first.AccessKey {
		t.Fatal("EnsureManual returned the revoked identity; manual install flow would ship a dead key")
	}
	if strings.TrimSpace(second.RevokedAt) != "" {
		t.Fatal("EnsureManual returned a revoked identity after minting fresh")
	}
}

func TestIdentityStoreBindInstallPathRepairsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-identities.json")
	store := NewIdentityStore(path)

	seeded, err := store.EnsureForInstall("claude", "/tmp/a", "")
	if err != nil {
		t.Fatalf("EnsureForInstall seed: %v", err)
	}

	updated, err := store.BindInstallPath(seeded.AccessKey, "/tmp/b")
	if err != nil {
		t.Fatalf("BindInstallPath: %v", err)
	}
	if updated.InstallPath == seeded.InstallPath {
		t.Fatalf("BindInstallPath did not repair stale binding: still %q", updated.InstallPath)
	}
}

func TestNormalizeInstallPathFoldsCaseOnCaseInsensitiveFS(t *testing.T) {
	probeDir := t.TempDir()
	if !caseInsensitiveFS(probeDir) {
		t.Skip("case-sensitive filesystem; path case folding is not applied here")
	}
	a := normalizeInstallPath(filepath.Join(probeDir, "Agent"))
	b := normalizeInstallPath(filepath.Join(strings.ToLower(probeDir), "agent"))
	if a != b {
		t.Fatalf("case-insensitive normalize mismatch: %q vs %q", a, b)
	}
}

func TestNormalizeInstallPathPreservesCaseOnCaseSensitiveFS(t *testing.T) {
	probeDir := t.TempDir()
	if caseInsensitiveFS(probeDir) {
		t.Skip("case-insensitive filesystem; this test validates the case-sensitive probe path")
	}
	// On a case-sensitive volume, /A and /a are distinct paths. Ensure the
	// normalizer keeps them distinct so rename/revoke actions target exactly
	// the right identity.
	a := normalizeInstallPath(filepath.Join(probeDir, "Agent"))
	b := normalizeInstallPath(filepath.Join(probeDir, "agent"))
	if a == b {
		t.Fatalf("case-sensitive normalize conflated distinct paths: %q", a)
	}
}

func TestMaskAccessKeyDoesNotLeakFullKey(t *testing.T) {
	cases := []struct {
		in   string
		long bool
	}{
		{"agent_1234567890abcdef", true},
		{"short", false},
		{"", false},
	}
	for _, c := range cases {
		got := MaskAccessKey(c.in)
		if got == c.in && c.in != "" {
			t.Fatalf("MaskAccessKey(%q) returned the full key", c.in)
		}
		if c.long && !strings.Contains(got, "...") {
			t.Fatalf("MaskAccessKey(%q) = %q, want masked form with ...", c.in, got)
		}
	}
}

type stubService struct {
	ds datasource.DataSource
}

func (s *stubService) ListDatasources(context.Context) ([]datasource.DataSource, error) {
	return nil, nil
}
func (s *stubService) GetDatasource(_ context.Context, id string) (datasource.DataSource, error) {
	if id == s.ds.ID {
		return s.ds, nil
	}
	return datasource.DataSource{}, nil
}
func (s *stubService) CreateDatasource(context.Context, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *stubService) UpdateDatasource(context.Context, string, datasourceops.DataSourcePayload) (datasource.DataSource, error) {
	return datasource.DataSource{}, nil
}
func (s *stubService) DeleteDatasource(context.Context, string) (bool, error) { return false, nil }
func (s *stubService) TestDatasource(context.Context, string) (bool, error)   { return false, nil }
func (s *stubService) TestDatasourcePayload(context.Context, datasourceops.DataSourcePayload) (bool, error) {
	return false, nil
}
func (s *stubService) ListDatabases(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
func (s *stubService) ListEntities(context.Context, string, string, string, string, bool) ([]string, error) {
	return nil, nil
}
func (s *stubService) DescribeEntity(context.Context, string, string, string, string) (console.DescribeResult, error) {
	return console.DescribeResult{}, nil
}
func (s *stubService) ListRiskRules(context.Context, bool) ([]riskengine.Rule, error) {
	return nil, nil
}
func (s *stubService) SetRiskRule(context.Context, riskengine.Rule) (riskengine.Rule, error) {
	return riskengine.Rule{}, nil
}
func (s *stubService) DeleteRiskRule(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubService) SetBuiltinRiskRuleEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
func (s *stubService) SetBuiltinRiskRuleThresholds(context.Context, string, riskengine.RuleThresholds) (riskengine.RuleThresholds, error) {
	return riskengine.RuleThresholds{}, nil
}
func (s *stubService) ExecuteStatement(context.Context, string, string, string, string, int, string, ...console.ExecuteBounds) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *stubService) AssessStatement(context.Context, string, string, string, string) (riskengine.RiskAssessment, error) {
	return riskengine.RiskAssessment{}, nil
}
func (s *stubService) ExecuteRedisCommand(context.Context, string, []string, string, string) (console.QueryResult, error) {
	return console.QueryResult{}, nil
}
func (s *stubService) AssessRedisCommand(context.Context, string, []string, string, string) (riskengine.RiskAssessment, error) {
	return riskengine.RiskAssessment{}, nil
}
func (s *stubService) ExplainStatement(context.Context, string, string, bool, string, string) (console.ExplainResult, error) {
	return console.ExplainResult{}, nil
}
func (s *stubService) ScanRedisKeys(context.Context, string, string, string) (datasourceops.RedisKeyPage, error) {
	return datasourceops.RedisKeyPage{}, nil
}
func (s *stubService) GetDatasourceMetrics(context.Context, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *stubService) GetDatasourceMetricsByNode(context.Context, string, string) (datasourceops.DatasourceMetrics, error) {
	return datasourceops.DatasourceMetrics{}, nil
}
func (s *stubService) GetRedisCommandDocs(context.Context, string, string) (console.RedisCommandDocsEntry, error) {
	return console.RedisCommandDocsEntry{}, nil
}
func (s *stubService) GetSchemaKnowledge(context.Context, string, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubService) GetERKnowledge(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubService) GetSensitivityConfig(context.Context) (map[string]any, error) { return nil, nil }
func (s *stubService) SetSensitivityCustomRules(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubService) GetSensitivityReport(context.Context, string) (map[string]any, error) {
	return nil, nil
}
func (s *stubService) SaveSensitivityReport(context.Context, datasourceops.SaveSensitivityReportInput) (map[string]any, error) {
	return nil, nil
}
func (s *stubService) DeleteSensitivityReport(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubService) D1DeployMigrations(context.Context, string) (bool, error) { return false, nil }
func (s *stubService) D1OAuthLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *stubService) D1OAuthReLogin(context.Context) (datasourceops.D1OAuthSession, error) {
	return datasourceops.D1OAuthSession{}, nil
}
func (s *stubService) D1IsWranglerInstalled(context.Context) (bool, error) { return false, nil }
func (s *stubService) D1ListCloudDatabases(context.Context, string, string) ([]datasourceops.D1CloudDatabase, error) {
	return nil, nil
}
func (s *stubService) D1CreateCloudDatabase(context.Context, string, string, string) (datasourceops.D1CloudDatabase, error) {
	return datasourceops.D1CloudDatabase{}, nil
}
func (s *stubService) DynamoDBSSOListProfiles(context.Context, string) ([]datasourceops.DynamoDBSSOProfile, error) {
	return nil, nil
}
func (s *stubService) DynamoDBSSOLogin(context.Context, string, string) (datasourceops.DynamoDBSSOLoginResult, error) {
	return datasourceops.DynamoDBSSOLoginResult{}, nil
}
func (s *stubService) DynamoDBSSOOAuthAuthorize(context.Context, string, string, string) (datasourceops.DynamoDBSSOOAuthResult, error) {
	return datasourceops.DynamoDBSSOOAuthResult{}, nil
}
func (s *stubService) DynamoDBSSOListAccounts(context.Context, string, string) ([]datasourceops.DynamoDBSSOAccount, error) {
	return nil, nil
}
func (s *stubService) DynamoDBSSOListAccountRoles(context.Context, string, string, string) ([]datasourceops.DynamoDBSSORole, error) {
	return nil, nil
}
func (s *stubService) DynamoDBSSOGetRoleCredentials(context.Context, string, string, string, string) (datasourceops.DynamoDBSSORoleCredentials, error) {
	return datasourceops.DynamoDBSSORoleCredentials{}, nil
}
