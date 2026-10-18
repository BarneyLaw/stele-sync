# 026: Shared canonical identities and safe counters

Status: accepted

Date: 2026-10-18

Source: [Accepted M4 proposal](../../internal/proto/M4%20proposal%20proto.md).

## Context

Storage and other adapters need identities but may not depend on proto.
JavaScript cannot represent arbitrary int64 counters exactly.

## Decision

Put distinct FileID, DeviceID and ClientOpID [16]byte types in internal/core/ids.
Accept only lowercase canonical UUIDv7 with RFC 9562 variant bits. Generate from
an explicit 48-bit Unix millisecond timestamp and ten bytes from an injected
entropy reader. Set version and variant bits after reading. No clock or random
source is hidden in the core. Invalid text does not overwrite an existing id.
Version ranges from 1 to 2^53−1; Seq from 0 to 2^53−1. Proto checks numeric tokens
for integer spelling, presence, sign and range before constructing these types.

## Consequences

Every layer can share the identifiers without breaking dependency direction.
Callers inject cryptographic randomness and enforce any desired monotonic-id
policy. Random UUIDv4 identifiers and uppercase legacy spellings are rejected.
TypeScript UUID generation takes the same explicit inputs.
