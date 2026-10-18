# M4 protocol verification

2026-10-18. Baseline: main `21f30d8`, Linux/amd64, Go 1.27.2.

## Step 0: measured before decoder implementation

Reproduce with `go run ./tools/proto-probe`.

- jsontext defaults reject every tested lone surrogate (`\ud800`, `\udc00`,
  `\ud800A`, `\udc00\ud800`); a valid pair succeeds. No extra scanner is needed.
- v2 rejects integer `1.0`, `1e3`, `01`, and int64 overflow. It accepts `-0`,
  `9007199254740992` and `null` (zero). Protocol counters therefore require
  explicit presence/non-null checks and the JavaScript safe-integer bound.
- RejectUnknownMembers rejects unknown and case-variant names at both levels
  of a nested struct.
- jsontext ReadValue allocations: 1 MiB string: 2,097,500 B / 17 allocations;
  65,536 integers (131,073 bytes): 524,603 B / 16 allocations;
  10,000 nested objects (60,001 bytes): 2,945,421 B / 20,033 allocations.
  Deep nesting has much larger amplification than shallow values. Protocol
  framing will bound nesting before constructing wire values, including unknown
  server fields, so hostile tiny nested frames cannot incur this cost.

## Contract clarifications

The raw annotation/inline-content byte limit is distinct from encoded frame size.
JSON string quoting can expand a byte into six bytes. Both budgets must hold;
EnvelopeReserve alone cannot guarantee arbitrary raw payloads fit. Clients must
measure the encoded frame and split or upload content when needed. Tests cover
both ordinary payloads at their raw limit and escape-heavy frame overflow.
