// Package textop implements canonical UTF-16 text operations. It is pure: no
// I/O, clocks, normalization, or mutable global policy. Doc and Op are immutable;
// a Builder is mutable and must not be copied or shared concurrently.
//
// Counts are UTF-16 units. All insertion text is Unicode scalar text; application
// boundaries may split grapheme clusters but must not split surrogate pairs.
// Canonical operations contain positive nonempty components, merge adjacent
// components of the same kind, and put insertion before deletion at a position.
// Retains consume and produce, deletes only consume, inserts only produce.
//
// The case tables below derive the cursor rules. R/I/D denote retain/insert/delete;
// x and y are current residual lengths and n=min(x,y). For paired consumption,
// x<y leaves y-x, x=y exhausts both, x>y leaves x-y. For independent consumption,
// the WHOLE component is consumed for all three length relations. Every iteration
// therefore advances a cursor or reduces a finite residual; draining independent
// components after the other cursor ends also terminates.
//
// Compose a then b (b sees a's output):
//
//	       b:R                 b:I                    b:D
//	a:R    emit R(n), both     emit b.I(y), b only     emit D(n), both
//	a:I    emit a.I(n), both   emit b.I(y), b only     emit nothing, both
//	a:D    emit a.D(x), a only emit a.D(x), a only     emit a.D(x), a only
//
// R/R preserves text through both edits. R/D removes original text. I/R keeps
// inserted text, while I/D cancels it. a.D never enters b's input, so it passes
// independently in every column. b.I has no a-output to consume and passes
// independently when a is not deleting; canonicalization places that insertion
// before a pending deletion. A split of a.I checks scalar boundaries even when
// b deletes it. Thus composed application has the sequential effect.
//
// Transform concurrent a and b (ap after b, bp after a):
//
//	       b:R                    b:I                       b:D
//	a:R    ap.R(n),bp.R(n),both    ap.R(y),bp.I(y),b only    bp.D(n),both
//	a:I    ap.I(x),bp.R(x),a only ap.I(x),bp.R(x),a only    ap.I(x),bp.R(x),a only
//	a:D    ap.D(n),both           ap.R(y),bp.I(y),b only    nothing,both
//
// R/R preserves the shared text on both paths. D/R and R/D remove it on the
// path where it survived the first edit. D/D removes it once, never twice.
// a.I is retained by bp and b.I by ap: neither concurrent insertion is deleted
// by the other's original range. I/I gives a priority, yielding a's text then
// b's on both paths. The rules give TP1 for context-valid operations; they do
// not assert arbitrary multi-party transformation properties or delivery rules.
//
// Transform cannot detect boundaries inside unseen original text. The engine
// must ValidateAgainst the actual base before rebasing, and the actual head
// before application. For base "😀", [1,-1] is invalid even though rebasing it
// against [-2] can hide that invalidity. This package accepts NUL; Class A file
// classification and engine ingress own its rejection before persistence.
//
// JSON equality means exact ordered typed components and exact insertion bytes,
// not identical escape spelling across Go and JavaScript. Algebraic codec
// round-trips use a MaxJSONBytes budget sufficient for the encoded output;
// default Parse remains a separate bounded ingress policy (ADR 002).
package textop

import (
	"unicode/utf16"
)

// Doc contains well-formed UTF-16, with immutable private storage.
// The zero value is the empty document Doc{}.
type Doc struct {
	u []uint16
}

// DocFromString converts text under DefaultLimits.
func DocFromString(s string) (Doc, error) {
	return DefaultLimits().DocFromString(s)
}

// DocFromString converts text under l's UTF-16 unit and UTF-8 byte bounds.
func (l Limits) DocFromString(s string) (Doc, error) {
	if err := l.check(); err != nil {
		return Doc{}, err
	}
	if len(s) > l.MaxDocBytes {
		return Doc{}, ErrLimit
	}
	n, err := unitsIn(s, l.MaxUnits)
	if err != nil {
		return Doc{}, err
	}
	u := make([]uint16, 0, n)
	for _, r := range s {
		u = utf16.AppendRune(u, r)
	}
	return Doc{u: u}, nil
}

// String returns exact UTF-8 text without newline or Unicode normalization.
func (d Doc) String() string {
	return string(utf16.Decode(d.u))
}

// Len returns the number of UTF-16 units, not bytes or Unicode scalars.
func (d Doc) Len() int {
	return len(d.u)
}
