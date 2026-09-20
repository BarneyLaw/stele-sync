# 011: Element-level sidecar operations

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

A pen stroke is atomic, and text OT applied to JSON can corrupt annotation structure.

## Decision

Use id-keyed element last-writer-wins in server order, with deletion winning over later puts to tombstoned ids. Reject text OT on sidecar JSON.

## Consequences

Unknown fields must survive, tombstones remain through operation retention, and deterministic serialization produces identical content. M2 must first verify the real Freedraw schema.
