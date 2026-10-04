package annot_test

import (
	"bytes"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
)

func fuzzLimits() annot.Limits {
	l := annot.DefaultLimits()
	l.MaxBytes = 64 << 10
	l.MaxElements = 128
	l.MaxIDBytes = 128
	l.MaxOpBytes = 16 << 10
	l.MaxStateBytes = 128 << 10
	l.MaxOps = 128
	l.MaxTombstones = 128
	l.MaxDepth = 16
	l.MaxNodes = 8192
	return l
}
func FuzzParse(f *testing.F) {
	for _, raw := range []string{`{"strokes":[]}`, `{"strokes":[{"id":"s","future":"<b>&"}]}`, `{"strokes":[],"x":"\ud800"}`, `{"strokes":[],"x":{"a":1,"a":2}}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		sc, err := annot.Parse(raw, fuzzLimits())
		if err != nil {
			return
		}
		out, err := annot.Marshal(sc)
		if err != nil {
			t.Fatal(err)
		}
		again, err := annot.Parse(out, fuzzLimits())
		if err != nil || annot.CanonicalHash(sc) != annot.CanonicalHash(again) {
			t.Fatalf("accepted input did not round trip: %v", err)
		}
	})
}
func FuzzParseOps(f *testing.F) {
	for _, raw := range []string{`{}`, `{"elements":[{"put":{"kind":"strokes","id":"a"},"value":{"id":"a"}}]}`, `{"meta":{"put":{"future":"<b>&"}}}`, `{"replace":true}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		o, err := annot.ParseOps(raw, fuzzLimits())
		if err != nil {
			return
		}
		out, err := o.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		p, err := annot.ParseOps(out, fuzzLimits())
		if err != nil {
			t.Fatal(err)
		}
		again, err := p.MarshalJSON()
		if err != nil || !bytes.Equal(out, again) {
			t.Fatalf("ops round trip: %v", err)
		}
	})
}
func FuzzDecodeState(f *testing.F) {
	empty, err := annot.EncodeState(annot.Empty())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(empty)
	r, err := annot.Apply(annot.Empty(), annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 1)}}, 1)
	if err != nil {
		f.Fatal(err)
	}
	state, err := annot.EncodeState(r.Sidecar)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(state)
	f.Fuzz(func(t *testing.T, raw []byte) {
		sc, err := annot.DecodeState(raw, fuzzLimits())
		if err != nil {
			return
		}
		out, err := annot.EncodeState(sc)
		if err != nil {
			t.Fatal(err)
		}
		next, err := annot.DecodeState(out, fuzzLimits())
		if err != nil {
			t.Fatal(err)
		}
		again, err := annot.EncodeState(next)
		if err != nil || !bytes.Equal(out, again) {
			t.Fatalf("state round trip: %v", err)
		}
	})
}
func FuzzApply(f *testing.F) {
	f.Add([]byte(`{"strokes":[]}`), []byte(`{"elements":[{"put":{"kind":"strokes","id":"a"},"value":{"id":"a"}}]}`), int64(1))
	f.Add([]byte(`{"strokes":[{"id":"a"}]}`), []byte(`{"elements":[{"del":{"kind":"strokes","id":"a"}}]}`), int64(1))
	f.Fuzz(func(t *testing.T, base, raw []byte, at int64) {
		sc, err := annot.Parse(base, fuzzLimits())
		if err != nil {
			return
		}
		o, err := annot.ParseOps(raw, fuzzLimits())
		if err != nil {
			return
		}
		before := annot.CanonicalHash(sc)
		r, err := annot.Apply(sc, o, at)
		if annot.CanonicalHash(sc) != before {
			t.Fatal("mutated operand")
		}
		if err != nil {
			return
		}
		encoded, err := annot.EncodeState(r.Sidecar)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := annot.DecodeState(encoded, fuzzLimits()); err != nil {
			t.Fatal(err)
		}
		replay, err := annot.Apply(sc, r.Effective, at)
		if err != nil {
			t.Fatal(err)
		}
		other, err := annot.EncodeState(replay.Sidecar)
		if err != nil || !bytes.Equal(encoded, other) {
			t.Fatalf("effective state differs: %v", err)
		}
	})
}
