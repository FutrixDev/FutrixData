# Product export evidence bundle

This directory contains a sanitized FutrixData product-export evidence bundle.

The shapes match the product contracts used by the desktop and Enterprise agent paths:

- `audit-log.jsonl` is a chained agent audit export.
- `masked-query-result.json` shows an agent-facing query result after PII masking.
- `risk-block-response.json` shows a blocked destructive statement.
- `approval-response.json` shows a statement held for approval with risk attribution.

The values are safe demo values, not a copy of a user's local database or audit history. This is intentional: public fixtures should prove the contract without publishing private customer or developer data.

Verify the whole bundle:

```bash
go run ./cmd/futrix-evidence-verify ./examples/product-export
```
