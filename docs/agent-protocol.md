# Agent protocol

## Tool call envelope

```json
{
  "tool": "execute_statement",
  "accessKey": "fxd_live_...",
  "protocol": "mcp",
  "params": {
    "datasourceId": "prod-postgres",
    "statement": "select * from users where id = 1042"
  }
}
```

## Success response

```json
{
  "tool": "execute_statement",
  "ok": true,
  "result": {
    "rows": []
  },
  "auditId": "audit_01HQ..."
}
```

## Approval response

```json
{
  "tool": "execute_statement",
  "ok": false,
  "approvalRequired": {
    "tool": "execute_statement",
    "summary": "Execute statement on datasource \"prod-postgres\"",
    "riskAttribution": {
      "source": "risk_engine",
      "action": "warn",
      "level": "medium",
      "ruleId": "sql-warn-update",
      "ruleCode": "SQL-008",
      "reasons": ["UPDATE"]
    }
  }
}
```

## Error response

```json
{
  "tool": "execute_statement",
  "ok": false,
  "error": {
    "code": "tool_error",
    "message": "DELETE without WHERE",
    "riskAttribution": {
      "source": "risk_engine",
      "action": "block",
      "level": "high",
      "ruleId": "sql-block-delete-no-where"
    }
  }
}
```

## Public tool names

The public package documents the stable tool names in `pkg/protocol`. The commercial product implements the full transport adapters for MCP, CLI, Skill, HTTP, and daemon IPC.
