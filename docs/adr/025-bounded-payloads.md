# 025: Bounded frames, referenced state and byte-exact strings

Status: accepted

Date: 2026-10-18

Source: [Accepted M4 proposal](../../internal/proto/M4%20proposal%20proto.md).

## Context

Core operations, transformed output and full heads can exceed a WebSocket
frame. A nested annotation JSON object would lose numeric spelling and escapes
when a JavaScript client forwards it.

## Decision

Bound client frames to 1 MiB, server frames to 2 MiB, with 16 KiB envelope
reserve and 512 KiB advisory split/inline threshold. Bound raw payloads separately
and validate configuration relationships. Inline measures encoded payload fields,
including quoting, then preserves metadata/len/hash when substituting omitted.
Also omit transformed text exceeding the receiver's component/unit budget even
when its encoded bytes fit. Decoded annotation normalization must remain within
the complete frame budget, so accepted values can be re-encoded.
Page owns encoded entries and stops at either byte or entry limits; records are
never split. Encoding verifies the final frame size.

Annotation ops and inline sidecars are JSON strings containing their exact bytes.
Text ops remain arrays. Inline sidecars stay unclassified until the engine runs
the same classification path used for blobs. Full creates/replaces can reference
uploaded blobs. Fetch heads only through authenticated HTTPS state endpoints,
with Stele-Version/Class/Len/Hash headers. Advertise shared classification, guard,
rate and split policy in welcome. A cursor ahead of head requires full resync.

Raw JSON string content can expand sixfold when encoded; envelope reserve alone
does not guarantee fit. Clients must also measure the final frame and split or
upload it. This clarifies the proposal's maximum-payload test premise.

## Consequences

Large edits may become several transactions; omitted content costs an HTTPS
fetch and a local rebase. Adopt a fetched version only when it covers the member.
M6/M8 supply blob/state I/O; M13 enforces splitting and adoption. No state endpoint
or runtime session is claimed by the M4 codec implementation.
