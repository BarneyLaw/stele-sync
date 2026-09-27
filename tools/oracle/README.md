# Text operation oracle

The pinned `ot@0.0.15` dependency is test-only. CI uses Node 24.12.0 and the Go
version in go.mod. Install with `npm ci --prefix tools/oracle`.

[Local verification results](../../docs/textop-verification.md) record the
executed budgets and the current failing performance gate.

The independent Go rune-slice model checks edits without copying an OT algorithm.
The oracle then checks exact ordered typed components against ot.js, both TP1
paths, sequential composition, exact UTF-8 text, lengths and immutability. JSON
escape spelling is allowed to differ; normalization is never allowed. See
[ADR 002](../../docs/adr/002-wire-encoding.md) for codec budgets and API contracts.

| Command | Budget / result |
| --- | --- |
| `node tools/tasks.mjs fixtures` | Preserve phase 1 generation and regenerate 2,000 apply + 2,000 compose + 3,000 transform cases |
| `node tools/tasks.mjs fixtures-check` | Require tracked fixtures; generate twice in temporary directories; compare bytes to each other and committed files |
| `node tools/tasks.mjs test` | Repository race tests, 10,000 rapid cases per required property, Node generator/report tests |
| `node tools/tasks.mjs textop-acceptance` | 1,000,000 cases **per property**, race/coverage >90%, 10 minutes per fuzz target |
| `node tools/tasks.mjs textop-perf` | 1,000 whole-workload timings, warmup 10, p50/p95/p99 and <50 ms p99 gate; then allocation benchmarks |
| `node tools/tasks.mjs oracle-scale` | Opt-in tagged streaming run, default 10,000,000 total cases, 110-minute process deadline |
| `node tools/tasks.mjs oracle-replay` | Recheck saved cases from `ORACLE_REPLAY` without Node or regenerating a prefix |

Ordinary `go test` consumes checked-in goldens and does not require Node. Its
rapid default is smaller; use the task budgets when reporting PR or M1 evidence.
Minimized rapid/Go fuzz failures are saved by those frameworks and should be
reviewed and committed as regression corpus; PR CI uploads them on test or
verification failure. Acceptance/performance commands
are explicit gates, not claims that they have already passed on every machine.

`invalid.json` is hand-written. Its stage/error/raw-byte expectations belong to
our contract, not ot.js. Embedded raw JSON strings and base64 bytes are passed
untouched to the public API. Generated fixtures are all validated in staging
before replacement; installation errors roll back prior renames. Generation
never overwrites `invalid.json`. An abrupt machine/process crash during the
multi-file install still requires regeneration.

Each stream starts with a versioned provenance header (normalized-source SHA256,
ot version, a-first tie policy, seed, profile, count and quotas). Case IDs and
indices are contiguous. The generator uses a bounded edit loop, validates scalar
boundaries against actual documents, self-checks negative-case premises and
algebra, and streams with Node pipeline backpressure. The Go reader rejects
duplicates, unknown fields/versions, null values, malformed/truncated records,
wrong counts, missing dependencies and child failures. It drains a bounded
stderr tail and cancels/reaps the child after the first failed check.

Scale reserves 100 exact 1 MiB UTF-8 cases and 100 additional ≥500-component cases
**per kind per shard**, inside the requested total. All six mandatory categories
(identity, Unicode/control/escaping, tie/cancellation, overlap, deletion and
replacement) also occur per kind. Counts below 618 cannot contain these quotas
and are rejected. Actual sizes/component counts are measured by the Go reader;
summary artifacts include per-kind/category totals and input/output maxima.
The 64 MiB record ceiling is a harness limit, never a WebSocket frame policy.

For a local scale smoke run in PowerShell:

```powershell
$env:ORACLE_SEED = '1'
$env:ORACLE_COUNT = '618'
node tools/tasks.mjs oracle-scale
```

`ORACLE_COUNT` accepts 618..10000000; `ORACLE_SEED` accepts 0..4294967295.
`ORACLE_TIMEOUT` can override the 110-minute process deadline for local runs
(the task's Go timeout is 115 minutes). `ORACLE_NODE` can name an explicit Node
binary. `ORACLE_FAILURES` sets the complete first-failure artifact path (default
`.cache/oracle-failures.ndjson`), and `ORACLE_SUMMARY` sets the coverage JSON path.
A failure artifact contains the complete case, header, diagnostic and stderr;
replay does not depend on the current generator hash. Infrastructure failures
have no replayable case and fail the replay command with an explanation.

Nightly runs at `17 18 * * *` UTC (02:17 Singapore the next day). Four independent
seeds each request 2,500,000 cases, totaling 10,000,000, with 120-minute job limits.
A manual seed runs **one** shard; it is not duplicated four times. The existing
soak matrix still runs. All shards retain summaries; failed shards retain complete
cases for 30 days. No completion-time guarantee is inferred from the case count.

The report job groups symptoms and updates one marked GitHub issue in place;
it neither creates an issue per failure nor posts a comment every night. Missing
artifacts are reported as infrastructure failures. Successful first runs create
no issue. A later successful run updates status, preserves the last failure, and does not auto-close an issue
based on a possibly smaller manual replay. Only default-branch runs update the
issue; other branches get the Actions summary/artifacts. `issues: write` exists
only on the report job. This is deterministic explanation of failed checks,
not an AI root-cause diagnosis; an agent can consume the same replay artifacts
later without being given write access or autonomous patch authority.

The performance workloads include exact 50,000 / 5,000,000 / 8,388,608 byte
documents and valid histories of 5,000 sequential operations, both append and
many-component Unicode replacements. Setup independently checks final text.
Aggregate Go benchmark ns/op is not reported as p99. The separate performance
gate records CPU, OS/architecture, Go/compiler, GOMAXPROCS, flags and workload
version; `TEXTOP_PERF_SAMPLES` (minimum 100) can shorten a local smoke run.
Full M14 delivery/durability and 16-client S16 scenarios are outside this suite.
