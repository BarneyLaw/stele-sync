package textop_test

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

func TestParseRegression(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`["a"]`, "a"}, {`["ab"]`, "ab"}, {`[]`, ""}, {`["😀"]`, "😀"},
		{`["\ud83d\ude00"]`, "😀"}, {`["\\ud800"]`, `\ud800`}, {`["�"]`, "�"},
		{`["\r\n\n\r\ufeffe\u0301é\"\\<>&\u2028\u2029\u0000"]`, "\r\n\n\r\ufeffe\u0301é\"\\<>&\u2028\u2029\x00"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			op := mustOp(t, tc.raw)
			got, err := applyChecked(textop.Doc{}, op)
			if err != nil || got.String() != tc.want {
				t.Fatalf("got %q, %v; want %q", got.String(), err, tc.want)
			}
			if err := checkOp(op); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, tc := range []struct {
		raw  string
		want error
	}{
		{`["\ud800x"]`, textop.ErrInvalidUnicode}, {`["\ud800"]`, textop.ErrInvalidUnicode},
		{`["\udc00"]`, textop.ErrInvalidUnicode}, {`["\udc00\ud800"]`, textop.ErrInvalidUnicode},
		{`["\ud800\u0041"]`, textop.ErrInvalidUnicode}, {`["\q"]`, textop.ErrInvalidJSON},
		{`["`, textop.ErrInvalidJSON}, {"[\"\xff\"]", textop.ErrInvalidUTF8},
		{`{}`, textop.ErrInvalidJSON}, {`null`, textop.ErrInvalidJSON}, {`1`, textop.ErrInvalidJSON},
		{`"x"`, textop.ErrInvalidJSON}, {`true`, textop.ErrInvalidJSON}, {`false`, textop.ErrInvalidJSON},
		{`[] []`, textop.ErrInvalidJSON}, {`[1`, textop.ErrInvalidJSON}, {`[1,]`, textop.ErrInvalidJSON},
		{`[0]`, textop.ErrNonCanonical}, {`[-0]`, textop.ErrNonCanonical}, {`[""]`, textop.ErrNonCanonical},
		{`[1,2]`, textop.ErrNonCanonical}, {`[-1,-2]`, textop.ErrNonCanonical}, {`["a","b"]`, textop.ErrNonCanonical},
		{`[-1,"x"]`, textop.ErrNonCanonical}, {`[1.0]`, textop.ErrInvalidCount}, {`[1e0]`, textop.ErrInvalidCount},
		{`[null]`, textop.ErrInvalidCount}, {`[true]`, textop.ErrInvalidCount}, {`[{}]`, textop.ErrInvalidCount}, {`[[]]`, textop.ErrInvalidCount},
		{`[-9223372036854775808]`, textop.ErrLimit}, {`[9223372036854775808]`, textop.ErrInvalidCount},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			_, err := textop.Parse([]byte(tc.raw))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v; want %v", err, tc.want)
			}
		})
	}
}

func TestApplyValidation(t *testing.T) {
	for _, tc := range []struct {
		doc, raw, want string
		err            error
	}{
		{"abcd", `[1,"XY",-2,1]`, "aXYd", nil}, {"A😀B", `[1,-2,1]`, "AB", nil},
		{"e\u0301", `[1,-1]`, "e", nil}, {"abc", `[3]`, "abc", nil},
		{"A😀B", `[2,"X",2]`, "", textop.ErrSplitsSurrogate}, {"A😀B", `[1,-1,2]`, "", textop.ErrSplitsSurrogate},
		{"ab", `[1]`, "", textop.ErrLengthMismatch}, {"", `[1]`, "", textop.ErrLengthMismatch},
	} {
		t.Run(tc.raw+tc.doc, func(t *testing.T) {
			d, op := mustDoc(t, tc.doc), mustOp(t, tc.raw)
			before := encoded(op)
			got, err := textop.Apply(d, op)
			validErr := textop.ValidateAgainst(d, op)
			if !errors.Is(err, tc.err) || !errors.Is(validErr, tc.err) {
				t.Fatalf("Apply=%v Validate=%v want=%v", err, validErr, tc.err)
			}
			if tc.err == nil && (got.String() != tc.want || got.Len() != op.TargetLen()) {
				t.Fatalf("got %q want %q", got.String(), tc.want)
			}
			if d.String() != tc.doc || !bytes.Equal(before, encoded(op)) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestAlgebraPipelines(t *testing.T) {
	for _, tc := range []struct{ doc, a, b, want, text string }{
		{"ab", `[1,"XY",1]`, `[2,-1,1]`, `[1,"X",1]`, "aXb"},
		{"", `["😀"]`, `[-2]`, `[]`, ""},
		{"ab", `[2]`, `[-2]`, `[-2]`, ""},
		{"ab", `[-2]`, `["XY"]`, `["XY",-2]`, "XY"},
	} {
		t.Run("compose/"+tc.a+tc.b, func(t *testing.T) {
			a, b := mustOp(t, tc.a), mustOp(t, tc.b)
			c, err := textop.Compose(a, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := equalOp(c, []byte(tc.want)); err != nil {
				t.Fatal(err)
			}
			got, err := applyChecked(mustDoc(t, tc.doc), c)
			if err != nil || got.String() != tc.text {
				t.Fatalf("%q %v", got.String(), err)
			}
		})
	}
	for _, tc := range []struct{ doc, a, b, ap, bp, text string }{
		{"ab", `[1,"X",1]`, `[1,"Y",1]`, `[1,"X",2]`, `[2,"Y",1]`, "aXYb"},
		{"ab", `[1,"Y",1]`, `[1,"X",1]`, `[1,"Y",2]`, `[2,"X",1]`, "aYXb"},
		{"abcd", `[1,-2,1]`, `[2,-2]`, `[1,-1]`, `[1,-1]`, "a"},
		{"abcd", `[1,-2,1]`, `[2,"X",2]`, `[1,-1,1,-1,1]`, `[1,"X",1]`, "aXd"},
		{"abcd", `[1,-2,1]`, `[-4]`, `[]`, `[-2]`, ""},
		{"abcd", `[-2,2]`, `[2,-2]`, `[-2]`, `[-2]`, ""},
		{"abcd", `[-1,3]`, `[3,-1]`, `[-1,2]`, `[2,-1]`, "bc"},
		{"abcd", `[1,-2,1]`, `[1,-2,1]`, `[2]`, `[2]`, "ad"},
	} {
		t.Run("transform/"+tc.a+tc.b, func(t *testing.T) {
			a, b := mustOp(t, tc.a), mustOp(t, tc.b)
			ap, bp, err := textop.Transform(a, b)
			if err != nil {
				t.Fatal(err)
			}
			if err := equalOp(ap, []byte(tc.ap)); err != nil {
				t.Fatal(err)
			}
			if err := equalOp(bp, []byte(tc.bp)); err != nil {
				t.Fatal(err)
			}
			d := mustDoc(t, tc.doc)
			da, err := applyChecked(d, a)
			if err != nil {
				t.Fatal(err)
			}
			db, err := applyChecked(d, b)
			if err != nil {
				t.Fatal(err)
			}
			left, err := applyChecked(da, bp)
			if err != nil {
				t.Fatal(err)
			}
			right, err := applyChecked(db, ap)
			if err != nil {
				t.Fatal(err)
			}
			if left.String() != tc.text || right.String() != tc.text {
				t.Fatalf("paths %q %q want %q", left.String(), right.String(), tc.text)
			}
			if !bytes.Equal(encoded(a), []byte(tc.a)) || !bytes.Equal(encoded(b), []byte(tc.b)) {
				t.Fatal("operands mutated")
			}
		})
	}
	_, err := textop.Compose(mustOp(t, `["😀"]`), mustOp(t, `[1,-1]`))
	if !errors.Is(err, textop.ErrSplitsSurrogate) {
		t.Fatal(err)
	}
	_, err = textop.Compose(mustOp(t, `[1]`), textop.Op{})
	if !errors.Is(err, textop.ErrLengthMismatch) {
		t.Fatal(err)
	}
	_, _, err = textop.Transform(mustOp(t, `[1]`), textop.Op{})
	if !errors.Is(err, textop.ErrLengthMismatch) {
		t.Fatal(err)
	}
	// Handoff: transform lacks the base text and may mask an invalid boundary.
	incoming, history := mustOp(t, `[1,-1]`), mustOp(t, `[-2]`)
	if err := textop.ValidateAgainst(mustDoc(t, "😀"), incoming); !errors.Is(err, textop.ErrSplitsSurrogate) {
		t.Fatal(err)
	}
	if _, _, err := textop.Transform(incoming, history); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredLimitsAndDefaults(t *testing.T) {
	l := textop.DefaultLimits()
	d := mustDoc(t, "a😀b")
	a := mustOp(t, `[1,"中",3]`)
	b := mustOp(t, `[3,-1]`)
	configured, err := l.DocFromString(d.String())
	if err != nil || configured.String() != d.String() || configured.Len() != d.Len() {
		t.Fatal("document wrapper differs")
	}
	parsed, err := l.Parse(encoded(a))
	if err != nil || !bytes.Equal(encoded(a), encoded(parsed)) {
		t.Fatal("parse wrapper differs")
	}
	left, err := textop.Apply(d, a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := l.Apply(d, a)
	if err != nil || left.String() != right.String() {
		t.Fatal("apply wrapper differs")
	}
	if err := l.ValidateAgainst(d, a); err != nil {
		t.Fatal(err)
	}
	ap, bp, err := textop.Transform(a, b)
	if err != nil {
		t.Fatal(err)
	}
	configuredAP, configuredBP, err := l.Transform(a, b)
	if err != nil || !bytes.Equal(encoded(ap), encoded(configuredAP)) || !bytes.Equal(encoded(bp), encoded(configuredBP)) {
		t.Fatal("transform wrapper differs")
	}
	identity := mustOp(t, fmt.Sprintf(`[%d]`, a.TargetLen()))
	c, err := textop.Compose(a, identity)
	if err != nil {
		t.Fatal(err)
	}
	cc, err := l.Compose(a, identity)
	if err != nil || !bytes.Equal(encoded(c), encoded(cc)) {
		t.Fatal("compose wrapper differs")
	}
	for _, policy := range []textop.Limits{
		{MaxUnits: 1, MaxDocBytes: 100, MaxComponents: 100, MaxJSONBytes: 100},
		{MaxUnits: 100, MaxDocBytes: 100, MaxComponents: 1, MaxJSONBytes: 100},
	} {
		builder, err := textop.NewBuilder(policy)
		if err != nil {
			t.Fatal(err)
		}
		builder.Insert("a").Retain(1)
		_, first := builder.Op()
		if !errors.Is(first, textop.ErrLimit) {
			t.Fatal(first)
		}
		builder.Insert("b").Delete(1)
		_, last := builder.Op()
		if !errors.Is(last, first) {
			t.Fatal("limit error not sticky")
		}
		before := encoded(a)
		if _, err := policy.Compose(a, identity); !errors.Is(err, textop.ErrLimit) {
			t.Fatal(err)
		}
		if _, err := policy.Compose(textop.Op{}, a); !errors.Is(err, textop.ErrLimit) {
			t.Fatal(err)
		}
		if _, _, err := policy.Transform(a, b); !errors.Is(err, textop.ErrLimit) {
			t.Fatal(err)
		}
		if _, _, err := policy.Transform(textop.Op{}, a); !errors.Is(err, textop.ErrLimit) {
			t.Fatal(err)
		}
		if err := policy.ValidateAgainst(d, a); !errors.Is(err, textop.ErrLimit) {
			t.Fatal(err)
		}
		if !bytes.Equal(before, encoded(a)) {
			t.Fatal("limit failure mutated operand")
		}
	}
	// Base byte limit is enforced even if the operation deletes all base text.
	l = textop.DefaultLimits()
	l.MaxDocBytes = 1
	if _, err := l.Apply(mustDoc(t, "中"), mustOp(t, `[-1]`)); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	// Compose can exceed a component budget even when both inputs are admitted.
	l = textop.DefaultLimits()
	l.MaxComponents = 2
	if _, err := l.Compose(mustOp(t, `[1,"x"]`), mustOp(t, `["y",2]`)); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	if strconv.IntSize == 64 {
		tooLarge := uint64(1) << 53
		l = textop.DefaultLimits()
		l.MaxUnits = int(tooLarge)
		if _, err := textop.NewBuilder(l); !errors.Is(err, textop.ErrInvalidLimits) {
			t.Fatal(err)
		}
	}
}

func TestBuilderSnapshots(t *testing.T) {
	var b textop.Builder
	zero, err := b.Op()
	if err != nil || string(encoded(zero)) != "[]" || (textop.Doc{}).Len() != 0 {
		t.Fatal("bad zero value")
	}
	b.Retain(0).Delete(0).Insert("").Delete(1).Insert("a").Insert("b")
	first, err := b.Op()
	if err != nil {
		t.Fatal(err)
	}
	if err := equalOp(first, []byte(`["ab",-1]`)); err != nil {
		t.Fatal(err)
	}
	before := encoded(first)
	b.Insert("c") // exercise growth that can reuse an insertion buffer's capacity
	if !bytes.Equal(before, encoded(first)) {
		t.Fatal("snapshot changed on short insertion")
	}
	b.Insert(strings.Repeat("x", 100)).Retain(2)
	if !bytes.Equal(before, encoded(first)) || first.BaseLen() != 1 || first.TargetLen() != 2 {
		t.Fatal("snapshot aliased builder")
	}
	for _, bad := range []func(*textop.Builder){func(b *textop.Builder) { b.Retain(-1) }, func(b *textop.Builder) { b.Delete(-1) }, func(b *textop.Builder) { b.Insert("\xff") }} {
		var b textop.Builder
		bad(&b)
		_, firstErr := b.Op()
		b.Retain(1).Insert("x").Delete(1)
		_, later := b.Op()
		if firstErr == nil || !errors.Is(later, firstErr) {
			t.Fatalf("not sticky: %v %v", firstErr, later)
		}
	}
	sharedDoc := mustDoc(t, "z")
	sharedIdentity := mustOp(t, `[2]`)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				if _, err := applyChecked(sharedDoc, first); err != nil {
					t.Error(err)
				}
				if _, err := textop.Compose(first, sharedIdentity); err != nil {
					t.Error(err)
				}
				if _, _, err := textop.Transform(first, first); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}

func TestLimits(t *testing.T) {
	for _, field := range []string{"units", "bytes", "components", "json"} {
		for _, n := range []int{0, -1} {
			t.Run(fmt.Sprintf("invalid/%s/%d", field, n), func(t *testing.T) {
				l := textop.DefaultLimits()
				switch field {
				case "units":
					l.MaxUnits = n
				case "bytes":
					l.MaxDocBytes = n
				case "components":
					l.MaxComponents = n
				case "json":
					l.MaxJSONBytes = n
				}
				_, e1 := textop.NewBuilder(l)
				_, e2 := l.Parse([]byte(`[]`))
				_, e3 := l.DocFromString("")
				e4 := l.ValidateAgainst(textop.Doc{}, textop.Op{})
				_, e5 := l.Apply(textop.Doc{}, textop.Op{})
				_, e6 := l.Compose(textop.Op{}, textop.Op{})
				_, _, e7 := l.Transform(textop.Op{}, textop.Op{})
				for _, err := range []error{e1, e2, e3, e4, e5, e6, e7} {
					if !errors.Is(err, textop.ErrInvalidLimits) {
						t.Fatal(err)
					}
				}
			})
		}
	}
	for _, n := range []int{2, 3, 4} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			l := textop.DefaultLimits()
			l.MaxUnits = 3
			_, err := l.DocFromString(strings.Repeat("a", n))
			want := error(nil)
			if n > 3 {
				want = textop.ErrLimit
			}
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = l.Parse(fmt.Appendf(nil, "[%d]", n))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			b, e := textop.NewBuilder(l)
			if e != nil {
				t.Fatal(e)
			}
			b.Retain(n)
			_, err = b.Op()
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			l = textop.DefaultLimits()
			l.MaxDocBytes = 3
			_, err = l.DocFromString(strings.Repeat("a", n))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = l.Apply(textop.Doc{}, mustOp(t, fmt.Sprintf(`["%s"]`, strings.Repeat("a", n))))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			l = textop.DefaultLimits()
			l.MaxJSONBytes = 3
			_, err = l.Parse([]byte("[]" + strings.Repeat(" ", n-2)))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			l = textop.DefaultLimits()
			l.MaxComponents = 3
			raw := []string{`[1,"a"]`, `[1,"a",1]`, `[1,"a",1,"b"]`}[n-2]
			_, err = l.Parse([]byte(raw))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
		})
	}
	l := textop.DefaultLimits()
	l.MaxUnits = 2
	l.MaxDocBytes = 3
	if _, err := l.DocFromString("中"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DocFromString("😀"); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	l.MaxDocBytes = 4
	if _, err := l.DocFromString("😀"); err != nil {
		t.Fatal(err)
	}
	l.MaxUnits = 1
	if _, err := l.DocFromString("😀"); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	// Legal inputs, oversized merged output (both policy dimensions).
	a, b := mustOp(t, `["a"]`), mustOp(t, `["b"]`)
	l = textop.DefaultLimits()
	l.MaxUnits = 1
	if _, _, err := l.Transform(a, b); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	l = textop.DefaultLimits()
	l.MaxComponents = 2
	a = mustOp(t, `["x",-1]`)
	b = mustOp(t, `[1,"y"]`)
	if _, _, err := l.Transform(a, b); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	// HTML escaping expands accepted ingress beyond the default codec budget.
	op := mustOp(t, `["`+strings.Repeat("<", 180000)+`"]`)
	if _, err := textop.Parse(encoded(op)); !errors.Is(err, textop.ErrLimit) {
		t.Fatal(err)
	}
	if err := checkOp(op); err != nil {
		t.Fatal(err)
	}
}
