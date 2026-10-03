# M2 proposal: core/annot

2026-10-03 · Status: proposed

M2 is a pure package that syncs Freedraw sidecars by merging individual annotation elements. Concurrent strokes merge automatically, an erased stroke never comes back, and a sidecar with added pages falls back to whole-document replacement with a base-version check.

This document replaces the M2 section of the Implementation Guide and records the reasoning behind its departures from the original plan. It follows the guide's five-part milestone format (Goal, Interface, Design notes, Proof, Exit criteria), preceded by background and decisions, and followed by wiring notes for later milestones.

## 1. Background: what Freedraw actually does

The original M2 plan assumed one elements path, one id field, and everything else as last-writer-wins meta. Reading the Freedraw source at tag 0.13.3 (`src/types.ts`, `src/notebook/pageLifecycle.ts`, `src/annotation/eraser.ts`, `src/annotation/renderOrder.ts`, `src/stores/annotationStore.ts`) shows the real format is more involved. These findings come from source reading, not captured samples; step 0 below must confirm them.

**Elements have stable ids.** Ids look like `stroke-<Date.now()>-<8 base36 chars>`, roughly 41 random bits, which is sufficient for one user. The core design (element-level LWW keyed by id) survives. Ids are treated as opaque, length-bounded strings.

**Elements live in five top-level collections:** `strokes`, `eraserPaths`, `textItems`, `shapes`, `imageItems`. They also appear nested in `removedPages[i].annotations.*`, and `NotebookPage` (used in `appendedPages`) declares its own element arrays.

**Trash moves elements and restore moves them back with the same ids.** `removeSyntheticPageToTrash` moves elements from the top-level arrays into `removedPages[]`; `restoreSyntheticPageFromTrash` moves them back unchanged. Under naive rules, trash diffs as deletes (tombstones) and restore diffs as puts to tombstoned ids, which are dropped. Restoring a page would silently lose its annotations.

**Page references on added pages are positional.** Elements carry `page: number`. `insertSyntheticPage` places added pages after the real PDF pages (`realPdfPageCount + index + 1`) and shifts `page` on every element on later added pages. Element LWW cannot correct this: if device A inserts a page while device B draws on a later added page, B's stroke lands on the wrong page.

**Real PDF pages never shift.** Hiding or permanently deleting a real page only adds its number to `deletedPdfPages` / `permanentlyDeletedPdfPages`. So both the positional problem and the trash problem are confined to added pages. A sidecar with no added pages and an empty trash has neither.

**Some fields change on every save.** Every save rewrites `updatedAt`, and `sourcePdf` includes the local PDF's `ctime` and `mtime`, which differ per device. Without masking, every save from every device emits meta puts that flip-flop between devices. `sourcePdf.path`, `name` and `basename` do carry meaning: a PDF rename rewrites them.

**Segment erasing keeps the original id for the first fragment** and mints new ids for the others.

**Render order is `zIndex`, falling back to array index.** New elements get `max + 1` per page, so concurrent strokes from two devices tie and are ordered by array position. Freedraw renormalizes `zIndex` and stroke scales on load and then saves.

Freedraw 0.13.5 exists; the files above are unchanged between 0.13.3 and 0.13.5, so the pin is not stale.

## 2. Options considered for added pages

The usage pattern is that pages are rarely added inside a PDF; extra notes go into a separate `.md` file. Added-page concurrency must still be handled: never silently wrong, never losing data.

| Option | Mechanism | Verdict |
| --- | --- | --- |
| A: accept and document | Element puts carry whatever `page` the client wrote | Rejected. Even with detection, ink lands on the wrong page and the convergence audit cannot see it, since all devices converge on the same wrong state. |
| B: structural barrier | Reject any op based before a committed structural op | Rejected. Rebasing lands on a client that usually has the PDF open and cannot rewrite the file; it degrades to conflict sidecars while rejecting real-page ops that were never at risk. |
| C: server-side transform of `page` | Intent ops (`InsertPage`, `TrashPage`, `RestorePage`) and a 1-D index transform | Rejected for now. Correct, but clients must infer intent from diffs and tell a shift from a genuine move, and the server must splice a field inside opaque element JSON. |
| D: anchor elements to page identity | Lift `page` numbers to `real:n` / `added:<pageId>` before diffing, lower them when writing | Upgrade path. Correct and automatic, no transform, fixes trash/restore by representation. Costs a lift/lower codec, `realPdfPageCount` via pdf.js and PDF sha agreement. |
| Quarantine | Element merge only when no added pages exist; otherwise whole-document replace at head | **Chosen.** Correct (no silent misplacement, no loss), small, and only degrades on a rarely used feature. |

The cost of quarantine: in whole-document mode, all concurrent edits conflict, including strokes on real pages, and a long deferred session on a stale device ends in a conflict copy rather than a merge. A user cannot practically merge two sets of ink by hand. This is accepted because it only fires when added pages exist and two devices edit the same PDF in the same window. The metric `obsync_annot_whole_document_conflicts_total` is the trigger for building D; D replaces quarantine mode without changing the stored format.

## 3. Decisions

1. **Element merge in the common case.** The five top-level collections merge per element, keyed by `(kind, id)`. Last writer wins in server order, and a delete wins over any later put.
2. **Whole-document mode for added pages.** A sidecar whose `appendedPages` or `removedPages` is non-empty is replaced as a unit, and the replacement must be based on the current head; a stale replacement becomes a conflict copy. The sidecar returns to element mode when both arrays are empty again.
3. **Page-keyed entries merge per page.** `deletedPdfPages` and `permanentlyDeletedPdfPages` are stored as a per-page state (`hidden` or `permanent`), and `pdfPageTemplates` as one entry per page number. They use LWW per page with ordinary deletion, so two devices hiding different pages concurrently both win.
4. **Device-local fields are masked when diffing.** Inside `sourcePdf`, the `ctime` and `mtime` subfields are ignored, and `updatedAt` is ignored entirely. A rename still syncs.
5. **Two encodings.** `Marshal` produces the Freedraw file. `EncodeState` produces the internal state, which also holds order keys and tombstones. Postgres and snapshots store the state form, never the Freedraw file.

## 4. Step 0: before writing code (2 to 3 days)

Install Freedraw 0.13.3 and produce sidecars covering:

- pen, highlighter, text, shape and image annotations;
- object erase and segment erase;
- a hidden PDF page and a page template;
- added page, trash and restore (the whole-document samples);
- a renamed PDF.

Commit them to `schema/annot/samples/`. Confirm these facts and record them in `doc.go`:

| Fact to confirm | Why it matters |
| --- | --- |
| The suffix is `.annot.json`, and elements carry a string `id` | Basis of element mode |
| Elements live only in the five top-level arrays plus `removedPages`; `NotebookPage` element arrays are empty in sidecars | Element mode needs no nested walk |
| Added pages are numbered `realPdfPageCount + index + 1` | Needed if D replaces quarantine |
| Real-page references are only `page` fields, `deletedPdfPages`, `permanentlyDeletedPdfPages` and `pdfPageTemplates` | The entry families are complete |
| A rename rewrites `sourcePdf.path`, `name` and `basename` | The volatile mask must not hide renames |

ADRs to write before code:

- **011 (amend):** entry families, volatile masks, the two encodings, accepted limitations (section 12).
- **013 (write):** referenced by the plan but missing from `docs/adr/`. Defer remote sidecar writes while the PDF is open; diff local saves against the on-disk base.
- **015 (new):** quarantine for added pages, D as the upgrade path, and the trigger metric.

## 5. Goal

A pure package that:

- parses a Freedraw sidecar into an id-keyed model;
- diffs two versions into element, entry and meta operations;
- applies operations under the conflict rules;
- marks a sidecar as requiring whole-document mode when it has added pages;
- serializes both the Freedraw file and the internal state deterministically.

## 6. Interface

```go
package annot

type Kind string             // "strokes", "eraserPaths", "textItems", "shapes", "imageItems"
type ElementKey struct{ Kind Kind; ID string }
type Family string           // "pdfPageState", "pdfPageTemplates"
type EntryKey struct{ Family Family; Page int }

type Sidecar struct{ /* unexported: elements, order keys, entries, meta, tombstones */ }

type OpKind uint8            // Put, Del (exhaustive linter covers switches)
type ElementOp struct{ Op OpKind; Key ElementKey; Value json.RawMessage }
type EntryOp   struct{ Op OpKind; Key EntryKey;   Value json.RawMessage }
type Ops struct {
    Elements []ElementOp      // document order; new elements take their ordinal from here
    Entries  []EntryOp        // sorted by key
    MetaPut  map[string]json.RawMessage
    MetaDel  []string         // sorted
    Replace  bool             // server-produced only: puts ignore and clear tombstones,
                              // structural keys allowed. Never accepted from a client.
}

type Limits struct{ MaxBytes, MaxElements, MaxIDBytes int }
func DefaultLimits() Limits   // 16 MiB, 200,000, 128

// Freedraw file form
func Parse(b []byte, l Limits) (Sidecar, error)
func Marshal(sc Sidecar) ([]byte, error)             // deterministic
func CanonicalHash(sc Sidecar) [32]byte              // sha256(Marshal(sc)), used by the audit

// Internal state form (Postgres heads, snapshots, plugin shadows)
func EncodeState(sc Sidecar) ([]byte, error)
func DecodeState(b []byte, l Limits) (Sidecar, error)

// Wire form of ops (proto calls ParseOps; rejects non-canonical, like textop.Parse)
func ParseOps(raw []byte, l Limits) (Ops, error)
func (o Ops) MarshalJSON() ([]byte, error)

func Empty() Sidecar
func Diff(base, cur Sidecar) (Ops, error)            // ErrWholeDocument if either side requires it
func ReplaceOps(head, doc Sidecar) Ops               // full diff with Replace=true, structural keys included
func RequiresWholeDocument(sc Sidecar) bool          // appendedPages or removedPages non-empty

type Result struct {
    Sidecar   Sidecar
    Effective Ops             // ops minus dropped puts; what gets logged and broadcast
    Dropped   []ElementKey    // puts to tombstoned ids; the engine counts them
}
func Apply(head Sidecar, ops Ops, at int64) (Result, error)
func PruneTombstones(sc Sidecar, before int64) Sidecar

var ( ErrInvalidJSON, ErrNotSidecar, ErrDuplicateKey, ErrDuplicateID, ErrMissingID,
      ErrStructuralKey, ErrWholeDocument, ErrNonCanonical, ErrLimit, ErrInvalidLimits,
      ErrInvalidState error )
```

Versions are plain `int64` because `core` cannot import the engine's `Version` type; the engine converts at the boundary.

Wire shape of an edit:

```json
{"elements":[{"put":{"kind":"strokes","id":"stroke-1"},"value":{"...":"..."}},
             {"del":{"kind":"shapes","id":"shape-9"}}],
 "entries":[{"put":{"family":"pdfPageState","page":3},"value":"hidden"}],
 "meta":{"put":{"sourcePdf":{"...":"..."}},"del":["someKey"]}}
```

## 7. Semantics (the rules `doc.go` must state)

**Element put (`Replace=false`).**

- To a tombstoned key: dropped and reported in `Result.Dropped`.
- To an existing key: replaces the value and keeps the element's order key.
- To a new key: inserts it with order key `(at, ordinal)`, where the ordinal is the put's index in `Ops.Elements`.

**Element delete.** Removes the element and tombstones the key at `at`. Deleting an unknown key still records a tombstone, which is harmless and safer.

**Replace semantics (`Replace=true`).** The resulting element set is exactly the submitted document. Puts clear tombstones for keys present in the document; removed keys get tombstoned. This makes trash and restore safe inside whole-document mode. Without it, restoring a page would be dropped by delete-wins. It is sound because a replace is only accepted at head, so it cannot be stale by definition.

**Entries and meta.** LWW put and delete, with no tombstones: hide, unhide and hide again must all work. Structural keys (`appendedPages`, `removedPages`) in a non-replace `Ops` fail with `ErrStructuralKey`. A non-replace `Ops` against a head that requires whole-document mode fails with `ErrWholeDocument`.

**Order.** Within each collection, elements are ordered by `(created version, ordinal)`. This preserves the device's relative array order, which matters because Freedraw uses array position as the fallback render order when `zIndex` is missing. Sorting by id would scramble it.

**Why it converges.** Ops on different keys commute. Ops on the same key are resolved by the server's total order. Every client applies the same `Effective` ops, with the same `at`, to the same prior state, so every client computes identical state and identical `Marshal` bytes. No transform is needed.

## 8. Implementation guide

Build in this order, with each file's tests green before the next.

1. **`profile.go`.** The Freedraw facts as data: collection names, entry families with their array encodings, structural keys, volatile masks, and the known top-level key order.

2. **`parse.go`.**
   - Check `len(b) <= MaxBytes` and `utf8.Valid` first.
   - Walk the top level with `json.Decoder.Token()`. Reject duplicate object keys at every level with a token-walk check, because the stdlib decoder silently keeps the last duplicate. This is the same class of trap as M1's lone surrogates.
   - Read each element as `json.RawMessage` and run `json.Compact` on it. Never decode values into Go types, so numbers, unknown fields and string escapes survive byte-exactly.
   - Extract only `id`, which must be a non-empty string within `MaxIDBytes`. Reject duplicate ids within a kind.
   - Convert the page arrays into entries.
   - Keep structural keys as opaque meta.

3. **`marshal.go`.**
   - Assemble the output bytes directly: known top-level keys in profile order, then unknown keys sorted, then `json.Indent` with two spaces to match Freedraw's style.
   - Do not run the document through `json.Marshal`. It HTML-escapes `<`, `>` and `&` inside `RawMessage` values, which would silently change text annotations. `json.Compact` and `json.Indent` do not escape.

4. **`state.go`.** A versioned envelope (`{"v":1,...}`) holding elements with their order keys, entries, meta and tombstones. Element values are embedded as raw bytes, and `DecodeState` validates every invariant (`ErrInvalidState`).

5. **`ops.go`.** `ParseOps` rejects non-canonical input:
   - a duplicate key within one op;
   - put and delete of the same key in one op;
   - unsorted entries or meta deletes;
   - `Replace` set, since clients cannot send it.

6. **`diff.go`.**
   - Elements: a put when the compacted bytes differ, in `cur` document order; deletes sorted.
   - Entries and meta: sorted. Meta is compared through the volatile masks.
   - Return `ErrWholeDocument` if either side requires it. The caller then sends the full document instead.

7. **`apply.go`.**
   - Copy on write: never mutate `head`'s maps.
   - Iterate only over op slices and sorted keys, never over a map, so the result is independent of map ordering.

8. **`prune.go`, `hash.go`, `mode.go`.** Small. Pruning never changes content.

Expected effort: about two part-time weeks including step 0, against the original plan's one.

## 9. Proof

Write the model test and the first two properties before `apply.go`. Rapid generators draw ids from a tiny alphabet (about 6 ids per kind). Uniform random ids almost never collide, and collisions are the only interesting cases.

| Test | Asserts |
| --- | --- |
| `TestPropDiffApply` | For `b` derived from `a` by random edits: content of `Apply(a, Diff(a,b))` equals content of `b`, comparing elements, entries and masked meta |
| `TestModelAgreement` | N simulated devices, each diffing from its own stale base, with ops interleaved randomly at the server. `Apply` matches a small, obviously correct reference model (plain maps, written test-only) on content and order |
| `TestModelFindsPlantedBug` | The same harness, given an apply wrapper that ignores tombstones, fails within 1,000 cases. A harness that cannot catch a planted bug proves nothing |
| `TestPropNoLostAckedPut` | In the model run, every applied put is present at the end unless a delete for its key was applied |
| `TestPropDeleteWins` | After a delete of `k`, no later state from non-replace ops contains `k` |
| `TestPropCommutesOnDisjointKeys` | Ops on disjoint keys give identical state in either order |
| `TestPropEffectiveReplays` | `Apply(head, r.Effective, at)` reproduces `r.Sidecar` with no drops; this is what clients rely on |
| `TestPropMarshalIdempotent` | `Marshal(Parse(Marshal(x))) == Marshal(x)` byte for byte |
| `TestPropStateRoundTrip` | `DecodeState(EncodeState(x))` deep-equals `x`, including order keys and tombstones |
| `TestPropOpsRoundTrip` | `ParseOps(o.MarshalJSON())` equals `o` |
| `TestMarshalDeterministic` | Same model built via 100 different op orders: one byte sequence |
| `TestUnknownFieldsPreserved` | Extra fields at top level, inside elements and inside entries survive a round trip, byte-exact |
| `TestNoHTMLEscaping` | A text annotation containing `<b>&` round-trips unchanged |
| `TestVolatileMasked` | Documents differing only in `updatedAt` or `sourcePdf.mtime` diff to empty ops; a changed `sourcePdf.path` diffs to a meta put |
| `TestEntryFamilies` | Concurrent hides of different pages both survive; hide, unhide, hide works |
| `TestModeSelection` | The mode follows `appendedPages` / `removedPages`; `Diff` and `Apply` return `ErrWholeDocument` and `ErrStructuralKey` correctly |
| `TestTrashRestoreUnderReplace` | Trash then restore via `ReplaceOps`: every element returns, and tombstones are cleared for restored keys |
| `TestPruneTombstones` | Tombstones older than the cutoff go, newer ones stay, content is unchanged |
| `TestSamplesRoundTrip` | Every committed sample: `Marshal(Parse(x))` re-parses to the same model; Freedraw opens the output (manual, recorded once in the PR) |
| `TestLimits` / `TestInvalid` | Oversize input, too many elements, a long id, a duplicate key, a duplicate id, a missing id and invalid UTF-8 each return their named error |
| `FuzzParse`, `FuzzParseOps`, `FuzzDecodeState`, `FuzzApply` | No panic. Anything accepted re-serializes, and the result re-parses |

**Cross-language fixtures.** `make fixtures` generates `schema/annot/{parse,diff,apply,state,invalid}.json` from Go, and CI fails on any diff. Unlike M1 there is no independent oracle. These fixtures prove the M13 TypeScript port matches Go, not that Go is right. Go's correctness rests on the model test, the planted bug and the properties.

## 10. Exit criteria

- Properties pass at 100,000 cases each, and the model test passes at 100,000 interleavings.
- The planted bug is caught.
- Four fuzz targets run 10 minutes each with no findings.
- Package coverage is above 90%.
- Real samples round-trip and still open in Freedraw.
- Fixtures are committed.
- ADRs 011 (amended), 013 and 015 are merged.

## 11. How it wires into later milestones

End to end, the flow is:

1. Freedraw saves the sidecar.
2. The plugin runs `Diff(on-disk base, saved file)` and submits either the ops (`AnnotEdit`) or the whole document (`AnnotReplace`).
3. `proto` parses the payload, and the engine actor runs `DecodeState(head)`, `Apply` and a size check, then commits the state, the `Effective` ops and the change in one transaction.
4. The hub broadcasts the `Effective` ops with the new version.
5. Other clients apply them to their shadow state, then `Marshal` and write the file, deferred until the PDF closes.

| Milestone | Uses from `annot` | What M2 requires of it |
| --- | --- | --- |
| M3 classify / rules | `Parse` decides Class B (suffix, parse success, at most 16 MiB) | New rule row: an `AnnotReplace` is accepted iff `base == head`, otherwise a conflict copy. The conflict-copy name must still end in `.annot.json` but must not pair with the PDF |
| M4 proto | `ParseOps` inside `AnnotEdit`; `Parse` inside a new `AnnotReplace` body | New reject code `whole_document_required`, so the client knows to resend as a replace. `Replace=true` from a client is impossible because `ParseOps` rejects it |
| M5 storage/pg | `EncodeState` / `DecodeState` | **Schema change.** `files.content_json` and Class B `ops.payload` must not be `jsonb` holding raw element values: `jsonb` does not preserve key order or duplicate keys and re-renders numbers, which breaks byte-exact preservation and the hash. Use `bytea` (or `text`) for the state and op encodings |
| M7 engine | `RequiresWholeDocument`, `Apply`, `ReplaceOps`, `Marshal` for the size check | **The retention window applies to Class B too.** Element ops do not need a base for correctness, but tombstone pruning does: a device offline past retention could otherwise resurrect an erased stroke. Creates go through `Apply(Empty(), ReplaceOps(Empty(), doc), v1)`, so the op log is uniform. The reference client and simulator gain Class B with a "PDF open" flag, plus the no-lost-acked-put invariant |
| M10 compaction / GC | `EncodeState` for snapshots; `PruneTombstones` | Snapshots must store the state form, or `TestSnapshotPlusOpsEqualsHead` fails, because a Freedraw file has no order keys or tombstones. Prune tombstones at the same cutoff as ops |
| M11 Canvas bridge | Nothing directly | A deleted Canvas PDF leaves its sidecar in place (user-owned). A re-upload with a different page count breaks Freedraw's own real-page anchoring; out of scope, but worth a notice |
| M13 plugin | A TypeScript port of `annot` that passes `schema/annot/*` | The shadow is the state form. While deferred, the convergence audit compares the shadow hash, not the disk. On `whole_document_required`, resubmit as a replace; a stale replace yields a conflict copy in the conflicts view |
| M14 verification | `CanonicalHash` in the S4 audit | Add metrics `obsync_annot_dropped_puts_total` and `obsync_annot_whole_document_conflicts_total`. The second one is ADR 015's trigger for building D |

## 12. Accepted limitations (record in ADR 011)

- **Stale LWW overwrite.** A put replaces the whole element, so a device editing an element against stale state overwrites a concurrent edit to the same element.
- **Duplicate ink from segment erase.** A segment erase that races a move of the same stroke can leave the whole original stroke beside the erased fragments.
- **Normalization bursts.** Freedraw renormalizes `zIndex` and stroke scales on load and then saves, so a device can emit a burst of puts after a merge. Those puts can overwrite concurrent remote edits to the same elements.

None of these lose data or diverge. All three are intent losses that LWW accepts by design.

## 13. Documents to update

| Document | Change |
| --- | --- |
| Architecture Specification, section 3 | Class B storage as `bytea` state encoding; op payload encoding for Class B |
| Architecture Specification, section 4.2 | Entry families, volatile masks, whole-document mode, replace semantics |
| Implementation Guide, M2 | Replace with sections 5 to 10 of this document |
| Implementation Guide, M4, M5, M7, M10, M13 | The requirements in section 11 |
| ADRs | Amend 011; write 013 and 015 |
