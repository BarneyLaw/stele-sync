# M2 annotation verification

Verified on 2026-10-04 (Singapore), Linux amd64, using Go 1.26.6 explicitly.
Host Node was 25.5.0; the repository Node pin was not changed. Local tool versions
come from tools/versions.json. This is local evidence, not a hosted CI result.

## Application compatibility

FreeDraw PDF **0.13.3**, installed assets from the user's test vault, under
Obsidian **1.13.7**. Source pin:
`e58a10ca5438b3a05a7642fc213d25d6b99f23fb`.

An isolated temporary Obsidian profile was launched under Xvfb, with a synthetic
one-page PDF and a copy of those plugin assets. The user's running vault was
only read for its supplied sample and plugin assets. Automation used local
Chromium debugging to invoke actual plugin pointer handlers, its inline editor,
image insertion, eraser and page methods, and the page-trash confirmation button.

Eight plugin-written captures are in
[schema/annot/samples](../schema/annot/samples/README.md). They cover pen,
highlighter, shape, text, embedded image, segment erase, object erase, page
template, real-page hiding, added page, trash, restoration and PDF rename.
All eight passed Go file/state round trips. All eight Go-canonicalized outputs
were then reopened in the actual plugin with matching live annotation, added-
page and trash counts. Rendering was requested for each reopened document.

Observed behaviors:

- Segment erase split a pen stroke and retained its original id for the first
  fragment; additional fragments received new ids.
- The inserted page was numbered 2 after a one-page PDF; its inline element
  arrays were empty and its ink lived in the top-level strokes collection.
- Trash moved that ink into removedPages; restore returned the same id.
- Renaming Synthetic.pdf to Synthetic-renamed.pdf rewrote sourcePdf path, name
  and basename.
- An externally added synthetic text item did not reload into an open session.
  Saving an independent local edit left the session dirty, retained the external
  disk content and generated a separate recovery sidecar.

The user's real format-8 sidecar (8,125 bytes, eight text annotations) also passed
a private round trip using ANNOT_SAMPLE. Its content is not committed.

These are automated checks against the actual desktop app, not a manual visual
or Android usability review. M13's two-device deferral scenario and M14's full
server convergence tests remain future milestones; this package does not claim
to implement those transports or sessions.

## Core verification

- Repository-wide `go test -race ./...`: passed.
- `node tools/tasks.mjs lint`: passed, including tidy, formatting, vet, all
  configured linters and negative dependency-import probes.
- `govulncheck ./...`: no vulnerabilities found.
- Annotation deterministic and property tests: passed at the routine budgets.
- Coverage with all fixture and boundary tests: **93.7%**.
- The contract gate passed, including annotation fixtures and the existing
  90 TypeScript tests. Two independent annotation fixture generations matched
  byte-for-byte; the repository fixture drift/tracking gate also passed.
- The independent stale-device model detects the intentionally broken
  tombstone-forgetting wrapper within the required 1,000-seed budget.
- Initial ten-minute fuzz runs passed: Parse 3,675,382 executions; ParseOps
  4,072,054; DecodeState 3,388,228; Apply 3,660,591. A second set with the race
  detector checked the final allocation-bound revision; all four passed.

The 100,000-case run passed for every property and the reference model in
1,694.888 seconds. Each model case uses 12 interleaved submissions across three
stale clients, so TestModelAgreement exercised 1.2 million submissions. The
additional no-lost-live-key property runs another eight-step model per case.
The strengthened effective-replay property also compared exact durable-state
bytes in a separate final 100,000-case run, passing in 53.698 seconds.

Final ten-minute runs with the race detector and two workers per target:

| Target | Executions | Result |
| --- | ---: | --- |
| FuzzParse | 355,211 | passed |
| FuzzParseOps | 402,530 | passed |
| FuzzDecodeState | 233,214 | passed |
| FuzzApply | 397,262 | passed |

`npm audit --prefix plugin --package-lock-only` reports three existing moderate
advisories through moment, obsidian and eslint-plugin-obsidianmd
([GHSA-4p3w-j4w9-5jqw](https://github.com/advisories/GHSA-4p3w-j4w9-5jqw)). The
lockfile is unchanged. npm suggests a breaking SDK downgrade, which is outside
M2. Consequently the complete CI audit gate is not green; Go vulnerability
checking is green. Infrastructure integration, worker builds and Android/manual
usability checks were not rerun for this pure-core change.

## Reproduction

```sh
GOTOOLCHAIN=go1.26.6 go test -race ./internal/core/annot
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs annot-acceptance
GOTOOLCHAIN=go1.26.6 go test ./internal/core/annot -run '^TestAnnotContractFixtures$' -count=1
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs lint
```

`ANNOT_SAMPLE=/absolute/path/to/file.annot.json` enables the optional private
sample check. `ANNOT_OUTPUT_DIR=/absolute/path/to/disposable/output` exports
canonical versions of the committed samples for an application reopen check.
Neither is required by CI. Use a disposable vault and close PDFs after saves
settle before swapping sidecars.

`node tools/tasks.mjs fixtures` regenerates five annotation contract files.
`fixtures-check` and `contract` verify them; CI also discovers all four new fuzz
targets. The TypeScript annotation port consumes these contracts in M13; these
goldens are not an independent oracle. The independent map model and properties
are the correctness evidence.

## Review and handoff

Implementation and tests are AI-drafted under the user's explicit instruction.
Review the supplied proposal, [implementation rationale](annot-development.md),
and ADRs 011/013/015 before merge. The proposed API's necessary clarifications
are documented there: effective ordinals, trusted replacement ordering and the
scope of disjoint-key commutation. Rollback is a revert of this branch's commits;
no deployment, migration or remote publication occurred.
