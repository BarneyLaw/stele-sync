# 013: Defer remote FreeDraw sidecar writes during open sessions

Status: accepted

Date: 2026-10-04

## Context

FreeDraw PDF 0.13.3 keeps annotation state and the raw loaded sidecar baseline in
memory. Its store checks the disk bytes before saving and inside Vault.process.
An external write does not reload the active session. A conflicting local save
is rejected, remains dirty and is preserved in a recovery copy.

This was verified against pinned source and in installed FreeDraw 0.13.3 under
Obsidian 1.13.7 using a synthetic PDF, an external synthetic text addition, and
an independent local text addition. The external content remained intact and
the session generated a recovery sidecar. See [verification](../annot-verification.md).

## Decision

While a PDF is open in any leaf, apply remote Class B operations to its durable
shadow, defer the disk write, and indicate pending annotations to the user.
Diff local saves against the on-disk baseline that FreeDraw actually loaded,
not against the newer shadow. After all PDF leaves close and local saves settle,
write the accumulated server materialization. Preserve recovery copies as
separate files; never silently merge or delete them.

## Consequences

M13 must track PDF leaves, pending writes, the disk baseline and shadow
transactionally. The convergence audit uses the shadow during deferral. This
decision prevents the sync client from deliberately triggering FreeDraw's save
conflict path; it cannot prevent conflicts from other writers or another plugin.
Re-run the compatibility exercise on every change to the pinned FreeDraw version.
