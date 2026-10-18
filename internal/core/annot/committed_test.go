package annot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"pgregory.net/rapid"
)

func TestPropCommittedOpsRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ops := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", rapid.IntRange(0, 1000).Draw(t, "n"))}, Replace: rapid.Bool().Draw(t, "replace")}
		if ops.Replace {
			ops.MetaPut = map[string]json.RawMessage{"appendedPages": []byte(`[{"page":1.0,"escaped":"\u0061<>&"}]`)}
		}
		result, e := annot.Apply(annot.Empty(), ops, 1)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := result.Effective.MarshalJSON()
		if e != nil {
			t.Fatal(e)
		}
		got, e := annot.ParseCommitted(raw, annot.DefaultLimits())
		if e != nil || !reflect.DeepEqual(got, result.Effective) {
			t.Fatal("committed round trip", e)
		}
		replay, e := annot.Apply(annot.Empty(), got, 1)
		if e != nil || annot.CanonicalHash(replay.Sidecar) != annot.CanonicalHash(result.Sidecar) {
			t.Fatal("replay", e)
		}
		again, e := got.MarshalJSON()
		if e != nil || !bytes.Equal(raw, again) {
			t.Fatal("changed opaque bytes")
		}
		if ops.Replace {
			if _, e := annot.ParseOps(raw, annot.DefaultLimits()); !errors.Is(e, annot.ErrNonCanonical) {
				t.Fatal("client accepted replace", e)
			}
		}
	})
}

func TestCommittedStrictness(t *testing.T) {
	for _, raw := range []string{`{"replace":null}`, `{"replace":0}`, `{"replace":"true"}`, `{"replace":true,"Replace":true}`, `{"replace":true,"replace":true}`} {
		if _, e := annot.ParseCommitted([]byte(raw), annot.DefaultLimits()); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`{"replace":false}`, `{"replace":true}`, `{}`} {
		if _, e := annot.ParseCommitted([]byte(raw), annot.DefaultLimits()); e != nil {
			t.Fatal(raw, e)
		}
	}
	l := annot.DefaultLimits()
	l.MaxStateBytes = 8
	if _, e := annot.ParseCommitted([]byte(`{"replace":true}`), l); !errors.Is(e, annot.ErrLimit) {
		t.Fatal(e)
	}
	l.MaxStateBytes = -1
	if _, e := annot.ParseCommitted([]byte(`{}`), l); !errors.Is(e, annot.ErrInvalidLimits) {
		t.Fatal(e)
	}
}

func TestConfiguredOperationEncoding(t *testing.T) {
	l := annot.DefaultLimits()
	l.MaxOpBytes = 8
	ops := annot.Ops{MetaPut: map[string]json.RawMessage{"x": []byte(`1.0`)}}
	if _, e := l.MarshalOps(ops); !errors.Is(e, annot.ErrLimit) {
		t.Fatal(e)
	}
	raw, e := l.MarshalCommitted(ops)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = annot.ParseCommitted(raw, l); e != nil {
		t.Fatal(e)
	}
	ops.Replace = true
	if _, e = l.MarshalOps(ops); !errors.Is(e, annot.ErrNonCanonical) {
		t.Fatal(e)
	}
	l.MaxBytes = 0
	if _, e = l.MarshalCommitted(ops); !errors.Is(e, annot.ErrInvalidLimits) {
		t.Fatal(e)
	}
}
