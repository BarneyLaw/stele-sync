# CI evolution for M1, M2 and later milestones

## Discovery and test policy

The M1/M2 follow-up replaces hard-coded property package lists with a Go-derived
inventory in `tools/core-ci.mjs`, executed only through `tools/tasks.mjs`.
`go list` identifies packages and `go test -json -list` identifies compiled test
functions, so commented-out examples cannot become bogus targets. Tests sharing
a name in different packages remain separate.

Routine checks require strictly more than 90% statement coverage independently
for each implemented `internal/core/*` package. A core package with any ordinary
Go file besides `doc.go` participates even if it has no tests; an empty coverage
profile fails. Documentation-only milestone skeletons are excluded. Coverage is
weighted by statements rather than rounded percentages or block counts.

`TestProp*` and `TestModelAgreement*` functions anywhere under `internal/` receive
1,000 rapid cases per routine run (M1 textop keeps 10,000). Nightly runs isolate
each function: textop gets 1,000,000 cases; other packages get 100,000. All runs
use the race detector. New stateful tests should use these naming conventions
and the repository's rapid library. Ordinary regressions still run through
`go test -race ./...`, regardless of naming or milestone.

Every compiled `Fuzz*` target under `internal/` is discovered, with two workers
and an explicit per-target timeout. `FUZZ_TIME` accepts whole positive s/m/h
durations up to one hour. Invalid selections, discovery errors, malformed or
insufficient coverage, and matrices exceeding GitHub's 256-job ceiling fail
explicitly. A larger matrix requires deliberate partitioning, not truncation.

Planner regression tests cover future core/protocol/engine packages, duplicate
target names, testless implementations, milestone budgets, race flags, exact
selection, malformed inputs and coverage boundary failures. Pinned actionlint
joins the ordinary lint gate to check all workflow files on every change.

## Commands

```sh
node --test tools/core-ci.test.mjs
node tools/tasks.mjs core-plan
node tools/tasks.mjs test
node tools/tasks.mjs fuzz-short
node tools/tasks.mjs lint
```

`test` is repository unit/tooling tests plus discovered routine core checks.
CI can run `test-unit` and matrix-selected `core-check` independently without
duplicating the full routine suite. `CORE_MODE=nightly` selects the larger plan;
`CORE_PACKAGE` and `CORE_TEST` select an exact discovered case. Omitting both
selections runs all cases locally. `core-plan` records its plan under ignored
`.cache/core-ci/`; routine coverage profiles are stored there per package.

New fixture generators, integration services, performance workloads and release
artifacts still need explicit task wiring: discovery cannot infer those
contracts. Existing manifest/textop/annotation fixture drift checks, plugin
tests, audit, worker builds and deployment rendering remain in scope.

The planner, task changes, tests and this documentation are AI-drafted under the
user's request. Changes remain on `m2/annot`, with only the user's configured Git
identity. No hosted workflow, protection update or deployment is triggered by
local validation.

## Workflow integration

The shared `core.yml` workflow runs the same plan for PRs and nightlies. Routine
checks split by package; nightly property checks split by test; fuzzing always
splits by target. Currently that means four routine core checks, thirteen nightly
property/model checks and seven fuzz targets. Each matrix permits four concurrent
jobs. Property jobs have 30-minute PR / six-hour nightly limits, with shorter Go
test deadlines; fuzz jobs have 90-minute limits for at most one hour of fuzzing.

`M0 / verification` depends on the entire reusable workflow before checking
fixtures and simulation. All eight existing protected contexts remain unchanged.
The result aggregator explicitly rejects discovery failure, failed/cancelled
children and unexpected skips. Tests execute that actual shell fragment over
160 result/empty-matrix combinations and compare required names with the tracked
protection configuration. No remote protection change is necessary.

Nightly retains the four oracle shards and simulator/e2e entry points, and adds
the existing dedicated non-race textop performance check. Logs, coverage,
discovery plans and minimized counterexamples are uploaded with distinct names.
Uploads opt into hidden files only for the explicit plan/coverage paths under
`.cache/core-ci`; otherwise upload-artifact would omit that evidence. A regression
test reproduced the missing opt-in before the workflow correction.
This resolves the prior seven-hour serial fuzz workload inside a six-hour job.
[ADR 016](adr/016-discovered-ci-checks.md) records the decision and alternatives.

## Verification — 2026-10-04

Local Linux/amd64 checks used Go 1.26.6 and Node 25.5.0; CI retains Node 24.12.0.

- `node tools/tasks.mjs test` passed repository-wide race tests, all routine
  property/model budgets and the tooling tests.
- Per-package race coverage passed: annot 93.74%, policy 93.20%, textop 95.66%,
  vpath 90.54% (unrounded statement ratios drive the gate).
- `node tools/tasks.mjs lint` passed actionlint 1.7.12, planner tests, tidy, vet,
  formatting, configured linters and negative dependency probes. Optional
  actionlint shellcheck/pyflakes integrations are disabled, as in M1 validation.
- Both real discovery plans matched the expected package/test counts. The
  workflow regression tests passed, including failure/cancellation propagation
  and unchanged required contexts.
- `fixtures-check` and `contract` passed, including regeneration/tracking checks
  and the existing 90 TypeScript contract tests.
- `FUZZ_TIME=5s node tools/tasks.mjs core-fuzz` passed all seven target invocations.
  This only smoke-tests wiring; several annotation targets spent that short
  budget loading the existing corpus. It is not new hour-long fuzz evidence.

The new hosted matrices and their full nightly budgets have not been executed
locally or dispatched. M1's previously recorded performance failures and M2's
three existing moderate npm audit advisories are not fixed or suppressed here.
See [textop verification](textop-verification.md) and
[annotation verification](annot-verification.md). No Go/npm dependency lockfiles,
test fixtures or production algorithms changed in this CI follow-up.

Subsequent follow-up: the Moment audit findings were resolved with a scoped SDK
dependency override. See [the remediation and passing audit/plugin checks](development.md#moment-audit-remediation--2026-10-04).
