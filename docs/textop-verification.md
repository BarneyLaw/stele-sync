# M1 textop verification

Current performance status (2026-10-18): **passed all four existing M1 local
latency gates**, with an unchanged 50 ms p99 limit. The slowest measured p99 was
20.874 ms for the 5,000-operation Unicode history. The
[Linux rerun](#2026-10-18-linux-rerun) records the command, environment, latency
percentiles and throughput measurements. The earlier Windows failure did not
reproduce; its cause remains unconfirmed. Full S16 system-load acceptance remains
pending M7/M14.

## 2026-09-27 Windows verification

Environment: Windows/amd64, Intel Core i7-9700K @ 3.60 GHz, Go 1.26.6,
Node 24.12.0, GOMAXPROCS=8, rapid 1.2.0, ot 0.0.15. These are local
observations, not hosted GitHub Actions results or a complete S16 system test.

### Correctness and harness checks

| Check executed | Result |
| --- | --- |
| `node tools/tasks.mjs test` | Passed repository race tests, 10,000 rapid cases per property under race, and Node generator/report tests |
| Four required rapid properties, seed 20260927, 1,000,000 cases **each** | Passed 4,000,000 checks without race; 403.7 seconds total with the four properties running concurrently |
| Checked-in oracle fixtures | All 7,000 passed; expected components are decoded independently of the implementation |
| Hand-written invalid fixtures and 54 literal case-table rows | Passed |
| Race coverage run on textop | 95.7% of production statements |
| FuzzParse, race, two workers, 10 minutes | Passed, 401,492 executions |
| FuzzInsertEscapes, race, two workers, 10 minutes | Passed, 384,876 executions |
| FuzzApply, race, two workers, 10 minutes | Passed, 219,223 executions |
| Stream run, seed 4294967295, 20,000 cases | Passed; final consumer run took 83.4 seconds |
| Saved-case replay command | Passed against a saved Unicode apply case |
| `node tools/tasks.mjs fixtures-check` | Passed after committing the fixtures: tracked-file guard, phase 1 regeneration and two independent temporary generations matched the committed goldens byte for byte; negative fixtures remained untouched |
| Full lint task | Passed tidy, formatting, vet, configuration validation, linters and forbidden-dependency probes |
| Tagged oracle harness lint | Passed with `oracle_scale` enabled |
| Both workflow files | Passed actionlint 1.7.7, with optional shellcheck/pyflakes integrations disabled |

The million-case run used:

```sh
go test ./internal/core/textop -run '^TestProp' -rapid.checks=1000000 -rapid.seed=20260927 -count=1 -v -timeout=2h
```

The stronger combined `textop-acceptance` task (including a million cases per
property **under race**) was not run as one task. The race budget actually run
was 10,000 per property plus the ordinary race suite and ten-minute fuzz runs.
The full 10,000,000-case nightly budget has not been executed locally.

After the comparison-helper performance changes, all cached fuzz baselines
(236 parser, 183 insertion-escape, 206 apply inputs) and an additional 15-second
race fuzz run per target passed against the final helpers.

Measured stream coverage: apply 6,667, compose 6,667, transform 6,666; exactly
100 base documents of 1 MiB per kind; ≥500-component input cases 215 / 224 / 210.
Maximum input components: 1,353; output components: 1,790; record: 2,100,846
bytes. All required named categories were present. Generator SHA256:
`1b3cbce8d360b4223b49a6e2d036ada3c011ae0128ebef9f01f29e552abab166`.

Harness tests deliberately reject duplicate/ambiguous fields, missing or null
payloads, invalid expectations, changed results, wrong versions/tie policy,
truncated streams, skipped indices, wrong counts, oversized records and reader
errors. Process tests exercise nonzero exit after complete output, stderr
backpressure, deadline cancellation, consumer cancellation and a missing binary.
Count and size failures use otherwise-valid records; process tests assert the
specific exit/deadline path and bounded stderr tail rather than accepting any error.
The Node tests exercise a closed stdout pipe. Reporter tests mock GitHub and
verify one issue across four shards, in-place updates, reopening, renaming,
missing artifacts, branch isolation and preservation of the last failure.
No issue was posted and no hosted workflow was triggered during local testing.

The core and oracle commits were also checked using isolated exports of their
staged files. The core snapshot passed race tests with 10,000 cases per property
without the oracle files; the oracle snapshot passed the Go race suite and Node
generator tests. The literal case-table tests perform their own convergence,
length and immutability checks without depending on the oracle consumer.

### Historical performance: gate failed locally

The dedicated non-race gate measured 1,000 complete workloads after ten warmups.
It includes setup correctness checks against independently constructed text.
The histories contain 5,000 valid sequential operations, not repeated operations
with incompatible bases. These percentile results must not be confused with
Go benchmark averages.

| Workload | p50 | p95 | p99 | 50 ms p99 gate |
| --- | ---: | ---: | ---: | --- |
| 5,000 append history operations | 4.546 ms | 17.570 ms | 28.125 ms | Passed |
| 5,000 many-component Unicode history operations | 27.790 ms | 49.651 ms | 67.874 ms | **Failed** |
| Exact 8 MiB apply, identity | 26.756 ms | 42.973 ms | 75.871 ms | **Failed** |
| Exact 8 MiB apply, 64 replacements | 26.937 ms | 39.927 ms | 55.676 ms | **Failed** |

One-iteration allocation smoke benchmarks after setup correction also passed:

| Workload | ns/op (one observation) | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Apply 50,000 bytes | 116,400 | 106,496 | 1 |
| Apply 5,000,000 bytes | 18,239,300 | 10,002,432 | 1 |
| Apply 8,388,608 bytes, identity | 25,638,000 | 16,777,216 | 1 |
| Apply 8,388,608 bytes, replacements | 24,106,100 | 16,777,216 | 1 |
| 5,000 append history operations | 3,848,000 | 3,280,016 | 55,000 |
| 5,000 Unicode history operations | 27,319,600 | 41,600,144 | 225,001 |

The performance thresholds were not relaxed. These failures prompted the
passing Linux rerun below; neither run establishes full S16 system-load acceptance.

### Defects and test-premise corrections

The parser regression tests first demonstrated that every valid array was
rejected: the closing-delimiter check compared with `[` instead of `]`. Only
that predicate was corrected; the repaired F1 escape predicate was retained.

The oracle's former negative-case premise was corrected by appending one ASCII
unit, checking base length, checking all other boundaries, and requiring ot.js
to throw its specific base-length error. More code points alone is insufficient.

The initial benchmark marker expectation ignored history-first insertion ties.
Its independent setup check caught the discrepancy before timing. The corrected
workload inserts the marker at the end; historical append ties put it after the
new head, and Unicode replacements leave that endpoint intact. No transform
algorithm change was made to force the benchmark to pass.

## 2026-10-18 Linux rerun

Re-ran the unchanged `node tools/tasks.mjs textop-perf` on main `21f30d8` before
M4 implementation: Linux/amd64, Intel Xeon W-11855M @ 3.20 GHz, Go 1.27.2,
GOMAXPROCS=12, 1,000 samples per workload after ten warmups, no race instrumentation.
All four 50 ms p99 gates passed. No thresholds or textop production code changed.

| Workload | p50 | p95 | p99 | Gate |
| --- | ---: | ---: | ---: | --- |
| 5,000 append operations | 2.561 ms | 3.658 ms | 4.114 ms | Passed |
| 5,000 Unicode operations | 14.959 ms | 18.182 ms | 20.874 ms | Passed |
| 8 MiB identity apply | 14.546 ms | 17.640 ms | 19.388 ms | Passed |
| 8 MiB apply, 64 replacements | 14.684 ms | 17.533 ms | 19.209 ms | Passed |

Five repetitions of each Go microbenchmark also passed:

| Workload | Range | Allocated bytes/op | Allocations/op |
| --- | --- | ---: | ---: |
| Apply 50,000 bytes | 91.8–94.5 µs, 529–545 MB/s | 106,497–106,498 | 1 |
| Apply 5,000,000 bytes | 8.86–9.19 ms, 544–564 MB/s | about 10,002,432 | 1 |
| Apply 8 MiB identity | 14.68–16.34 ms, 513–571 MB/s | about 16,777,216 | 1 |
| Apply 8 MiB replacements | 14.59–15.91 ms, 527–575 MB/s | about 16,777,216 | 1 |
| Transform 5,000 append ops | 2.424–2.513 ms | about 3,280,016 | 55,000 |
| Transform 5,000 Unicode ops | 15.084–16.016 ms | about 41,600,074 | 225,000 |

The earlier Windows latency failure did not reproduce. These measurements cannot
isolate whether hardware, Go/runtime changes, OS scheduling or GC caused that
historical tail. The Unicode gap still allocates roughly 41.6 MB and 225,000
objects per complete rebase, so allocation/GC remains a concrete profiling target
if the failure returns. Apply allocates a 16 MiB UTF-16 result for 8 MiB of ASCII.
A historical cause is not claimed without profiles from the failing environment.

The one-hour 10× system-load requirement and simulator portions of S16 require
M7/M14 and are still unimplemented. The microbenchmarks report throughput; they
have no separate fixed MB/s acceptance threshold. Passing this rerun establishes
only the existing M1 local latency gates.
