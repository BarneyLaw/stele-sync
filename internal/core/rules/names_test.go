package rules

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
	"pgregory.net/rapid"
)

func TestPropFreeName(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "occupied")
		stem := rapid.StringMatching(`[a-zé]{1,60}`).Draw(t, "stem")
		ext := rapid.SampledFrom([]string{".md", ".PDF", ".annot.json", ""}).Draw(t, "ext")
		p, e := vpath.Parse(stem + ext)
		if e != nil {
			t.Fatal(e)
		}
		cfg := DefaultConfig()
		cfg.MaxAttempts = 8
		calls := 0
		occupied := map[vpath.Key]bool{}
		for i := 0; i < n; i++ {
			q, err := vpath.Parse(fmt.Sprintf("%s (%d)%s", stem, i+2, ext))
			if err != nil {
				t.Fatal(err)
			}
			occupied[q.Key()] = true
		}
		taken := func(q vpath.Path) (bool, error) { calls++; return occupied[q.Key()], nil }
		q, e := FreeName(p, Marker{Kind: Numbered}, cfg, taken)
		if n == 8 {
			if !errors.Is(e, ErrNoFreeName) || calls != 8 {
				t.Fatal(e, calls)
			}
			return
		}
		if e != nil {
			t.Fatal(e)
		}
		if occupied[q.Key()] {
			t.Fatal("allocated an occupied key")
		}
		if _, e := vpath.Parse(q.String()); e != nil || !strings.HasSuffix(q.String(), ext) || len(q.Base()) > 200 {
			t.Fatal(q, e)
		}
		calls = 0
		r, e := FreeName(p, Marker{Kind: Numbered}, cfg, taken)
		if e != nil || q != r {
			t.Fatal("nondeterministic")
		}
	})
}
func TestNames(t *testing.T) {
	cfg := DefaultConfig()
	free := func(vpath.Path) (bool, error) { return false, nil }
	for _, tc := range []struct{ in, want string }{{"a.md", "a (2).md"}, {"a (2).md", "a (3).md"}, {"a (1).md", "a (1) (2).md"}, {".gitignore", ".gitignore (2)"}, {"x.pdf.annot.json", "x.pdf (2).annot.json"}, {"x.ANNOT.JSON", "x (2).ANNOT.JSON"}} {
		p, e := FreeName(path(t, tc.in), Marker{Kind: Numbered}, cfg, free)
		if e != nil || p.String() != tc.want {
			t.Fatal(tc, p, e)
		}
	}
	m := Marker{Kind: Conflict, Device: `bad/name`, At: time.Date(2026, 10, 11, 23, 7, 0, 0, time.FixedZone("display", 8*3600))}
	p, e := FreeName(path(t, "Lecture.pdf.annot.json"), m, cfg, free)
	if e != nil || p.String() != "Lecture.pdf (conflict 2026-10-11 2307 bad-name).annot.json" {
		t.Fatal(p, e)
	}
	if _, ok := PDFOf(p); ok || classify.Expect(p, 1, classify.DefaultConfig()) != classify.Opaque {
		t.Fatal("paired conflict")
	}
	m.Device = "\x00"
	p, e = FreeName(path(t, "a.md"), m, cfg, free)
	if e != nil || !strings.Contains(p.String(), " device)") {
		t.Fatal(p, e)
	}
	sentinel := errors.New("lookup")
	_, e = FreeName(path(t, "a.md"), m, cfg, func(vpath.Path) (bool, error) { return false, sentinel })
	if !errors.Is(e, sentinel) || reflect.ValueOf(e).Pointer() != reflect.ValueOf(sentinel).Pointer() {
		t.Fatal(e)
	}
	cfg.MaxAttempts = 0
	if _, e = FreeName(path(t, "a.md"), m, cfg, free); !errors.Is(e, ErrInvalidConfig) {
		t.Fatal(e)
	}
	cfg = DefaultConfig()
	p, e = FreeName(path(t, strings.Repeat("é", 98)+".md"), m, cfg, free)
	if e != nil || len(p.Base()) > 200 || !utf8.ValidString(p.String()) {
		t.Fatal(p, e)
	}
	long := strings.Repeat("a/", 253) + "x.md"
	if _, e = FreeName(path(t, long), m, cfg, free); !errors.Is(e, ErrNameTooLong) {
		t.Fatal("total", e)
	}
}
func TestGroupNumbering(t *testing.T) {
	cfg := DefaultConfig()
	p := path(t, "a.pdf")
	calls := []string{}
	taken := func(q vpath.Path) (bool, error) {
		calls = append(calls, q.String())
		return q.String() == "a (2).pdf.annot.json", nil
	}
	q, e := FreeName(p, Marker{Kind: Numbered}, cfg, GroupTaken(taken))
	if e != nil || q.String() != "a (3).pdf" {
		t.Fatal(q, e, calls)
	}
	sc, e := SidecarOf(q)
	if e != nil {
		t.Fatal(e)
	}
	pdf, ok := PDFOf(sc)
	if !ok || pdf != q {
		t.Fatal("pair")
	}
	if _, ok := PDFOf(path(t, "a.annot.json")); ok {
		t.Fatal("not pdf")
	}
	if _, e := SidecarOf(path(t, "a.md")); e == nil {
		t.Fatal("not pdf")
	}
}

func TestPairingUsesClassifierSuffix(t *testing.T) {
	if _, ok := PDFOf(path(t, "x.pdf.annot.jſon")); ok {
		t.Fatal("Unicode folding must not recognize a non-ASCII sidecar suffix")
	}
}
