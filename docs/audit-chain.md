# Audit-chain specification

## Record format

FutrixData audit logs are JSONL. New chained rows contain these fields:

| Field | Meaning |
| --- | --- |
| `seq` | Physical non-empty row number, starting at 1. |
| `prev_hash` | Previous chained row hash, or the genesis hash for the first chained row. |
| `payload_hash` | SHA-256 of the canonical row payload after removing chain fields. |
| `chain_hash` | SHA-256 of `seq`, `prev_hash`, `payload_hash`, and `chain_version`. |
| `chain_version` | Current value: `local-sha256-v1`. |

Rows without any chain fields are legacy rows. A legacy prefix is accepted. Once the chain starts, later legacy rows fail verification.

## Canonical payload

To compute `payload_hash`:

1. Parse the JSON row.
2. Remove `seq`, `prev_hash`, `payload_hash`, `chain_hash`, and `chain_version`.
3. JSON-encode the resulting object using Go's standard JSON encoder.
4. Hash the encoded bytes with SHA-256.

## Chain hash

To compute `chain_hash`, JSON-encode:

```json
{
  "chain_version": "local-sha256-v1",
  "payload_hash": "<payload_hash>",
  "prev_hash": "<prev_hash>",
  "seq": 1
}
```

Then hash the encoded bytes with SHA-256.

## Verification

Run:

```bash
go run ./cmd/futrix-audit-verify ./examples/audit-log/valid.jsonl
```

The JSON result reports:

- `pass`;
- `verified_records`;
- `legacy_records`;
- `total_records`;
- `first_broken_position`;
- `expected_hash`;
- `actual_hash`;
- `source`;
- `path`.

## Limits

This is local tamper evidence. It can show that the current file no longer matches the hashes written into it. It is not remote signing, object lock, SIEM export, external timestamping, or immutable storage.
