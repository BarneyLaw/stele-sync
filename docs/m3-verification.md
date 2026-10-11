# M3 verification record

Initial verification on 2026-10-11, Linux amd64, used **Go 1.26.6 explicitly selected**,
Node 25.5.0 and the repository's pinned Go tools. CI continues to use its Node 24
pin. This initial section records local results; the later follow-up separately
records hosted evidence. See
[implementation notes and authoritative sources](m3-development.md) for the
contract and its necessary corrections.

The [Go 1.27.2 follow-up](#ci-follow-up-go-1272) below supersedes the original
audit blocker and records the CI property-generator fix. Original measurements
remain labeled with their original toolchain.

## Automated acceptance

| Check | Result |
| --- | --- |
| Repository `test-unit` | Passed: Go race suite (including inherited phase 1) and 17 tooling regressions |
| Complete `lint` gate | Passed: formatting, tidy, vet, all configured linters, actionlint and negative import probes |
| M3 discovered `core-check` jobs | Passed for vpath, classify and rules |
| vpath statement coverage | 96.25% |
| classify statement coverage | 96.88% |
| rules statement coverage | 97.96% |
| `fixtures-check` | Passed: six M3 contracts tracked and stable; existing manifest, annotation and oracle fixtures unchanged |
| `contract` | Passed in Go and TypeScript |
| Plugin gate after clean dependency install | Passed: strict core typing, lint, 166 tests and production bundle |
| npm audit | Zero findings after the isolated source-map-js patch |
| Windows compile | Path tests and both phase 1 worker commands compile; no Windows binaries were executed locally |

All nine M3 properties passed **100,000 cases each with the race detector**:
ParseRoundTrip, SanitizeIsStrict, ParseCharacterRules, KeyCoarserThanFold,
KeyCoarserThanUpper, KeyIdempotent, ConfirmNeverUpgrades, FreeName and DeleteGuard.
The guard cases each perform 20 steps against an independent timestamp-list
model: **2 million checks**. The final guard run took 35.540 seconds after replacing
reflection with an equivalent exact comparison of the comparable value. The
occupied-key FreeName property took 53.230 seconds; the final six vpath properties
took 34.489 seconds. ConfirmNeverUpgrades took 5.104 seconds.

Final relevant ten-minute native fuzz runs, race detector on, two workers:

| Target | Executions | Result |
| --- | ---: | --- |
| vpath/FuzzParse | 594,802 | passed |
| vpath/FuzzKey | 191,174 | passed |
| classify/FuzzClassify | 460,274 | passed |

The parse run was repeated after adding Windows console/spaced-device aliases;
the classification run was repeated after reserving recovery sidecars as opaque.
There were no minimized failure corpora to commit. Tests deliberately introduced
before fixes demonstrate Windows alias rejection, Unicode-version skew and
stream-safe normalization mismatch. Go/TS fixtures also cover NUL after 8 KiB,
truncated UTF-8, oversized input, malformed/duplicate JSON, exact annotation
budgets, all eight actual M2 FreeDraw captures, nanosecond guard boundaries and
permanent opaque classes.

A supplementary normalization probe compared all Unicode scalars and every
native-JS canonical decomposition against the pinned Go NFC. Single scalars
matched, but **20 decomposed sequences differed** across Unicode versions. The
client now uses generated assignment boundaries plus x/text stream-safe counts;
regressions cover new scripts, long combining runs, Jamo and compatibility marks.
This follows [UAX #15's older-version process rules](https://www.unicode.org/reports/tr15/#Stability_Prior_Version).
The runtime must implement at least the pinned Unicode 15 repertoire; actual
supported Obsidian/mobile runtimes remain a platform acceptance item.

## NTFS and application measurements

Docker is installed, but the current user cannot access its daemon; noninteractive
sudo also requires a password. A **64 MiB disposable NTFS image** was successfully
formatted using `mkfs.ntfs` / ntfs-3g 2022.10.3. `lowntfs-3g -o ignore_case` could
not mount it because the host denied mount permission. These were host denials,
not failed namespace assertions.

The image's `$UpCase` was extracted with `ntfscat`. TestNTFSUpcase checked **all
65,536 entries**, including **973 non-identity scalar mappings**; every mapped
pair shares the M3 key. Extracted table SHA-256:
`41c26bc7a12bdaeb26025c93118697c7e3ef81ee048b00fe5cce2a472e0e0742`.
This is useful actual on-disk-table evidence, not a Windows-driver creation test.

The reusable Core workflow now runs TestFilesystemCollisions on a Windows runner,
first checking that its temporary volume is NTFS, then creating each golden pair
with exclusive-create semantics. Any filesystem collision missed by Key fails the
existing required verification gate. Measurements are uploaded as an artifact.
**This hosted Windows job has not been run from this local session.**
The later CI follow-up below records the now-observed hosted result for the
original implementation, separately from the upgraded toolchain.

macOS/APFS and iOS measurements are deferred at the user's explicit request.
Android/Obsidian path-event normalization and FreeDraw partial-write visibility
were not measured here (no Android test environment was available). The **10-second
quiet default remains provisional**. M13 must measure these in a disposable vault
and preserve original local spelling when mapping paths to the NFC wire form.
Docker-dependent integration/worker-image tests were not rerun; their existing
CI gates remain enabled. Phase 2 engine/storage/protocol integration is still the
future milestones' responsibility, not functionality supplied by this pure core.

## Original Go audit blocker (resolved by the follow-up)

The combined audit gate was **not green** with the original Go 1.26.6 pin.
Govulncheck reported ten reachable standard-library advisories:
GO-2026-6617, 6613, 6612, 6611, 6610, 6609, 6608, 6607, 6605 and 6603.
An isolated checkout of the original branch head **cf698b0** produced the same
advisory set. The scanner also reports uncalled package/module findings.

The reported standard-library fix is Go 1.26.9; see, for example,
[GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617). The repository's
[Engineering Guidelines §3.1](../architecture/obsync-single%20Engineering%20Guidelines.md#31-tooling-ci-fails-on-any-finding)
normally require toolchain upgrades in their own PR. The user subsequently
requested the latest stable Go release on this branch, authorizing the update
recorded below. No vulnerability suppression or audit bypass was added.

The npm finding was repaired independently by updating only the source-map-js
lock entry from 1.2.1 to 1.2.2, preserving all optional platform dependencies.
That patch addresses [GHSA-68fv-2mgg-jv7q](https://github.com/advisories/GHSA-68fv-2mgg-jv7q).
A clean npm ci, audit, lint, tests, type checking and bundle all passed afterward.

## CI follow-up: Go 1.27.2

CI seed `3346917770462226820` generated `aux.md` in TestPropFreeName. This is a
reserved Windows name: Parse correctly rejected it before FreeName ran. The
generator now filters stems through Parse, honoring FreeName's validated-Path
precondition. Its original assertions and 100,000-case budget remain intact.
The minimized Rapid counterexample is committed and the original seed passed
100,000 cases on Go 1.26.6 before the toolchain change (52.990 seconds).
An explicit replay of both the committed failfile and the reported seed also
passes on Go 1.27.2 with the CI 1,000-case budget.

The [official download list](https://go.dev/dl/) identified Go 1.27.2 as the latest
stable release. go.mod and the Docker builder now pin that version; both GitHub
Go setup jobs read go.mod. The official `golang:1.27.2-alpine` image tag was also
verified through Docker Hub's tag API. The local Docker execution limitation
recorded above still applies.

govulncheck v1.1.4 crashed on new Go syntax and golangci-lint v2.11.3 could not
read the new export data. Tool pins advance to govulncheck v1.9.0 and
golangci-lint v2.14.0, built with Go 1.27.2. Both version files participate in the
existing CI tool-cache key. **The combined audit now passes**: govulncheck reports
no vulnerabilities, and npm audit reports zero findings.

Go 1.27 selects Unicode 17 tables in the existing x/text module. New regression
tests failed before the compatibility fix: post-Unicode-15 compositions, combining
marks and case mappings changed existing paths. NFC and legacy CollisionKey now
preserve Unicode 15 behavior. A pre-upgrade SHA-256 digest over every Unicode
scalar's legacy key still matches:
`03362113a2efd5ac944e0fcb198d64b6da42f3a0dcca6f297e052f0b46ce6844`.
The wire normalization fixture remains byte-identical and Go/TS contracts pass.
Only a fixture label's JSON replacement-character escaping changes under Go 1.27;
its base64 input and expected parse result do not change.

The server-only KeyScheme explicitly advances to
`v1:upper=17.0.0:fold=17.0.0:norm=17.0.0`, with five additional hand-written
collision pairs. [ADR 022](adr/022-go-upgrade-unicode-compatibility.md) explains
the split between wire spelling and keys and the future M5 rekey requirement.

All nine properties again passed 100,000 cases each with race detection on Go
1.27.2: vpath 56.717 seconds, classify 5.004 seconds, rules 164.118 seconds.
The repository race suite and its 17 tooling regressions also passed, as did
fixture regeneration and all 97 tests in the cross-language contract command.
The complete plugin gate also passes: strict typing, lint, 166 tests and a
production bundle.

The exact CI core-check commands now pass for rules, vpath and classify, including
the formerly failing 1,000-case rules job and coverage gates. Updated coverage is
96.59% for vpath, 96.88% for classify and 97.96% for rules. The full lint gate passes
with golangci-lint v2.14.0: zero findings, including successful negative depguard
probes. The extracted NTFS table still passes all 65,536 entries. The Windows path
test binary cross-compiles and both Linux worker commands build statically with
the Dockerfile's trimpath/linker flags.

PR #5's [Windows NTFS job](https://github.com/BarneyLaw/stele-sync/actions/runs/38136941120/job/114462638292)
was inspected after the CI report. It passed on commit 531c827 using Go 1.26.6:
15 single-component golden pairs were measured, including the observed sigma
collision, with no missed key collision. The same run's integration and worker
jobs also passed. These hosted results predate the Go upgrade; the updated
toolchain and added key pairs still need their next hosted CI run.

All three ten-minute native fuzz runs completed on Go 1.27.2 with race detection
and two workers per target, with no finding:

| Target | Executions | Result |
| --- | ---: | --- |
| vpath/FuzzParse | 384,022 | passed (603.272 seconds) |
| vpath/FuzzKey | 184,370 | passed (603.335 seconds) |
| classify/FuzzClassify | 321,942 | passed (603.281 seconds) |

To replay the reported CI failure against the fix:

```sh
GOTOOLCHAIN=go1.27.2 go test -race -count=1 ./internal/core/rules \
  -run '^TestPropFreeName$' -rapid.seed=3346917770462226820 -rapid.checks=1000
GOTOOLCHAIN=go1.27.2 CORE_PACKAGE=./internal/core/rules CORE_MODE=pr \
  node tools/tasks.mjs core-check
```

## Reproduction

```sh
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs tools
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs test-unit
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs lint
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs contract
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs fixtures-check
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs namespace-acceptance
GOTOOLCHAIN=go1.27.2 node tools/tasks.mjs audit
node tools/tasks.mjs plugin
npm audit --prefix plugin
```

For supplemental NTFS-table evidence, create a new disposable image, extract its
`$UpCase`, and run:

```sh
GOTOOLCHAIN=go1.27.2 M3_NTFS_UPCASE=/absolute/path/upcase.bin \
  go test -v ./internal/core/vpath -run '^TestNTFSUpcase$'
```

`M3_FS_PROBE=1 go test -race -v ./internal/core/vpath -run
'^TestFilesystemCollisions$'` measures the temporary directory's actual volume;
verify its filesystem before interpreting the results. Ordinary unit/contract
commands deliberately clear these optional probe variables.

Changes are committed by logical area on m3/namespace under the configured user
identity. There was no push, merge, migration, deployment or remote publication.
Revert the branch commits to roll back; phase 1 manifest contents remain frozen.
