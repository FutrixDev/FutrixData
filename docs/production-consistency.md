# Production consistency statement

This repository contains public security code and specs extracted from FutrixData's production design. It is not a full source release of the product.

## Directly aligned with production concepts

These public packages track production concepts closely:

| Public package | Production alignment | Commercial additions |
| --- | --- | --- |
| `pkg/auditchain` | Matches the local audit hash-chain field names, hash inputs, version string, and verifier result shape used by FutrixData audit exports. | Secure file storage, file locking, encrypted local data handling, product CLI integration, UI display, Enterprise audit aggregation. |
| `pkg/masking` | Matches the L1-L5 sensitivity model and `masked:v1:<16 hex>` deterministic HMAC-SHA256 output contract. | OS keyring secret management, migration fallback, SQL result-column origin tracking, datasource classification store, product UI flows. |
| `pkg/protocol` | Matches the public agent-facing concepts: tool names, tool-call envelope, approval response, error response, audit IDs, masked columns, and risk attribution. | Actual MCP server, Skill CLI, daemon IPC, HTTP/Enterprise transport, access-key validation, revocation, schema-egress gates. |

## Portable subset

`pkg/riskengine` is a portable public subset, not the complete commercial risk engine.

It opens:

- rule data model;
- lightweight statement parsing;
- built-in baseline rules;
- user rule priority and scope handling;
- allow / warn / require_approval / block evaluator.

The commercial product additionally includes:

- richer SQL parser integrations;
- live datasource adapters;
- EXPLAIN and query-plan probes;
- datasource trust-mode storage;
- approval routing;
- daemon rule-cache reload behavior;
- product audit writing;
- UI configuration and Enterprise policy management.

## Public examples

`examples/product-export` contains sanitized product-export fixtures. They use safe demo values and product-shaped JSON contracts. They are not copied from a real customer's data or from a developer's private local database.

The purpose is to let reviewers run the public verifier against the same kinds of outputs a product evaluation should request:

- an audit log export;
- an agent-facing masked query result;
- a risk-block response;
- an approval-required response with risk attribution.

## Wording guidance

Accurate public wording:

- "FutrixData publishes security specifications, verifiers, protocol types, masking code, and a partial risk-engine core."
- "The audit-chain verifier can check product-exported audit logs."
- "The public risk engine is a portable subset; the commercial product adds live execution, EXPLAIN probes, trust modes, approval routing, and Enterprise policy controls."

Avoid:

- "FutrixData is fully open source."
- "The entire risk engine is open source."
- "Public masked fixtures prove customer data is anonymized."
- "Local audit hash chains are immutable."
