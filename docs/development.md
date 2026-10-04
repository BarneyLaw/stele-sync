# Development and M0

The root module is github.com/BarneyLaw/stele-sync. The imported, tracked phase 1
snapshot remains in legacy/stele-pull. Root cmd, internal, plugin, schema, scripts,
and phase 1 deployment files are its active promotion. No phase 2 sync algorithm
or server has been implemented in M0.

## Prerequisites

Use Go 1.26.6 (pinned in go.mod, CI, and the worker image), Node 24 with npm,
Docker with Compose v2, and kubectl or kustomize for phase 1 deployment rendering.
Windows needs a C compiler on PATH for go test -race, and Git Bash at its standard
installation path for the inherited rendering script. Go can automatically
download the pinned patch toolchain when GOTOOLCHAIN=auto.

```sh
npm ci --prefix plugin
node tools/tasks.mjs tools
```

The second command installs the versions in tools/versions.json into ignored
.tools/. No globally installed linter is used. npm ci installs the reviewed lockfile.

## Local services

```sh
docker compose up --build --detach --wait
```

Equivalently, run node tools/dev.mjs up or make dev-up. Garage initializes its
single-node layout, obsync and obsync-sync buckets, and a distinct generated key
for each bucket. Initialization is idempotent and services have health checks.
The phase 1 garage-dev.sh and garage-dev.ps1 helpers remain available for an
independent Garage; do not run both setups on the same ports.

Postgres is reachable at 127.0.0.1:55432, database/user obsync, password
obsync-local-only. Garage S3 listens on 127.0.0.1:3900 and the phase 1 website on
127.0.0.1:3902. POSTGRES_DEV_PORT, GARAGE_DEV_S3_PORT, and GARAGE_DEV_WEB_PORT can
override the published ports. Set overrides in the shell when running tasks,
so the integration client and Compose use the same values.

These are local-only configurations; the RPC secret and database password are
public development constants. Only localhost ports are published. Integration
tests read generated Garage credentials directly from the local container and
pass them to the test process without writing credentials to the repository.

docker compose down stops the services and preserves named volumes. Adding
--volumes deliberately deletes this development database and object store.

## Checks

Every Makefile recipe is one directly runnable command on Windows and Unix:

| Target | Direct command | Proof |
| --- | --- | --- |
| lint | node tools/tasks.mjs lint | tidy, gofmt/goimports, vet, configured linters, negative dependency probes |
| test | node tools/tasks.mjs test | repository race tests, discovered core coverage >90%, 10,000 textop / 1,000 other property/model cases, tooling tests |
| test-unit | node tools/tasks.mjs test-unit | repository race and tooling tests without repeating matrix checks |
| core-plan | node tools/tasks.mjs core-plan | discover implemented core packages and internal property/model/fuzz targets |
| core-check | node tools/tasks.mjs core-check | execute selected or all discovered coverage/property checks |
| test-integration | node tools/tasks.mjs test-integration | ready Postgres, real SQL query, six phase 1 Garage tests with race detection; missing Garage fails |
| fixtures | node tools/tasks.mjs fixtures | regenerate manifest, textop oracle and annotation fixtures |
| contract | node tools/tasks.mjs contract | shared Go/TypeScript contracts |
| fuzz-short / core-fuzz | node tools/tasks.mjs core-fuzz | discover internal fuzz targets; 60 s each by default, two race workers |
| sim | node tools/tasks.mjs sim | 500 runs once M7 provides tools/sim/main.go |
| plugin | node tools/tasks.mjs plugin | lint, strict core type-check, Vitest, full type-check/bundle, release metadata |
| audit | node tools/tasks.mjs audit | govulncheck and npm audit, including dev dependencies |
| annot-acceptance | node tools/tasks.mjs annot-acceptance | 100,000 annotation property/model cases, race coverage above 90%, four ten-minute race fuzz runs |
| ci | node tools/tasks.mjs ci | routine gates plus fixture drift, worker image smoke tests, cross-compilation and deployment rendering; excludes long acceptance/performance runs |

The inherited Go tests only change package/import names and paths needed for
their new locations. Contract fixture bytes are preserved. Strict lint exceptions
are enumerated by inherited filename in .golangci.yml and explained in
[ADR 014](adr/014-m0-phase1-compatibility.md). New files receive all enabled rules.
The plugin's inherited UI warnings have equivalent narrow compatibility exceptions.

The dependency proof creates and removes a temporary module with deliberately
forbidden storage imports; it never adds a broken import to the working tree.
This repeats the M0 throwaway-branch proof on every lint run.

M1 adds textop deterministic/property/oracle tests and three fuzz targets. See
[the oracle guide](../tools/oracle/README.md) for acceptance and performance gates,
coverage quotas, failure artifacts and replay commands. Simulation and nightly
e2e still report deferral to M7 and M14. The only
current runnable Go commands are stele-pull and stele-pull-worker; obsync-server
and obsyncctl contain package documentation only.

M2 implements `internal/core/annot` per the supplied annotation proposal. See
[its design record](annot-development.md), [verification](annot-verification.md),
and [captured FreeDraw samples](../schema/annot/samples/README.md). Annotation
contracts are generated by `fixtures` and checked by `fixtures-check` and
`contract`; the TypeScript annotation port remains M13 work.

## CI and main protection

CI runs on every PR and manual dispatch, with stable required M0 check names and
no workflow-level path filters. The reusable core workflow discovers coverage,
properties/models and fuzz targets for M1, M2 and later milestones. PR fuzz jobs
run for 60 s each. Nightly runs isolate each property/model and each one-hour
fuzz target, alongside 10,000 simulator runs, the future end-to-end entry point,
textop's dedicated performance gate, and four 2.5M-case oracle shards. A single
rolling issue collates oracle failures. Production publishing/GitOps writes
are not included. The phase 1 image and deployment checks still run.

See [CI discovery, budgets and verification](ci-development.md) and
[ADR 016](adr/016-discovered-ci-checks.md). New ordinary Go tests, compiled
internal fuzz/property targets and implemented core packages participate without
editing workflow matrices. Specialized integration/release tasks still need
explicit wiring.

The required-check configuration is .github/main-protection.json. Its contexts
match the eight always-reported required contexts. `M0 / verification` includes
the discovered core matrix result, so new jobs need no protection edits.
Repository admins can apply it with:

```sh
gh api --method PUT repos/BarneyLaw/stele-sync/branches/main/protection --input .github/main-protection.json
```

This configuration was applied to main on 2026-09-20: checks must be current,
admins are included, and force pushes/deletion are forbidden. A PR is required;
the approval count is zero so a solo maintainer can merge after reviewing the
diff and passing every check. Hosted CI executes when a PR is opened or updated,
or by manual dispatch; local verification does not substitute for that run.

ADRs 001–012 are recorded from the architecture decision log. ADR 001 remains
proposed per the explicit M0 instruction; its text records the architecture/M1
accepted-status discrepancy. M2 records FreeDraw verification in ADR 013 and
added-page quarantine in ADR 015, and amends ADR 011 per the supplied proposal.

## Verification recorded on 2026-09-20

node tools/tasks.mjs ci completed successfully on Windows with Go 1.26.6,
Node 24.12.0, and Docker Desktop. This is the exact command behind make ci.

- All promoted Go unit tests passed with the race detector.
- Lint reported zero findings; deliberate core/engine/proto imports of
  storage/pg each produced a depguard failure in an isolated temporary module.
- Generated fixture checks were stable; shared contract tests passed in Go
  and TypeScript (90 selected TypeScript contract/preview/policy tests).
- All 157 plugin tests passed, with lint, strict core type-checking, full
  type-checking, bundle generation, and metadata consistency checks passing.
- govulncheck and npm audit reported no vulnerabilities.
- Postgres 17 answered SELECT 1; all six Garage integration tests passed,
  including ranges, conformance, conditional-write capability detection,
  missing-bucket handling, presigning, and a full worker course pass.
- The phase 1 worker image built and passed smoke checks with a read-only root
  filesystem. Both phase 1 commands cross-compiled for Windows and macOS.
- Phase 1 Kubernetes manifests rendered (9 Garage resources, 4 worker
  resources). The inherited renderer used temporary placeholders for absent
  sealed secrets; this validates structure, not a production deployment.
- Main branch protection was applied and its eight required checks verified
  from the GitHub API. No commit, push, or deployment had been performed at the
  time of this local verification.

The code and bootstrap documentation were AI-drafted; the promoted phase 1
production logic and test assertions were preserved. Review ADR 014's explicitly
listed compatibility exceptions before merging. Local services remain running;
docker compose down stops them while preserving their development volumes.
