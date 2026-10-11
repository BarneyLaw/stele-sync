# 019: Complete-content classification and permanent demotion

Status: accepted

Date: 2026-10-11

## Context

An 8 KiB prefix misses later NULs and truncated UTF-8. M2 recognizes a bounded,
strict FreeDraw format; generic JSON parsing would accept inputs M2 rejects.
Transient partial writes must not irreversibly demote an existing file.

## Decision

Expect uses metadata; Confirm validates all committed bytes. The default text
extensions are md/txt/mdx, with lowercase ASCII code extensions configurable.
Text is at most 8 MiB, valid UTF-8 and NUL-free. BOMs and line endings are content.
A nonempty-stem `.pdf.annot.json` name (ASCII case-insensitive), at most 16 MiB,
is B only if annot.Parse accepts it with the same configured MaxBytes.

Non-pairing `.annot.json` archives remain C even when json is configured as text.
This closes a gap in the proposal's extension table and protects conflict/recovery
copies from accidental text merging. Other .json files may be configured as text.
The 512 MiB blob ceiling and exclusion policy remain outside classification.

The server determines the expected category from path and size, confirms the
committed bytes, and rejects a different client claim with class_mismatch. A
client claim alone must never be used as Confirm's expected argument.

Classes never change in place or upgrade. Content demotions wait 10 seconds of
quiet time, including metadata demotion (Expect already returned C with Kept).
After settling, new files submit as C; existing A/B files request a conflict copy
of local bytes, then restore the head at the original path. Existing C stays C.

## Consequences

The plugin's recognition-only parser preserves the M2 distinction between raw
JSON spelling and decoded keys. It checks duplicate keys/ids, surrogate pairs,
page numbers, depth/node/id/element budgets and formatted materialized size.
It does not implement M13 annotation merge/diff or FreeDraw session coordination.
The Go fixtures constrain compatibility; Go's independent tests remain the
correctness evidence. Quiet time remains provisional pending actual write-event
measurements, not established by these fixtures.
