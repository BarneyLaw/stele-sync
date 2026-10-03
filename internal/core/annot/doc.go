// Package annot implements pure, bounded FreeDraw sidecar synchronization.
//
// FreeDraw 0.13.3 (e58a10ca5438b3a05a7642fc213d25d6b99f23fb) appends
// .annot.json to the complete PDF path: Lecture.pdf.annot.json. Format version 8
// stores id-bearing annotations in strokes (pen/highlighter), eraserPaths,
// textItems, shapes, and imageItems. Other top-level fields include version,
// sourceFile, sourcePdf, updatedAt, pdfPageTemplates, nativePageTemplatesEditable,
// appendedPages, deletedPdfPages, permanentlyDeletedPdfPages, and removedPages.
// These and future fields remain opaque JSON, including nested page archives.
//
// Sidecar owns immutable compact JSON. Ordinary operations replace whole
// elements keyed by (kind, id); deletion wins over later puts. Added pages or
// trash require a whole-document replacement authorized at the current head.
// Per-page state and templates use ordinary last-writer-wins puts and deletes.
// Materialization orders each collection by creation version and ordinal,
// preserving imported array order. CanonicalHash hashes the exact Marshal bytes.
// Client clocks never participate in ordering. Diff masks updatedAt and the
// sourcePdf ctime/mtime fields; all other unknown fields remain opaque.
//
// JSON input rejects duplicate keys, duplicate ids within a kind, invalid UTF-8
// and unpaired surrogate escapes. Compact raw values retain number spelling,
// key order and escapes; output uses two-space indentation without HTML escaping.
// Limits bound bytes, depth, nodes, elements, operations, ids, and tombstones.
// The server retains order keys and tombstones in EncodeState, never in the
// FreeDraw file. Pruning requires the retention owner's proof that older
// operations cannot arrive; enforce the Class B retention window before Apply.
//
// FreeDraw keeps an open session's baseline and rejects conflicting disk saves.
// The future client must defer remote writes while the PDF is open and diff its
// saves against the disk baseline. See docs/annot-development.md for pinned
// source evidence, contract decisions, and the pending manual Obsidian checks.
package annot
