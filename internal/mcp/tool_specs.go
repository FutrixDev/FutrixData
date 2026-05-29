package mcp

import (
	"futrixdata/platform/internal/toolreg"

	gomcp "github.com/mark3labs/mcp-go/mcp"
)

// buildMCPSchema converts a toolreg.ToolDef into a gomcp.Tool with the
// appropriate JSON Schema for its parameters.
func buildMCPSchema(def toolreg.ToolDef) gomcp.Tool {
	opts := []gomcp.ToolOption{
		gomcp.WithDescription(def.Description),
	}
	for _, p := range def.Params {
		opts = append(opts, paramOption(p))
	}
	return gomcp.NewTool(def.Name, opts...)
}

func paramOption(p toolreg.Param) gomcp.ToolOption {
	return func(t *gomcp.Tool) {
		schema := propertySchema(p)
		if p.Required {
			t.InputSchema.Required = append(t.InputSchema.Required, p.Name)
		}
		t.InputSchema.Properties[p.Name] = schema
	}
}

func propertySchema(p toolreg.Param) map[string]any {
	schema := map[string]any{
		"type": toolParamTypeName(p.Type),
	}
	if p.Description != "" {
		schema["description"] = p.Description
	}
	if len(p.Enum) > 0 {
		schema["enum"] = append([]string(nil), p.Enum...)
	}
	if len(p.Properties) > 0 {
		props := make(map[string]any, len(p.Properties))
		required := make([]string, 0, len(p.Properties))
		for _, child := range p.Properties {
			childSchema := propertySchema(child)
			if child.Required {
				required = append(required, child.Name)
			}
			props[child.Name] = childSchema
		}
		schema["properties"] = props
		if len(required) > 0 {
			schema["required"] = required
		}
	}
	if p.Items != nil {
		switch typed := p.Items.(type) {
		case toolreg.Param:
			itemSchema := propertySchema(typed)
			delete(itemSchema, "required")
			schema["items"] = itemSchema
		case *toolreg.Param:
			itemSchema := propertySchema(*typed)
			delete(itemSchema, "required")
			schema["items"] = itemSchema
		case map[string]any:
			schema["items"] = typed
		}
	}
	if p.MinItems > 0 {
		schema["minItems"] = p.MinItems
	}
	return schema
}

func toolParamTypeName(kind toolreg.ParamType) string {
	switch kind {
	case toolreg.TypeNumber:
		return "number"
	case toolreg.TypeBoolean:
		return "boolean"
	case toolreg.TypeObject:
		return "object"
	case toolreg.TypeArray:
		return "array"
	default:
		return "string"
	}
}
