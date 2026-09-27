# M1 textop review and test plan

Reviewed 2026-09-27 against the uploaded Go files, generate.mjs, package files,
and the three project specifications dated 2026-09-20.

This deliverable is an implementation-ready test design, not a completed test
suite. The uploaded files have not been changed. Node's syntax check passed for
generate.mjs on Node 24.19.0. Go is unavailable here and ot is not installed, so
neither the Go package nor the ot.js differential suite was executed.

## 1. Review findings, in priority order

### F1. Blocking: validEscapes skips backslashes rather than ordinary bytes

In json.go, line 110, the condition is `raw[i] == '\\'`. It must be `!=` for
the surrounding algorithm: skip ordinary bytes; inspect escapes at backslashes.

Source tracing yields these regressions:

| Input to Parse | Required result | Current source behavior |
| --- | --- | --- |
| `["a"]` | Valid insertion of a | ErrInvalidJSON |
| `["ab"]` | Valid insertion of ab | Accepted, showing content-dependent behavior |
| `["\ud800x"]` | ErrInvalidUnicode | Scan succeeds; Go decoding repairs to U+FFFD followed by x |
| `["\ud83d\ude00"]` | Valid insertion of emoji | ErrInvalidJSON |

These are static traces, not claims of executed Go tests. Start with regression
tests through the exported Parse function, then change the predicate. Add both
valid and invalid escaped/raw Unicode; testing only that invalid input is
rejected would miss the incorrect error class and widespread valid-input failures.

Go's legacy encoding/json behavior substitutes U+FFFD for malformed surrogate
escapes, so permitting the escape through loses content fidelity.

### F2. Missing explicit top-level array validation

json.go, lines 31-34, obtains the first token but checks only err. Restore the
explicit `tok != json.Delim('[')` rejection. I am not claiming that this omission
alone currently makes arbitrary non-array JSON succeed: later decoder checks
can still reject it. It removes the intended boundary guard and can alter error
classification. Require ErrInvalidJSON for objects, strings, numbers, booleans,
null and multiple top-level values.

### F3. The oracle's negative apply generator has an invalid premise

generate.mjs, lines 158-161, makes a document with more code points and labels its
operation `length_mismatch` without running the purported failing operation.
More code points need not mean more UTF-16 units: emoji is one code point and
two units; ab is two code points and also two units.

An isolated execution of the supplied PRNG/text-generation functions produced
`i中🔥` versus `w語\r\n`: respectively 3 and 4 code points, but both 4 UTF-16 units.
This proves the generator assumption false; it is not a report of a particular
full oracle fixture run or a Go differential failure.

Proposed repair: build `other = [...docCps, 'x']`. Then build the operation over
other; assert its baseLength is doc.length + 1 and independently require that
ot.js apply(doc) throws. Only then emit the error expectation. Do not use a
generic catch to classify all oracle exceptions as length mismatch.

### F4. Streaming ignores backpressure

generate.mjs, lines 247 and 252, discards process.stdout.write's boolean result.
At ten million records, a fast producer can accumulate pending output while Go
is slower. Make main async; await drain when write returns false and propagate
stream errors. Cancel/terminate the child when the consumer fails. Do not retain
the entire stream on either side.

### F5. Required large-case coverage is probabilistic and unmeasured

docMax is a code-point maximum, not an exact UTF-8 byte size. The current scale
profile neither guarantees a 1 MiB document nor proves a 500-component operation
was exercised. Build mandatory cases at measured sizes and component counts,
in addition to the random mixture. Report actual coverage counts. The comment
that 10M cases takes a few hours is an estimate with no measurement supplied.

### F6. Missing harnesses and negative fixture contract in the supplied set

No *_test.go, fuzz targets, benchmarks, golden fixture consumers, scale consumer
or invalid.json was attached. They may exist elsewhere in the repo; this review
does not infer their absence there. README(1).md still describes the M0 placeholder.
The supplied generator produces only the three positive/mixed oracle files.

### F7. Two specification mismatches need explicit decisions

**JSON bytes versus operation equality.** Go MarshalJSON uses json.Marshal for
insertions; JS uses JSON.stringify. HTML-sensitive text and U+2028/U+2029 can be
escaped differently even though components are identical. The current random
pool largely misses this. Prefer exact typed-component equality and exact UTF-8
document bytes. If the spec literally demands identical JSON bytes, define a
shared serializer and verify that separately. Do not silently reinterpret the
exit criterion in the harness. Regenerated fixture files themselves remain
byte-for-byte deterministic regardless of this decision.

**Round-trip versus parser budget.** Builder/Compose/Transform can produce ops
larger than MaxJSONBytes. Even a small accepted op can expand when serialized:
180,000 '<' characters occupy 180,004 raw JSON bytes, but Go's escaped output
occupies 1,080,004 bytes, over the default 1 MiB parser budget. This applies after
F1 is fixed. Define algebraic round-trip with an explicit adequate codec budget,
and separately check strict ingress limits. The unqualified default-Parse
round-trip property in M1 cannot hold for all constructed operations as written.

### F8. Documentation and integration obligations

- doc.go still says "will implement" and omits the required invariants/case-table
  rationale. Several exported methods lack comments.
- Builder.Op returning an error, configurable Limits, and ValidateAgainst differ
  from/extend the original API. Record these accepted API changes in the project
  documentation; do not leave future clients following the old signature.
- MaxUnits is UTF-16 units, not MB; MaxDocBytes and MaxJSONBytes are bytes/MiB.
- Raw NUL remains accepted by this generic text package. That is defensible if
  the Class A classifier/engine rejects it before persistence, as architecture
  requires. Record the owner; do not imply M1 enforces that policy today.
- Transform cannot validate boundaries in unseen original text. The engine
  must validate at the actual base and again at head; include the masked-invalid
  example below in handoff documentation.
- No cursor-loop logic defect was established by this review. That is not proof
  of correctness. The loops still need independent expected results and properties.

## 2. What integration means for M1

M1's dependency graph is a pure text library. Exercise its components together:

1. Raw JSON -> Parse -> ValidateAgainst -> Apply -> exact document bytes.
2. Raw sequential ops -> Parse -> Compose -> Apply -> exact final text.
3. Raw concurrent ops -> Parse -> Transform -> both application paths -> exact text.
4. Builder -> immutable Op -> MarshalJSON -> Parse -> same components/effect.
5. Pinned Node/ot.js generator -> serialized expected cases -> Go public API.

Use external package textop_test for these tests. A small same-package test is
reasonable only if needed for a private invariant unreachable through the API.
Do not fabricate private invalid Op structs and call resulting panics protocol
bugs; clients cannot construct those states through Parse/Builder.

No database, Garage, WebSocket, actor, or full client state-machine fixture is
needed. Storage integration belongs to M5/M6, protocol simulation to M7, and
editor integration to M13. A short in-memory history exercise can verify M1's
algebra, but must not be counted as M7 delivery/convergence evidence.

Suggested files, grouped by purpose:

| File | Owns |
| --- | --- |
| textop_test.go | Deterministic API pipelines, regression cases, limits, immutability, case tables |
| properties_test.go | Four required rapid properties and independent edit generator/model |
| oracle_test.go | Checked-in fixtures, strict fixture schema validation, differential comparisons |
| oracle_scale_test.go | Dedicated opt-in subprocess/NDJSON scale consumer |
| fuzz_test.go | Parser, raw insertion escapes and Apply fuzz targets |
| benchmark_test.go | Apply sizes, real 5,000-op history, allocation measurements |

Keep common fixture comparison helpers small. A table-test helper that fails
via testing.T is not suitable inside rapid if it prevents shrinking; rapid
properties must report failures through their rapid.T.

## 3. Concrete regression and integration cases

All operations below are raw JSON arrays unless stated otherwise. Compare errors
with errors.Is, never exact error strings.

### 3.1 Parser and Unicode contract

| Case | Input | Expected |
| --- | --- | --- |
| ASCII single character | `["a"]` | Success; apply to empty -> a |
| Empty document identity | `[]` on empty | Success; empty output |
| Nonempty identity | `[3]` on abc | abc |
| Raw supplementary scalar | `["😀"]` | TargetLen 2; exact emoji bytes |
| Escaped supplementary scalar | `["\ud83d\ude00"]` | Same content as raw emoji |
| Escaped high surrogate plus suffix | `["\ud800x"]` | ErrInvalidUnicode |
| Lone high / low | `["\ud800"]`, `["\udc00"]` | ErrInvalidUnicode |
| Reversed pair | `["\udc00\ud800"]` | ErrInvalidUnicode |
| Literal slash-u text | `["\\ud800"]` | Success; six literal characters |
| Real replacement character | `["�"]` | Success; preserve it |
| Unknown escape | `["\q"]` | ErrInvalidJSON |
| Truncated JSON string | unterminated raw string | ErrInvalidJSON |
| Invalid UTF-8 | raw byte sequence containing ff | ErrInvalidUTF8 |
| Wrong root / trailing values | `{}`, `null`, `1`, `"x"`, `[] []` | ErrInvalidJSON |
| Noncanonical empty component | `[0]`, `[-0]`, `[""]` | ErrNonCanonical |
| Adjacent components | `[1,2]`, `[-1,-2]`, `["a","b"]` | ErrNonCanonical |
| Wrong replacement order | `[-1,"x"]` | ErrNonCanonical |
| Unsupported count spelling | `[1.0]`, `[1e0]` | ErrInvalidCount |
| Out-of-policy signed count | `[-9223372036854775808]` | ErrLimit with current policy |
| Integer outside int64 | `[9223372036854775808]` | ErrInvalidCount with current parser |

Resolve the last two error categories deliberately in the contract; do not infer
that every large numeric token must get the same error. Attach distinct malformed
JSON, invalid type, invalid count, and policy-limit examples.

Round-trip exact content including CRLF, LF, lone CR, BOM, combining sequences,
precomposed/decomposed accents, quotes, backslashes, '<>&', U+2028 and U+2029.
Protect surrogate pairs, not whole grapheme clusters. Splitting e and its
combining accent is permitted by the UTF-16/scalar-boundary contract.

### 3.2 Apply and validation

| Base | Op | Expected |
| --- | --- | --- |
| abcd | `[1,"XY",-2,1]` | aXYd |
| A😀B | `[1,-2,1]` | AB |
| A😀B | `[2,"X",2]` | ErrSplitsSurrogate |
| A😀B | `[1,-1,2]` | ErrSplitsSurrogate |
| ab | `[1]` | ErrLengthMismatch |
| empty | `[1]` | ErrLengthMismatch |

For valid inputs, ValidateAgainst and Apply must agree on acceptance. On success,
Doc.String must be valid UTF-8 and preserve exact bytes; Doc.Len must equal
TargetLen. Confirm neither operation nor original document changes on success
or error. A missing trailing retain is a length error, not implicit identity.

### 3.3 Compose and Transform pipelines

| Operation | Base / inputs | Required result |
| --- | --- | --- |
| Compose | ab; a=`[1,"XY",1]`, b=`[2,-1,1]` | `[1,"X",1]`; aXb |
| Compose cancellation | empty; a=`["😀"]`, b=`[-2]` | `[]`; empty |
| Compose invalid split | empty; a=`["😀"]`, b=`[1,-1]` | ErrSplitsSurrogate |
| Transform tie | ab; a=`[1,"X",1]`, b=`[1,"Y",1]` | ap=`[1,"X",2]`, bp=`[2,"Y",1]`; aXYb |
| Transform overlap | abcd; a=`[1,-2,1]`, b=`[2,-2]` | ap=bp=`[1,-1]`; a |
| Insert inside deletion | abcd; a=`[1,-2,1]`, b=`[2,"X",2]` | ap=`[1,-1,1,-1,1]`, bp=`[1,"X",1]`; aXd |

Cover both argument orders for asymmetric cases, empty/nonempty identities,
whole-document deletion, insert at zero/end, replacement, disjoint deletions,
nested deletions, adjacent nonoverlapping deletion ranges, equal deletions and
multiple edits in one operation. Assert transformed lengths and canonicality
in addition to final text. Different ops can have the same effect on repetitive
text, so final-text equality alone is not enough.

TestCaseTable is the systematic version: enumerate all 9 component pairings for
each algorithm, with shorter/equal/longer residual spans where pairing occurs.
Give independent-priority rows each length relation too, verifying that the
chosen insertion/deletion is consumed whole. Include trailing insert/delete
draining and verify that end conditions terminate. Use manually derived expected
operations; do not generate the expected table with the implementation under test.

### 3.4 Immutability and bounds

- Finalize a builder; continue building a longer operation. The first op's bytes,
  lengths and behavior must stay unchanged, including insertion buffer capacity.
- Snapshot operands before Compose/Transform/Apply and compare afterwards.
- Run read-only operations concurrently on the same immutable values under -race.
  Do not concurrently mutate a Builder, which is explicitly unsupported.
- Check zero Doc, Op and Builder values. Check sticky errors after negative counts,
  invalid UTF-8, exceeded components and exceeded lengths.
- For each Limits field, exercise below/exactly/above with all other limits loose.
  Verify zero/negative limits are rejected by every configured entry point.
- Verify default wrappers and methods called with DefaultLimits agree.
- Separate MaxUnits from UTF-8 MaxDocBytes using CJK and supplementary characters.
- Use tiny configured limits for arithmetic/limit properties; no huge allocation
  is needed to exercise numeric overflow rejection in Parse.
- Include legal inputs whose merged Transform output exceeds MaxUnits/components.
  Expect ErrLimit and unchanged operands; never skip such failures silently.
- Compose/Transform do not know unchanged document bytes. Check MaxDocBytes at
  ValidateAgainst/Apply, not by inventing unavailable base content inside algebra.

The masked-invalid example is a HANDOFF test/documentation case: base emoji,
incoming `[1,-1]`, history `[-2]`. Base validation must reject incoming. Do not
incorrectly require Transform alone to reject it; it has no base document.

## 4. Property-based testing

Use rapid, pinned as a test-only dependency. Check the repo's depguard rules
permit it in *_test.go without allowing production imports. Do not copy the
Node generator into Go and call that an independent model.

Generate Unicode scalar sequences and a sequence of logical retain/insert/delete
actions. Construct both (a) a plain independent rune-slice reference edit result
and (b) UTF-16 operation counts for the Go API. Count UTF-16 using independent
stdlib logic, not textop.unitsIn. This lets the model catch a broken Builder or
Apply rather than use either one as the source of truth.

Concurrent operations share the same reference base. For Compose, generate b
against the reference result of a, not an unchecked result from the SUT. Valid
generators should make forward progress by construction and bound insertions
per source position. Shrinking must preserve valid context and boundaries.

| Required property | Assertions |
| --- | --- |
| TestPropConvergence | Both TP1 paths succeed and have identical UTF-8 bytes; outputs canonical; inputs unchanged |
| TestPropComposeApply | Combined and sequential application equal the independent reference result |
| TestPropLengths | ap.base=b.target; bp.base=a.target; ap.target=bp.target; compose.base=a.base; compose.target=b.target; successful Apply length matches target |
| TestPropCanonical | Builder, Compose and Transform outputs satisfy an independent component checker; round-trip under the agreed codec budget; no mutation |

For algebra properties, choose bounded small docs and a policy large enough for
all generated intermediates. Unexpected ErrLimit is a test failure. Use separate
limit properties with deliberately tight budgets. Avoid a convergence test that
skips every failing operation or every emoji case and reports a million successes.

Require 1,000,000 generated cases PER required property for M1 exit, not a million
shared across four properties. PR budgets may be smaller (proposed 10,000 per
property) but are not exit evidence. Save seeds and minimized counterexamples.

## 5. Oracle contract and generator changes

Keep ot 0.0.15 pinned in both package.json and package-lock.json; use npm ci.
Pin the Node toolchain in CI too. The package must remain outside runtime imports.

Changes before trusting generated fixtures:

1. Repair and self-check mismatched-length cases (F3).
2. Require a-first in detectTieBreak. Returning b-first does NOT currently fail,
   despite the comment saying it will. Detect first, compare to the contract,
   then abort generation if it differs; do not silently change Go's convention.
3. Respect stdout backpressure and reject invalid CLI inputs: kind membership,
   seed integer in 0..2^32-1, positive integer count with a configured ceiling,
   valid profile, and safe option combinations. Seed coercion currently masks
   invalid/fractional values and count=NaN can emit no cases.
4. Add composition self-check: composed.apply(doc) equals b.apply(a.apply(doc)).
5. Inspect operation boundaries against their actual documents, not merely
   well-formedness of string fields. A string-valid object can still contain a
   retain/delete that bisects a surrogate. Outputs should be checked too.
6. Bound steps, insertions and components in randomOp; the insertion-only branch
   currently permits arbitrarily many iterations before input advances.
7. Add explicit Unicode escaping/control/HTML/BOM categories and deterministic
   large/complex cases. Measure sizes and component counts after canonicalization.
8. Add metadata: schema version, generator revision/hash, ot version, tie policy,
   profile, seed, count, and case IDs. Record per-kind and mandatory-category totals.
9. Write fixtures to a temporary directory, validate all, then install generated
   outputs. A failure should not leave a partially replaced fixture set.

### Positive fixture schemas

Retain the existing header/cases wrapper but add an explicit schema_version and
kind. Example payloads:

```json
{"id":"apply-ascii","doc":"ab","op":[1,"X",1],"want":"aXb"}
{"id":"compose-cancel","doc":"ab","a":[1,"XY",1],"b":[2,-1,1],"want":[1,"X",1]}
{"id":"transform-tie","doc":"ab","a":[1,"X",1],"b":[1,"Y",1],"want_ap":[1,"X",2],"want_bp":[2,"Y",1]}
```

Exactly one of want/error is present in an apply case. Decode with field-presence
tracking or pointers, not empty-string-as-missing: empty output is a valid result.
Reject null payloads, unknown required schema versions, duplicate/ambiguous
fields, empty case lists, missing mandatory fixtures, count mismatches, trailing
JSON, wrong ot version and wrong tie policy. Never quietly skip a missing fixture.

### Invalid fixture schema

Negative validation expectations come from YOUR contract. ot.js is not the
authority for canonical rejection, UTF-8 validity, surrogate rejection or limits.
Mark invalid.json as hand-written, as the specification already permits.

```json
{
  "header": {"schema_version":1,"kind":"invalid","hand_written":true},
  "cases": [
    {"id":"lone-high-suffix","stage":"parse","raw":"[\"\\ud800x\"]","error":"invalid_unicode"},
    {"id":"delete-half-emoji","stage":"apply","doc":"😀","raw":"[1,-1]","error":"splits_surrogate"},
    {"id":"invalid-utf8","stage":"parse","raw_base64":"WyL/Il0=","error":"invalid_utf8"}
  ]
}
```

The raw field is JSON text held INSIDE an outer JSON string. Decode the outer
fixture once, then pass the untouched raw bytes to textop.Parse. Do not decode
the embedded operation into a Go string slice first, which would repair the
very escape being tested. raw_base64 handles byte sequences invalid in UTF-8;
require exactly one raw representation and fail on invalid base64.

Add stage=doc, compose, transform and explicit limits fields as needed. Every
stage has a narrow allowed-field schema and a named expected error. Keep a fixed
error-code -> sentinel map; reject unknown error labels rather than guessing.

### Comparing results independently

- Apply: exact UTF-8 bytes and UTF-16 target length.
- Compose: exact ordered component kinds, counts and insertion text against want;
  then compare sequential and composed application on doc.
- Transform: exact components of BOTH ap and bp, lengths, and both application paths.
- Do not compare only final text. Do not normalize components before comparing;
  otherwise extra retains or noncanonical components can disappear from evidence.
- Decode expected operations using a small independent typed JSON component
  reader, not the SUT's Parse or Builder. It must reject unsupported types and
  use integer parsing rather than float64 conversion.
- Compare strings without NFC, line-ending, or BOM normalization.
- Literal JSON-byte equality requires the shared serializer decision in F7.

Fixture drift: regenerate the three generated files twice into temporary output
directories and compare bytes, then compare with committed paths. Leave the
hand-written invalid.json alone. Explicitly detect untracked/missing expected
files; a bare git diff alone can miss newly created untracked fixtures.

## 6. Scale consumer and runtime integration

TestOracleScale should be a dedicated tagged test/job, using the same comparison
helper as golden tests. Ordinary go test must not launch ten million cases or
require Node. A requested scale run must fail if Node, ot, or the generator is
missing; it must not skip and turn the job green.

Proposed algorithm:

```text
resolve repo root independently of current working directory
construct context with explicit scale-run deadline
spawn Node once with pinned generator, --stream, --count, --seed, --profile
read and strictly validate exactly one header
for each bounded NDJSON record:
    validate kind, index, required fields, and record byte ceiling
    execute the Go oracle comparator
    update counts and measured coverage categories
    discard the case after checking it
on failure:
    preserve header + complete failing case + mismatch diagnostics
    cancel child, drain/close as appropriate, wait/reap, fail
at EOF:
    require exact requested count, contiguous indices, expected kind counts
    require mandatory large/complex categories were exercised
    require child exit status zero and successful stderr handling
```

Drain stderr concurrently into a bounded tail buffer so the child cannot block;
report it on failure. Propagate read errors and child failures, even if all
records seen so far pass. Never buffer all stdout using cmd.Output/CombinedOutput.

A default bufio.Scanner has a roughly 64 KiB token ceiling. Configure an explicit
record bound large enough for doc plus all input/output ops and JSON escaping,
and assert the producer respects the same bound. A proposed 64 MiB hard record
ceiling is a harness ceiling, NOT a permitted WebSocket frame size. Prefer much
smaller actual records; report the largest observed record. Test the harness's
truncated stream, bad header, skipped index, malformed record, wrong count,
oversized record, nonzero exit, timeout and missing dependency failure paths.

The 10,000,000 count is total generated cases, consistent with the supplied
--kind all cycling; report the count for each kind so it is not misread as 10M
per kind. Explicit proposed mandatory quotas per full run:

- At least 100 cases per kind with a base doc of EXACTLY 1 MiB UTF-8 bytes.
- At least 100 cases per kind containing an input op with >=500 canonical
  components; for compose/transform also report maxima for outputs.
- Include all mandatory tie, overlap, identity, cancellation and Unicode categories.

These quotas are a proposed stronger coverage policy; M1 supplies the sizes and
total count but does not specify the quotas. Allocate them WITHIN the reported
total or report them as an additional suite, never double-count them.

For replay, preserve the case itself plus seed, index, kind, profile, generator
hash and ot version. Seed/index alone can require replaying millions of prefix
cases and will change when generator code changes. Add a replay-one-case route
using the existing comparison helper. Do not invent an independent seed per case
unless you intentionally version the fixture-generation algorithm.

## 7. Fuzzing, performance and CI gates

### Fuzz targets

- FuzzParse(raw): no panic. Accepted input serializes to an independently valid,
  canonical operation and round-trips under the agreed output codec budget.
- FuzzInsertEscapes(raw): through exported Parse with a focused corpus of raw
  JSON string escapes. Valid literals are preserved; malformed surrogate escapes
  never become silent U+FFFD substitution. Include the current regression cases.
- FuzzApply(docBytes, raw): invalid UTF-8/ops yield errors; successful parses and
  document creation feed Apply. On success require valid Unicode, TargetLen and
  an independent reference effect. Domain errors such as length mismatch are
  expected for arbitrary input, not a reason to suppress panics.

Add structured Compose/Transform fuzzing later if it closes a concrete gap;
rapid already supplies the valid paired-operation space. Go fuzzing saves
minimized failure inputs for replay; commit relevant corpus entries.

### Benchmarks

- BenchmarkApply: preserve 50 KB and 5 MB baselines; record exact byte counts to
  remove KB/MiB ambiguity. Construct docs/ops outside timed regions.
- BenchmarkApplyLarge: exactly 8 MiB UTF-8 document, identity plus realistic
  bounded replacements. Ensure targets stay within document limits. Validate
  correctness outside timing. Report ns/op, B/op and allocs/op.
- BenchmarkTransformGap: 5,000 VALID sequential historical ops where each op's
  base is the previous head. Rebase one concurrent submission through them in
  order. Use short/many-component and Unicode cases, and verify final result
  against precomputed independent expectations outside timing. Repeating one
  incompatible op 5,000 times is not a realistic gap benchmark.
- Standard Go ns/op is an aggregate, not p99. For S16 gap p99 <50 ms, collect
  individual whole-gap timings on a documented runner (proposed 1,000 samples,
  bounded warmup), then sort/report p50/p95/p99. Include CPU, Go version, build
  flags, GOMAXPROCS and workload seed. Use a dedicated performance gate without
  the race detector, plus a separate correctness/race gate. Do not disguise
  a normal benchmark mean as the required percentile.

| Run | Required/proposed budget |
| --- | --- |
| PR | All deterministic regressions/integration cases and 7,000 committed oracle cases; proposed 10,000 rapid cases/property; 60 s per fuzz target; -race; fixture drift |
| M1 acceptance | 1,000,000 rapid cases PER property; 10 min per fuzz target; all golden/invalid cases; >90% core line coverage; S16 benchmark evidence |
| Nightly | 10,000,000 streamed oracle cases including mandatory coverage; 1 h per fuzz target; replay/report artifacts |

The 7,000 golden count comes from the supplied plan: 2,000 apply + 2,000 compose
+ 3,000 transform. Mandatory explicit golden cases can be added with the new
total recorded. PR budgets are not substitutes for the M1 acceptance budget.
M14's complete 16-client/million-op S16 scenario and 10x-load run remain later
system evidence; only textop's performance slice is covered here.

Proposed commands AFTER the named tests/harness exist (repo root):

```sh
npm ci --prefix tools/oracle
node tools/oracle/generate.mjs --out schema/textop
go test -race -count=1 ./internal/core/textop -rapid.checks=10000
go test -race -count=1 -timeout=2h ./internal/core/textop -run '^TestProp' -rapid.checks=1000000
go test -race -count=1 ./internal/core/textop -coverprofile=coverage.out
go tool cover -func=coverage.out
go test -race ./internal/core/textop -run '^$' -fuzz '^FuzzParse$' -fuzztime=10m -timeout=15m
go test -race ./internal/core/textop -run '^$' -fuzz '^FuzzInsertEscapes$' -fuzztime=10m -timeout=15m
go test -race ./internal/core/textop -run '^$' -fuzz '^FuzzApply$' -fuzztime=10m -timeout=15m
go test ./internal/core/textop -run '^$' -bench 'Benchmark(Apply|TransformGap)' -benchmem -count=5
go test -race -tags=oracle_scale -count=1 -timeout=8h ./internal/core/textop -run '^TestOracleScale$'
```

The scale test must explicitly default/request 10M and validate that count.
The 8h timeout is an initial safety budget, not a measured completion-time claim.
Wire these into existing tools/tasks.mjs rather than introducing a second CI
orchestrator. Inspect the actual task runner before modifying it; it was not
among the uploads. Keep phase 1 fixture regeneration intact.

## 8. Implementation order and done criteria

1. Add failing parser/Unicode regression cases; fix F1 and restore F2 guard.
2. Resolve/document JSON equality and round-trip budget contracts.
3. Add independent edit model and the four rapid properties.
4. Add deterministic case tables, immutable-value tests and bound/error cases.
5. Repair generator assumptions, tie enforcement and stream handling.
6. Generate/review golden fixtures and hand-written invalid fixtures; add the
   strict Go fixture consumer. Demonstrate intentional corrupt fixtures fail.
7. Add scale subprocess consumer, quotas, replay and failure handling.
8. Add fuzz/benchmark gates and run the documented M1 acceptance budgets.

Done means evidence from these runs, not merely the presence of test functions.
Keep the case tables and rationale in package documentation and the appropriate
decision record. Preserve the distinction between M1 algebra correctness and
later milestones' delivery, durability and full client/server convergence.

## References

- Uploaded implementation guide, M1, and engineering guidelines sections 7-9.
- Go encoding/json, string replacement and escaping behavior:
  https://pkg.go.dev/encoding/json
- Go fuzzing and saved failure corpus:
  https://go.dev/doc/security/fuzz/
  https://go.dev/doc/tutorial/fuzz
- rapid properties, shrinking and -rapid.checks:
  https://pkg.go.dev/pgregory.net/rapid
- Node streams, write return value and drain:
  https://nodejs.org/api/stream.html
- Go bufio scanner record ceiling:
  https://pkg.go.dev/bufio

No tests, benchmark results, oracle comparisons, or million-case run successes
are claimed by this review. Executed checks were the Node syntax check, the
isolated text-generator counterexample, and JSON validity checks on this plan's
fixture-schema examples.
