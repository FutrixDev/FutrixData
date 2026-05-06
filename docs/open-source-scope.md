# Open-source scope analysis

## Goal

Enterprise buyers need enough code to verify FutrixData's security claims before purchase. At the same time, the public repository should not contain enough product code to rebuild the full desktop app or Enterprise server.

The best boundary is a public security package: specs, testable verifiers, protocol types, and portable rule/masking code.

## Claims that should be reviewable

The public FutrixData site and documentation emphasize these security claims:

- agents get query results, not raw credentials;
- risky statements are checked before execution;
- sensitive fields can be masked before results reach agents;
- agent calls are attributed and audited;
- audit logs use a local hash-chain verifier;
- Enterprise users can reason about protocol behavior, approval behavior, and revocation/error surfaces.

Those claims map cleanly to small public modules.

## Recommended public modules

### 1. Audit-chain verifier

Open `pkg/auditchain` and `cmd/futrix-audit-verify`.

This gives buyers something concrete to run against exported audit logs. It is useful without revealing the whole audit store, identity store, desktop UI, daemon, or Enterprise audit pipeline.

### 2. Masking algorithm and sensitivity types

Open `pkg/masking`.

This exposes the deterministic HMAC-SHA256 masking contract, default L1-L5 level model, and row-level masking behavior. It does not expose credential storage, OS keyring integration, datasource adapters, UI workflows, or AI classification orchestration.

### 3. Partial risk-engine core

Open `pkg/riskengine`.

This lets reviewers inspect the built-in rule model, lightweight statement parser, matching priority, and allow/warn/approval/block evaluator. It intentionally omits production datasource execution, richer parser integrations, EXPLAIN adapters, database clients, trust-mode storage, and daemon cache handling.

### 4. Agent protocol types

Open `pkg/protocol`.

This documents tool names, request/response envelopes, approval-required responses, risk attribution, and masked-column reporting. It does not expose the real dispatcher, access-key store, IPC server, MCP server implementation, or Enterprise HTTP service.

### 5. Release verification helper

Open `release-verification/verify-checksums.sh`.

This supports binary integrity checks without publishing signing credentials, private CI secrets, notarization keys, or release automation internals.

### 6. Specs and examples

Open `docs/` and `examples/`.

Specs make security behavior easier to review. Examples let users test the packages quickly.

### 7. Assurance evidence verifier

Open `pkg/evidence` and `cmd/futrix-evidence-verify`.

This gives buyers a single command that checks the public evidence bundle: audit-chain validity, masked agent results, blocked risk response, and approval-required response. It is not a substitute for a full product evaluation, but it turns the public repository into a runnable assurance package.

## Keep private

These areas should stay outside the public repository:

- desktop app shell and UX;
- datasource adapters and connection logic;
- credential encryption and OS keyring integration;
- auth, license, account, billing, and entitlement code;
- Enterprise server, SSO, RBAC, tenant management, and deployment automation;
- release signing, notarization, certificates, CI secrets, and private packaging tokens;
- commercial support workflows and internal roadmap;
- customer data, telemetry, logs, or operational endpoints.

## Why this boundary works

The selected modules prove the most important security contracts:

- what is audited;
- how local audit tampering is detected;
- how sensitive values are transformed before reaching agents;
- how rules decide allow, warn, approval, or block;
- what an agent sees when a call succeeds, fails, or needs approval.

They do not include the product shell that wires everything into a complete commercial application.

## Future phases

Phase 1 should publish this package once reviewed.

Phase 2 can add more test vectors and exported audit samples.

Phase 3 can consider additional runtime components only when their APIs are stable and their release will not expose commercial implementation details.
