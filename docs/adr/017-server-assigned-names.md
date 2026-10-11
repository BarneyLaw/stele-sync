# 017: Server-assigned names and namespace ownership

Status: accepted

Date: 2026-10-11

## Context

The specification assigned conflict names to both the client and server. Clients
cannot reserve portable names while disconnected, and a free PDF name does not
imply its sidecar name is free. Canvas ownership must survive case-only spelling.

## Decision

The server assigns every name the user did not type. Create/rename carry
`on_taken` (default `number`, otherwise `reject`); conflict creates carry
`copy_of`. Acks and changes include the final path and class. The engine calls
`FreeName` with a key lookup inside its locked transaction and retains the unique
index as the final guard. Conflicts always have a marker, including deleted-file
recovery. The shell supplies an already-localized display timestamp.

PDF renames with a live sidecar are atomic groups. Check ownership, liveness,
name categories and both target holders before committing any member. Numbered
PDFs reserve 11 component bytes for `.annot.json`. GroupTaken checks both names.

Devices own A/B everywhere and user-owned C outside CanvasRoot. The bridge owns
only C inside CanvasRoot. Admin is the system authority. Compare CanvasRoot by
path key, including case variants. Every Decide function receives validated
configuration, including the non-rename functions, so custom roots work.

## Consequences

Clients wait for the ack to learn generated names. Exhaustion rejects instead of
spinning. Folder operations remain per file. A Canvas PDF and a user sidecar
cannot both be renamed under Bridge authority; the future engine must coordinate
such a group under explicit system authority, not grant Bridge user-file access.
The pure core neither reserves names nor commits transactions itself.
