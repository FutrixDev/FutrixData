# Partial risk-engine specification

This repository opens the portable core of the FutrixData risk engine: rule types, a lightweight statement parser, rule matching, priority ordering, and the final allow/warn/approval/block evaluator.

It is intentionally not the full commercial runtime. The desktop and Enterprise products add datasource adapters, richer SQL parser integrations, EXPLAIN probes, trust-mode storage, approval dispatch, daemon cache behavior, and audit writing.

## Decisions

The rule engine returns one of four actions:

| Action | Meaning |
| --- | --- |
| `allow` | The statement can proceed. |
| `warn` | The statement is risky enough to surface to policy. |
| `require_approval` | The statement must be explicitly approved. |
| `block` | The statement should not run through an agent path. |

Risk levels are derived from actions:

- `allow` -> `low`;
- `warn` -> `medium`;
- `require_approval` and `block` -> `high`.

## Rule shape

```json
{
  "id": "sql-block-delete-no-where",
  "code": "SQL-005",
  "description": "Block DELETE without WHERE",
  "scope": {
    "dsTypes": ["mysql", "postgresql", "d1"]
  },
  "enabled": true,
  "priority": 90,
  "action": "block",
  "reason": "DELETE without WHERE",
  "when": {
    "command": ["delete"],
    "hasWhere": false
  }
}
```

User rules are evaluated before built-in rules. More specific scope and higher priority win.

## Open risk-engine coverage

The public package includes portable built-ins and evaluator logic for:

- SQL-family sources: MySQL, PostgreSQL, Cloudflare D1;
- Redis and Redis Cluster;
- MongoDB;
- Elasticsearch;
- DynamoDB PartiQL.

The commercial product adds live datasource execution, richer SQL parsing, EXPLAIN probes, trust-mode storage, approval routing, and runtime cache behavior.
