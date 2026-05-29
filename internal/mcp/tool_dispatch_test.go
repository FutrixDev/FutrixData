package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/daemon"
	"futrixdata/platform/internal/ipc"
)

// TestIsInfraErrorClassifiesConnDrop pins the Round 12 fix on the MCP side:
// the predicate is duplicated between cli and mcp, so the spawn-and-retry
// recovery for "daemon restarted while we held an open conn" must work
// equally for MCP tool calls. A bare EOF / broken pipe / closed-conn error
// from ipc.Client.Roundtrip must classify as infra so dispatchViaDaemon
// reaches the SpawnDaemon path instead of returning the raw error.
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

func TestDecodeMCPDaemonApprovalDoesNotOfferApproveRetry(t *testing.T) {
	raw, err := json.Marshal(daemon.ToolCallApprovalResult{
		ApprovalRequired: daemon.ToolCallApprovalDetail{
			Kind:    "execute_statement",
			Summary: "execute_statement on ds_1",
		},
	})
	if err != nil {
		t.Fatalf("marshal approval result: %v", err)
	}
	res := decodeMCPDaemonResponse(ipc.Response{OK: true, Result: raw}, "execute_statement")
	if !res.IsError {
		t.Fatalf("expected approval-required daemon response to become MCP error")
	}
	msg := textOf(res)
	if !strings.Contains(msg, "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection message, got %q", msg)
	}
	if !strings.Contains(msg, "requires approval") {
		t.Fatalf("expected requires-approval wording, got %q", msg)
	}
	if strings.Contains(msg, `"approve"`) || strings.Contains(msg, "approve: true") {
		t.Fatalf("MCP approval message must not instruct approve retry, got %q", msg)
	}
}

func TestDecodeMCPDaemonApprovalIncludesRiskAttribution(t *testing.T) {
	raw, err := json.Marshal(daemon.ToolCallApprovalResult{
		ApprovalRequired: daemon.ToolCallApprovalDetail{
			Kind:    "execute_statement",
			Summary: "execute_statement on ds_1",
			RiskAttribution: &agentaudit.RiskAttribution{
				Source:          agentaudit.AttributionSourceRiskEngine,
				Action:          "require_approval",
				Level:           "high",
				RuleID:          "sql-require-approval-drop",
				RuleCode:        "SQL-007",
				RuleDescription: "DROP statements require approval",
				Reasons:         []string{"DROP TABLE can destroy data"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal approval result: %v", err)
	}
	res := decodeMCPDaemonResponse(ipc.Response{OK: true, Result: raw}, "execute_statement")
	if !res.IsError {
		t.Fatalf("expected approval-required daemon response to become MCP error")
	}
	msg := textOf(res)
	if !strings.Contains(msg, "rejected because third-party agents cannot approve") {
		t.Fatalf("expected approval rejection message, got %q", msg)
	}
	if !strings.Contains(msg, `"approvalRequired"`) {
		t.Fatalf("expected structured approval body, got %q", msg)
	}
	if !strings.Contains(msg, `"ruleCode": "SQL-007"`) {
		t.Fatalf("expected ruleCode in MCP approval body, got %q", msg)
	}
}

func TestDecodeMCPDaemonBlockedErrorIncludesRiskAttribution(t *testing.T) {
	res := decodeMCPDaemonResponse(ipc.Response{
		OK: false,
		Error: &ipc.Error{
			Code:    ipc.CodeToolError,
			Message: "statement blocked by rule USR-001",
			Details: map[string]any{
				"riskAttribution": &agentaudit.RiskAttribution{
					Source:          agentaudit.AttributionSourceRiskEngine,
					Action:          "block",
					Level:           "high",
					RuleID:          "user-redis-pd-delete",
					RuleCode:        "USR-001",
					RuleDescription: "Protect pd keys from delete",
					Reasons:         []string{"pd keys cannot be deleted"},
				},
			},
		},
	}, "execute_statement")
	if !res.IsError {
		t.Fatalf("expected blocked daemon response to become MCP error")
	}
	msg := textOf(res)
	if !strings.Contains(msg, `"riskAttribution"`) {
		t.Fatalf("expected structured blocked error body, got %q", msg)
	}
	if !strings.Contains(msg, `"ruleCode": "USR-001"`) {
		t.Fatalf("expected ruleCode in MCP blocked body, got %q", msg)
	}
}
