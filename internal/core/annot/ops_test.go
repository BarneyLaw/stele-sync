package annot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
)

func TestOpaqueValuesAndIndentBudget(t *testing.T) {
	a := parse(t, `{"strokes":[{"id":"a","value":1}],"pdfPageState":{"future":true}}`)
	b := parse(t, `{"strokes":[{"id":"a","value":1.0}],"pdfPageState":{"future":false}}`)
	ops, err := annot.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ops.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"value":1.0`)) || len(ops.MetaPut) != 1 {
		t.Fatal("opaque JSON was normalized or reserved")
	}
	l := annot.DefaultLimits()
	l.MaxBytes = 1024
	nested := `{"strokes":[],"x":` + strings.Repeat("[", 15) + strings.Repeat("0,", 99) + "0" + strings.Repeat("]", 15) + "}"
	if len(nested) >= l.MaxBytes {
		t.Fatal("bad test input")
	}
	if _, err := annot.Parse([]byte(nested), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatalf("indented size must be bounded: %v", err)
	}
	// Punctuation inside strings must not count as structural indentation.
	parse(t, `{"strokes":[{"id":"a","text":"{}[],: \" \\"}]}`)
	o := annot.Ops{MetaPut: map[string]json.RawMessage{"sourcePdf": json.RawMessage(`{"path":"<a>&.pdf"}`)}}
	raw, err = o.MarshalJSON()
	if err != nil || !bytes.Contains(raw, []byte("<a>&.pdf")) {
		t.Fatalf("opaque escapes: %v", err)
	}
}
