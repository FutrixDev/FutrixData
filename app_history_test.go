package main

import (
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/bootstrap"
)

func TestAppListAgentAuditMatchesAgentIdentityKeyword(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	app := &App{cfg: Config{DataPath: dataPath}}

	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := auditStore.Append(agentaudit.AuditEntry{
		AccessKey:  identity.AccessKey,
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT * FROM users",
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-23T10:00:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	items, err := app.ListAgentAudit(AgentAuditFilterPayload{Keyword: "warehouse", Limit: 50})
	if err != nil {
		t.Fatalf("ListAgentAudit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered audit entry, got %#v", items)
	}
	if items[0].AgentName != "warehouse-bot" {
		t.Fatalf("agent name = %q, want warehouse-bot", items[0].AgentName)
	}
}

func TestAppListAgentAuditSearchesBeyondInitialLimitWhenMatchingIdentity(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	app := &App{cfg: Config{DataPath: dataPath}}

	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	matchingIdentity, err := identityStore.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual matching identity: %v", err)
	}
	otherIdentity, err := identityStore.CreateManual("query-bot")
	if err != nil {
		t.Fatalf("CreateManual other identity: %v", err)
	}

	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := auditStore.Append(agentaudit.AuditEntry{
		AccessKey:  matchingIdentity.AccessKey,
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT * FROM warehouse_inventory",
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-23T10:00:00Z",
	}); err != nil {
		t.Fatalf("Append matching entry: %v", err)
	}
	for i := 0; i < 6; i++ {
		if err := auditStore.Append(agentaudit.AuditEntry{
			AccessKey:  otherIdentity.AccessKey,
			Protocol:   "skill",
			ToolName:   "execute_statement",
			Summary:    "SELECT * FROM users",
			Status:     agentaudit.StatusSuccess,
			ExecutedAt: "2026-04-23T10:00:00Z",
		}); err != nil {
			t.Fatalf("Append other entry %d: %v", i, err)
		}
	}

	items, err := app.ListAgentAudit(AgentAuditFilterPayload{Keyword: "warehouse-bot", Limit: 1})
	if err != nil {
		t.Fatalf("ListAgentAudit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered audit entry, got %#v", items)
	}
	if items[0].AgentName != "warehouse-bot" {
		t.Fatalf("agent name = %q, want warehouse-bot", items[0].AgentName)
	}
	if items[0].AccessKey != matchingIdentity.AccessKey {
		t.Fatalf("access key = %q, want %q", items[0].AccessKey, matchingIdentity.AccessKey)
	}
}

func TestAppListAgentAuditKeywordMatchesRiskAttribution(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	app := &App{cfg: Config{DataPath: dataPath}}

	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := auditStore.Append(agentaudit.AuditEntry{
		AccessKey:  identity.AccessKey,
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "DELETE on users",
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-23T10:00:00Z",
		RiskAttribution: &agentaudit.RiskAttribution{
			Source:          agentaudit.AttributionSourceRiskEngine,
			Action:          "require_approval",
			Level:           "high",
			RuleID:          "rule_delete",
			RuleCode:        "delete_full_table",
			RuleDescription: "DELETE without WHERE clause",
			Reasons:         []string{"DELETE statement on `users` does not include a WHERE clause"},
		},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Each visible RiskAttribution field should be reachable by keyword
	// search; the audit card displays them, so excluding them from the
	// haystack would silently filter the row out.
	cases := []string{"delete_full_table", "DELETE without WHERE clause", "WHERE clause", "rule_delete", "high", "risk_engine"}
	for _, kw := range cases {
		t.Run(kw, func(t *testing.T) {
			items, err := app.ListAgentAudit(AgentAuditFilterPayload{Keyword: kw, Limit: 50})
			if err != nil {
				t.Fatalf("ListAgentAudit(%q): %v", kw, err)
			}
			if len(items) != 1 {
				t.Fatalf("keyword %q expected 1 entry, got %#v", kw, items)
			}
			if items[0].RiskAttribution == nil || items[0].RiskAttribution.RuleID != "rule_delete" {
				t.Fatalf("keyword %q expected rule_delete attribution, got %#v", kw, items[0].RiskAttribution)
			}
		})
	}
}

func TestAppListAgentAuditKeywordMatchesPolicyAttribution(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	app := &App{cfg: Config{DataPath: dataPath}}

	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	// Policy-source attribution (e.g. create_datasource is always
	// approval-required by protocol). The card shows source=policy with no
	// rule id, so keyword search must reach the source bucket.
	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := auditStore.Append(agentaudit.AuditEntry{
		AccessKey:  identity.AccessKey,
		Protocol:   "mcp",
		ToolName:   "create_datasource",
		Summary:    "Create datasource warehouse",
		Status:     agentaudit.StatusApprovalRequired,
		ExecutedAt: "2026-04-23T10:00:00Z",
		RiskAttribution: &agentaudit.RiskAttribution{
			Source: agentaudit.AttributionSourcePolicy,
			Action: "require_approval",
		},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	items, err := app.ListAgentAudit(AgentAuditFilterPayload{Keyword: "policy", Limit: 50})
	if err != nil {
		t.Fatalf("ListAgentAudit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected policy-attributed entry to surface for keyword=policy, got %#v", items)
	}
	if items[0].RiskAttribution == nil || items[0].RiskAttribution.Source != agentaudit.AttributionSourcePolicy {
		t.Fatalf("expected policy attribution, got %#v", items[0].RiskAttribution)
	}
}

func TestAppListAgentAuditReturnsFullStatement(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "datasources.json")
	app := &App{cfg: Config{DataPath: dataPath}}

	identityStore := agentaudit.NewIdentityStore(bootstrap.AgentIdentityPath(dataPath))
	identity, err := identityStore.CreateManual("warehouse-bot")
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	statement := "SELECT id, email\nFROM users\nORDER BY id DESC\nLIMIT 50"
	auditStore := agentaudit.NewAuditStore(bootstrap.AgentAuditPath(dataPath))
	if err := auditStore.Append(agentaudit.AuditEntry{
		AccessKey:  identity.AccessKey,
		Protocol:   "skill",
		ToolName:   "execute_statement",
		Summary:    "SELECT id, email",
		Statement:  statement,
		Status:     agentaudit.StatusSuccess,
		ExecutedAt: "2026-04-23T10:00:00Z",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	items, err := app.ListAgentAudit(AgentAuditFilterPayload{Keyword: "order by id desc", Limit: 50})
	if err != nil {
		t.Fatalf("ListAgentAudit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered audit entry, got %#v", items)
	}
	if items[0].Statement != statement {
		t.Fatalf("statement = %q, want %q", items[0].Statement, statement)
	}
}
