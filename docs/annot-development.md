# M2 annotation core: implementation record

The authoritative contract is the user-supplied [M2 proposal](../internal/core/annot/M2-core-annot-proposal.md), replacing the old Implementation Guide's M2 section. The initial uncommitted codec draft was superseded when that proposal arrived.

The user explicitly authorized implementation and documentation on a separate branch, overriding the engineering guidelines' default human-only core authorship. Code and tests are AI-drafted and require review and explain-back before merge. All commits use the configured author and committer barney <leifsenlaw993@gmail.com>, with no additional authors.

## Evidence

FreeDraw 0.13.3 is pinned at e58a10ca5438b3a05a7642fc213d25d6b99f23fb.

- [Types](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/types.ts) define five annotation collections and all page metadata.
- [Store](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/stores/annotationStore.ts) appends .annot.json to the full PDF path, writes format version 8, and rejects saves against an externally changed baseline.
- [Page lifecycle](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/notebook/pageLifecycle.ts) shifts numeric references on page insertion and preserves annotation ids through trash/restore. This motivates quarantine.
- [Rendering](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/annotation/renderOrder.ts) falls back to array order when zIndex is missing or tied.
- The user's local Lecture 7 sidecar is 8,125 bytes, version 8, with eight text annotations and the expected 16 top-level keys. Its vault has FreeDraw 0.13.3 installed. Lecture content is not copied into committed fixtures.

## Logical portions

1. Profile, bounded strict parser, deterministic file encoding, and codec tests.
2. Operation codec, diff, atomic apply, entry families, masking, quarantine, and their properties/model tests.
3. Durable state encoding and retention-controlled pruning with tests.
4. Shared fixtures, compatibility evidence, acceptance tooling, and documentation.

Each portion is committed with its tests and rationale.

## Executable contract clarifications

- Dropped puts disappear from Effective, shifting subsequent indices. Allocate new order ordinals from the effective sequence for both original apply and replay; otherwise the proposal's effective-replay property is false.
- Whole-document replacement puts every target element in document order and resets order keys. Ordinary puts preserve existing order. Replacement must reproduce the submitted array order.
- Disjoint operations commute on content with existing creation order held fixed. Swapping server creation versions of new elements necessarily changes order; byte convergence requires the same effective operations and assigned versions in the same sequence.
- Raw compact JSON is content: 1 and 1.0, or differently ordered objects, may diff. This follows the proposal's preservation rule. CanonicalHash is SHA-256 of actual Marshal bytes.
- Limits add optional budgets for operation/state bytes, tombstones, depth, and token count. Zero optional fields select defaults; primary limits must be positive.
- No client operation can set Replace. The engine must check base == head before constructing trusted ReplaceOps, and enforce the retention window before applying ordinary edits or pruning tombstones.

## Verification log

Started on clean main at 1a887d6; branch m2/annot. Host tools are Go 1.27.1 / Node 25.5.0. Go 1.26.6 was downloaded and selected explicitly for package verification. Repository-pinned lint tools are installed locally. Baseline core tests passed.

The file codec passed its deterministic tests, 1,000 randomized round trips,
and the user's real sample round trip under the race detector on Go 1.26.6.
The parser preserves the sample without exposing or committing lecture content.

The operation layer passed its deterministic tests and 1,000 cases per property
under the race detector, including the independent three-device stale-baseline
model and planted-tombstone-bug detection. The model compares both values and
array order against plain maps plus independent insertion ranks. The effective
sequence regression specifically puts a suppressed element before a new one.

The state codec passed race tests and strict package lint. Its envelope validates
version, complete unique order keys, live/dead disjointness, tombstone versions,
sorted bookkeeping and agreement between stored order and the embedded file.
Pruning is strictly before the supplied retention cutoff and preserves content.

Review found that checking the file-size limit after json.Indent could allocate
far more memory than the configured limit. Marshal now counts exact indentation
expansion before allocation, with a nested-input regression. A second regression
keeps the virtual entry-family name pdfPageState available as opaque top-level
metadata; only actual FreeDraw array keys are reserved. Ordinal bounds and trusted
batch bounds also avoid adding configurable integer ceilings together.

Verification now includes eight actual-plugin synthetic captures, all reopened
after Go canonicalization; five generated cross-language contract files; and
an acceptance task wired into fixture/contract checks and automatic fuzz-target
discovery. The complete measured results and the pre-existing npm audit failure
are recorded in [annot-verification.md](annot-verification.md).

The final documentation portion updates the architecture and downstream
milestones together: storing only a materialized file would lose tombstones,
JSONB would rewrite preserved bytes, and immediate remote disk writes would
conflict with FreeDraw's open-session baseline. ADRs 013/015 record the deferral
and page-quarantine decisions; ADR 011 is amended as the supplied proposal
explicitly requires. These are integration requirements, not claims that the
future engine or client has already been implemented.
