package annot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"pgregory.net/rapid"
)

func TestPropStateRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		o := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "z", rapid.IntRange(0, 9).Draw(t, "z")), put(annot.Strokes, "a", rapid.IntRange(0, 9).Draw(t, "a"))}}
		r, err := annot.Apply(annot.Empty(), o, 1)
		if err != nil {
			t.Fatal(err)
		}
		r, err = annot.Apply(r.Sidecar, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: "dead"}}}}, 2)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := annot.EncodeState(r.Sidecar)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := annot.DecodeState(raw, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		again, err := annot.EncodeState(restored)
		if err != nil || !bytes.Equal(raw, again) || annot.CanonicalHash(r.Sidecar) != annot.CanonicalHash(restored) {
			t.Fatalf("state round trip: %v", err)
		}
		stale, err := annot.Apply(restored, annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "dead", 1)}}, 3)
		if err != nil || len(stale.Dropped) != 1 {
			t.Fatal("lost tombstone")
		}
	})
}
func TestPruneTombstones(t *testing.T) {
	a := annot.ElementKey{Kind: annot.Strokes, ID: "a"}
	b := annot.ElementKey{Kind: annot.Strokes, ID: "b"}
	sc := apply(t, annot.Empty(), annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: a}}}, 1).Sidecar
	sc = apply(t, sc, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: b}}}, 2).Sidecar
	p := annot.PruneTombstones(sc, 2)
	if annot.CanonicalHash(p) != annot.CanonicalHash(sc) {
		t.Fatal("prune changed content")
	}
	puts := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 1), put(annot.Strokes, "b", 1)}}
	if r := apply(t, p, puts, 3); len(r.Dropped) != 1 || r.Dropped[0] != b {
		t.Fatal("wrong cutoff")
	}
	if r := apply(t, sc, puts, 3); len(r.Dropped) != 2 {
		t.Fatal("mutated original")
	}
}
func TestInvalidState(t *testing.T) {
	raw, err := annot.EncodeState(parse(t, `{"strokes":[{"id":"a"},{"id":"b"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ key, value string }{
		{"v", "2"}, {"version", "-1"}, {"order", "[]"}, {"tombstones", `[{"key":{"kind":"strokes","id":"a"},"version":1}]`},
		{"order", `[{"key":{"kind":"strokes","id":"a"},"version":1,"ordinal":0},{"key":{"kind":"strokes","id":"b"},"version":0,"ordinal":1}]`},
		{"order", `[{"key":{"kind":"strokes","id":"a"},"version":0,"ordinal":0},{"key":{"kind":"strokes","id":"b"},"version":0,"ordinal":0}]`},
	} {
		saved := envelope[change.key]
		envelope[change.key] = json.RawMessage(change.value)
		bad, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := annot.DecodeState(bad, annot.DefaultLimits()); !errors.Is(err, annot.ErrInvalidState) {
			t.Fatalf("%s: %v", change.key, err)
		}
		envelope[change.key] = saved
	}
}
func TestPropOpsRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		o := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", rapid.IntRange(0, 9).Draw(t, "n"))}, MetaPut: map[string]json.RawMessage{"future": json.RawMessage(`"<b>&"`)}}
		raw, err := o.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		p, err := annot.ParseOps(raw, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		again, err := p.MarshalJSON()
		if err != nil || !bytes.Equal(raw, again) {
			t.Fatalf("ops round trip: %v", err)
		}
	})
}
