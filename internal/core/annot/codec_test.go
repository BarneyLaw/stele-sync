package annot_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"pgregory.net/rapid"
)

func parse(t testing.TB, raw string) annot.Sidecar {
	t.Helper()
	sc, err := annot.Parse([]byte(raw), annot.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return sc
}
func materialize(t testing.TB, sc annot.Sidecar) []byte {
	t.Helper()
	raw, err := annot.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Write the round-trip property before the codec: opaque content and original
// collection order must survive normalization.
func TestPropMarshalIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := rapid.SampledFrom([]string{"", "中", "😀", "<b>&", "\\ud800", "\x00"}).Draw(t, "text")
		raw, err := json.Marshal(map[string]any{"strokes": []any{map[string]any{"id": "z", "text": text}, map[string]any{"id": "a", "n": rapid.Int64().Draw(t, "n")}}})
		if err != nil {
			t.Fatal(err)
		}
		sc, err := annot.Parse(raw, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		a, err := annot.Marshal(sc)
		if err != nil {
			t.Fatal(err)
		}
		sc, err = annot.Parse(a, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		b, err := annot.Marshal(sc)
		if err != nil || !bytes.Equal(a, b) {
			t.Fatalf("unstable: %v", err)
		}
		if annot.CanonicalHash(sc) != sha256.Sum256(b) {
			t.Fatal("hash is not file bytes")
		}
	})
}
func TestUnknownFieldsPreserved(t *testing.T) {
	raw := `{"strokes":[{"id":"z","n":9007199254740993,"future":{"b":1.00,"a":"\u0061"}},{"id":"a"}],"future":{"z":true,"a":null},"pdfPageTemplates":[{"page":2,"future":1e0}]}`
	sc := parse(t, raw)
	out := materialize(t, sc)
	var compact bytes.Buffer
	if err := json.Compact(&compact, out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`9007199254740993`, `"future":{"b":1.00,"a":"\u0061"}`, `"future":{"z":true,"a":null}`, `{"page":2,"future":1e0}`} {
		if !strings.Contains(compact.String(), want) {
			t.Fatalf("lost raw content %s: %s", want, out)
		}
	}
	if bytes.Index(out, []byte(`"id": "z"`)) > bytes.Index(out, []byte(`"id": "a"`)) {
		t.Fatal("import order changed")
	}
}
func TestNoHTMLEscaping(t *testing.T) {
	out := materialize(t, parse(t, `{"strokes":[],"textItems":[{"id":"t","text":"<b>&"}],"future":"<>&"}`))
	if !bytes.Contains(out, []byte("<b>&")) || bytes.Contains(out, []byte(`\u003c`)) {
		t.Fatalf("HTML escaped: %s", out)
	}
}
func TestInvalid(t *testing.T) {
	cases := []struct {
		raw  string
		want error
	}{
		{`null`, annot.ErrNotSidecar}, {`{}`, annot.ErrNotSidecar}, {`{"strokes":null}`, annot.ErrNotSidecar},
		{`{"strokes":[{}]}`, annot.ErrMissingID}, {`{"strokes":[{"id":2}]}`, annot.ErrMissingID},
		{`{"strokes":[{"id":"a"},{"id":"a"}]}`, annot.ErrDuplicateID},
		{`{"strokes":[],"strokes":[]}`, annot.ErrDuplicateKey},
		{`{"strokes":[],"x":{"a":1,"\u0061":2}}`, annot.ErrDuplicateKey},
		{`{"strokes":[],"x":"\ud800"}`, annot.ErrInvalidJSON},
		{`{"strokes":[],"x":"\udc00"}`, annot.ErrInvalidJSON},
		{`{"strokes":[],"x":"\ud800\u0041"}`, annot.ErrInvalidJSON},
		{`{"strokes":[]} {}`, annot.ErrInvalidJSON},
		{`{"strokes":[],"appendedPages":{}}`, annot.ErrNotSidecar},
		{`{"strokes":[],"deletedPdfPages":[0]}`, annot.ErrNotSidecar},
		{`{"strokes":[],"pdfPageTemplates":[{"page":1},{"page":1}]}`, annot.ErrDuplicateKey},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if _, err := annot.Parse([]byte(tc.raw), annot.DefaultLimits()); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	parse(t, `{"strokes":[{"id":"a"}],"shapes":[{"id":"a"}]}`)
}
func TestLimits(t *testing.T) {
	l := annot.DefaultLimits()
	l.MaxBytes = 1
	if _, err := annot.Parse([]byte(`{"strokes":[]}`), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
	l = annot.DefaultLimits()
	l.MaxElements = 1
	if _, err := annot.Parse([]byte(`{"strokes":[{"id":"a"},{"id":"b"}]}`), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
	l = annot.DefaultLimits()
	l.MaxIDBytes = 1
	if _, err := annot.Parse([]byte(`{"strokes":[{"id":"ab"}]}`), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
	if _, err := annot.Parse([]byte(`{"strokes":[]}`), annot.Limits{}); !errors.Is(err, annot.ErrInvalidLimits) {
		t.Fatal(err)
	}
	l = annot.DefaultLimits()
	l.MaxDepth = 3
	if _, err := annot.Parse([]byte(`{"strokes":[],"x":[[[0]]]}`), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
}
func TestLocalSample(t *testing.T) {
	path := os.Getenv("ANNOT_SAMPLE")
	if path == "" {
		t.Skip("set ANNOT_SAMPLE for a private local compatibility check")
	}
	raw, err := os.ReadFile(path) // #nosec G304 G703 -- explicit opt-in local test input, never a client path.
	if err != nil {
		t.Fatal(err)
	}
	a, err := annot.Parse(raw, annot.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	out := materialize(t, a)
	b, err := annot.Parse(out, annot.DefaultLimits())
	if err != nil || annot.CanonicalHash(a) != annot.CanonicalHash(b) {
		t.Fatalf("sample round trip: %v", err)
	}
}
