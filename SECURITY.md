# Security Policy

FutrixData is a security-sensitive data gateway. Please do not post secrets,
credentials, private logs, customer data, exploit details, or production
connection strings in public issues, pull requests, or discussions.

Report suspected vulnerabilities through a private disclosure channel controlled
by FutrixData before publishing details. Include only the minimum information
needed to reproduce the issue, and redact datasource names, hostnames, tokens,
query results, and customer identifiers.

Areas that are especially sensitive:

- datasource credential handling and secret providers;
- agent access keys, authorization, revocation, and IPC envelopes;
- risk-rule bypasses or unsafe auto-execution;
- sensitivity classification and masking failures;
- audit-chain integrity and log redaction;
- release, packaging, signing, and update behavior.

If you are unsure whether a report contains sensitive material, treat it as
private first.
