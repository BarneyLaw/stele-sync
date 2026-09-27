package textop_test

import (
	"bytes"
	"testing"
	"unicode/utf16"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
	"pgregory.net/rapid"
)

var scalar = rapid.SampledFrom([]rune{'a', 'b', '中', '😀', '🔥', '\r', '\n', '\u0301', '\ufeff', '\x00', '<', '>', '&', '"', '\\', '\u2028', '\u2029', '�'})

func drawDoc(t *rapid.T) []rune { return rapid.SliceOfN(scalar, 0, 32).Draw(t, "doc") }

// Each source rune advances exactly once. At most one bounded insertion occurs
// at each gap; deletion and retention act on scalars, independently of the SUT.
// b is always generated against this reference output, never Apply's output.
func drawEdit(t *rapid.T, base []rune) (textop.Op, []rune) {
	var b textop.Builder
	out := make([]rune, 0, len(base))
	for i := 0; i <= len(base); i++ {
		if rapid.Bool().Draw(t, "insert") {
			ins := rapid.SliceOfN(scalar, 1, 3).Draw(t, "text")
			out = append(out, ins...)
			b.Insert(string(ins))
		}
		if i == len(base) {
			break
		}
		n := len(utf16.Encode([]rune{base[i]}))
		if rapid.Bool().Draw(t, "keep") {
			out = append(out, base[i])
			b.Retain(n)
		} else {
			b.Delete(n)
		}
	}
	op, err := b.Op()
	if err != nil {
		t.Fatalf("valid model edit rejected: %v", err)
	}
	got, err := applyChecked(mustDoc(t, string(base)), op)
	if err != nil || got.String() != string(out) {
		t.Fatalf("Builder/Apply differs from model: %v got %q want %q", err, got.String(), string(out))
	}
	return op, out
}

func property(t *rapid.T, mode string) {
	base := drawDoc(t)
	a, mid := drawEdit(t, base)
	b, _ := drawEdit(t, base)
	next, want := drawEdit(t, mid)
	as, bs, ns := encoded(a), encoded(b), encoded(next)
	ap, bp, err := textop.Transform(a, b)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	c, err := textop.Compose(a, next)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	for _, op := range []textop.Op{a, b, next, ap, bp, c} {
		if err := checkOp(op); err != nil {
			t.Fatalf("canonical: %v", err)
		}
	}
	d := mustDoc(t, string(base))
	switch mode {
	case "convergence":
		da, e := applyChecked(d, a)
		if e != nil {
			t.Fatalf("a: %v", e)
		}
		db, e := applyChecked(d, b)
		if e != nil {
			t.Fatalf("b: %v", e)
		}
		left, e := applyChecked(da, bp)
		if e != nil {
			t.Fatalf("bp: %v", e)
		}
		right, e := applyChecked(db, ap)
		if e != nil {
			t.Fatalf("ap: %v", e)
		}
		if left.String() != right.String() {
			t.Fatalf("TP1: %q != %q", left.String(), right.String())
		}
	case "compose":
		combined, e := applyChecked(d, c)
		if e != nil {
			t.Fatalf("combined: %v", e)
		}
		middle, e := applyChecked(d, a)
		if e != nil {
			t.Fatalf("a: %v", e)
		}
		sequential, e := applyChecked(middle, next)
		if e != nil {
			t.Fatalf("b: %v", e)
		}
		if combined.String() != string(want) || sequential.String() != string(want) {
			t.Fatalf("Compose differs from independent edit result")
		}
	case "lengths":
		if ap.BaseLen() != b.TargetLen() || bp.BaseLen() != a.TargetLen() || ap.TargetLen() != bp.TargetLen() || c.BaseLen() != a.BaseLen() || c.TargetLen() != next.TargetLen() {
			t.Fatalf("algebra length identities failed")
		}
	case "canonical": // all six operations checked above, including codec round-trip
	default:
		t.Fatalf("unknown property %s", mode)
	}
	if !bytes.Equal(as, encoded(a)) || !bytes.Equal(bs, encoded(b)) || !bytes.Equal(ns, encoded(next)) {
		t.Fatalf("algebra mutated operand")
	}
}

func TestPropConvergence(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) { property(t, "convergence") })
}
func TestPropComposeApply(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) { property(t, "compose") })
}
func TestPropLengths(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) { property(t, "lengths") })
}
func TestPropCanonical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) { property(t, "canonical") })
}
