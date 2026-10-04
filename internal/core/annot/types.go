package annot

import (
	"encoding/json"
	"errors"
)

// Kind identifies one FreeDraw annotation collection.
type Kind string

const (
	// Strokes contains pen and highlighter annotations.
	Strokes Kind = "strokes"
	// EraserPaths contains legacy eraser annotations.
	EraserPaths Kind = "eraserPaths"
	// TextItems contains text boxes.
	TextItems Kind = "textItems"
	// Shapes contains lines, rectangles, and ellipses.
	Shapes Kind = "shapes"
	// ImageItems contains embedded images.
	ImageItems Kind = "imageItems"
)

// ElementKey scopes an opaque id to its collection.
type ElementKey struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

// Family identifies a per-page entry collection.
type Family string

const (
	// PDFPageState holds "hidden" or "permanent" for one real PDF page.
	PDFPageState Family = "pdfPageState"
	// PDFPageTemplates holds the entire template object for one real PDF page.
	PDFPageTemplates Family = "pdfPageTemplates"
)

// EntryKey identifies one page in an entry family.
type EntryKey struct {
	Family Family `json:"family"`
	Page   int    `json:"page"`
}

var (
	// ErrInvalidJSON denotes malformed JSON or invalid Unicode.
	ErrInvalidJSON = errors.New("annot: invalid JSON")
	// ErrNotSidecar denotes a document outside the pinned FreeDraw layout.
	ErrNotSidecar = errors.New("annot: not a sidecar")
	// ErrDuplicateKey denotes duplicate JSON fields or page entries.
	ErrDuplicateKey = errors.New("annot: duplicate key")
	// ErrDuplicateID denotes a duplicate id within one collection.
	ErrDuplicateID = errors.New("annot: duplicate id")
	// ErrMissingID denotes a missing or invalid annotation id.
	ErrMissingID = errors.New("annot: missing id")
	// ErrStructuralKey denotes an ordinary edit to added-page structure.
	ErrStructuralKey = errors.New("annot: structural key")
	// ErrWholeDocument requests a base-checked whole-document replacement.
	ErrWholeDocument = errors.New("annot: whole document required")
	// ErrNonCanonical denotes an ambiguous, unordered, or invalid operation set.
	ErrNonCanonical = errors.New("annot: non-canonical operations")
	// ErrLimit denotes an exceeded configured budget.
	ErrLimit = errors.New("annot: limit exceeded")
	// ErrInvalidLimits denotes invalid limit configuration.
	ErrInvalidLimits = errors.New("annot: invalid limits")
	// ErrInvalidState denotes inconsistent persisted state or server ordering.
	ErrInvalidState = errors.New("annot: invalid state")
)

// Limits bounds all inputs and retained state. The first three fields must be
// positive. Zero optional fields use defaults; negative fields are invalid.
// MaxBytes includes the indented materialized file, not just its compact input.
type Limits struct {
	MaxBytes, MaxElements, MaxIDBytes                                    int
	MaxOpBytes, MaxStateBytes, MaxOps, MaxTombstones, MaxDepth, MaxNodes int
}

// DefaultLimits supplies 16 MiB files, 200,000 live elements/entries, 128-byte
// ids, 1 MiB client ops, 64 MiB state, 200,000 tombstones, 10,000 operations,
// 64 JSON levels and 2,000,000 JSON values. Exhaustion rejects the entire edit.
func DefaultLimits() Limits {
	return Limits{MaxBytes: 16 << 20, MaxElements: 200000, MaxIDBytes: 128,
		MaxOpBytes: 1 << 20, MaxStateBytes: 64 << 20, MaxOps: 10000, MaxTombstones: 200000, MaxDepth: 64, MaxNodes: 2000000}
}
func (l Limits) normalized() (Limits, error) {
	if l.MaxBytes <= 0 || l.MaxElements <= 0 || l.MaxIDBytes <= 0 {
		return Limits{}, ErrInvalidLimits
	}
	d := DefaultLimits()
	pairs := [][2]*int{{&l.MaxOpBytes, &d.MaxOpBytes}, {&l.MaxStateBytes, &d.MaxStateBytes}, {&l.MaxOps, &d.MaxOps}, {&l.MaxTombstones, &d.MaxTombstones}, {&l.MaxDepth, &d.MaxDepth}, {&l.MaxNodes, &d.MaxNodes}}
	for _, p := range pairs {
		if *p[0] < 0 {
			return Limits{}, ErrInvalidLimits
		}
		if *p[0] == 0 {
			*p[0] = *p[1]
		}
	}
	return l, nil
}

type orderKey struct {
	Version int64 `json:"version"`
	Ordinal int   `json:"ordinal"`
}
type element struct {
	value json.RawMessage
	order orderKey
}

// Sidecar owns immutable compact JSON and server bookkeeping. The zero value is
// the same empty document as Empty. None of its internal maps or bytes escape.
type Sidecar struct {
	elements   map[ElementKey]element
	entries    map[EntryKey]json.RawMessage
	meta       map[string]json.RawMessage
	tombstones map[ElementKey]int64
	version    int64
	limits     Limits
}

// Empty creates an empty sidecar using default limits.
func Empty() Sidecar { return newSidecar(DefaultLimits()) }
func newSidecar(l Limits) Sidecar {
	return Sidecar{elements: map[ElementKey]element{}, entries: map[EntryKey]json.RawMessage{}, meta: map[string]json.RawMessage{}, tombstones: map[ElementKey]int64{}, limits: l}
}
func (sc Sidecar) configured() Sidecar {
	if sc.limits.MaxBytes == 0 {
		return Empty()
	}
	return sc
}
