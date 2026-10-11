package vpath

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"golang.org/x/text/unicode/norm"
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
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
	// Unicode 16 compositions must stay legal decomposed paths under the pinned
	// Unicode 15 server tables, even on newer JavaScript runtimes.
	inputs = append(inputs, "\U000105d2\u0307.md", "\U00011382\U000113c9.md", "\U0001611e\U0001611e.md", "\U00016d67\U00016d67.md")
	inputs = append(inputs, "x"+strings.Repeat("\u0301", 31), strings.Repeat("\u1161", 31), "x"+strings.Repeat("\uff9e", 31), "a\u0315\u0897\u0300")
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

// TestNormalizationFixture freezes which scalars the pinned normalization table
// knows. Newer JS normalizers treat older-unassigned scalars as barriers, matching
// Go, rather than introducing new decompositions/combining classes/compositions.
func TestNormalizationFixture(t *testing.T) {
	var ranges [][2]int
	start := -1
	for r := rune(0); r <= utf8.MaxRune+1; r++ {
		assigned := r <= utf8.MaxRune && unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs)
		if assigned && start < 0 {
			start = int(r)
		}
		if !assigned && start >= 0 {
			ranges = append(ranges, [2]int{start, int(r) - 1})
			start = -1
		}
	}
	if norm.Version != unicode.Version {
		t.Fatal("normalization and assignment table versions must match")
	}
	var stream [][4]int
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		decomposition := []rune(norm.NFKD.String(string(r)))
		lead, trail := 0, 0
		for _, d := range decomposition {
			if norm.NFKD.PropertiesString(string(d)).BoundaryBefore() {
				break
			}
			lead++
		}
		for i := len(decomposition) - 1; i >= 0; i-- {
			if norm.NFKD.PropertiesString(string(decomposition[i])).BoundaryBefore() {
				break
			}
			trail++
		}
		if lead == 0 && trail == 0 {
			continue
		}
		if len(stream) > 0 {
			last := &stream[len(stream)-1]
			if last[1]+1 == int(r) && last[2] == lead && last[3] == trail {
				last[1] = int(r)
				continue
			}
		}
		stream = append(stream, [4]int{int(r), int(r), lead, trail})
	}
	type sample struct {
		Input string `json:"input"`
		NFC   string `json:"nfc"`
	}
	var samples []sample
	for _, r := range []rune{'\u0301', '\u0344', '\u1161', '\uff9e', '\u0f77', 'é', '각', '\U000105d2', '\U00011382'} {
		for _, n := range []int{1, 29, 30, 31, 32, 61} {
			s := "a" + strings.Repeat(string(r), n) + "\u0300"
			samples = append(samples, sample{s, NFC(s)})
		}
	}
	fixture := struct {
		Version string   `json:"unicode_version"`
		Ranges  [][2]int `json:"assigned_ranges"`
		Stream  [][4]int `json:"stream_ranges"`
		Samples []sample `json:"samples"`
	}{unicode.Version, ranges, stream, samples}
	data, e := json.Marshal(fixture)
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, '\n')
	name := "../../../schema/vpath/normalization.json"
	if *updatePaths {
		if e := os.WriteFile(name, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	got, e := os.ReadFile(name)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("normalization assignment fixture drift; review Unicode version")
	}
}
