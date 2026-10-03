package annot_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"pgregory.net/rapid"
)

func apply(t testing.TB, sc annot.Sidecar, ops annot.Ops, at int64) annot.Result {
	t.Helper()
	r, err := annot.Apply(sc, ops, at)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func put(kind annot.Kind, id string, n int) annot.ElementOp {
	return annot.ElementOp{Op: annot.Put, Key: annot.ElementKey{Kind: kind, ID: id}, Value: json.RawMessage(fmt.Sprintf(`{"id":%q,"n":%d}`, id, n))}
}
func content(t testing.TB, sc annot.Sidecar) map[string]json.RawMessage {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(materialize(t, sc), &root); err != nil {
		t.Fatal(err)
	}
	delete(root, "updatedAt")
	return root
}

// The generator edits a tiny id space with collisions, preserving retained
// element order and appending new elements, as FreeDraw does.
func TestPropDiffApply(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		old := []map[string]any{{"id": "a", "n": 0}, {"id": "b", "n": 0}, {"id": "c", "n": 0}}
		next := []map[string]any{}
		for _, e := range old {
			if rapid.Bool().Draw(t, "keep") {
				next = append(next, map[string]any{"id": e["id"], "n": rapid.IntRange(0, 3).Draw(t, "value")})
			}
		}
		if rapid.Bool().Draw(t, "new") {
			next = append(next, map[string]any{"id": "d", "n": 1})
		}
		aRaw, err := json.Marshal(map[string]any{"strokes": old})
		if err != nil {
			t.Fatal(err)
		}
		bRaw, err := json.Marshal(map[string]any{"strokes": next})
		if err != nil {
			t.Fatal(err)
		}
		a, err := annot.Parse(aRaw, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		b, err := annot.Parse(bRaw, annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		ops, err := annot.Diff(a, b)
		if err != nil {
			t.Fatal(err)
		}
		r, err := annot.Apply(a, ops, 1)
		if err != nil {
			t.Fatal(err)
		}
		if annot.CanonicalHash(r.Sidecar) != annot.CanonicalHash(b) {
			t.Fatal("diff/apply differs from independently edited target")
		}
	})
}
func TestPropEffectiveReplays(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base := annot.Empty()
		tomb := annot.ElementKey{Kind: annot.Strokes, ID: "dead"}
		r, err := annot.Apply(base, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: tomb}}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		ops := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "dead", 1), put(annot.Strokes, "new", rapid.IntRange(0, 5).Draw(t, "n"))}}
		got, err := annot.Apply(r.Sidecar, ops, 2)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := annot.Apply(r.Sidecar, got.Effective, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Dropped) != 1 || len(replay.Dropped) != 0 || annot.CanonicalHash(got.Sidecar) != annot.CanonicalHash(replay.Sidecar) {
			t.Fatal("effective replay failed")
		}
		originalState, err := annot.EncodeState(got.Sidecar)
		if err != nil {
			t.Fatal(err)
		}
		replayedState, err := annot.EncodeState(replay.Sidecar)
		if err != nil || !bytes.Equal(originalState, replayedState) {
			t.Fatalf("effective replay changed order keys or tombstones: %v", err)
		}
	})
}
func TestDeleteWins(t *testing.T) {
	key := annot.ElementKey{Kind: annot.Strokes, ID: "a"}
	base := parse(t, `{"strokes":[{"id":"a"}]}`)
	deleted := apply(t, base, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: key}}}, 1).Sidecar
	r := apply(t, deleted, annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 1), put(annot.Shapes, "a", 2)}}, 2)
	if !reflect.DeepEqual(r.Dropped, []annot.ElementKey{key}) || len(r.Effective.Elements) != 1 {
		t.Fatalf("wrong dropped/effective: %+v", r)
	}
	if bytes.Contains(content(t, r.Sidecar)["strokes"], []byte(`"id"`)) {
		t.Fatal("resurrected")
	}
}
func TestEntryFamilies(t *testing.T) {
	base := parse(t, `{"strokes":[]}`)
	hide := func(page int) annot.Ops {
		return annot.Ops{Entries: []annot.EntryOp{{Op: annot.Put, Key: annot.EntryKey{Family: annot.PDFPageState, Page: page}, Value: json.RawMessage(`"hidden"`)}}}
	}
	r := apply(t, base, hide(2), 1)
	r = apply(t, r.Sidecar, hide(3), 2)
	key := annot.EntryKey{Family: annot.PDFPageState, Page: 2}
	r = apply(t, r.Sidecar, annot.Ops{Entries: []annot.EntryOp{{Op: annot.Del, Key: key}}}, 3)
	r = apply(t, r.Sidecar, hide(2), 4)
	var pages []int
	if err := json.Unmarshal(content(t, r.Sidecar)["deletedPdfPages"], &pages); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pages, []int{2, 3}) {
		t.Fatalf("concurrent hide lost: %v", pages)
	}
}
func TestVolatileMasked(t *testing.T) {
	a := parse(t, `{"strokes":[],"updatedAt":"old","sourcePdf":{"path":"a.pdf","ctime":1,"mtime":2}}`)
	b := parse(t, `{"strokes":[],"updatedAt":"new","sourcePdf":{"path":"a.pdf","ctime":3,"mtime":4}}`)
	ops, err := annot.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops.MetaPut) != 0 || len(ops.MetaDel) != 0 {
		t.Fatal("volatile diff")
	}
	c := parse(t, `{"strokes":[],"updatedAt":"new","sourcePdf":{"path":"b.pdf","ctime":3,"mtime":4}}`)
	ops, err = annot.Diff(a, c)
	if err != nil || len(ops.MetaPut) != 1 {
		t.Fatalf("rename masked: %v %+v", err, ops)
	}
}
func TestModeSelection(t *testing.T) {
	plain := parse(t, `{"strokes":[],"appendedPages":[],"removedPages":[]}`)
	added := parse(t, `{"strokes":[],"appendedPages":[{"id":"page-1"}]}`)
	if annot.RequiresWholeDocument(plain) || !annot.RequiresWholeDocument(added) {
		t.Fatal("mode")
	}
	if _, err := annot.Diff(plain, added); !errors.Is(err, annot.ErrWholeDocument) {
		t.Fatal(err)
	}
	if _, err := annot.Apply(added, annot.Ops{}, 1); !errors.Is(err, annot.ErrWholeDocument) {
		t.Fatal(err)
	}
	if _, err := annot.Apply(plain, annot.Ops{MetaPut: map[string]json.RawMessage{"appendedPages": json.RawMessage("[]")}}, 1); !errors.Is(err, annot.ErrStructuralKey) {
		t.Fatal(err)
	}
}
func TestTrashRestoreUnderReplace(t *testing.T) {
	live := parse(t, `{"strokes":[{"id":"a","page":3}],"appendedPages":[{"id":"p"}],"removedPages":[]}`)
	trash := parse(t, `{"strokes":[],"appendedPages":[],"removedPages":[{"page":{"id":"p"},"annotations":{"strokes":[{"id":"a","page":3}]}}]}`)
	head := apply(t, annot.Empty(), annot.ReplaceOps(annot.Empty(), live), 1).Sidecar
	head = apply(t, head, annot.ReplaceOps(head, trash), 2).Sidecar
	r := apply(t, head, annot.ReplaceOps(head, live), 3)
	if len(r.Dropped) != 0 || annot.CanonicalHash(r.Sidecar) != annot.CanonicalHash(live) {
		t.Fatal("restore lost annotations")
	}
}
func TestOpsInvalid(t *testing.T) {
	for _, raw := range []string{
		`{"replace":true}`, `{"replace":false}`, `{"elements":[{"put":{"kind":"strokes","id":"a"},"value":{"id":"b"}}]}`,
		`{"elements":[{"del":{"kind":"strokes","id":"a"}},{"del":{"kind":"strokes","id":"a"}}]}`,
		`{"meta":{"del":["z","a"]}}`, `{"meta":{"put":{"x":1},"del":["x"]}}`,
		`{"entries":[{"put":{"family":"pdfPageState","page":1},"value":"other"}]}`,
		`{"meta":{"put":{"strokes":[]}}}`, `{"elements":null}`, `{"unknown":1}`,
	} {
		if _, err := annot.ParseOps([]byte(raw), annot.DefaultLimits()); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestAtomicAndImmutable(t *testing.T) {
	base := parse(t, `{"strokes":[{"id":"a"}]}`)
	before := materialize(t, base)
	bad := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "b", 1), {Op: annot.Del, Key: annot.ElementKey{Kind: annot.Strokes, ID: "a"}, Value: json.RawMessage("null")}}}
	if _, err := annot.Apply(base, bad, 1); err == nil {
		t.Fatal("accepted bad suffix")
	}
	if !bytes.Equal(before, materialize(t, base)) {
		t.Fatal("partial apply")
	}
	ops := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "b", 1)}}
	r := apply(t, base, ops, 1)
	saved := materialize(t, r.Sidecar)
	ops.Elements[0].Value[0] = 'x'
	r.Effective.Elements[0].Value[0] = 'x'
	if !bytes.Equal(saved, materialize(t, r.Sidecar)) {
		t.Fatal("aliased raw bytes")
	}
}
