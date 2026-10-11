# M3 verification record

Verified locally on 2026-10-11, Linux amd64, with **Go 1.26.6 explicitly selected**,
Node 25.5.0 and the repository's pinned Go tools. CI continues to use its Node 24
pin. These are local results, not a claim that hosted CI has run. See
[implementation notes and authoritative sources](m3-development.md) for the
contract and its necessary corrections.

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

macOS/APFS and iOS measurements are deferred at the user's explicit request.
Android/Obsidian path-event normalization and FreeDraw partial-write visibility
were not measured here (no Android test environment was available). The **10-second
quiet default remains provisional**. M13 must measure these in a disposable vault
and preserve original local spelling when mapping paths to the NFC wire form.
Docker-dependent integration/worker-image tests were not rerun; their existing
CI gates remain enabled. Phase 2 engine/storage/protocol integration is still the
future milestones' responsibility, not functionality supplied by this pure core.

## Existing Go audit blocker

The combined audit gate is **not green** with the existing Go 1.26.6 pin.
Govulncheck reports ten reachable standard-library advisories:
GO-2026-6617, 6613, 6612, 6611, 6610, 6609, 6608, 6607, 6605 and 6603.
An isolated checkout of the original branch head **cf698b0** produced the same
advisory set. The scanner also reports uncalled package/module findings.

The reported standard-library fix is Go 1.26.9; see, for example,
[GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617). The repository's
[Engineering Guidelines §3.1](../architecture/obsync-single%20Engineering%20Guidelines.md#31-tooling-ci-fails-on-any-finding)
require toolchain upgrades in their own PR. The Go pin and the audit gate remain
unchanged here; a separate toolchain security update is required for a fully green
CI run. No vulnerability suppression or audit bypass was added.

The npm finding was repaired independently by updating only the source-map-js
lock entry from 1.2.1 to 1.2.2, preserving all optional platform dependencies.
That patch addresses [GHSA-68fv-2mgg-jv7q](https://github.com/advisories/GHSA-68fv-2mgg-jv7q).
A clean npm ci, audit, lint, tests, type checking and bundle all passed afterward.

## Reproduction

```sh
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs test-unit
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs lint
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs contract
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs fixtures-check
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs namespace-acceptance
node tools/tasks.mjs plugin
npm audit --prefix plugin
```

For supplemental NTFS-table evidence, create a new disposable image, extract its
`$UpCase`, and run:

```sh
GOTOOLCHAIN=go1.26.6 M3_NTFS_UPCASE=/absolute/path/upcase.bin \
  go test -v ./internal/core/vpath -run '^TestNTFSUpcase$'
```

`M3_FS_PROBE=1 go test -race -v ./internal/core/vpath -run
'^TestFilesystemCollisions$'` measures the temporary directory's actual volume;
verify its filesystem before interpreting the results. Ordinary unit/contract
commands deliberately clear these optional probe variables.

Changes are committed by logical area on m3/namespace under the configured user
identity. There was no push, merge, migration, deployment or remote publication.
Revert the branch commits to roll back; phase 1 manifest contents remain frozen.
