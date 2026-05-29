package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"futrixdata/platform/internal/agentaudit"
	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
	"futrixdata/platform/internal/toolexec"
	"futrixdata/platform/internal/toolreg"
)

func (r *Runner) runConsole(ctx context.Context, service Service, opts Options, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing console subcommand.\n\n%s", consoleUsage())
	}
	fs := flag.NewFlagSet("console "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var datasourceID, pattern, database, executionMode, name, statement, pagingToken, cursor, node, command string
	var pageSize int
	var analyze, approve bool
	fs.StringVar(&datasourceID, "datasource", "", "datasource id")
	fs.StringVar(&pattern, "pattern", "", "pattern")
	fs.StringVar(&database, "database", "", "database")
	fs.StringVar(&executionMode, "execution-mode", "", "execution mode")
	fs.StringVar(&name, "name", "", "entity name")
	fs.StringVar(&statement, "statement", "", "statement")
	fs.StringVar(&pagingToken, "paging-token", "", "paging token")
	fs.IntVar(&pageSize, "page-size", 0, "page size")
	fs.BoolVar(&analyze, "analyze", false, "analyze explain")
	fs.BoolVar(&approve, "approve", false, "approve live execution")
	fs.StringVar(&cursor, "cursor", "", "cursor")
	fs.StringVar(&node, "node", "", "node")
	fs.StringVar(&command, "command", "", "command")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	switch args[0] {
	case "databases", "list-databases":
		items, err := auditedCall(ctx, opts, service, "list_databases", map[string]any{"datasourceId": datasourceID, "pattern": pattern, "executionMode": executionMode}, func(c context.Context) ([]string, error) {
			return service.ListDatabases(c, datasourceID, pattern, executionMode)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return strings.Join(items, "\n") + "\n" })
	case "entities", "list-entities":
		items, err := auditedCall(ctx, opts, service, "list_entities", map[string]any{"datasourceId": datasourceID, "database": database, "pattern": pattern, "executionMode": executionMode}, func(c context.Context) ([]string, error) {
			return service.ListEntities(c, datasourceID, pattern, database, executionMode, false)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, items, func() string { return strings.Join(items, "\n") + "\n" })
	case "describe":
		item, err := auditedCall(ctx, opts, service, "describe_entity", map[string]any{"datasourceId": datasourceID, "name": name, "database": database, "executionMode": executionMode}, func(c context.Context) (console.DescribeResult, error) {
			return service.DescribeEntity(c, datasourceID, name, database, executionMode)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Columns: %d\n", len(item.Columns)) })
	case "execute":
		execParams := map[string]any{
			"datasourceId":  datasourceID,
			"database":      database,
			"statement":     statement,
			"pagingToken":   pagingToken,
			"pageSize":      pageSize,
			"executionMode": executionMode,
		}
		execOp := func(c context.Context) (console.QueryResult, error) {
			return service.ExecuteStatement(c, datasourceID, statement, database, pagingToken, pageSize, executionMode)
		}
		if approve {
			if err := rejectAgentApproveFlag(opts, service, "execute_statement", execParams); err != nil {
				return err
			}
			item, err := auditedCall(ctx, opts, service, "execute_statement", execParams, execOp)
			if err != nil {
				return err
			}
			return r.printResult(opts, item, func() string { return fmt.Sprintf("Rows: %d\n", len(item.Rows)) })
		}
		// Pre-flight access-key validation BEFORE any service access.
		// AssessStatementApproval below reads the datasource and runs the
		// riskengine; without this gate, an unknown/revoked key would still
		// trigger that read (and could surface a misleading "datasource not
		// found" error in place of the access-key rejection).
		if err := preflightAgentAccess(opts, service, "execute_statement", execParams); err != nil {
			return err
		}
		decision, err := toolreg.AssessStatementApproval(ctx, service, datasourceID, statement, database, executionMode)
		if err != nil {
			return err
		}
		if decision.Blocked {
			blockedErr := toolreg.BlockedErrorFromDecision(decision)
			if strings.TrimSpace(opts.AgentAccessKey) != "" {
				_ = agentaudit.AppendToolCallWithAttribution(
					opts.DataPath,
					service,
					string(toolexec.SourceCLI),
					opts.AgentAccessKey,
					"execute_statement",
					execParams,
					agentaudit.StatusError,
					blockedErr.Error(),
					agentaudit.AttributionFromError(blockedErr),
				)
			}
			return blockedErr
		}
		var approvalAttribution *agentaudit.RiskAttribution
		if decision.Assessment != nil {
			approvalAttribution = agentaudit.AttributionFromAssessment(*decision.Assessment)
		}
		if !decision.NeedsApproval {
			// Optimistic call: static check said "allow" but the adapter Guard's
			// EXPLAIN probe may upgrade the statement to require approval at
			// execute time. Use auditedCallApprovalRedirect so a RiskInfo err is
			// returned without a misleading StatusError row — the canonical
			// StatusApprovalRequired row is written by validateAgentAccessForApproval
			// below. Any non-RiskInfo err still produces a StatusError row.
			item, err := auditedCallApprovalRedirect(ctx, opts, service, "execute_statement", execParams, execOp)
			if err != nil {
				info, ok := console.RiskInfoFromError(err)
				if !ok {
					return err
				}
				// Block is the only RiskInfo action that's a hard rejection
				// here. The audit row was already written as StatusError by
				// auditedCallApprovalRedirect; the agent should see the block,
				// not an approval ask. Mirrors toolexec.Dispatch (which writes
				// a StatusError row for blocked statements and never prompts).
				if info.Action == string(riskengine.ActionBlock) {
					return err
				}
				// Action=require_approval or warn (or any non-block) — fall
				// through to the approval prompt path. Warn isn't a hard
				// error in the daemon path (toolexec.Dispatch sets
				// WithUserApproved which suppresses the adapter Guard
				// entirely); routing warn to the approval prompt here matches
				// the legacy CLI behavior of "any RiskInfo → approval ask"
				// without regressing block.
				//
				// Adapter Guard escalated: prefer its rule attribution over
				// the static-check fallback (which is nil here, since the
				// static check said "allow"). Without this, the approval row
				// records PolicyAttribution and Agent Audit loses the
				// matched-rule detail that the daemon `tool call` path
				// records for the same statement.
				if execAttribution := agentaudit.AttributionFromError(err); execAttribution != nil {
					approvalAttribution = execAttribution
				}
			} else {
				return r.printResult(opts, item, func() string { return fmt.Sprintf("Rows: %d\n", len(item.Rows)) })
			}
		}
		if err := validateAgentAccessForApproval(opts, service, "execute_statement", execParams, approvalAttribution); err != nil {
			return err
		}
		approvalDetail := map[string]any{}
		if decision.WritePreview != nil {
			approvalDetail["writePreview"] = decision.WritePreview
		}
		return r.approvalRequiredWithDetail(opts, "execute_statement", toolreg.ApprovalSummary("execute_statement", map[string]any{"datasourceId": datasourceID, "statement": statement}), execParams, approvalDetail, approvalAttribution)
	case "explain":
		item, err := auditedCall(ctx, opts, service, "explain_statement", map[string]any{"datasourceId": datasourceID, "database": database, "statement": statement, "analyze": analyze, "executionMode": executionMode}, func(c context.Context) (console.ExplainResult, error) {
			return service.ExplainStatement(c, datasourceID, statement, analyze, database, executionMode)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return "Explain complete.\n" })
	case "scan-redis", "redis-scan":
		item, err := auditedCall(ctx, opts, service, "scan_redis_keys", map[string]any{"datasourceId": datasourceID, "pattern": pattern, "cursor": cursor}, func(c context.Context) (datasourceops.RedisKeyPage, error) {
			return service.ScanRedisKeys(c, datasourceID, pattern, cursor)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return strings.Join(item.Keys, "\n") + "\n" })
	case "metrics":
		if strings.TrimSpace(node) != "" {
			item, err := auditedCall(ctx, opts, service, "get_datasource_metrics_by_node", map[string]any{"datasourceId": datasourceID, "node": node}, func(c context.Context) (datasourceops.DatasourceMetrics, error) {
				return service.GetDatasourceMetricsByNode(c, datasourceID, node)
			})
			if err != nil {
				return err
			}
			return r.printResult(opts, item, func() string { return "Metrics collected.\n" })
		}
		item, err := auditedCall(ctx, opts, service, "get_datasource_metrics", map[string]any{"datasourceId": datasourceID}, func(c context.Context) (datasourceops.DatasourceMetrics, error) {
			return service.GetDatasourceMetrics(c, datasourceID)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return "Metrics collected.\n" })
	case "redis-docs":
		item, err := auditedCall(ctx, opts, service, "get_redis_command_docs", map[string]any{"datasourceId": datasourceID, "command": command}, func(c context.Context) (console.RedisCommandDocsEntry, error) {
			return service.GetRedisCommandDocs(c, datasourceID, command)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Commands: %d\n", len(item.Commands)) })
	case "schema":
		// Param key is "entity", not "pattern": the canonical tool registry
		// (toolreg.get_schema_knowledge) names the target entity that way, and
		// the audit row's BuildSummary / PrimaryTarget only inspect "entity" or
		// "name". Recording it as "pattern" would still write the row, but the
		// Agent Audit table would render an empty target column.
		item, err := auditedCall(ctx, opts, service, "get_schema_knowledge", map[string]any{"datasourceId": datasourceID, "database": database, "entity": pattern}, func(c context.Context) (map[string]any, error) {
			return service.GetSchemaKnowledge(c, datasourceID, pattern, database)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("Schema entities: %v\n", item["entityCount"]) })
	case "er":
		item, err := auditedCall(ctx, opts, service, "get_er_knowledge", map[string]any{"datasourceId": datasourceID, "database": database}, func(c context.Context) (map[string]any, error) {
			return service.GetERKnowledge(c, datasourceID, database)
		})
		if err != nil {
			return err
		}
		return r.printResult(opts, item, func() string { return fmt.Sprintf("%v\n", item["content"]) })
	default:
		return fmt.Errorf("unknown console subcommand: %s\n\n%s", args[0], consoleUsage())
	}
}

func consoleUsage() string {
	return `Usage: futrixdata-cli console <subcommand> [flags]

Subcommands:
  databases    List databases for a datasource
  entities     List entities (tables, collections, etc.)
  describe     Describe an entity's schema
  execute      Execute a statement against a datasource
  explain      Explain a statement's execution plan
  scan-redis   Scan Redis keys by pattern
  metrics      Get datasource metrics
  redis-docs   Get Redis command documentation
  schema       Get schema knowledge for a datasource
  er           Get ER diagram knowledge

Common flags:
  --datasource <id>       Datasource ID
  --database <name>       Database name
  --pattern <pattern>     Filter pattern
  --execution-mode <mode> Execution mode
  --statement <sql>       SQL/query statement (for execute/explain)
  --approve               Approve live execution (for execute)`
}
