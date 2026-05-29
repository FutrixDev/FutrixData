package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"futrixdata/platform/internal/toolreg"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
)

func (r *Runner) runDatasource(ctx context.Context, service Service, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing datasource subcommand.\n\n%s", datasourceUsage())
	}
	switch args[0] {
	case "list":
		items, err := auditedCall(ctx, opts, service, "list_datasources", nil, func(c context.Context) ([]datasource.DataSource, error) {
			return service.ListDatasources(c)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string {
			if len(items) == 0 {
				return "No datasources.\n"
			}
			var b strings.Builder
			for _, item := range items {
				_, _ = fmt.Fprintf(&b, "%s\t%s\t%s\n", item.ID, item.Name, item.Type)
			}
			return b.String()
		})
	case "get":
		fs := flag.NewFlagSet("datasource get", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var id string
		fs.StringVar(&id, "id", "", "datasource id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		item, err := auditedCall(ctx, opts, service, "get_datasource", map[string]any{"datasourceId": id}, func(c context.Context) (datasource.DataSource, error) {
			return service.GetDatasource(c, id)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string {
			return fmt.Sprintf("%s (%s)\n", item.Name, item.ID)
		})
	case "delete":
		fs := flag.NewFlagSet("datasource delete", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var id string
		var approve bool
		fs.StringVar(&id, "id", "", "datasource id")
		fs.BoolVar(&approve, "approve", false, "approve datasource deletion")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deleteParams := map[string]any{"datasourceId": id}
		if approve {
			if err := rejectAgentApproveFlag(opts, service, "delete_datasource", deleteParams); err != nil {
				return err
			}
		}
		if !approve {
			if err := validateAgentAccessForApproval(opts, service, "delete_datasource", deleteParams, nil); err != nil {
				return err
			}
			return r.approvalRequired(opts, "delete_datasource", toolreg.ApprovalSummary("delete_datasource", deleteParams), deleteParams)
		}
		deleted, err := auditedCall(ctx, opts, service, "delete_datasource", deleteParams, func(c context.Context) (bool, error) {
			return service.DeleteDatasource(c, id)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, map[string]any{"deleted": deleted}, func() string { return "Deleted.\n" })
	case "test":
		fs := flag.NewFlagSet("datasource test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var id string
		fs.StringVar(&id, "id", "", "datasource id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		connected, err := auditedCall(ctx, opts, service, "test_datasource", map[string]any{"datasourceId": id}, func(c context.Context) (bool, error) {
			return service.TestDatasource(c, id)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, map[string]any{"connected": connected}, func() string { return "Connected.\n" })
	case "test-payload", "create", "update":
		return r.runDatasourcePayloadCommand(ctx, service, opts, args)
	default:
		return fmt.Errorf("unknown datasource subcommand: %s\n\n%s", args[0], datasourceUsage())
	}
}

func (r *Runner) runDatasourcePayloadCommand(ctx context.Context, service Service, opts Options, args []string) error {
	fs := flag.NewFlagSet("datasource "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		id         string
		filePath   string
		useStdin   bool
		name       string
		dsType     string
		host       string
		port       int
		username   string
		password   string
		database   string
		authSource string
		optionsRaw string
		approve    bool
	)
	fs.StringVar(&id, "id", "", "datasource id")
	fs.StringVar(&filePath, "file", "", "json payload file")
	fs.BoolVar(&useStdin, "stdin", false, "read payload json from stdin")
	fs.StringVar(&name, "name", "", "datasource name")
	fs.StringVar(&dsType, "type", "", "datasource type")
	fs.StringVar(&host, "host", "", "host")
	fs.IntVar(&port, "port", 0, "port")
	fs.StringVar(&username, "username", "", "username")
	fs.StringVar(&password, "password", "", "password")
	fs.StringVar(&database, "database", "", "database")
	fs.StringVar(&authSource, "auth-source", "", "auth source")
	fs.StringVar(&optionsRaw, "options-json", "", "options json object")
	fs.BoolVar(&approve, "approve", false, "approve datasource mutation")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	// Validate agent-access-key BEFORE reading payload from --file / --stdin.
	// Otherwise a revoked or unknown key still triggers a local file open
	// (or blocks on stdin) and JSON-parse errors surface ahead of the access
	// rejection — and a read failure leaves no revocation row at all. The
	// preflight uses the canonical tool registry name so the row aligns with
	// the StatusError / StatusApprovalRequired row written further below.
	preflightTool := payloadPreflightToolName(args[0])
	if approve {
		if err := rejectAgentApproveFlag(opts, service, preflightTool, nil); err != nil {
			return err
		}
	}
	if err := preflightAgentAccess(opts, service, preflightTool, nil); err != nil {
		return err
	}

	var payload datasourceops.DataSourcePayload
	if strings.TrimSpace(filePath) != "" || useStdin {
		if err := r.readJSONInput(filePath, useStdin, &payload); err != nil {
			return err
		}
	} else {
		payload = datasourceops.DataSourcePayload{
			Name:       name,
			Type:       datasource.DataSourceType(strings.TrimSpace(dsType)),
			Host:       host,
			Port:       port,
			Username:   username,
			Password:   password,
			Database:   database,
			AuthSource: authSource,
		}
		if strings.TrimSpace(optionsRaw) != "" {
			if err := parseJSONObject(optionsRaw, &payload.Options); err != nil {
				return err
			}
		}
	}

	switch args[0] {
	case "test-payload":
		connected, err := auditedCall(ctx, opts, service, "test_datasource_payload", payloadAuditParams(payload, ""), func(c context.Context) (bool, error) {
			return service.TestDatasourcePayload(c, payload)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, map[string]any{"connected": connected}, func() string { return "Connected.\n" })
	case "create":
		if !approve {
			if err := validateAgentAccessForApproval(opts, service, "create_datasource", payloadAuditParams(payload, ""), nil); err != nil {
				return err
			}
			return r.approvalRequired(opts, "create_datasource", toolreg.ApprovalSummary("create_datasource", map[string]any{"name": payload.Name, "type": payload.Type}), payload)
		}
		item, err := auditedCall(ctx, opts, service, "create_datasource", payloadAuditParams(payload, ""), func(c context.Context) (datasource.DataSource, error) {
			return service.CreateDatasource(c, payload)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Created %s (%s).\n", item.Name, item.ID) })
	case "update":
		if !approve {
			if err := validateAgentAccessForApproval(opts, service, "update_datasource", payloadAuditParams(payload, id), nil); err != nil {
				return err
			}
			return r.approvalRequired(opts, "update_datasource", toolreg.ApprovalSummary("update_datasource", map[string]any{"datasourceId": id, "name": payload.Name}), payload)
		}
		item, err := auditedCall(ctx, opts, service, "update_datasource", payloadAuditParams(payload, id), func(c context.Context) (datasource.DataSource, error) {
			return service.UpdateDatasource(c, id, payload)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Updated %s (%s).\n", item.Name, item.ID) })
	default:
		return fmt.Errorf("unknown datasource subcommand: %s\n\n%s", args[0], datasourceUsage())
	}
}

func datasourceUsage() string {
	return `Usage: futrixdata-cli datasource <subcommand> [flags]

Subcommands:
  list          List all datasources
  get           Get a datasource by ID
  create        Create a new datasource
  update        Update an existing datasource
  delete        Delete a datasource
  test          Test connectivity for a saved datasource
  test-payload  Test connectivity with inline connection parameters

Common flags:
  --id <id>             Datasource ID (for get/delete/test/update)
  --file <path>         JSON payload file (for create/update/test-payload)
  --stdin               Read JSON payload from stdin
  --approve             Approve mutation (required for create/update/delete)`
}

// payloadPreflightToolName maps the payload-flavored datasource subcommand
// verb (test-payload / create / update) to the canonical tool registry
// name used for audit rows. Used by the agent-key preflight that fires
// BEFORE --file / --stdin payload reads, so a revoked/unknown key cannot
// trigger local I/O before the access rejection lands.
func payloadPreflightToolName(verb string) string {
	switch verb {
	case "test-payload":
		return "test_datasource_payload"
	case "create":
		return "create_datasource"
	case "update":
		return "update_datasource"
	default:
		return "datasource_" + verb
	}
}

// payloadAuditParams projects a DataSourcePayload onto the parameter shape the
// audit row expects. Connection secrets (password) and the wider Options map
// are intentionally excluded — datasourceops.RedactValue would zero them out
// downstream, but elision here is cheaper than rebuilding the row through the
// redaction path. datasourceId is omitted when empty so create-flows don't
// emit a "datasourceId":"" key that confuses downstream filters.
func payloadAuditParams(payload datasourceops.DataSourcePayload, datasourceID string) map[string]any {
	params := map[string]any{
		"name": payload.Name,
		"type": string(payload.Type),
	}
	if strings.TrimSpace(datasourceID) != "" {
		params["datasourceId"] = datasourceID
	}
	return params
}

func parseJSONObject(raw string, target *map[string]any) error {
	if target == nil {
		return nil
	}
	*target = map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode --options-json: %w", err)
	}
	return nil
}
