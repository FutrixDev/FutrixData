package toolreg

import (
	"context"
	"fmt"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/datasourceops"
	"futrixdata/platform/internal/riskengine"
)

// AllTools returns the canonical list of tool definitions.
// Both the CLI and MCP server consume this single source of truth.
func AllTools() []ToolDef {
	return []ToolDef{
		// --- Datasource management ---
		{
			Name:        "list_datasources",
			Description: "List all configured datasources with their connection details, environment tags, and query dialect metadata",
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				items, err := svc.ListDatasources(ctx)
				if err != nil {
					return nil, err
				}
				return datasource.ToAgentViews(items), nil
			},
		},
		{
			Name:              "get_datasource",
			Description:       "Get a single datasource by ID, including environment and query dialect metadata",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				item, err := svc.GetDatasource(ctx, stringValue(p, "datasourceId", "id"))
				if err != nil {
					return nil, err
				}
				return datasource.ToAgentView(item), nil
			},
		},
		datasourceCreateTool("create_datasource", "Create a new datasource. Requires user approval unless the agent has datasource-management grant."),
		datasourceCreateTool("add_datasource", "Add a new datasource. Alias of create_datasource; requires user approval unless the agent has datasource-management grant."),
		{
			Name:             "update_datasource",
			Description:      "Update an existing datasource. Requires user approval.",
			ApprovalRequired: true,
			// Intentionally NOT DangerousScopable: update_datasource accepts a
			// full connection payload (host/credentials/options), so bypassing
			// approval would let a caller silently repoint a trusted dev ID at
			// a different target (e.g. production) and continue operating under
			// the dangerous bypass. Keep reconfiguration behind approval.
			Params: append([]Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID to update"},
			}, datasourcePayloadParams(false)...),
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				var input datasourceops.DataSourcePayload
				if err := MapToStruct(p, &input); err != nil {
					return nil, err
				}
				return svc.UpdateDatasource(ctx, stringValue(p, "datasourceId", "id"), input)
			},
		},
		{
			Name:             "delete_datasource",
			Description:      "Delete a datasource by ID. Requires user approval.",
			ApprovalRequired: true,
			// Intentionally NOT DangerousScopable: dangerous mode authorizes
			// operations *within* a datasource, not removal of the datasource
			// itself (which is irreversible and invalidates the trust boundary
			// the user set up).
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID to delete"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DeleteDatasource(ctx, stringValue(p, "datasourceId", "id"))
			},
		},
		{
			Name:              "test_datasource",
			Description:       "Test connectivity of an existing datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID to test"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.TestDatasource(ctx, stringValue(p, "datasourceId", "id"))
			},
		},
		{
			Name:        "test_datasource_payload",
			Description: "Test connectivity with a datasource payload without saving it",
			Params:      datasourcePayloadParams(true),
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				var input datasourceops.DataSourcePayload
				if err := MapToStruct(p, &input); err != nil {
					return nil, err
				}
				return svc.TestDatasourcePayload(ctx, input)
			},
		},

		// --- Schema inspection ---
		{
			Name:              "list_databases",
			Description:       "List databases (or schemas) on a datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "pattern", Type: TypeString, Description: "Filter pattern (e.g. 'test%')"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.ListDatabases(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "pattern"), stringValue(p, "executionMode"))
			},
		},
		{
			Name:              "list_entities",
			Description:       "List tables, collections, or indexes on a datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "pattern", Type: TypeString, Description: "Filter pattern"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.ListEntities(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "pattern"), stringValue(p, "database"), stringValue(p, "executionMode"), false)
			},
		},
		{
			Name:              "describe_entity",
			Description:       "Describe the structure of a table, collection, or index",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "name", Type: TypeString, Required: true, Description: "Entity name (table, collection, index)"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DescribeEntity(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "name", "entity"), stringValue(p, "database"), stringValue(p, "executionMode"))
			},
		},
		{
			Name:        "list_risk_rules",
			Description: "List configured risk rules with their codes and current enabled state",
			Params: []Param{
				{Name: "includeBuiltin", Type: TypeBoolean, Description: "Set to false to return only custom rules"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				includeBuiltin := true
				if _, ok := p["includeBuiltin"]; ok {
					includeBuiltin = boolValue(p, "includeBuiltin")
				}
				return svc.ListRiskRules(ctx, includeBuiltin)
			},
		},
		{
			Name:             "set_risk_rule",
			Description:      "Insert or update one user risk rule in the live store. Requires user approval; trusted local automation may be granted RiskRuleManagementGrant on its agent identity to bypass the prompt.",
			ApprovalRequired: true,
			Params:           riskRuleParams(),
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				var rule riskengine.Rule
				if err := MapToStruct(p, &rule); err != nil {
					return nil, err
				}
				// Builtin is a runtime-only marker that distinguishes engine-
				// shipped rules from user rules. set_risk_rule only ever
				// creates/updates user rules, so a caller that round-trips
				// list_risk_rules output (which surfaces builtin=true on
				// shipped rules) must not be able to smuggle that flag back
				// onto a user rule. If it were preserved, the engine would
				// treat the user rule as a built-in on the next assessment
				// — bypassing the user-rule approval treatment that warn-
				// level rules receive on Trusted datasources.
				rule.Builtin = false
				return svc.SetRiskRule(ctx, rule)
			},
		},
		{
			Name:             "delete_risk_rule",
			Description:      "Remove one user risk rule by ID from the live store. Requires user approval; trusted local automation may be granted RiskRuleManagementGrant on its agent identity to bypass the prompt.",
			ApprovalRequired: true,
			Params: []Param{
				{Name: "id", Type: TypeString, Required: true, Description: "Rule ID to delete"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				ok, err := svc.DeleteRiskRule(ctx, stringValue(p, "id"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": ok, "id": stringValue(p, "id")}, nil
			},
		},
		{
			// Toggles a builtin or probe-catalog rule's enabled state in the
			// live daemon store and refreshes the engine cache atomically.
			// Provided so the regression harness (and other trusted local
			// automation) can flip overrides like sql-allow-insert without
			// writing YAML out-of-band — direct YAML writes from a separate
			// store instance are invisible to the daemon's in-memory cache
			// until a full reload, which produces silent false-negatives.
			Name:             "set_builtin_risk_rule_enabled",
			Description:      "Enable or disable one built-in / probe-catalog risk rule in the live store. Requires user approval; trusted local automation may be granted RiskRuleManagementGrant on its agent identity to bypass the prompt.",
			ApprovalRequired: true,
			Params: []Param{
				{Name: "id", Type: TypeString, Required: true, Description: "Built-in rule ID (e.g. sql-allow-insert)"},
				{Name: "enabled", Type: TypeBoolean, Required: true, Description: "Target enabled state for the override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				// `enabled` is the entire payload of this tool; treating a
				// missing or non-boolean value as the zero-value `false`
				// would silently disable the target rule on a typo or
				// omitted field. Trusted local automation that calls this
				// tool with malformed input deserves a clear error, not a
				// quiet policy weakening.
				rawEnabled, present := p["enabled"]
				if !present {
					return nil, fmt.Errorf("enabled boolean is required")
				}
				enabled, ok := rawEnabled.(bool)
				if !ok {
					return nil, fmt.Errorf("enabled must be a boolean, got %T", rawEnabled)
				}
				okResult, err := svc.SetBuiltinRiskRuleEnabled(ctx, stringValue(p, "id"), enabled)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": okResult, "id": stringValue(p, "id"), "enabled": enabled}, nil
			},
		},
		{
			// Persists threshold overrides for a probe-catalog rule and
			// refreshes the engine. Same rationale as set_builtin_risk_rule_enabled
			// — the harness needs the engine cache to pick up the new
			// thresholds atomically with the YAML write.
			Name:             "set_builtin_risk_rule_thresholds",
			Description:      "Update threshold overrides for one probe-catalog risk rule (e.g. probe-wide-scan). Requires user approval; trusted local automation may be granted RiskRuleManagementGrant on its agent identity to bypass the prompt.",
			ApprovalRequired: true,
			Params: []Param{
				{Name: "id", Type: TypeString, Required: true, Description: "Probe-catalog rule ID (e.g. probe-wide-scan)"},
				{Name: "thresholds", Type: TypeObject, Required: true, Description: "RuleThresholds: maxExaminedRows, maxJoinCount, maxFullScans, maxEstimatedJoinRows, seqScanRowsThreshold, costThreshold, allowSafeSeqScan, maxDynamoDBPages, maxDynamoDBEvaluatedItems"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				// thresholds may be present-but-empty when the caller is
				// clearing the override (e.g. the regression harness's
				// snapshot-restore path). Reject only when the key is
				// missing entirely or the value is the wrong shape — an
				// empty object is a legitimate "drop overrides" request.
				rawValue, present := p["thresholds"]
				if !present {
					return nil, fmt.Errorf("thresholds object is required")
				}
				rawThresholds, ok := rawValue.(map[string]any)
				if !ok {
					if rawValue == nil {
						rawThresholds = map[string]any{}
					} else {
						return nil, fmt.Errorf("thresholds must be an object")
					}
				}
				var thresholds riskengine.RuleThresholds
				if err := MapToStruct(rawThresholds, &thresholds); err != nil {
					return nil, err
				}
				persisted, err := svc.SetBuiltinRiskRuleThresholds(ctx, stringValue(p, "id"), thresholds)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "id": stringValue(p, "id"), "thresholds": persisted}, nil
			},
		},

		// --- Query execution ---
		{
			Name:              "execute_statement",
			Description:       "Execute a SQL statement, MongoDB command, Redis command, or Elasticsearch query. Write operations may require user approval.",
			ApprovalRequired:  true,
			DangerousScopable: true,
			NeedsApproval: func(ctx context.Context, svc Service, p map[string]any) (bool, error) {
				return ShouldRequireStatementApproval(ctx, svc, stringValue(p, "datasourceId", "id"), stringValue(p, "statement"), stringValue(p, "database"), stringValue(p, "executionMode"))
			},
			AssessApproval: func(ctx context.Context, svc Service, p map[string]any) (ApprovalDecision, error) {
				return AssessStatementApproval(ctx, svc, stringValue(p, "datasourceId", "id"), stringValue(p, "statement"), stringValue(p, "database"), stringValue(p, "executionMode"))
			},
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "statement", Type: TypeString, Required: true, Description: "The statement to execute (SQL, MongoDB command, Redis command, etc.)"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
				{Name: "pagingToken", Type: TypeString, Description: "Paging token for next page of results"},
				{Name: "pageSize", Type: TypeNumber, Description: "Number of rows per page. For DynamoDB, this maps to evaluated items and is not guaranteed returned/matched rows."},
				{Name: "maxReturnedRows", Type: TypeNumber, Description: "DynamoDB bounded pagination: stop after returning this many matched rows"},
				{Name: "maxPages", Type: TypeNumber, Description: "DynamoDB bounded pagination: requested maximum DynamoDB pages to fetch; effective value may be capped by risk policy"},
				{Name: "maxEvaluatedItems", Type: TypeNumber, Description: "DynamoDB bounded pagination: requested evaluated-item budget across fetched pages; effective value may be capped by risk policy; DynamoDB PartiQL does not return actual ScannedCount per page"},
				{Name: "strictLimits", Type: TypeBoolean, Description: "DynamoDB bounded pagination: when true, reject maxPages or maxEvaluatedItems requests above risk-policy caps instead of clamping them"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				bounds := console.ExecuteBounds{
					MaxReturnedRows:   intValue(p, "maxReturnedRows"),
					MaxPages:          intValue(p, "maxPages"),
					MaxEvaluatedItems: intValue(p, "maxEvaluatedItems"),
					StrictLimits:      boolValue(p, "strictLimits"),
				}
				datasourceID := stringValue(p, "datasourceId", "id")
				pageSize := intValue(p, "pageSize")
				if err := validateDynamoDBToolExecutionLimits(ctx, svc, datasourceID, pageSize, bounds); err != nil {
					return nil, err
				}
				return svc.ExecuteStatement(ctx, datasourceID, stringValue(p, "statement"), stringValue(p, "database"), stringValue(p, "pagingToken"), pageSize, stringValue(p, "executionMode"), bounds)
			},
		},
		{
			Name:              "explain_statement",
			Description:       "Get the execution plan for a statement",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "statement", Type: TypeString, Required: true, Description: "The statement to explain"},
				{Name: "analyze", Type: TypeBoolean, Description: "Run EXPLAIN ANALYZE (actually executes the statement)"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.ExplainStatement(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "statement"), boolValue(p, "analyze"), stringValue(p, "database"), stringValue(p, "executionMode"))
			},
		},

		// --- Redis-specific ---
		{
			Name:              "execute_redis_command",
			Description:       "Execute a Redis command using argv-style arguments. Prefer this over execute_statement for Redis so values with spaces remain a single argument. Write/admin commands may require user approval.",
			ApprovalRequired:  true,
			DangerousScopable: true,
			AssessApproval: func(ctx context.Context, svc Service, p map[string]any) (ApprovalDecision, error) {
				args, err := stringSliceValue(p, "args")
				if err != nil {
					return ApprovalDecision{NeedsApproval: true}, nil
				}
				return AssessRedisCommandApproval(ctx, svc, stringValue(p, "datasourceId", "id"), args, stringValue(p, "database"), stringValue(p, "executionMode"))
			},
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Redis datasource ID"},
				{Name: "args", Type: TypeArray, Required: true, Description: "Redis command argv, e.g. [\"GET\", \"user:1\"] or [\"SET\", \"user:1\", \"value with spaces\"]", Items: Param{Name: "arg", Type: TypeString, Description: "One Redis command argument"}, MinItems: 1},
				{Name: "database", Type: TypeString, Description: "Redis logical database index override, if applicable"},
				{Name: "executionMode", Type: TypeString, Description: "Execution mode override"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				args, err := stringSliceValue(p, "args")
				if err != nil {
					return nil, err
				}
				return svc.ExecuteRedisCommand(ctx, stringValue(p, "datasourceId", "id"), args, stringValue(p, "database"), stringValue(p, "executionMode"))
			},
		},
		{
			Name:              "scan_redis_keys",
			Description:       "Scan Redis keys matching a pattern",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID (must be a Redis datasource)"},
				{Name: "pattern", Type: TypeString, Description: "Key pattern (e.g. 'user:*')"},
				{Name: "cursor", Type: TypeString, Description: "Cursor for pagination"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.ScanRedisKeys(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "pattern"), stringValue(p, "cursor"))
			},
		},
		{
			Name:              "execute_redis_batch",
			Description:       "Execute a bounded Redis pipeline batch with structured command arguments. Every operation is risk-assessed before execution; results include per-operation partial failures.",
			ApprovalRequired:  true,
			DangerousScopable: true,
			AssessApproval: func(ctx context.Context, svc Service, p map[string]any) (ApprovalDecision, error) {
				input, err := redisBatchInputFromParams(p)
				if err != nil {
					return ApprovalDecision{}, err
				}
				return AssessRedisBatchApproval(ctx, svc, input.DatasourceID, input.Operations, input.ExecutionMode)
			},
			Params: redisBatchParams(),
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				input, err := redisBatchInputFromParams(p)
				if err != nil {
					return nil, err
				}
				executor, ok := svc.(redisBatchExecutorService)
				if !ok {
					return nil, console.ErrUnsupported
				}
				return executor.ExecuteRedisBatch(ctx, input.DatasourceID, input.BatchID, input.Operations, input.ExecutionMode)
			},
		},
		{
			Name:              "get_redis_command_docs",
			Description:       "Get documentation for a Redis command",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID (must be a Redis datasource)"},
				{Name: "command", Type: TypeString, Required: true, Description: "Redis command name (e.g. 'GET', 'HSET')"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetRedisCommandDocs(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "command"))
			},
		},

		// --- Metrics ---
		{
			Name:              "get_datasource_metrics",
			Description:       "Get performance metrics for a datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetDatasourceMetrics(ctx, stringValue(p, "datasourceId", "id"))
			},
		},
		{
			Name:              "get_datasource_metrics_by_node",
			Description:       "Get performance metrics for a specific node in a clustered datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "node", Type: TypeString, Required: true, Description: "Node identifier"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetDatasourceMetricsByNode(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "node"))
			},
		},

		// --- Knowledge ---
		{
			Name:              "get_schema_knowledge",
			Description:       "Get schema knowledge (column descriptions, relationships) for a table or collection",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "entity", Type: TypeString, Required: true, Description: "Entity name (table or collection)"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetSchemaKnowledge(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "entity"), stringValue(p, "database"))
			},
		},
		{
			Name:              "get_er_knowledge",
			Description:       "Get entity-relationship knowledge for a database",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "database", Type: TypeString, Description: "Database name (if applicable)"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetERKnowledge(ctx, stringValue(p, "datasourceId", "id"), stringValue(p, "database"))
			},
		},
		// --- Sensitivity ---
		{
			Name:        "get_sensitivity_config",
			Description: "Get the current sensitivity mode, reusable classification rules, and level definitions",
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetSensitivityConfig(ctx)
			},
		},
		{
			Name:        "set_sensitivity_custom_rules",
			Description: "Save reusable free-form classification rules for future local-agent sensitivity runs",
			Params: []Param{
				{Name: "rules", Type: TypeString, Required: true, Description: "Reusable sensitivity classification rules"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				ok, err := svc.SetSensitivityCustomRules(ctx, stringValue(p, "rules"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": ok}, nil
			},
		},
		{
			Name:              "get_sensitivity_report",
			Description:       "Get the saved sensitivity classification report for one datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.GetSensitivityReport(ctx, stringValue(p, "datasourceId", "id"))
			},
		},
		{
			Name:              "save_sensitivity_report",
			Description:       "Save a full datasource sensitivity classification report produced by a local agent",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
				{Name: "schemaHash", Type: TypeString, Description: "Optional schema hash captured by the local agent"},
				{Name: "database", Type: TypeString, Description: "Optional database name override stored with the report"},
				{Name: "customRules", Type: TypeString, Description: "Optional reusable classification rules to save before the report"},
				{Name: "entities", Type: TypeArray, Required: true, Description: "Classified entities and fields", Items: sensitivityEntityItemsSchema(), MinItems: 1},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				var input datasourceops.SaveSensitivityReportInput
				if err := MapToStruct(p, &input); err != nil {
					return nil, err
				}
				return svc.SaveSensitivityReport(ctx, input)
			},
		},
		{
			Name:              "delete_sensitivity_report",
			Description:       "Delete the saved sensitivity classification report for one datasource",
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DeleteSensitivityReport(ctx, stringValue(p, "datasourceId", "id"))
			},
		},

		// --- Cloudflare D1 ---
		{
			Name:        "d1_oauth_login",
			Description: "Login to Cloudflare D1 via OAuth browser flow",
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.D1OAuthLogin(ctx)
			},
		},
		{
			Name:        "d1_oauth_relogin",
			Description: "Re-login to Cloudflare D1 via OAuth (refreshes session)",
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.D1OAuthReLogin(ctx)
			},
		},
		{
			Name:        "d1_is_wrangler_installed",
			Description: "Check if the Wrangler CLI tool is installed and available",
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				installed, err := svc.D1IsWranglerInstalled(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"installed": installed}, nil
			},
		},
		{
			Name:        "d1_list_cloud_databases",
			Description: "List Cloudflare D1 cloud databases",
			Params: []Param{
				{Name: "accountId", Type: TypeString, Required: true, Description: "Cloudflare account ID"},
				{Name: "token", Type: TypeString, Required: true, Description: "Cloudflare API token"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.D1ListCloudDatabases(ctx, stringValue(p, "accountId"), stringValue(p, "token"))
			},
		},
		{
			Name:             "d1_create_cloud_database",
			Description:      "Create a new Cloudflare D1 cloud database. Requires user approval.",
			ApprovalRequired: true,
			Params: []Param{
				{Name: "accountId", Type: TypeString, Required: true, Description: "Cloudflare account ID"},
				{Name: "token", Type: TypeString, Required: true, Description: "Cloudflare API token"},
				{Name: "name", Type: TypeString, Required: true, Description: "Database name"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.D1CreateCloudDatabase(ctx, stringValue(p, "accountId"), stringValue(p, "token"), stringValue(p, "name"))
			},
		},
		{
			Name:              "d1_deploy_migrations",
			Description:       "Deploy D1 migrations for a datasource. Requires user approval.",
			ApprovalRequired:  true,
			DangerousScopable: true,
			Params: []Param{
				{Name: "datasourceId", Type: TypeString, Required: true, Description: "Datasource ID"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.D1DeployMigrations(ctx, stringValue(p, "datasourceId", "id"))
			},
		},

		// --- DynamoDB SSO ---
		{
			Name:        "dynamodb_sso_list_profiles",
			Description: "List AWS SSO profiles from local AWS config",
			Params: []Param{
				{Name: "configPath", Type: TypeString, Description: "Path to AWS config file (defaults to ~/.aws/config)"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOListProfiles(ctx, stringValue(p, "configPath"))
			},
		},
		{
			Name:        "dynamodb_sso_login",
			Description: "Login to an AWS SSO profile",
			Params: []Param{
				{Name: "profile", Type: TypeString, Required: true, Description: "AWS SSO profile name"},
				{Name: "configPath", Type: TypeString, Description: "Path to AWS config file"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOLogin(ctx, stringValue(p, "profile"), stringValue(p, "configPath"))
			},
		},
		{
			Name:        "dynamodb_sso_authorize",
			Description: "Authorize AWS SSO and return temporary credentials",
			Params: []Param{
				{Name: "profile", Type: TypeString, Required: true, Description: "AWS SSO profile name"},
				{Name: "region", Type: TypeString, Required: true, Description: "AWS region"},
				{Name: "configPath", Type: TypeString, Description: "Path to AWS config file"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOOAuthAuthorize(ctx, stringValue(p, "profile"), stringValue(p, "region"), stringValue(p, "configPath"))
			},
		},
		{
			Name:        "dynamodb_sso_list_accounts",
			Description: "List AWS accounts available via SSO",
			Params: []Param{
				{Name: "accessToken", Type: TypeString, Required: true, Description: "SSO access token"},
				{Name: "region", Type: TypeString, Required: true, Description: "AWS region"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOListAccounts(ctx, stringValue(p, "accessToken"), stringValue(p, "region"))
			},
		},
		{
			Name:        "dynamodb_sso_list_account_roles",
			Description: "List available roles for an AWS SSO account",
			Params: []Param{
				{Name: "accountId", Type: TypeString, Required: true, Description: "AWS account ID"},
				{Name: "accessToken", Type: TypeString, Required: true, Description: "SSO access token"},
				{Name: "region", Type: TypeString, Required: true, Description: "AWS region"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOListAccountRoles(ctx, stringValue(p, "accountId"), stringValue(p, "accessToken"), stringValue(p, "region"))
			},
		},
		{
			Name:        "dynamodb_sso_get_role_credentials",
			Description: "Get temporary AWS credentials for an SSO role",
			Params: []Param{
				{Name: "accountId", Type: TypeString, Required: true, Description: "AWS account ID"},
				{Name: "roleName", Type: TypeString, Required: true, Description: "IAM role name"},
				{Name: "accessToken", Type: TypeString, Required: true, Description: "SSO access token"},
				{Name: "region", Type: TypeString, Required: true, Description: "AWS region"},
			},
			Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
				return svc.DynamoDBSSOGetRoleCredentials(ctx, stringValue(p, "accountId"), stringValue(p, "roleName"), stringValue(p, "accessToken"), stringValue(p, "region"))
			},
		},
	}
}

func validateDynamoDBToolExecutionLimits(ctx context.Context, svc Service, datasourceID string, pageSize int, bounds console.ExecuteBounds) error {
	ds, err := svc.GetDatasource(ctx, datasourceID)
	if err != nil {
		return nil
	}
	if ds.Type != datasource.TypeDynamoDB {
		return nil
	}
	return console.ValidateDynamoDBToolExecutionLimits(pageSize, bounds)
}

func stringSliceValue(p map[string]any, key string) ([]string, error) {
	raw, ok := p[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s array is required", key)
	}
	switch typed := raw.(type) {
	case []string:
		if len(typed) == 0 {
			return nil, fmt.Errorf("%s array must not be empty", key)
		}
		return append([]string(nil), typed...), nil
	case []any:
		if len(typed) == 0 {
			return nil, fmt.Errorf("%s array must not be empty", key)
		}
		out := make([]string, len(typed))
		for i, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string", key, i)
			}
			out[i] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func datasourceCreateTool(name, description string) ToolDef {
	return ToolDef{
		Name:             name,
		Description:      description,
		ApprovalRequired: true,
		Params:           datasourcePayloadParams(true),
		Call: func(ctx context.Context, svc Service, p map[string]any) (any, error) {
			var input datasourceops.DataSourcePayload
			if err := MapToStruct(p, &input); err != nil {
				return nil, err
			}
			return svc.CreateDatasource(ctx, input)
		},
	}
}

// datasourcePayloadParams returns the common parameter set for datasource
// create/update/test operations.
func datasourcePayloadParams(nameRequired bool) []Param {
	params := []Param{
		{Name: "name", Type: TypeString, Required: nameRequired, Description: "Display name"},
		{Name: "type", Type: TypeString, Required: nameRequired, Description: "Database type: mysql, postgresql, mongodb, redis, elasticsearch, dynamodb, d1"},
		{Name: "host", Type: TypeString, Description: "Hostname or IP"},
		{Name: "port", Type: TypeNumber, Description: "Port number"},
		{Name: "database", Type: TypeString, Description: "Default database name"},
		{Name: "username", Type: TypeString, Description: "Username"},
		{Name: "password", Type: TypeString, Description: "Password"},
		{Name: "authSource", Type: TypeString, Description: "Auth source (MongoDB auth database)"},
		{Name: "options", Type: TypeObject, Description: "Type-specific options. Common: sslEnabled (bool). D1: databaseId, accountId, binding, supportDev, devProjectPath. DynamoDB: region. Redis cluster: nodes (array of host:port). SQL URI mode: uri (string)."},
	}
	return params
}

// riskRuleParams returns the parameter schema for the set_risk_rule tool.
// The shape mirrors riskengine.Rule's JSON tags so a caller can pass the
// same JSON structure used by the rule YAML / list_risk_rules output.
// The tool's Call closure decodes the full param map via MapToStruct, so
// callers may include nested scope/when fields beyond what the schema
// explicitly enumerates here. The enumerated fields are the ones the
// approval-summary surface and IDE schema completion benefit from.
func riskRuleParams() []Param {
	return []Param{
		{Name: "id", Type: TypeString, Required: true, Description: "Rule ID. Stable identifier used by list_risk_rules and audit attribution."},
		{Name: "code", Type: TypeString, Description: "Optional human-readable code (e.g. USR-001). Auto-assigned when blank."},
		{Name: "description", Type: TypeString, Description: "Human description of the rule"},
		{Name: "enabled", Type: TypeBoolean, Description: "Whether the rule is active"},
		{Name: "priority", Type: TypeNumber, Description: "Higher priority wins when multiple rules match the same statement"},
		{Name: "action", Type: TypeString, Description: "Rule action: allow, warn, require_approval, or block", Enum: []string{"allow", "warn", "require_approval", "block"}},
		{Name: "reason", Type: TypeString, Description: "Reason text shown to users when the rule fires"},
		{Name: "scope", Type: TypeObject, Description: "RuleScope: dsTypes (array of strings), datasourceId, entity, entityPattern, keyPattern"},
		{Name: "when", Type: TypeObject, Description: "RuleCondition: command (array), operationClass (array, all listed classes must match: read/write/admin/scan/script), statementPattern, statementNotPattern, hasWhere, sqlMultiStatement, sqlParseFailed, httpMethod (array), pathPattern, bodyPattern, bodyNotPattern, any (array), not (object)"},
		{Name: "thresholds", Type: TypeObject, Description: "RuleThresholds: maxExaminedRows, maxJoinCount, maxFullScans, maxEstimatedJoinRows, seqScanRowsThreshold, costThreshold, allowSafeSeqScan, maxDynamoDBPages, maxDynamoDBEvaluatedItems"},
	}
}

func sensitivityEntityItemsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{
				"type":        "string",
				"description": "Optional database name override for this entity",
			},
			"entity": map[string]any{
				"type":        "string",
				"description": "Entity name",
			},
			"fields": map[string]any{
				"type":        "array",
				"description": "Classified fields for the entity",
				"minItems":    1,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Field name",
						},
						"level": map[string]any{
							"type":        "string",
							"description": "Configured sensitivity level key such as L1-L5",
						},
						"category": map[string]any{
							"type":        "string",
							"description": "Sensitivity category",
							"enum":        []string{"pii", "credential", "financial", "behavioral", "medical", "location", "contact", "identifier", "none"},
						},
						"reason": map[string]any{
							"type":        "string",
							"description": "Why the field was classified this way",
						},
					},
					"required": []string{"name", "level", "category"},
				},
			},
		},
		"required": []string{"entity", "fields"},
	}
}
