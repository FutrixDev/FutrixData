package protocol

import "testing"

func TestPublicToolsExposeSecurityCriticalSurfaces(t *testing.T) {
	seen := map[ToolName]bool{}
	for _, tool := range PublicTools() {
		seen[tool.Name] = true
	}
	for _, required := range []ToolName{
		ToolExecuteStatement,
		ToolListRiskRules,
		ToolSaveSensitivityReport,
		ToolGetSchemaKnowledge,
	} {
		if !seen[required] {
			t.Fatalf("missing tool %s", required)
		}
	}
}
