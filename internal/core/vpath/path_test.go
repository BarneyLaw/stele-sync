package vpath

import (
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"
)

func mustPath(t testing.TB, s string) Path {
	t.Helper()
	p, err := Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestParseReasons(t *testing.T) {
	for _, tc := range []struct {
		s         string
		r         Reason
		component int
	}{
		{"\xff", InvalidUTF8, -1}, {"e\u0301", NotNFC, -1}, {"", Empty, -1}, {"/a", Absolute, -1}, {`a\b`, Backslash, -1},
		{"a//b", EmptyComponent, 1}, {"a/.", DotComponent, 1}, {"a/..", Traversal, 1}, {"a/x?", IllegalChar, 1}, {"a/x\u0085", ControlChar, 1},
		{"a/ b", LeadingSpace, 1}, {"a/b.", TrailingDotOrSpace, 1}, {"a/COM1.txt", ReservedName, 1}, {"a/COM¹.txt", ReservedName, 1},
		{strings.Repeat("x", 201), ComponentTooLong, 0}, {strings.Repeat("a/", 256) + "b", PathTooLong, -1},
		// Structural checks precede character checks across the whole path.
		{"x?/..", Traversal, 1}, {strings.Repeat("x", 201) + "/y?", IllegalChar, 1},
	} {
		t.Run(tc.r.String()+tc.s[:min(len(tc.s), 10)], func(t *testing.T) {
			p, err := Parse(tc.s)
			var pe *PathError
			if !p.IsZero() || !errors.As(err, &pe) || pe.Reason != tc.r || pe.Component != tc.component || pe.Input != tc.s {
				t.Fatalf("%q: %v %#v", tc.s, err, pe)
			}
			if pe.Error() == "" {
				t.Fatal("empty error")
			}
		})
	}
	for _, s := range []string{"a", ".gitignore", "é/讲义.pdf", strings.Repeat("x", 200), strings.Repeat("a/", 255) + "bc"} {
		if _, e := Parse(s); e != nil {
			t.Fatal(e)
		}
	}
	if Reason(255).String() != "unknown" || ReasonNone.String() != "none" {
		t.Fatal("unstable reason")
	}
}
func TestPathAccessors(t *testing.T) {
	p := mustPath(t, "canvas/Notes.MD")
	d, ok := p.Dir()
	if !ok || d.String() != "canvas" || p.Base() != "Notes.MD" || p.Ext() != "md" || !p.Within(mustPath(t, "Canvas")) {
		t.Fatal(p)
	}
	if mustPath(t, "Canvasx/a").Within(mustPath(t, "Canvas")) || p.Within(Path{}) || (Path{}).Within(d) {
		t.Fatal("bad containment")
	}
	if !d.Within(d) {
		t.Fatal("self containment")
	}
	if _, ok := d.Dir(); ok {
		t.Fatal("root dir")
	}
	for _, s := range []string{".gitignore", "name"} {
		if mustPath(t, s).Ext() != "" {
			t.Fatal(s)
		}
	}
	q, e := d.Join("a.md")
	if e != nil || q.String() != "canvas/a.md" {
		t.Fatal(q, e)
	}
	q, e = p.WithBase("b.pdf")
	if e != nil || q.String() != "canvas/b.pdf" {
		t.Fatal(q, e)
	}
	q, e = d.WithBase("b")
	if e != nil || q.String() != "b" {
		t.Fatal(q, e)
	}
	for _, s := range []string{"x/y", "../x", "/x", ""} {
		if _, e := d.Join(s); e == nil {
			t.Fatalf("Join accepted %q", s)
		}
		if _, e := p.WithBase(s); e == nil {
			t.Fatalf("WithBase accepted %q", s)
		}
	}
	if q, e = (Path{}).Join("a"); e != nil || q.String() != "a" {
		t.Fatal(q, e)
	}
}
func TestPropParseRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		p, e := Parse(s)
		if e == nil {
			q, e := Parse(p.String())
			if e != nil || q != p {
				t.Fatal("round trip")
			}
		}
	})
}
func TestPropSanitizeIsStrict(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "s")
		p, e := Sanitize(s)
		if e == nil {
			q, e := Parse(p.String())
			if e != nil || q != p {
				t.Fatal("sanitize not strict")
			}
		}
	})
}
func TestPropParseCharacterRules(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := NFC(rapid.String().Draw(t, "s"))
		if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\") || len(s) > 120 {
			return
		}
		_, e := Parse(s)
		want := characterPass(s) == s && !deviceReservedName(s)
		if (e == nil) != want {
			t.Fatalf("%q %v", s, e)
		}
		if want {
			c, e := Component(s)
			if e != nil || c != s {
				t.Fatalf("not fixed %q -> %q", s, c)
			}
		}
	})
}
func TestPropKeyIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := NFC(rapid.String().Draw(t, "s"))
		k := pathKey(s)
		if pathKey(string(k)) != k {
			t.Fatalf("%q -> %q", s, k)
		}
	})
}
func TestPropKeyCoarserThanFold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := NFC(rapid.String().Draw(t, "s"))
		f := norm.NFC.String(cases.Fold().String(norm.NFD.String(s)))
		if pathKey(s) != pathKey(f) {
			t.Fatalf("%q != fold %q", s, f)
		}
	})
}
func TestPropKeyCoarserThanUpper(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := NFC(rapid.String().Draw(t, "s"))
		u := norm.NFC.String(strings.Map(unicode.ToUpper, s))
		if pathKey(s) != pathKey(u) {
			t.Fatalf("%q != upper %q", s, u)
		}
	})
}
func FuzzParse(f *testing.F) {
	for _, s := range []string{"a.md", "e\u0301", "a/..", "\xff", "COM¹", "α"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, e := Parse(s)
		if e != nil {
			var pe *PathError
			if !errors.As(e, &pe) || pe.Reason == ReasonNone || !p.IsZero() {
				t.Fatal(e)
			}
			return
		}
		if !utf8.ValidString(p.String()) {
			t.Fatal(p)
		}
		if q, e := Parse(p.String()); e != nil || q != p {
			t.Fatal("round trip")
		}
	})
}
func FuzzKey(f *testing.F) {
	for _, s := range []string{"ß", "ı", "İ", "ᾀ", "Σ", "ﬁ", "\u0345\u0301"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		s = NFC(s)
		k := pathKey(s)
		if pathKey(string(k)) != k {
			t.Fatalf("not idempotent %q -> %q", s, k)
		}
	})
}

func TestWindowsConsoleAndSpacedDeviceNames(t *testing.T) {
	for _, s := range []string{"CONIN$", "conout$.txt", "CON .txt", "COM¹ .md", "LPT1 .pdf"} {
		_, err := Parse(s)
		var pe *PathError
		if !errors.As(err, &pe) || pe.Reason != ReservedName {
			t.Errorf("accepted Windows device alias %q: %v", s, err)
		}
	}
}
