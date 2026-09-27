# 002: ot.js encoding and UTF-16 positions

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Go and JavaScript must apply identical operations at identical text positions.

## Decision

Use ot.js JSON operation arrays and UTF-16 code units. Reject CodeMirror ChangeSet JSON and code-point indexing.

## Consequences

The wire format is compact, documented, and has an independent oracle. Editor positions require no per-keystroke conversion; Go must count UTF-16 units and reject split surrogate pairs.

## M1 codec and API clarification (2026-09-27)

Oracle equality compares exact ordered component kinds, integer counts and
insertion text, plus exact UTF-8 document bytes. Go and JavaScript may spell JSON
escapes differently (`<>&`, U+2028 and U+2029); this is not an operation difference.
Neither side normalizes Unicode, BOMs or line endings. Generated fixture files
themselves must regenerate byte for byte.

`Builder.Op()` returns `(Op, error)` so invalid counts, Unicode and policy limits
cannot silently produce a value. Its first error is sticky. `NewBuilder(Limits)`,
configured `Limits` methods, and `ValidateAgainst(Doc, Op)` extend the M1 sketch.
Zero Builder/Doc/Op values are usable. Default wrapper functions use DefaultLimits.

`MaxUnits` counts UTF-16 units; `MaxDocBytes` and `MaxJSONBytes` count bytes.
Algebraic operations can exceed a network frame, and Go HTML escaping can expand
an accepted JSON input past 1 MiB. Round-trip tests therefore supply a codec budget
at least as large as the serialized operation. Default Parse's ingress limit is
tested independently and is not relaxed in production.

Malformed JSON and non-array roots return ErrInvalidJSON. Unsupported component
types, exponent/fractional counts and integer tokens outside int64 return
ErrInvalidCount. Representable counts outside configured policy, including
MinInt64 under default policy, return ErrLimit before negation. Zero components,
adjacent equal kinds and delete-before-insert return ErrNonCanonical.

NUL remains valid generic text here. The Class A classifier/engine must reject
it before persistence. The engine must validate the actual base before transform
and the actual head afterwards: transforming `[1,-1]` against `[-2]` cannot reveal
that the original base was one emoji and the incoming operation split it.
