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
