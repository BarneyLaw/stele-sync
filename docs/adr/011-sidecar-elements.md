# 011: Element-level sidecar operations

Status: accepted

Date: 2026-09-20

Amended: 2026-10-04, as explicitly requested by the supplied M2 proposal.

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

A pen stroke is atomic, and text OT applied to JSON can corrupt annotation structure.

## Decision

Use `(kind, id)`-keyed element last-writer-wins in server order across FreeDraw's
five top-level collections. Ordinary deletes tombstone the key and suppress later
puts. Reject text OT on sidecar JSON. Preserve imported array order and assign
new order keys from the server version and effective-operation ordinal.

Merge `pdfPageState` and `pdfPageTemplates` per page with ordinary put/delete
semantics, so unrelated page changes survive and hide/unhide/hide works. Diff
masks `updatedAt` and `sourcePdf.ctime` / `sourcePdf.mtime`; PDF identity changes
still sync. Preserve compact raw JSON values, including unknown fields, key
order, number spelling and escapes. Emit known top-level keys in profile order,
unknown keys sorted, and two-space indentation without an HTML-escaping pass.

Use `Marshal` for the FreeDraw file and `EncodeState` for server/client state.
The latter retains creation order and tombstones. Hash exactly `Marshal` bytes
for convergence. Persist Class B state and operation bytes as `bytea` or text,
not JSONB: re-encoding unknown JSON values would change the preservation contract.

[ADR 015](015-annot-page-quarantine.md) governs added pages and trusted replaces.

## Consequences

Unknown fields survive within this core; FreeDraw itself can discard unknown
fields on load/save. Tombstones remain through operation retention. The engine
must reject Class B submissions outside that window before pruning them.

Whole-element LWW accepts stale overwrites, duplicate ink when segment erase
races a move, and normalization bursts when FreeDraw rewrites scales or zIndex.
These can lose editing intent; they are not stronger per-field conflict merges.
Disjoint existing keys commute, while newly created elements still follow server
order. All replicas converge by replaying the same effective ops and versions.

The [M2 record](../annot-development.md) documents source and actual-plugin
evidence, tests, limits and the effective-ordinal correction needed for replay.
