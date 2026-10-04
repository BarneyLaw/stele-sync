# 016: Discover and isolate milestone verification jobs

Status: accepted

Date: 2026-10-04

## Context

M1 and M2 already participate in repository race tests and fixture checks, but
routine coverage was not enforced and property packages were hard-coded. Their
seven one-hour fuzz targets ran serially inside a six-hour nightly job, making
the documented budget impossible to finish. Future milestones would extend that
same serial path and could silently miss explicit package lists.

## Decision

Generate a plan from Go's package and compiled-test inventory. A reusable
workflow runs one routine coverage/property job per implemented core package,
one nightly job per property/model test, and one job per fuzz target. Include
future properties and decoders throughout `internal/`. Preserve textop's larger
budgets and default future packages to the documented annotation budgets.

Keep `M0 / verification` as the stable required aggregate for all discovered
checks, fixture drift and simulation. Every required child must succeed; skipped
matrix jobs are accepted only when discovery explicitly reports an empty matrix.
Keep the other seven protected contexts unchanged, with no path filters. This
avoids requiring a remote branch-protection edit each time a package is added.

Use four concurrent jobs per matrix, two workers per fuzz target, and explicit
timeouts. Reject plans over 256 jobs so growth requires deliberate partitioning.
Retain plans, coverage and counterexamples as artifacts. Validate workflow syntax
and failure propagation in the ordinary lint/test gates. Run textop's existing
non-race performance check nightly without weakening its thresholds.

## Alternatives

Increasing the serial job timeout cannot fit seven one-hour targets within the
[hosted runner's six-hour job limit](https://docs.github.com/en/actions/reference/limits).
Hard-coded per-milestone matrices require
repeated wiring and invite omissions. New required context names would need
coordinated branch-protection changes and could leave merges unprotected.

## Consequences

Nightly checks use more concurrent jobs and retain the full per-target budget;
they are not shortened smoke tests. Hosted duration and performance results must
be observed after merge, not inferred from local planning tests. New fixture
generators, tagged integration tests, services and release outputs still require
explicit integration; discovery does not manufacture missing tests.

See [CI implementation and verification](../ci-development.md). Revert the CI
commits to roll back; there are no migrations or remote protection changes.
