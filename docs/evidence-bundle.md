# Evidence bundle

`examples/product-export` is a buyer-review fixture that demonstrates how the public code can verify product-shaped outputs.

It contains:

- `audit-log.jsonl` — local hash-chain audit export;
- `masked-query-result.json` — agent-facing result with masked PII columns;
- `risk-block-response.json` — destructive query blocked by risk attribution;
- `approval-response.json` — update query held for approval with risk attribution.

Run:

```bash
go run ./cmd/futrix-evidence-verify ./examples/product-export
```

The verifier checks:

- audit hash-chain validity;
- masked columns use `masked:v1:` values;
- rows do not contain obvious raw email or phone values;
- the block response matches the public partial risk engine's `DELETE FROM users` decision;
- the approval response matches the public partial risk engine's `UPDATE ... WHERE ...` decision.

These fixtures are sanitized. During a real Enterprise evaluation, ask FutrixData to export equivalent evidence from a disposable datasource in the evaluated environment, then run the same verifier or the narrower audit verifier against that export.
