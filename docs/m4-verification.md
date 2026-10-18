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


Implementation uses presence-aware raw object readers with explicit per-variant
field tables. This provides the proposal's required/null and case-sensitive
semantics without duplicating pointer structs. jsontext validates the full frame
before any correlation is trusted. Encoded annotation normalization must also fit
the frame budget. Default optional client fields are omitted on encoding.

Transformed text can exceed the ingress component limit while fitting the byte
threshold. Inline omits that payload too; otherwise the receiving parser would
reject a server-produced change. Explicit annotation MarshalOps/MarshalCommitted
methods apply the configured ingress/state budgets, rather than default budgets.

## Automated evidence

Final race coverage is **95.8% in proto** and **97.5% in ids**, both above 90%.
The full repository race suite and 18 Node harness/workflow tests passed with
external integration settings cleared.
The initial sandbox run failed only because existing HTTP tests could not bind
localhost; rerunning with loopback permission passed. Repository lint, vet,
go mod tidy, actionlint, dependency-boundary probes and workflow tests passed.
The workflow tests and fixture tracker likewise need subprocess permission.
`make fixtures` regenerated all fixtures without a schema diff; the independent
fixture drift check also passed. CI discovery includes both ids/proto coverage,
all four new core/protocol property families and all three M4 fuzz targets.
Go and npm vulnerability audits found no vulnerabilities.

Initial ten-minute race fuzz runs (two workers each):

| Target | Executions | Result |
| --- | ---: | --- |
| FuzzParseIDs | 775,365 | Passed |
| FuzzDecodeClient | 1,011,669 | Passed |
| FuzzDecodeServer | 609,809 | Passed |

Go properties passed at 100,000 cases each for ids, committed annotation replay,
client round trips, server round trips and error-tier soundness. The final race property runs took 15.8 s (ids), 140.6 s (three proto
properties) and 266.5 s (committed annotation replay). All passed. The final
ten-minute race fuzz reruns after the decoder boundary fixes passed with
1,172,647 client-decoder executions and 811,694 server-decoder executions.
No fuzz failure corpus was produced.

The TypeScript suite validates generated Go client/server frames and handwritten
invalid server frames, plus every present server member replaced with null,
Unicode surrogates, integer limits, UUIDv7 vectors, annotation byte preservation,
trusted replacements and frame expansion. Full plugin tests (242 tests in ten files), strict core type checking, lint and
production build pass. Go catalogue tests discover sealed message/body implementers and ensure
fixture coverage, and enumerate rejection/query constants against the catalogue.

Regression tests cover canonical-frame growth near the exact byte limit,
transformed component overflow, maximum ordinary text/annotation payloads,
entry/byte paging boundaries, immutable page storage, mixed content shapes,
zero/missing/null IDs and counters, duplicate names, invalid Unicode, strict
client nesting and lenient server extensions, and redaction through fmt/slog/JSON.

## Reproduction

`make proto-acceptance` runs properties under race, >90% coverage gates,
10-minute fuzz targets, both-language contracts and fixture drift checks.
`make fixtures` regenerates only generated files; handwritten invalid fixtures
are preserved. `make lint`, `make test-unit` and `make plugin` cover the repository
and phase 1. All protocol tests remain pure and require no live service.

## Runtime work remains in later milestones

M4 supplies codecs and value helpers. It does not implement transport sessions,
the state HTTP endpoint, database tables, persisted client in-flight state,
real-device behavior or the M14 load generator. ADRs 023–027 and the updated
architecture/implementation guide specify those integration requirements.
The M1 local performance rerun is recorded separately in textop-verification.md;
it is not an end-to-end S16 throughput claim.


Maximum-payload allocation canaries measured 11,395,880 bytes allocated for a
1,032,351-byte text frame and 35,236,784 bytes for a 1,032,132-byte annotation frame.
These are cumulative allocations across core validation and canonical encoding,
not peak resident memory. Under race the annotation sample allocated 38,342,360
bytes. Both remain below the default 64 MiB annotation state
budget used by the representative allocation regression. Oversized and deeply
nested frames separately allocate at most three objects before rejection.

Local Compose services were not started. The Docker CLI in this environment
does not have the Compose plugin; live Postgres/Garage integration remains for
the storage/blob milestones. The development defaults were reviewed statically.

The follow-up CI review found and corrected a stale `obsync` user/database in
the integration task's Postgres readiness query after Compose switched to `stele`.
A configuration-consistency regression test failed on that mismatch and passed
after the fix; it checks the probe and client URL against Compose's identity.
This static check does not replace the live integration gate.
