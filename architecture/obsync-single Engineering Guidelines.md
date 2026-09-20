# obsync-single: Engineering Guidelines

2026-09-20 · @Someone

These rules are binding for all obsync-single code: a pull request that breaks one either fixes it or records the exception in an ADR.

## 1. Principles

When two goals conflict, the higher one wins: **correctness, then durability, then availability, then latency, then features.** A faster path that can lose an acked edit is a bug, not a trade-off.

1. **Functional core, imperative shell.** Every decision (transform, validation, conflict rules, class detection, compaction triggers) lives in pure packages: values in, values out, no I/O, no clock, no goroutines. The shell (sessions, database, Garage, timers) only moves data and calls the core. Phase 1 already works this way; keep it.
2. **Make illegal states unrepresentable.** Parse at the boundary into types that cannot be invalid (`textop.Op` is canonical by construction; `vpath.Path` is portable by construction). Inside the core, never re-check what a type already guarantees.
3. **One owner per piece of mutable state.** Each hot document has one goroutine; each connection has one writer goroutine. If you cannot name the owner of a field, the design is wrong.
4. **Bound everything.** Every queue, map, cache, read and retry has a documented ceiling, and hitting it has a defined behaviour. "It won't get that big" is not a ceiling.
5. **Fail loudly, degrade narrowly.** A broken invariant panics in tests and returns an internal error in production; it never corrupts silently. A failure affecting one client or file must not spread to others.
6. **Interfaces are small and owned by the consumer.** Define an interface in the package that uses it, with only the methods it calls. Return concrete types.
7. **Explicit over clever.** State machines as named states and a transition function, not flags. Protocol codes as constants, not strings scattered through code.
8. **Decisions are written down.** Anything that took more than a paragraph to decide gets an ADR, with the alternatives you rejected and why.
9. **No speculative generality.** Build for phase 2. A phase 3 hook is added only when it costs nothing now (for example, a `user_id` column that is always 1 costs nothing; a plugin framework does).

## 2. Repository layout and dependency rules

One monorepo holds phase 1 and phase 2, with phase 1 moved in history-preserved (`git subtree add`) during M0. One repo matches the one-name, many-subcommands plan and lets phase 2 import `portable` and `policy` directly instead of copying them.

```
cmd/
  obsync-server/        wiring only: config, construct, run, shutdown
  obsyncctl/            admin CLI: devices, history, restore
  obsync-worker/        phase 1, unchanged
internal/
  core/                 PURE. stdlib and golang.org/x/text only
    textop/             Class A operations: apply, compose, transform
    annot/              Class B sidecar model and element ops
    vpath/              portable paths (from phase 1 portable)
    policy/             exclusion rules (from phase 1)
    classify/           file class detection
    rules/              namespace, window and conflict decisions
  proto/                wire messages, codec, limits validation
  engine/               document actors, submission pipeline, ports
  session/              WebSocket sessions and hub
  httpapi/              blob, enroll, health handlers
  auth/                 enrollment codes, tokens
  storage/pg/           Postgres repositories and migrations
  storage/objects/      S3/Garage store (from phase 1 store)
  blob/  compact/  gc/  canvasbridge/  obs/  config/
plugin/                 Obsidian plugin (TypeScript)
schema/                 contract fixtures shared by Go and TypeScript
tools/oracle/           Node script: generates textop fixtures from ot.js
tools/sim/              deterministic simulation harness
tools/loadgen/          load and soak client
deploy/  docs/adr/  docs/runbooks/
```

### Allowed imports

Anything not listed is forbidden.

| Package | May import |
| --- | --- |
| `core/*` | stdlib, `golang.org/x/text`, other `core/*` |
| `proto` | `core/*` |
| `engine` | `core/*`, `proto`, `obs`. Declares the interfaces (ports) it needs: `Repo`, `Snapshots`, `Publisher`, `Clock`. |
| `storage/*`, `blob`, `compact`, `gc`, `canvasbridge` | `core/*`, `engine` (to implement its ports), `obs`, `config` |
| `session`, `httpapi` | `engine`, `proto`, `auth`, `obs` |
| `cmd/*` | everything; contains no logic beyond wiring |

The engine never imports `storage/pg`, and `core` never imports anything with I/O. This is what lets the whole sync engine run inside the simulation harness with in-memory fakes and a virtual clock.

**Enforcement.** `depguard` rules in `.golangci.yml` encode this table, so a forbidden import fails CI. The plugin mirrors it with `eslint-plugin-boundaries` or `no-restricted-imports`: `src/core/**` may not import `obsidian`.

## 3. Go standards

The baseline is Effective Go, Go Code Review Comments and the Google Go Style Guide, in that order of precedence; the Uber guide settles anything they leave open. The rules below are the ones this project enforces on top.

### 3.1 Tooling (CI fails on any finding)

- `gofmt` and `goimports`; `go vet`; `go test -race`; `govulncheck`.
- `golangci-lint` with at least: `errcheck`, `staticcheck`, `govet`, `revive`, `errorlint`, `wrapcheck` (boundaries only), `contextcheck`, `noctx`, `bodyclose`, `gosec`, `exhaustive` (every switch over a state or message type), `depguard`, `forbidigo` (bans `fmt.Print*`, `log.*`, `time.Now` outside `cmd/` and `obs`), `gocritic`, `unparam`.
- Go version pinned in `go.mod` and CI to the same minor; toolchain upgrades are their own PR.

### 3.2 Errors

- Wrap with context using `%w`: `fmt.Errorf("load head %s: %w", fileID, err)`. The message says what was being done, lower case, no "failed to".
- Sentinel errors (`var ErrStale = errors.New(...)`) for conditions callers branch on; typed errors when they need fields. Callers use `errors.Is` and `errors.As`, never string matching.
- Map domain errors to protocol reject codes in exactly one place (`session`). The core never knows wire codes.
- Handle an error once: either log it or return it, never both.
- `panic` only for violated internal invariants that indicate a bug. A recovered panic in a session closes that session with 1011 and increments a metric; it never kills the process.

### 3.3 Context and cancellation

- `ctx context.Context` is the first parameter of every function that blocks, does I/O, or may be cancelled. Never store a context in a struct.
- Every outbound call carries a deadline: database 2 s default, Garage 30 s per request, WebSocket write 5 s.
- Cancellation is honoured promptly in loops (`select` on `ctx.Done()`).

### 3.4 Concurrency

- Every goroutine has an owner that starts it and waits for it (`errgroup` or a `sync.WaitGroup`). No fire-and-forget goroutines.
- The sender closes a channel; receivers never do. Every channel send that could block sits in a `select` with `ctx.Done()`.
- Prefer an owning goroutine plus channels for stateful components (document actors, session writers); use a `sync.Mutex` for small shared structures, with a comment listing the fields it guards.
- Never hold a lock across I/O, a channel send, or a call into another component.
- Time is injected. Tests that involve timers use `testing/synctest` or a fake clock, never `time.Sleep`.

### 3.5 Types and APIs

- Accept interfaces, return structs. Interfaces live with their consumer and stay small.
- Constructors validate and return `(T, error)`; a constructed value is always usable. No `Init()` methods.
- No package-level mutable state, no `init()` with side effects.
- Use named types for identifiers (`type FileID uuid.UUID`, `type Version int64`) so a version cannot be passed where a sequence number is expected.
- Generics only where they remove real duplication (a bounded LRU), not for style.

### 3.6 Logging and metrics

- `log/slog` with a JSON handler; logger passed explicitly, never global.
- `msg` is a stable dotted event name (`submit.rejected`); details are attributes. Changing an event name is a breaking change for alerts and dashboards.
- Levels: Debug for no-ops, Info for decisions and writes, Warn for degraded outcomes, Error for failures needing a human.
- Never log file content, tokens, or enrollment codes. A test asserts this by running a session with a canary token and grepping the logs.
- Metric labels are bounded: never a file id, path or device token as a label.

### 3.7 Configuration

- Parsed once in `main` from flags and environment into a typed, immutable `Config`, validated with all errors reported at once, then passed down. No package reads the environment itself.
- Every limit in the architecture doc is a config field with the documented default.

### 3.8 Documentation

- Every package has a doc comment that states its responsibility and its invariants. Every exported identifier has a doc comment that begins with its name.
- Comments explain why, not what. A non-obvious ordering (commit before ack) gets a comment naming the invariant it protects.

## 4. TypeScript plugin standards

The plugin may be AI-assisted, so its guard rails are mechanical: strict compiler, strict lint, and a pure core that must pass the same contract fixtures as the Go server.

### 4.1 Compiler and lint

- `tsconfig`: `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `noImplicitOverride`, `noFallthroughCasesInSwitch`.
- ESLint with `eslint-plugin-obsidianmd` (already in phase 1) and `@typescript-eslint/strict-type-checked`. `any` is banned; `unknown` plus a type guard is the escape hatch.
- Every `switch` over a message type or sync state ends with an exhaustiveness check (`const _: never = x`).

### 4.2 Structure

```
plugin/src/
  core/        PURE: textop, annot, sync state machine, diff, classify, policy
  transport/   WebSocket client, reconnect with jitter, blob HTTP
  vault/       Obsidian Vault and Editor adapters, file watcher, shadow store
  ui/          status bar, conflicts view, Canvas notices, settings, confirm dialogs
  main.ts      wiring only
```

`core/` never imports `obsidian`, never touches the DOM, never reads a clock directly. It runs in Vitest without mocks.

### 4.3 Protocol handling

- Messages are a discriminated union on `t`. Every inbound frame is validated at runtime by a hand-written guard or a schema (for example Valibot) before use; a TypeScript cast is not validation.
- Reject codes are a `const` union shared with the fixtures in `schema/`.

### 4.4 Obsidian rules

- Edit the active file through the Editor (CodeMirror 6) API; edit background files through `Vault.process`, never `Vault.modify`, so a concurrent change is not overwritten.
- Remote edits are dispatched with a CodeMirror annotation marking them as remote, so the change listener does not echo them back.
- Use `requestUrl` for HTTP. Use `this.app`, never the global `app`. Use `normalizePath` for every user path. Never cast `vault.adapter` to `FileSystemAdapter` without an `instanceof` check; mobile uses `CapacitorAdapter`.
- Register every listener, interval and view through `this.register*` so unload leaves nothing behind.
- No `console.log` in normal operation. A debug toggle in settings routes structured logs to a ring buffer the user can copy into a bug report.

### 4.5 Local persistence

Shadows and per-file sync state live in IndexedDB, written transactionally: the shadow and `base_version` for a file change together or not at all. Settings stay in the plugin's `data.json`.

## 5. Database rules

Postgres is the system of record, so its rules are the strictest in the project: the database enforces invariants itself, and application code is only the first line of defense.

### 5.1 Access

- Driver: `pgx` v5 with `pgxpool`. Queries: `sqlc`, so every query is plain SQL checked at generate time and every result is a typed struct. No ORM, no query builders, no string-concatenated SQL.
- Every query runs with a context deadline. Pool sized explicitly (default 10) with `pool_max_conn_lifetime` set.
- The advisory-lock connection is acquired outside the pool and never returned to it.

### 5.2 Transactions

- Short and local. **No network call other than to Postgres inside a transaction**: no Garage request, no WebSocket write, no waiting on a channel.
- Row locks are always taken in a fixed order: file rows ascending by `file_id`, then the `vault` row last. This rules out deadlocks between a `rename_group` and a single edit.
- Default isolation `READ COMMITTED` plus explicit `SELECT ... FOR UPDATE` on the rows a decision depends on. Any use of another level is documented at the call site.
- A serialization or deadlock error (`40001`, `40P01`) is retried by one helper with a bounded attempt count; nothing else retries database writes.

### 5.3 Schema and migrations

- Migrations with `goose`, SQL files, numbered, forward-only in production. Each migration states in a header comment how to back it out by hand.
- Constraints live in the schema: `NOT NULL`, `CHECK`, foreign keys, the partial unique index on `path_key`. If the application has a rule the database can express, the database expresses it.
- Expand, then contract: add a column, deploy code that writes both, backfill, deploy code that reads new, drop old. Never rename a column in one step.
- Migrations run as a separate step (init container or `obsyncctl migrate`) before the server starts; the server refuses to start on an unexpected schema version.
- Every new index states which query needs it; every query on a hot path has its `EXPLAIN` captured in the PR.

## 6. Reliability patterns checklist

Reviewers walk this list for any PR that touches I/O, concurrency or the protocol. Each item is a known way distributed systems fail.

### 6.1 Timeouts

- [ ] `http.Server` sets `ReadHeaderTimeout` (5 s), `ReadTimeout`, `WriteTimeout` for non-WebSocket routes, `IdleTimeout` (120 s), and `MaxHeaderBytes`.
- [ ] Every WebSocket read is bounded by the heartbeat deadline; every write by a 5 s context.
- [ ] Every database and Garage call has a deadline (section 3.3).

### 6.2 Retries

- [ ] Only idempotent operations are retried: submissions carry `client_op_id`, blob uploads are content-addressed, snapshot keys are write-once.
- [ ] Backoff is exponential with full jitter and a cap. Retries are bounded by count and by total time.
- [ ] Retries happen at one layer only. If the plugin retries a submission, the transport underneath does not also retry it.
- [ ] Permanent errors (`invalid_op`, 401, `too_large`) are never retried.

### 6.3 Bounds and load shedding

- [ ] Every body and frame is read through a size limit before parsing (`io.LimitReader`, `Conn.SetReadLimit`).
- [ ] Every queue and mailbox has a capacity and a full-queue behaviour (reject or close), chosen deliberately and tested.
- [ ] Caches are bounded by bytes, not entry count.
- [ ] Under overload, shed the newest work from the noisiest client first; never degrade everyone equally.

### 6.4 Idempotency and ordering

- [ ] A handler can receive the same message twice and produce one effect.
- [ ] No ordering assumption depends on wall-clock time from a client.
- [ ] Commit happens before acknowledgement, everywhere, with a comment naming invariant 1.

### 6.5 Lifecycle

- [ ] Graceful shutdown on SIGTERM in this order: mark not ready, stop accepting connections, send `close 1001` to sessions, drain in-flight submissions (10 s budget), stop compaction, close the pool, release the advisory lock. Total under the pod's `terminationGracePeriodSeconds` (30 s).
- [ ] Startup refuses to serve until migrations match, the advisory lock is held, and a test query succeeds.
- [ ] `/livez` checks only that the process loop is alive. `/readyz` checks dependencies. A liveness probe that checks the database restarts a healthy pod during a database blip; do not do that.

### 6.6 Crash safety

- [ ] Every multi-step write is ordered so that a crash at any step leaves either the old state or garbage that GC removes, never a half-visible state.
- [ ] Write-once object keys; the database row that references an object is written after the object exists.
- [ ] Client-side, shadow and `base_version` are updated in one IndexedDB transaction after the ack, never before.

## 7. Testing standards and definition of done

Tests are the proof that a component works; a component without its tests is not done, however finished the code looks. The more a package decides, the stronger its tests must be.

### 7.1 Test types and where they apply

| Type | Tool | Required for |
| --- | --- | --- |
| Example (table-driven) | `testing` | Every package |
| Property-based | `pgregory.net/rapid` (Go), `fast-check` (TS) | Every `core/*` package; algebraic laws stated as properties |
| Stateful property | `rapid` state machines | Engine, sync client state machine, namespace rules |
| Contract / golden | JSON fixtures in `schema/`, checked by both languages | Anything both Go and TypeScript implement: textop, annot, proto, policy, classify |
| Oracle | `tools/oracle` runs ot.js to produce expected outputs | `core/textop` |
| Fuzz | native `go test -fuzz` | Every decoder and validator that reads client bytes |
| Integration | `testcontainers-go` Postgres; phase 1 Garage script | `storage/*`, `blob`, `compact`, `gc` |
| Simulation | `tools/sim`: in-memory engine, virtual clock, seeded randomness | Engine plus client state machine end to end |
| End to end | real server, real Postgres, scripted WebSocket clients | Release gate |
| Chaos and soak | kill -9 harness, Toxiproxy, 7-day soak | Success criteria S2 to S4, S11 |

### 7.2 Rules

- **Write the property before the implementation** for every `core` package. For textop the first test written is convergence: `apply(apply(d, a), b') == apply(apply(d, b), a')`.
- **Every bug gets a regression test first.** The failing test lands in the same PR as the fix. A rapid or fuzz failure's seed or corpus entry is committed.
- **Determinism.** No test depends on wall time, goroutine scheduling or network timing. Randomized tests log their seed and replay from it.
- **Real dependencies over mocks** at the storage boundary. A fake Postgres proves nothing about locking. Fakes are fine for engine ports in simulation, where the point is scheduling, not SQL.
- **Race detector always on** in CI.
- **Coverage is a signal, not a target.** `core/*` should sit above 90% of lines; a PR that drops a core package's coverage explains why. Nothing else has a number.
- **Fixtures are generated, never hand-edited**, except the ones marked hand-written in their file header (as in phase 1).

### 7.3 Definition of done (per component)

- [ ] Code follows sections 1 to 6; lint, vet, race and vulnerability checks green.
- [ ] The component's tests from the implementation guide exist and pass, including properties and fuzz targets.
- [ ] Package doc comment states responsibility and invariants.
- [ ] Every limit and timeout is configurable, with the documented default.
- [ ] Metrics and log events from the architecture doc are emitted and asserted in at least one test.
- [ ] ADRs for any decision made along the way are merged.
- [ ] A reviewer (human, or AI with a human reading the review) has walked the reliability checklist.
- [ ] The implementation guide's exit criteria for the milestone are demonstrated and recorded in the PR.

## 8. Workflow

Trunk-based development with small pull requests: `main` is always deployable, and every change reaches it through a reviewed, green PR.

### 8.1 Branches and commits

- Short-lived branches off `main`, named `m7/engine-actor` (milestone, then topic). Merge within a few days; split anything larger.
- Commits follow Conventional Commits: `feat(textop): add compose`, `fix(session): close on pong timeout`. Breaking protocol changes use `!` and a `BREAKING CHANGE:` footer.
- Squash-merge, so `main` reads as one commit per reviewed change.
- Releases are tagged with semantic versions. Server and plugin share the protocol version, not the release version.

### 8.2 Pull requests

A PR description has five parts, and review does not start until all five exist:

1. **What** changed, in one paragraph.
2. **Why**, linking the milestone and any ADR.
3. **How it is proven**: which tests, which properties, and the command to reproduce.
4. **Risk**: what could break, and which invariant it touches.
5. **Rollback**: how to undo it in production (usually: revert; for migrations: the back-out note).

Target under 400 changed lines excluding generated code and fixtures. The reviewer checks design, then correctness, then tests, then style, following Google's code review guide. Style nits are prefixed `nit:` and never block.

### 8.3 CI gates (required checks on `main`)

| Gate | Runs on |
| --- | --- |
| gofmt, goimports, vet, golangci-lint (incl. depguard) | Go changes |
| `go test -race ./...` | Go changes |
| Fuzz targets, 60 s each | Go changes to `proto`, `core/*` |
| Contract fixtures regenerate with no diff; both languages pass them | Changes to `schema/`, `core/*`, `proto`, `plugin/src/core` |
| Integration tests (Postgres container, Garage) | Changes to `storage/*`, `blob`, `compact`, `gc` |
| Simulation, 500 seeded runs | Changes to `engine`, `core/*`, plugin sync core |
| Plugin lint, type-check, Vitest, bundle | Plugin changes |
| govulncheck, npm audit | Always |
| Nightly: 1 h fuzz, 10,000 simulation runs, e2e | Schedule |

Keep phase 1's path-filter lesson: a workflow that does not run on a PR never reports, so required checks must be the ones that always run, with path logic inside the job.

### 8.4 ADRs and docs

- ADRs live in `docs/adr/NNN-title.md` with status (proposed, accepted, superseded by NNN), context, decision, consequences. An accepted ADR is never edited; it is superseded.
- Runbooks live in `docs/runbooks/`: deploy, rollback, revoke device, restore file version, restore from backup, rotate TLS. Each is tested by being followed, at least once, by someone reading it cold.
- The architecture doc changes in the same PR as any code that makes it wrong.

## 9. AI and agent use policy

The server is human-written; AI is a reviewer, tutor and test generator there, and a co-author only for the plugin. The goal is that you can defend every line of the Go code in an interview, because you wrote it.

### 9.1 Who writes what

| Area | Human writes | AI may |
| --- | --- | --- |
| `core/*`, `proto`, `engine`, `session`, `storage/pg` | All production code | Explain concepts, review diffs, propose additional test cases and properties, find bugs |
| `blob`, `compact`, `gc`, `canvasbridge`, `httpapi`, `auth` | Production code | Same as above, plus draft boilerplate you then rewrite (flag parsing, handler plumbing) |
| Tests | Properties and invariants | Generate extra example cases and fuzz seeds once the property exists |
| `tools/oracle`, `tools/loadgen`, CI YAML, Kubernetes manifests | Review and own | Draft |
| Plugin `core/` | Review line by line; own the fixtures it must pass | Draft |
| Plugin `ui/`, `vault/`, `transport/` | Review | Write |

### 9.2 Rules

1. **Contracts before code.** An agent never writes code for a component whose property tests and fixtures do not yet exist. The tests are the specification it is held to.
2. **Explain-back.** You merge AI-drafted code only after you can explain every line and why it is correct. If you cannot, delete it and write it.
3. **Disclose.** The PR description says which files had AI-drafted content. Commits can carry an `Assisted-by:` trailer.
4. **No invariant logic from an agent without a human-written test that would catch it being wrong.** Specifically: transform, idempotency, commit ordering, lock ordering, conflict rules.
5. **No secrets in prompts.** Tokens, keys and real vault content never go into an AI tool. Use fixtures.
6. **Review AI output harder, not softer.** Plausible code that compiles is the specific failure mode to guard against: check error paths, bounds and cancellation first.
7. **Agents run in a sandbox branch.** An agent never pushes to `main`, never edits `schema/` fixtures, never edits ADRs.

## 10. Sources

**Go**

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Go Proverbs](https://go-proverbs.github.io/)
- [Go Fuzzing](https://go.dev/doc/security/fuzz/)
- [Testing concurrent code with testing/synctest](https://go.dev/blog/synctest)
- [rapid: property-based testing for Go](https://github.com/flyingmutant/rapid)
- *100 Go Mistakes and How to Avoid Them*, Harsanyi (Manning); *The Go Programming Language*, Donovan and Kernighan

**Design and architecture**

- [Boundaries](https://www.destroyallsoftware.com/talks/boundaries), Bernhardt: functional core, imperative shell
- [Parse, don't validate](https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/), King
- [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions), Nygard
- [The Twelve-Factor App](https://12factor.net/): config and process model

**Data**

- [pgx](https://github.com/jackc/pgx), [sqlc](https://docs.sqlc.dev/), [goose](https://github.com/pressly/goose)
- [PostgreSQL: Explicit Locking](https://www.postgresql.org/docs/current/explicit-locking.html)
- [Testcontainers for Go](https://golang.testcontainers.org/)

**Reliability**

- [AWS Builders' Library: Timeouts, retries and backoff with jitter](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/)
- [Google SRE Book: Addressing Cascading Failures](https://sre.google/sre-book/addressing-cascading-failures/)
- [Kubernetes: Configure Liveness, Readiness and Startup Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
- *Release It!*, Nygard

**Process**

- [Google Engineering Practices: Code Review](https://google.github.io/eng-practices/review/)
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)

**Obsidian and TypeScript**

- [Obsidian Plugin guidelines](https://docs.obsidian.md/Plugins/Releasing/Plugin+guidelines)
- [typescript-eslint shared configs](https://typescript-eslint.io/users/configs/)
- [fast-check](https://fast-check.dev/)
