# 015: Quarantine added pages behind whole-document replacement

Status: accepted

Date: 2026-10-04

## Context

The [supplied M2 proposal](../../internal/core/annot/M2-core-annot-proposal.md)
identifies two unsafe element-merge cases. Added pages use positional numbers
that shift on insertion; a stale stroke can target the wrong page. Trash moves
annotations into an archive and restores them with the same ids, so ordinary
tombstones would suppress a legitimate restore. Both behaviors were reproduced
with actual FreeDraw 0.13.3 on a synthetic PDF.

## Decision

When either appendedPages or removedPages is nonempty, Diff requests whole-
document replacement and ordinary Apply rejects the edit. Structural metadata
keys are forbidden in ordinary operations. The engine accepts AnnotReplace
only at the current head; stale replacements become separate conflict copies.

Only trusted server code constructs ReplaceOps after that base check. ParseOps
rejects every client replace field. Replacement reproduces the target element
set and array order, clears tombstones for restored keys, and tombstones removed
keys. Empty added-page and trash arrays permit a return to element mode.

## Alternatives

Accepting numeric page references risks silent misplacement. A structural barrier
still produces conflicts for open stale sessions. Transforming inferred page
intent adds substantial complexity. Anchoring annotations to stable page ids
would be the automatic merge solution, but requires PDF page-count/hash agreement
and a lift/lower codec. It is deferred, not approximated by element LWW.

## Consequences

Concurrent edits to quarantined documents conflict even when they affect real
PDF pages. M14's obsync_annot_whole_document_conflicts_total metric is the trigger
for building stable page anchors. M4/M7 must provide trusted replacement routing,
the base check and conflict copies; the pure core has no storage or transport
authority. Effective operations use post-filter ordinals so clients replay the
same order after tombstoned puts are dropped.
