package annot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
)

func TestBoundaryRejections(t *testing.T) {
	raws := []string{
		`null`, `[]`, `{"elements":[null]}`, `{"elements":[{"other":1}]}`,
		`{"elements":[{"put":{"kind":"strokes","id":"a"}}]}`,
		`{"elements":[{"del":{"kind":"strokes","id":"a"},"value":null}]}`,
		`{"elements":[{"put":null,"value":{}}]}`, `{"elements":[{"del":{"kind":"strokes"}}]}`,
		`{"elements":[{"del":{"kind":1,"id":"a"}}]}`,
		`{"entries":[{"del":{"family":"pdfPageState"}}]}`, `{"entries":[{"del":{"family":"pdfPageState","page":"1"}}]}`,
		`{"entries":[{"put":{"family":"pdfPageTemplates","page":2},"value":{"page":1}}]}`,
		`{"entries":[{"put":{"family":"pdfPageTemplates","page":2},"value":null}]}`,
		`{"entries":[{"del":{"family":"unknown","page":1}}]}`,
		`{"entries":[{"del":{"family":"pdfPageState","page":0}}]}`,
		`{"entries":[{"del":{"family":"pdfPageState","page":2}},{"del":{"family":"pdfPageState","page":1}}]}`,
		`{"meta":null}`, `{"meta":{"unknown":1}}`, `{"meta":{"put":null}}`, `{"meta":{"del":null}}`,
		`{"meta":{"del":[1]}}`, `{"meta":{"del":["strokes"]}}`, `{"meta":{"del":["appendedPages"]}}`,
		`{"meta":{"del":["x","x"]}}`,
		`{"elements":[{"del":{"kind":"strokes","id":"z"}},{"del":{"kind":"strokes","id":"a"}}]}`,
		`{"elements":[{"del":{"kind":"strokes","id":"a"}},{"put":{"kind":"strokes","id":"b"},"value":{"id":"b"}}]}`,
	}
	for _, raw := range raws {
		t.Run(raw, func(t *testing.T) {
			if _, err := annot.ParseOps([]byte(raw), annot.DefaultLimits()); err == nil {
				t.Fatal("accepted invalid control structure")
			}
		})
	}
	for _, o := range []annot.Ops{
		{Elements: []annot.ElementOp{{Op: 99, Key: annot.ElementKey{Kind: annot.Strokes, ID: "a"}}}},
		{Elements: []annot.ElementOp{{Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: ""}}}},
		{Elements: []annot.ElementOp{{Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: strings.Repeat("a", 129)}}}},
		{Elements: []annot.ElementOp{{Op: annot.Put, Key: annot.ElementKey{Kind: annot.Strokes, ID: "a"}, Value: json.RawMessage("{")}}},
		{Entries: []annot.EntryOp{{Op: 99, Key: annot.EntryKey{Family: annot.PDFPageState, Page: 1}}}},
		{Entries: []annot.EntryOp{{Op: annot.Del, Key: annot.EntryKey{Family: annot.PDFPageState, Page: 1}, Value: json.RawMessage("null")}}},
		{MetaPut: map[string]json.RawMessage{"x": json.RawMessage("{")}},
		{MetaPut: map[string]json.RawMessage{string([]byte{255}): json.RawMessage("1")}},
		{Replace: true, MetaPut: map[string]json.RawMessage{"appendedPages": json.RawMessage("1")}},
	} {
		if _, err := annot.Apply(annot.Empty(), o, 1); err == nil {
			t.Fatalf("accepted bad public op: %+v", o)
		}
	}
}
func TestApplyBudgets(t *testing.T) {
	for _, configure := range []func(*annot.Limits){
		func(l *annot.Limits) { l.MaxElements = 1 }, func(l *annot.Limits) { l.MaxOps = 1 },
		func(l *annot.Limits) { l.MaxOpBytes = 80 }, func(l *annot.Limits) { l.MaxStateBytes = 80 },
	} {
		l := annot.DefaultLimits()
		configure(&l)
		sc, err := annot.Parse([]byte(`{"strokes":[]}`), l)
		if err != nil {
			t.Fatal(err)
		}
		_, err = annot.Apply(sc, annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 1), put(annot.Strokes, "b", 1)}}, 1)
		if !errors.Is(err, annot.ErrLimit) {
			t.Fatalf("limit: %v", err)
		}
	}
	l := annot.DefaultLimits()
	l.MaxTombstones = 1
	sc, err := annot.Parse([]byte(`{"strokes":[]}`), l)
	if err != nil {
		t.Fatal(err)
	}
	_, err = annot.Apply(sc, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: "a"}}, {Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: "b"}}}}, 1)
	if !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
	l = annot.DefaultLimits()
	l.MaxNodes = 2
	if _, err := annot.Parse([]byte(`{"strokes":[],"x":1}`), l); !errors.Is(err, annot.ErrLimit) {
		t.Fatal(err)
	}
	l = annot.DefaultLimits()
	l.MaxDepth = -1
	if _, err := annot.Parse([]byte(`{"strokes":[]}`), l); !errors.Is(err, annot.ErrInvalidLimits) {
		t.Fatal(err)
	}
	l = annot.Limits{MaxBytes: 16 << 20, MaxElements: 10, MaxIDBytes: 128}
	if _, err := annot.Parse([]byte(`{"strokes":[]}`), l); err != nil {
		t.Fatal(err)
	}
	if _, err := annot.Apply(annot.Empty(), annot.Ops{}, 0); !errors.Is(err, annot.ErrInvalidState) {
		t.Fatal(err)
	}
	if _, err := annot.ParseOps([]byte("{}"), annot.Limits{}); !errors.Is(err, annot.ErrInvalidLimits) {
		t.Fatal(err)
	}
	if _, err := annot.Marshal(annot.Sidecar{}); err != nil {
		t.Fatal(err)
	}
}
func TestMetadataDeletionAndEntries(t *testing.T) {
	a := parse(t, `{"strokes":[],"sourcePdf":null,"x":1,"y":null,"pdfPageTemplates":[{"page":1,"paperColor":"white","future":{"n":1.0}}],"deletedPdfPages":[2]}`)
	b := parse(t, `{"strokes":[],"sourcePdf":{"path":"renamed.pdf"},"x":null,"z":true,"pdfPageTemplates":[{"page":3,"paperColor":"green"}],"permanentlyDeletedPdfPages":[4]}`)
	ops, err := annot.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	r := apply(t, a, ops, 1)
	if annot.CanonicalHash(r.Sidecar) != annot.CanonicalHash(b) {
		t.Fatal("entry and metadata diff differs")
	}
	encoded, err := ops.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := annot.ParseOps(encoded, annot.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(materialize(t, apply(t, a, decoded, 1).Sidecar), materialize(t, b)) {
		t.Fatal("wire entries differ")
	}
}
func TestMarshalDeterministic(t *testing.T) {
	base := parse(t, `{"strokes":[{"id":"a"},{"id":"b"}]}`)
	var want []byte
	for i := 0; i < 100; i++ {
		ops := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 1), put(annot.Strokes, "b", 2)}, MetaPut: map[string]json.RawMessage{}}
		if i%2 == 0 {
			ops.Elements[0], ops.Elements[1] = ops.Elements[1], ops.Elements[0]
			ops.MetaPut["z"] = json.RawMessage("1")
			ops.MetaPut["a"] = json.RawMessage("2")
		} else {
			ops.MetaPut["a"] = json.RawMessage("2")
			ops.MetaPut["z"] = json.RawMessage("1")
		}
		got := materialize(t, apply(t, base, ops, 1).Sidecar)
		if i == 0 {
			want = got
		} else if !bytes.Equal(want, got) {
			t.Fatal("map/update order changed output")
		}
	}
}
func TestAdditionalStateCorruption(t *testing.T) {
	sc := apply(t, annot.Empty(), annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: "dead"}}}}, 1).Sidecar
	raw, err := annot.EncodeState(sc)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ k, v string }{
		{"order", "null"}, {"content", `{}`}, {"tombstones", "null"},
		{"tombstones", `[{"key":{"kind":"strokes","id":"a"},"version":0}]`},
		{"tombstones", `[{"key":{"kind":"strokes","id":"a"},"version":2}]`},
		{"tombstones", `[{"key":{"kind":"strokes","id":"a"},"version":1,"extra":1}]`},
		{"tombstones", `[{"key":{"kind":"strokes"},"version":1}]`},
		{"tombstones", `[{"key":{"kind":"strokes","id":"a"},"version":1},{"key":{"kind":"strokes","id":"a"},"version":1}]`},
		{"tombstones", `[{"key":{"kind":"unknown","id":"a"},"version":1}]`},
	} {
		save := envelope[tc.k]
		envelope[tc.k] = json.RawMessage(tc.v)
		bad, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := annot.DecodeState(bad, annot.DefaultLimits()); !errors.Is(err, annot.ErrInvalidState) {
			t.Fatalf("%s: %v", tc.v, err)
		}
		envelope[tc.k] = save
	}
	for _, bad := range []string{"null", "{}", `{"v":1}`, "{"} {
		if _, err := annot.DecodeState([]byte(bad), annot.DefaultLimits()); !errors.Is(err, annot.ErrInvalidState) {
			t.Fatal(err)
		}
	}
}
