package vpath

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
)

var updatePaths = flag.Bool("update-vpath", false, "regenerate path legality fixtures")

type parseRow struct {
	Name      string `json:"name"`
	Input     string `json:"input_base64"`
	Reason    string `json:"reason"`
	Component int    `json:"component"`
}

func TestParseFixtures(t *testing.T) {
	inputs := []string{"a.md", "Canvas/讲义.pdf.annot.json", ".gitignore", "\xff", "e\u0301.md", "", "/a", `a\b`, "a//b", "a/.", "a/..", "a/what?", "a/\x00b", "a/\u0085b", "a/ b", "a/b.", "a/b ", "CON.txt", "COM¹.txt", "LPT³", "nul.tar.gz", "COM0", "x?/..", strings.Repeat("x", 201) + "/y?", strings.Repeat("a", 200), strings.Repeat("a", 201), strings.Repeat("é", 100), strings.Repeat("é", 101), strings.Repeat("a/", 255) + "bc", strings.Repeat("a/", 256) + "b", strings.Repeat("x", 120) + ".annot.json"}
	rows := make([]parseRow, 0, len(inputs))
	for _, s := range inputs {
		_, e := Parse(s)
		r, i := ReasonNone, -1
		var pe *PathError
		if errors.As(e, &pe) {
			r, i = pe.Reason, pe.Component
		}
		rows = append(rows, parseRow{s, base64.StdEncoding.EncodeToString([]byte(s)), r.String(), i})
	}
	data, e := json.MarshalIndent(rows, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, '\n')
	path := "../../../schema/vpath/parse.json"
	if *updatePaths {
		if e := os.MkdirAll("../../../schema/vpath", 0750); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	got, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("path fixture drift; regenerate and review")
	}
}

type keyPair struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Equal bool   `json:"equal"`
}
type keyGolden struct {
	Note   string    `json:"note"`
	Scheme string    `json:"scheme"`
	Pairs  []keyPair `json:"pairs"`
}

func readGolden(t testing.TB) keyGolden {
	t.Helper()
	b, e := os.ReadFile("testdata/key-golden.json")
	if e != nil {
		t.Fatal(e)
	}
	var g keyGolden
	if e = json.Unmarshal(b, &g); e != nil {
		t.Fatal(e)
	}
	return g
}
func TestKeyGolden(t *testing.T) {
	g := readGolden(t)
	if g.Scheme != KeyScheme() {
		t.Fatalf("key scheme changed: %s != %s; explicit rekey review required", KeyScheme(), g.Scheme)
	}
	for _, p := range g.Pairs {
		if (pathKey(p.A) == pathKey(p.B)) != p.Equal {
			t.Errorf("%q vs %q", p.A, p.B)
		}
	}
}
