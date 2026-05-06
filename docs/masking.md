# Masking specification

## Default sensitivity levels

FutrixData uses an extensible L1-L5 model by default.

| Level | Meaning | Examples |
| --- | --- | --- |
| L1 Public | Non-sensitive operational data | `id`, `created_at`, `status` |
| L2 Internal | Internal identifiers and metadata | `user_id`, `session_id`, `request_id` |
| L3 Confidential | Indirect personal, behavior, or location data | `ip_address`, `user_agent`, `device_id` |
| L4 Sensitive | Direct PII, financial, or medical data | `email`, `phone`, `salary`, `date_of_birth` |
| L5 Critical | Credentials and high-sensitivity personal data | `password`, `credit_card`, `api_secret`, `home_address` |

By default, agents can receive L1-L3 fields. L4, L5, unconfirmed, and unknown levels are masked.

## Masking algorithm

For each value:

1. Start with a local root secret.
2. Derive a per-field key with HMAC-SHA256 over:

```text
futrixdata:masking:v1
datasource:<datasource id>
field:<field name>
```

3. HMAC-SHA256 the raw value string with the derived key.
4. Take the first 16 hex characters.
5. Return `masked:v1:<16 hex chars>`.

The same value in the same datasource and field masks to the same output. The same value in a different field or datasource masks differently.

## Limits

Deterministic masking is not anonymization. It keeps equality useful for agents, but low-cardinality fields can still be guessed by enumeration. Treat masked values as pseudonymous data, not public data.
