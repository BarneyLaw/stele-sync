# M2 annotation core: design and verification

## Scope and provenance

This branch implements the pure Class B core described in the architecture and
M2 guide. Phase 1 still imports Canvas files through immutable manifests; phase 2
will add one server authority, per-file versions, and a committed vault feed.
`annot` decides values only: storage, wall clocks, PDF sessions, transport,
idempotency, and retention policy belong to later milestones.

The user explicitly requested implementation, documentation, a separate branch,
and commits under their Git identity. This authorizes AI-drafted core code and
tests for this phase despite Engineering Guidelines §9's default division of
work. The author must review and explain the implementation before merging.
Commit author and committer use the existing `barney <leifsenlaw993@gmail.com>`
configuration, without additional author trailers.

## FreeDraw evidence

The compatibility target remains **0.13.3**, tag commit
`e58a10ca5438b3a05a7642fc213d25d6b99f23fb`, not the moving default branch.

- [Types](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/types.ts):
  `AnnotationDocument` contains five top-level id-bearing arrays: `strokes`,
  `eraserPaths`, `textItems`, `shapes`, `imageItems`. Pen and highlighter both
  live in `strokes`; images include embedded `dataUrl` content. Each element
  has an `id`; annotation `page` is a numeric reference.
- [Store](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/stores/annotationStore.ts):
  `getSidecarPath` appends `.annot.json` to the whole PDF path, e.g.
  `Lecture.pdf.annot.json`. `createEmptyDocument` writes format version 8.
  Other fields include `version`, `sourceFile`, `sourcePdf`, `updatedAt`,
  `pdfPageTemplates`, `nativePageTemplatesEditable`, `appendedPages`,
  `deletedPdfPages`, `permanentlyDeletedPdfPages`, and `removedPages`.
  Inserted-page descriptors and removed-page archives remain opaque metadata;
  their arrays are not independently merged by this milestone.
- The same store compares the current raw file with the loaded baseline before
  saving and again inside `Vault.process`. A mismatch rejects the write.
  `saveRecoveryCopy` uses `<pdf path>.recovery-….annot.json`.
- [Session save path](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/main.ts):
  `savePendingDocument` calls `preserveFailedSave` on failure. The future client
  must defer remote disk writes while a PDF is open and diff local saves against
  the disk baseline, not the newer server shadow.
- [Render ordering](https://github.com/vividasasana/freedraw-pdf/blob/e58a10ca5438b3a05a7642fc213d25d6b99f23fb/src/annotation/renderOrder.ts):
  explicit `zIndex` takes precedence over array-order fallbacks. Preserve it as
  content; do not infer it from client timestamps.

These are source observations, not a claim of a manual Obsidian test. No real
vault/sample was included in this checkout. Synthetic fixtures will be labeled
as such. Capturing real pen/highlighter/eraser/text/page/image samples and opening
the canonical output in Obsidian remains a release gate.

## Implementation sequence and rationale

1. Bounded strict JSON plus the immutable sidecar model, tests first. Preserve
   unknown fields recursively, reject ambiguous duplicate keys/ids and malformed
   Unicode, and retain numeric values without a float64 conversion. Model all
   configured top-level collections; reserve them from metadata writes.
2. Diff and atomic apply, with properties written first. Replace whole elements,
   retain deletion tombstones, report suppressed puts, and preserve first-created
   versions. Add explicit metadata deletes because absence and JSON null differ.
3. Durable state encoding with validation. A PDF sidecar cannot carry server
   tombstones or creation versions; snapshotting only the materialized JSON would
   allow resurrection after restart. Keep that state in a separate server codec.
4. Compatibility fixtures, adversarial/fuzz/property verification, and executable
   acceptance commands. Record measured results and remaining manual checks.

Each logical implementation portion includes its tests and an update here, and
is committed separately using Conventional Commits.

## Contract clarifications

- Schema configuration lists the top-level collections and id field. FreeDraw's
  actual layout needs multiple collections; arbitrary nested JSON-path machinery
  is unnecessary for this version.
- Imported elements start at creation version zero, with ids breaking ties.
  New elements use the server-assigned version; replacements retain creation
  order. Optional missing arrays normalize to empty arrays. Existing `zIndex`
  values survive; legacy files relying on array order need manual visual review.
- `CanonicalHash` compares semantic content (id-sorted collections and metadata),
  excluding server bookkeeping. Hashing creation-ordered materialization instead
  makes M2's unrestricted diff/apply and disjoint-id properties false: importing
  a target has no creation history, and swapping server order changes creation
  order. The byte-convergence audit must also compare SHA-256 of `Marshal` output
  for replicas replaying the same assigned versions. Neither check replaces the
  other.
- Diff/apply equivalence concerns ordinary imported snapshots, not a request to
  resurrect an id already tombstoned in server state. Disjoint-id commutation
  concerns semantic content; metadata conflicts and creation order obey server
  order.
- Apply is atomic and returns both the new sidecar and dropped put ids. Diff can
  fail for incompatible schemas. All operations are validated before use, even
  puts that a tombstone will suppress.
- No tombstone-pruning API is added: only the future retention owner can prove
  that a tombstone is safe to remove.

## Verification log

Initial checkout: clean `main` at `1a887d6`; work branch `m2/annot`.
Host tools report Go 1.27.1 and Node 25.5.0; repository pins Go 1.26.6/Node 24.
Acceptance commands must state the toolchain actually used.
