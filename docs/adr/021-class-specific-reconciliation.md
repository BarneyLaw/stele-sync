# 021: Class-specific reconciliation bounds

Status: accepted; supersedes 009

Date: 2026-10-11

## Context

Text OT work/semantic drift and annotation tombstone safety have different
limits. Applying one time/op window to both rejects safe sidecar work without
proving that erased strokes cannot be resurrected.

## Decision

A requires retained operations and at most 5,000 operations and 24 hours since
the server commit time of the base. Equality is within the window; client offline
time is irrelevant. B requires retained tombstones; its op count and age do not
bound reconciliation. C has no reconciliation window: put_blob compares its base
to head. All negative/future bases are invalid; stale puts and AnnotReplace
preserve submitted bytes as a server-named conflict copy. A stale delete is refused.

## Consequences

M10 must derive tombstone pruning and Gap.TombstonesPruned from the same retention
cutoff. M7 must call Reconcile before applying stale A/B work, and DecideReplace
before trusted M2 replacement. This core does not prune history or supply the
server commit timestamp. ADR 015's structural quarantine remains mandatory.
