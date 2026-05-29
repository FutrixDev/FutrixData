package mcp

import (
	"strings"
	"testing"

	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/startuprecovery"
)

func TestDecodeMCPDaemonResponseRendersStartupRecoveryDetails(t *testing.T) {
	res := decodeMCPDaemonResponse(ipc.Response{
		OK: false,
		Error: &ipc.Error{
			Code:    ipc.CodeStartupRecovery,
			Message: "The local encrypted data could not be opened.",
			Details: map[string]any{
				"startupRecovery": startuprecovery.Info{
					Reason:  startuprecovery.ReasonCorruptFile,
					Message: "The local encrypted data could not be opened.",
					Actions: []startuprecovery.Action{
						startuprecovery.ActionOpenLogs,
						startuprecovery.ActionMoveAsideAndRestart,
					},
				},
			},
		},
	}, "list_datasources")

	if !res.IsError {
		t.Fatalf("expected MCP error result")
	}
	text := textOf(res)
	if !strings.Contains(text, `"startupRecovery"`) || !strings.Contains(text, string(startuprecovery.ReasonCorruptFile)) {
		t.Fatalf("expected structured startup recovery text, got %q", text)
	}
}
