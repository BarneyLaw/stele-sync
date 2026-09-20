# 003: Keep live text and sidecar heads in Postgres

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Live editing should continue during a Garage outage; text vaults are small.

## Decision

Keep Class A/B head content in Postgres. Reject storing live heads only in Garage.

## Consequences

Head content and operation history commit atomically. Garage stores durable snapshots and opaque blobs; a Garage outage delays those operations without stopping live text sync.
