# Threat model

## Protected assets

FutrixData is designed to reduce risk around:

- database credentials;
- sensitive row values returned to AI agents;
- destructive or expensive database statements;
- schema metadata sent to AI tooling;
- agent identity and access-key misuse;
- local audit-log tampering after records are written.

## Trusted components

The local desktop installation or Enterprise deployment is trusted to enforce policy before database execution.

The configured database is trusted to execute accepted statements and return truthful results.

The local secret store is trusted to protect the masking root secret.

The person approving a held action is trusted to understand the displayed summary.

## Untrusted or partially trusted components

AI agents are treated as partially trusted. They can request actions but should not receive database credentials or bypass policy.

LLM providers are treated as external processors. Sensitive row values should be masked before they reach an agent context when policy requires masking.

Local files are not immutable. The audit hash chain can detect changes to the current chained section, but it cannot stop a fully privileged local attacker from rewriting all rows and recomputing hashes.

## Out of scope

This package does not prove:

- endpoint hardening of the commercial Enterprise server;
- correctness of every datasource adapter;
- protection against a compromised operating system;
- remote audit anchoring;
- signing-key custody;
- billing or license enforcement;
- SSO or RBAC implementation details.

Those areas remain part of the commercial product review.
