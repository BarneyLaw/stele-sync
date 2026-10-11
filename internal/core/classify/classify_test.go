package classify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
	"pgregory.net/rapid"
)

func path(t testing.TB, s string) vpath.Path {
	t.Helper()
	p, e := vpath.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestClassification(t *testing.T) {
	c := DefaultConfig()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		b      []byte
		want   Class
		reason Demotion
	}{
		{"a.md", []byte("\ufeffa\r\n"), Text, Kept}, {"a.txt", append(bytes.Repeat([]byte{'x'}, 9000), 0), Opaque, ContainsNUL},
		{"a.md", []byte{0xc3}, Opaque, InvalidUTF8}, {"a.pdf.annot.json", []byte(`{"strokes":[]}`), Sidecar, Kept},
		{"a.PDF.ANNOT.JSON", []byte(`{"strokes":[]}`), Sidecar, Kept}, {".pdf.annot.json", []byte(`{"strokes":[]}`), Opaque, Kept},
		{"a.pdf.annot.json", []byte(`{"strokes":[],"strokes":[]}`), Opaque, NotFreedraw}, {"a.pdf (conflict x).annot.json", []byte(`{"strokes":[]}`), Opaque, Kept},
		{"a.json", []byte(`{"strokes":[]}`), Opaque, Kept},
	} {
		t.Run(tc.name+tc.reason.String(), func(t *testing.T) {
			got, d := Classify(path(t, tc.name), tc.b, c)
			if got != tc.want || d != tc.reason {
				t.Fatalf("%c %v", got, d)
			}
		})
	}
	c.TextExts = append(c.TextExts, "json")
	if got := Expect(path(t, "a.pdf.annot.json"), 1, c); got != Sidecar {
		t.Fatal(got)
	}
	if Expect(path(t, "a.md"), -1, c) != Opaque {
		t.Fatal("negative size")
	}
	for _, tc := range []struct {
		class Class
		size  int
	}{{Text, c.MaxText}, {Sidecar, c.MaxSidecar}} {
		b := bytes.Repeat([]byte{'x'}, tc.size+1)
		if got, d := Confirm(tc.class, b, c); got != Opaque || d != TooLarge {
			t.Fatal(got, d)
		}
	}
	if Expect(path(t, "a.md"), int64(c.MaxText+1), c) != Opaque || Expect(path(t, "a.pdf.annot.json"), int64(c.MaxSidecar+1), c) != Opaque {
		t.Fatal("size ceiling")
	}
}
func TestConfig(t *testing.T) {
	for _, mut := range []func(*Config){func(c *Config) { c.MaxText = 0 }, func(c *Config) { c.MaxSidecar++ }, func(c *Config) { c.Annot.MaxDepth = -1 }, func(c *Config) { c.TextExts = []string{"MD"} }, func(c *Config) { c.TextExts = []string{".md"} }, func(c *Config) { c.TextExts = []string{"md", "md"} }, func(c *Config) { c.TextExts = []string{""} }, func(c *Config) { c.TextExts = []string{"a/b"} }} {
		c := DefaultConfig()
		mut(&c)
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
}
func TestSamplesAreSidecars(t *testing.T) {
	files, e := filepath.Glob("../../../schema/annot/samples/*.annot.json")
	if e != nil || len(files) == 0 {
		t.Fatal(e)
	}
	for _, f := range files {
		b, e := os.ReadFile(f) // #nosec G304 -- fixed repository sample glob.
		if e != nil {
			t.Fatal(e)
		}
		p := path(t, "sample.pdf.annot.json")
		if c, d := Classify(p, b, DefaultConfig()); c != Sidecar || d != Kept {
			t.Fatalf("%s: %c %s", f, c, d)
		}
	}
}
func TestPropConfirmNeverUpgrades(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := rapid.SampledFrom([]Class{Text, Sidecar, Opaque}).Draw(t, "class")
		b := rapid.SliceOfN(rapid.Byte(), 0, 128).Draw(t, "bytes")
		a, _ := Confirm(c, b, DefaultConfig())
		if a != c && a != Opaque {
			t.Fatal("upgraded")
		}
	})
}
func FuzzClassify(f *testing.F) {
	f.Add("x.pdf.annot.json", []byte(`{"strokes":[]}`))
	f.Add("x.md", []byte{0xff})
	f.Fuzz(func(t *testing.T, s string, b []byte) {
		p, e := vpath.Parse(s)
		if e != nil {
			return
		}
		cfg := DefaultConfig()
		c, d := Classify(p, b, cfg)
		q, r := Classify(p, b, cfg)
		if c != q || d != r {
			t.Fatal("nondeterministic")
		}
		expected := Expect(p, int64(len(b)), cfg)
		if c != expected && c != Opaque {
			t.Fatal("upgraded")
		}
	})
}

func TestRecoverySidecarsNeverBecomeText(t *testing.T) {
	c := DefaultConfig()
	c.TextExts = append(c.TextExts, "json")
	for _, name := range []string{"a.pdf (conflict date laptop).annot.json", "a.pdf.recovery.annot.json", ".pdf.annot.json"} {
		if got := Expect(path(t, name), 1, c); got != Opaque {
			t.Fatalf("%s became %c", name, got)
		}
	}
}
