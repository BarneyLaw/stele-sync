# 008: Client-minted UUIDv7 file identities

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Devices must be able to create files offline and preserve identity across renames.

## Decision

Clients mint UUIDv7 file ids. Reject server-minted ids.

## Consequences

Creation needs no round trip, ids index in time order, and paths remain mutable attributes rather than identities. Client clocks never order edits.
