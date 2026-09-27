# M1 textop verification — 2026-09-27

Environment: Windows/amd64, Intel Core i7-9700K @ 3.60 GHz, Go 1.26.6,
Node 24.12.0, GOMAXPROCS=8, rapid 1.2.0, ot 0.0.15. These are local
observations, not hosted GitHub Actions results or a complete S16 system test.

## Correctness and harness checks

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

## Performance: gate failed locally

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

The performance thresholds were not relaxed. These results warrant follow-up
measurement/optimization; the testing implementation does not assert S16 passed.

## Defects and test-premise corrections

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
