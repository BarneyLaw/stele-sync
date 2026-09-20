# 006: Opaque hashed device tokens

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Devices need simple enrollment and immediate revocation without signing-key infrastructure.

## Decision

Use opaque random device tokens, store only their SHA-256 hashes, and compare in constant time. Reject JWTs.

## Consequences

Revocation is a database update checked on subsequent frames. No signing keys are managed; tokens must never enter URLs or logs.
