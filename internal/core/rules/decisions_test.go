package rules

import (
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

func path(t testing.TB, s string) vpath.Path {
	t.Helper()
	p, e := vpath.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func file(t testing.TB, s string, c classify.Class, owner Owner) File {
	return File{Live: true, Head: 10, Class: c, Owner: owner, Path: path(t, s), Size: 1}
}
func TestDecisionTables(t *testing.T) {
	cfg := DefaultConfig()
	for _, a := range []Actor{Device, Bridge, Admin} {
		for _, c := range []classify.Class{classify.Text, classify.Sidecar, classify.Opaque} {
			for _, p := range []string{"notes.md", "canvas/a.pdf"} {
				for _, h := range []Holder{Free, Self, Other} {
					for _, on := range []OnTaken{Number, Reject} {
						want := Apply
						if h != Free {
							want = ApplyNumbered
							if on == Reject {
								want = PathTaken
							}
						}
						inside := p == "canvas/a.pdf"
						if a == Device && c == classify.Opaque && inside || a == Bridge && (c != classify.Opaque || !inside) {
							want = ReadOnlyPath
						}
						if got := DecideCreate(a, path(t, p), c, h, on, cfg); got != want {
							t.Fatalf("create %d %c %s %d %d: %v != %v", a, c, p, h, on, got, want)
						}
					}
				}
			}
		}
	}
	for _, a := range []Actor{Device, Bridge, Admin} {
		for _, owner := range []Owner{User, Canvas} {
			for _, live := range []bool{true, false} {
				for _, base := range []int64{-1, 9, 10, 11} {
					f := file(t, "Canvas/a.pdf", classify.Opaque, owner)
					f.Live = live
					denied := a == Device || a == Bridge && owner != Canvas
					want := Apply
					if base < 10 {
						want = ConflictCopy
					}
					if base < 0 || base > 10 {
						want = InvalidBase
					}
					if !live {
						want = FileDeleted
					}
					if denied {
						want = ReadOnlyPath
					}
					if got := DecidePut(a, f, base, cfg); got != want {
						t.Fatal("put", a, owner, live, base, got, want)
					}
					if want == ConflictCopy {
						want = StaleDelete
					}
					if got := DecideDelete(a, f, base, cfg); got != want {
						t.Fatal("delete", got, want)
					}
				}
			}
		}
	}
	f := file(t, "a.pdf.annot.json", classify.Sidecar, User)
	for _, tc := range []struct {
		base int64
		want Decision
	}{{9, ConflictCopy}, {10, Apply}, {11, InvalidBase}, {-1, InvalidBase}} {
		if got := DecideReplace(Device, f, tc.base, cfg); got != tc.want {
			t.Fatal("replace", got)
		}
	}
	if DecideEdit(Device, f, cfg) != Apply {
		t.Fatal("edit")
	}
	f.Live = false
	if DecideEdit(Device, f, cfg) != FileDeleted || DecideReplace(Device, f, 10, cfg) != FileDeleted {
		t.Fatal("deleted")
	}
	f = file(t, "a.md", classify.Text, User)
	if DecidePut(Device, f, 10, cfg) != ClassChange || DecideReplace(Device, f, 10, cfg) != ClassChange {
		t.Fatal("wrong operation class")
	}
	f.Class = classify.Opaque
	if DecideEdit(Device, f, cfg) != ClassChange {
		t.Fatal("opaque edit")
	}
	f = file(t, "outside.pdf", classify.Opaque, Canvas)
	if DecideDelete(Bridge, f, 10, cfg) != ReadOnlyPath {
		t.Fatal("bridge outside root")
	}
}
func TestRename(t *testing.T) {
	cfg := DefaultConfig()
	f := file(t, "a.md", classify.Text, User)
	for _, tc := range []struct {
		to   string
		h    Holder
		on   OnTaken
		want Decision
	}{{"a.md", Other, Reject, Apply}, {"A.md", Self, Reject, Apply}, {"b.md", Free, Number, Apply}, {"b.md", Other, Number, ApplyNumbered}, {"b.md", Other, Reject, PathTaken}, {"b.pdf", Free, Number, ClassChange}} {
		r := RenameRequest{Primary: f, To: path(t, tc.to), Holder: tc.h, OnTaken: tc.on}
		if got := DecideRename(Device, r, cfg); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	f.Class = classify.Opaque
	if got := DecideRename(Device, RenameRequest{Primary: f, To: path(t, "b.md")}, cfg); got != Apply {
		t.Fatal("demoted rename", got)
	}
	f.Live = false
	if DecideRename(Device, RenameRequest{Primary: f, To: path(t, "b.md")}, cfg) != FileDeleted {
		t.Fatal("deleted")
	}
	f = file(t, "a.pdf", classify.Opaque, User)
	sc := file(t, "a.pdf.annot.json", classify.Sidecar, User)
	r := RenameRequest{Primary: f, To: path(t, "b.pdf"), SidecarLive: true}
	if DecideRename(Device, r, cfg) != GroupRequired {
		t.Fatal("ungrouped")
	}
	r.Sidecar = &sc
	r.SidecarTo = path(t, "b.pdf.annot.json")
	if DecideRename(Device, r, cfg) != Apply {
		t.Fatal("group")
	}
	r.SidecarHolder = Other
	if DecideRename(Device, r, cfg) != ApplyNumbered {
		t.Fatal("sidecar collision")
	}
	r.OnTaken = Reject
	if DecideRename(Device, r, cfg) != PathTaken {
		t.Fatal("sidecar taken")
	}
	r.SidecarTo = path(t, "wrong.pdf.annot.json")
	if DecideRename(Device, r, cfg) != GroupRequired {
		t.Fatal("pair")
	}
	r.SidecarTo = path(t, "b.pdf.annot.json")
	sc.Live = false
	if DecideRename(Device, r, cfg) != FileDeleted {
		t.Fatal("dead member")
	}
	sc.Live = true
	sc.Owner = Canvas
	if DecideRename(Device, r, cfg) != ReadOnlyPath {
		t.Fatal("member ownership")
	}
	r = RenameRequest{Primary: f, To: path(t, "canvas/b.pdf")}
	if DecideRename(Device, r, cfg) != ReadOnlyPath {
		t.Fatal("case ownership")
	}
}
func TestReconcileBoundaries(t *testing.T) {
	w := DefaultConfig().Window
	for _, tc := range []struct {
		c    classify.Class
		g    Gap
		want Decision
	}{{classify.Text, Gap{Ops: 5000, Age: 24 * time.Hour}, Apply}, {classify.Text, Gap{Ops: 5001}, RebaseRequired}, {classify.Text, Gap{Age: 24*time.Hour + 1}, RebaseRequired}, {classify.Text, Gap{OpsPruned: true}, RebaseRequired}, {classify.Text, Gap{TombstonesPruned: true}, Apply}, {classify.Sidecar, Gap{Ops: 999999, Age: 100 * time.Hour, OpsPruned: true}, Apply}, {classify.Sidecar, Gap{TombstonesPruned: true}, RebaseRequired}, {classify.Opaque, Gap{OpsPruned: true, TombstonesPruned: true}, Apply}} {
		if got := Reconcile(tc.c, tc.g, w); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
