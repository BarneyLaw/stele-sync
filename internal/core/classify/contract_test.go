package classify

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var updateClassify = flag.Bool("update-classify", false, "regenerate classification contracts")

type expectRow struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Class  string `json:"class"`
	Config Config `json:"config"`
}
type confirmRow struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Content  string `json:"content_base64"`
	Repeat   int    `json:"repeat"`
	Class    string `json:"class"`
	Demotion string `json:"demotion"`
	Config   Config `json:"config"`
}

func TestClassifyFixtures(t *testing.T) {
	cfg := DefaultConfig()
	var ex []expectRow
	for _, p := range []string{"a.md", "a.MDX", "a.txt", "a.pdf.annot.json", "a.PDF.ANNOT.JSON", ".pdf.annot.json", "x.annot.json", "a.pdf (conflict x).annot.json", "x.json", "x.pdf"} {
		for _, size := range []int64{-1, 0, 8 << 20, (8 << 20) + 1, 16 << 20, (16 << 20) + 1} {
			ex = append(ex, expectRow{p, size, string(Expect(path(t, p), size, cfg)), cfg})
		}
	}
	cfg.TextExts = append(cfg.TextExts, "json")
	for _, p := range []string{"a.json", "a.pdf.annot.json", "a.pdf (conflict x).annot.json"} {
		ex = append(ex, expectRow{p, 1, string(Expect(path(t, p), 1, cfg)), cfg})
	}
	var co []confirmRow
	add := func(name string, c Class, b []byte, n int, cfg Config) {
		got, d := Confirm(c, bytes.Repeat(b, n), cfg)
		co = append(co, confirmRow{name, string(c), base64.StdEncoding.EncodeToString(b), n, string(got), d.String(), cfg})
	}
	cfg = DefaultConfig()
	for _, b := range []string{"", "hello\r\n", "\ufeffa", strings.Repeat("x", 9000) + "\x00", "cut\xc3"} {
		add("text", Text, []byte(b), 1, cfg)
	}
	for _, c := range []Class{Text, Sidecar, Opaque} {
		add("oversized", c, []byte{'x'}, cfg.MaxSidecar+1, cfg)
	}
	for _, b := range []string{`{"strokes":[]}`, `{}`, `null`, `{"strokes":null}`, `{"strokes":[],"strokes":[]}`, `{"strokes":[{"id":"a"},{"id":"a"}]}`, `{"strokes":[{"id":""}]}`, `{"strokes":[{"id":1}]}`, `{"strokes":[{"id":"\ud800"}]}`, `{"strokes":[],"x":{"a":1,"\u0061":2}}`, `{"strokes":[],"x":1e999}`, `{"strokes":[],"deletedPdfPages":[1,1]}`, `{"strokes":[],"deletedPdfPages":[1.0]}`, `{"strokes":[],"deletedPdfPages":[2147483648]}`, `{"strokes":[],"deletedPdfPages":[1],"permanentlyDeletedPdfPages":[1]}`, `{"strokes":[],"pdfPageTemplates":[{"page":1}]}`, `{"strokes":[],"appendedPages":[]}`, `{"strokes":[],"removedPages":{}}`, `{"strokes":[],"future":"<&>"}`, `{"strokes":[],"future":"\u2028\u2029"}`, `{"strokes":[],"future":"\ud83d\ude00"}`} {
		add("sidecar", Sidecar, []byte(b), 1, cfg)
	}
	for _, mut := range []func(*Config){func(c *Config) { c.MaxSidecar = 64; c.Annot.MaxBytes = 64 }, func(c *Config) { c.Annot.MaxDepth = 2 }, func(c *Config) { c.Annot.MaxNodes = 3 }, func(c *Config) { c.Annot.MaxElements = 1 }, func(c *Config) { c.Annot.MaxIDBytes = 1 }} {
		c := DefaultConfig()
		mut(&c)
		for _, b := range []string{`{"strokes":[]}`, `{"strokes":[{"id":"abc"},{"id":"b"}]}`, `{"strokes":[],"future":[[0]]}`, `{"strokes":[{"id":"a"}],"deletedPdfPages":[1]}`} {
			add("annotation-budget", Sidecar, []byte(b), 1, c)
		}
	}
	writeFixture(t, "expect", ex)
	writeFixture(t, "confirm", co)
}
func writeFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, '\n')
	dir := "../../../schema/classify"
	p := dir + "/" + name + ".json"
	if *updateClassify {
		if e := os.MkdirAll(dir, 0750); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	got, e := os.ReadFile(p) // #nosec G304 -- fixed repository fixture path.
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("%s fixture drift", name)
	}
}
