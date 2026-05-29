package mcp

import (
	"testing"

	"futrixdata/platform/internal/toolreg"
)

func TestBuildMCPSchemasHaveNames(t *testing.T) {
	for _, def := range toolreg.AllTools() {
		schema := buildMCPSchema(def)
		if schema.Name == "" {
			t.Errorf("tool %q produced schema with empty name", def.Name)
		}
	}
}

func TestApprovalToolsDoNotExposeApproveParam(t *testing.T) {
	for _, def := range toolreg.AllTools() {
		schema := buildMCPSchema(def)
		if _, hasApprove := schema.InputSchema.Properties["approve"]; hasApprove {
			t.Errorf("tool %q must not expose an approve parameter in MCP schema", def.Name)
		}
	}
}

func TestSensitivityReportSchemaUsesArrayEntities(t *testing.T) {
	def, ok := toolreg.ByName("save_sensitivity_report")
	if !ok {
		t.Fatal("expected save_sensitivity_report tool")
	}
	schema := buildMCPSchema(def)
	prop, ok := schema.InputSchema.Properties["entities"].(map[string]any)
	if !ok {
		t.Fatal("expected entities schema property")
	}
	if prop["type"] != "array" {
		t.Fatalf("entities type = %#v, want array", prop["type"])
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Fatal("expected entities items schema")
	}
	if items["type"] != "object" {
		t.Fatalf("entities items type = %#v, want object", items["type"])
	}
}

func TestExecuteStatementMCPSchemaIncludesDynamoDBBounds(t *testing.T) {
	def, ok := toolreg.ByName("execute_statement")
	if !ok {
		t.Fatal("expected execute_statement tool")
	}
	schema := buildMCPSchema(def)
	for _, name := range []string{"maxReturnedRows", "maxPages", "maxEvaluatedItems"} {
		prop, ok := schema.InputSchema.Properties[name].(map[string]any)
		if !ok {
			t.Fatalf("missing schema property %q", name)
		}
		if prop["type"] != "number" {
			t.Fatalf("%s type = %#v, want number", name, prop["type"])
		}
	}
	strictProp, ok := schema.InputSchema.Properties["strictLimits"].(map[string]any)
	if !ok {
		t.Fatal("missing schema property strictLimits")
	}
	if strictProp["type"] != "boolean" {
		t.Fatalf("strictLimits type = %#v, want boolean", strictProp["type"])
	}
}

func TestExecuteRedisCommandMCPSchemaUsesArgvArray(t *testing.T) {
	def, ok := toolreg.ByName("execute_redis_command")
	if !ok {
		t.Fatal("expected execute_redis_command tool")
	}
	schema := buildMCPSchema(def)
	prop, ok := schema.InputSchema.Properties["args"].(map[string]any)
	if !ok {
		t.Fatal("missing args schema property")
	}
	if prop["type"] != "array" {
		t.Fatalf("args type = %#v, want array", prop["type"])
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Fatalf("args items = %#v, want schema", prop["items"])
	}
	if items["type"] != "string" {
		t.Fatalf("args item type = %#v, want string", items["type"])
	}
}
