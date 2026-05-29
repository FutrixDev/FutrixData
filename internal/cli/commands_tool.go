package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/startuprecovery"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

func (r *Runner) runTool(ctx context.Context, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing tool subcommand.\n\n%s", toolUsage())
	}
	switch args[0] {
	case "list":
		return r.runToolList(opts, args[1:])
	case "describe":
		return r.runToolDescribe(opts, args[1:])
	case "call":
		// `tool call` always routes through the main app's IPC daemon —
		// the CLI process never loads datasources.json itself.
		return r.runToolCall(ctx, opts, args[1:])
	default:
		return fmt.Errorf("unknown tool subcommand: %s\n\n%s", args[0], toolUsage())
	}
}

func toolUsage() string {
	return `Usage: futrixdata-cli tool <subcommand> [flags]

Subcommands:
  list      List all available tools with descriptions
  describe  Show one tool and its parameter schema
  call      Call a specific tool with a JSON payload

Flags for list:
  --schema       Include parameter schema in output

Usage for describe:
  futrixdata-cli tool describe <tool-name>

Usage for call:
  futrixdata-cli tool call <tool-name> [--file FILE|--stdin]

Flags for call:
  --file <path>  JSON payload file
  --stdin        Read JSON payload from stdin`
}

func (r *Runner) runToolList(opts Options, args []string) error {
	fs := flag.NewFlagSet("tool list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var includeSchema bool
	fs.BoolVar(&includeSchema, "schema", false, "include parameter schema in output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tools := toolreg.AllTools()
	payload := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		payload = append(payload, toolMetadata(tool, includeSchema))
	}
	return r.printResult(opts, payload, func() string {
		var b strings.Builder
		for _, tool := range tools {
			_, _ = fmt.Fprintf(&b, "%s\t%s\n", tool.Name, tool.Description)
			if !includeSchema {
				continue
			}
			for _, p := range tool.Params {
				required := ""
				if p.Required {
					required = " required"
				}
				_, _ = fmt.Fprintf(&b, "  - %s (%s%s): %s\n", p.Name, toolParamTypeName(p.Type), required, p.Description)
			}
		}
		return b.String()
	})
}

func (r *Runner) runToolDescribe(opts Options, args []string) error {
	if err := requiredArgs(args, 1, "Usage: futrixdata-cli tool describe <tool-name>"); err != nil {
		return err
	}
	def, ok := toolreg.ByName(args[0])
	if !ok {
		return fmt.Errorf("unknown tool: %s", args[0])
	}
	payload := toolMetadata(def, true)
	return r.printResult(opts, payload, func() string {
		var b strings.Builder
		_, _ = fmt.Fprintf(&b, "Name: %s\nDescription: %s\nApproval required: %t\n", def.Name, def.Description, def.ApprovalRequired)
		if len(def.Params) == 0 {
			_, _ = fmt.Fprintln(&b, "Parameters: none")
			return b.String()
		}
		_, _ = fmt.Fprintln(&b, "Parameters:")
		for _, p := range def.Params {
			required := ""
			if p.Required {
				required = " required"
			}
			_, _ = fmt.Fprintf(&b, "  - %s (%s%s): %s\n", p.Name, toolParamTypeName(p.Type), required, p.Description)
		}
		return b.String()
	})
}

func toolMetadata(def toolreg.ToolDef, includeSchema bool) map[string]any {
	meta := map[string]any{
		"name":             def.Name,
		"description":      def.Description,
		"approvalRequired": def.ApprovalRequired,
	}
	if !includeSchema {
		return meta
	}
	params := make([]map[string]any, 0, len(def.Params))
	for _, p := range def.Params {
		params = append(params, toolParamMetadata(p))
	}
	meta["parameters"] = params
	return meta
}

func toolParamMetadata(p toolreg.Param) map[string]any {
	meta := map[string]any{
		"name":        p.Name,
		"type":        toolParamTypeName(p.Type),
		"required":    p.Required,
		"description": p.Description,
	}
	if len(p.Enum) > 0 {
		meta["enum"] = append([]string(nil), p.Enum...)
	}
	if len(p.Properties) > 0 {
		props := make([]map[string]any, 0, len(p.Properties))
		for _, child := range p.Properties {
			props = append(props, toolParamMetadata(child))
		}
		meta["properties"] = props
	}
	if p.Items != nil {
		switch typed := p.Items.(type) {
		case toolreg.Param:
			meta["items"] = toolParamMetadata(typed)
		case *toolreg.Param:
			meta["items"] = toolParamMetadata(*typed)
		case map[string]any:
			meta["items"] = typed
		default:
			meta["items"] = typed
		}
	}
	if p.MinItems > 0 {
		meta["minItems"] = p.MinItems
	}
	return meta
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

func (r *Runner) runToolCall(ctx context.Context, opts Options, args []string) error {
	if err := requiredArgs(args, 1, "Usage: futrixdata-cli tool call <tool-name> [--file FILE|--stdin]"); err != nil {
		return r.toolCallFailure(opts, "", err)
	}
	toolName := args[0]
	def, ok := toolreg.ByName(toolName)
	if !ok {
		return r.toolCallFailure(opts, toolName, fmt.Errorf("unknown tool: %s", toolName))
	}
	if containsApproveFlag(args[1:]) {
		return r.toolCallFailure(opts, def.Name, r.unsupportedToolCallApprove(opts, def))
	}
	fs := flag.NewFlagSet("tool call "+toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var filePath string
	var useStdin bool
	fs.StringVar(&filePath, "file", "", "json payload file")
	fs.BoolVar(&useStdin, "stdin", false, "read json payload from stdin")
	if err := fs.Parse(args[1:]); err != nil {
		return r.toolCallFailure(opts, def.Name, err)
	}
	payload := map[string]any{}
	if err := r.readJSONInputIfProvided(filePath, useStdin, &payload); err != nil {
		return r.toolCallFailure(opts, def.Name, err)
	}
	// `tool call` is the agent-routed invocation surface — skill templates and
	// MCP wrappers always thread --agent-access-key through. A missing key
	// here means the caller stripped it; allowing the call would let an agent
	// bypass revocation simply by dropping the flag and produce un-attributed
	// audit writes. Fail closed before we even reach the daemon.
	if strings.TrimSpace(opts.AgentAccessKey) == "" {
		return r.toolCallFailure(opts, def.Name, fmt.Errorf("--agent-access-key is required for `tool call`"))
	}

	// Every tool call goes through the main app process via IPC. There is
	// deliberately no in-process fallback: the CLI process must not load
	// datasources.json itself (sandboxed agents can't reach the keychain
	// anyway, and a divergent local Service would write to the same files
	// the main app is editing). If the daemon path fails after spawn-on-miss,
	// surface the underlying error so the user can fix the install.
	response, gated, err := r.dispatchToolCallViaDaemon(ctx, opts, def, payload)
	if err != nil {
		return r.toolCallFailure(opts, def.Name, err)
	}
	return r.renderDaemonSuccess(opts, response, gated)
}

func containsApproveFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--approve" || arg == "-approve" || strings.HasPrefix(arg, "--approve=") || strings.HasPrefix(arg, "-approve=") {
			return true
		}
	}
	return false
}

func (r *Runner) unsupportedToolCallApprove(opts Options, def toolreg.ToolDef) error {
	msg := "--approve is rejected for `tool call`; third-party agents cannot approve FutrixData operations"
	accessKey := strings.TrimSpace(opts.AgentAccessKey)
	if accessKey == "" {
		return errors.New(msg)
	}
	if os.Getenv("FUTRIXDATA_APPROVAL_PROBE_INIT_LOCAL_CRYPTO") == "1" {
		if err := initSecurefileKey(opts.DataPath); err != nil {
			return err
		}
	}
	if _, err := agentaudit.CheckAccess(opts.DataPath, accessKey); err != nil {
		if errors.Is(err, agentaudit.ErrAccessRevoked) {
			agentaudit.LogRevokedAccess(opts.DataPath, nil, string(toolexec.SourceSkill), accessKey, def.Name, nil, err.Error())
		}
		if errors.Is(err, agentaudit.ErrAccessExpired) {
			_ = agentaudit.AppendToolCall(opts.DataPath, nil, string(toolexec.SourceSkill), accessKey, def.Name, nil, agentaudit.StatusError, err.Error())
		}
		return err
	}
	_ = agentaudit.AppendToolCall(opts.DataPath, nil, string(toolexec.SourceSkill), accessKey, def.Name, nil, agentaudit.StatusError, msg)
	return errors.New(msg)
}

func (r *Runner) toolCallFailure(opts Options, toolName string, err error) error {
	errorBody := map[string]any{
		"message": err.Error(),
	}
	if attr := riskAttributionFromError(err); attr != nil {
		errorBody["riskAttribution"] = attr
	}
	if info, ok := startuprecovery.FromError(err); ok {
		errorBody["startupRecovery"] = info
	}
	return r.commandFailure(opts, map[string]any{
		"ok":    false,
		"tool":  strings.TrimSpace(toolName),
		"error": errorBody,
	}, err)
}
