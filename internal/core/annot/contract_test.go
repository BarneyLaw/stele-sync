package annot_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
)

var updateAnnot = flag.Bool("update-annot", false, "regenerate reviewed annotation contract fixtures")
var annotFixtureDir = flag.String("annot-fixtures-dir", "../../../schema/annot", "annotation fixture output/check directory")

func errorName(err error) string {
	for _, e := range []struct {
		name string
		err  error
	}{
		{"invalid_json", annot.ErrInvalidJSON}, {"not_sidecar", annot.ErrNotSidecar},
		{"duplicate_key", annot.ErrDuplicateKey}, {"duplicate_id", annot.ErrDuplicateID}, {"missing_id", annot.ErrMissingID},
		{"structural_key", annot.ErrStructuralKey}, {"whole_document", annot.ErrWholeDocument},
		{"non_canonical", annot.ErrNonCanonical}, {"limit", annot.ErrLimit}, {"invalid_limits", annot.ErrInvalidLimits}, {"invalid_state", annot.ErrInvalidState},
	} {
		if errors.Is(err, e.err) {
			return e.name
		}
	}
	if err == nil {
		return ""
	}
	return "unexpected"
}
func stateBytes(t testing.TB, sc annot.Sidecar) string {
	t.Helper()
	b, err := annot.EncodeState(sc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func opBytes(t testing.TB, o annot.Ops) string {
	t.Helper()
	b, err := o.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func fixtureData(t *testing.T) map[string][]map[string]any {
	t.Helper()
	data := map[string][]map[string]any{}
	names, err := filepath.Glob("../../../schema/annot/samples/*.annot.json")
	if err != nil || len(names) == 0 {
		t.Fatalf("samples: %v", err)
	}
	for _, name := range names {
		raw, err := os.ReadFile(name) // #nosec G304 G703 -- fixed repository fixture glob.
		if err != nil {
			t.Fatal(err)
		}
		sc := parse(t, string(raw))
		hash := annot.CanonicalHash(sc)
		data["parse"] = append(data["parse"], map[string]any{"name": filepath.Base(name), "input": string(raw), "canonical": string(materialize(t, sc)), "hash": hex.EncodeToString(hash[:]), "whole_document": annot.RequiresWholeDocument(sc)})
	}
	for _, tc := range []struct{ name, base, cur string }{
		{"element-and-metadata", `{"strokes":[{"id":"a","v":1}],"remove":1}`, `{"strokes":[{"id":"a","v":2},{"id":"b"}],"future":null}`},
		{"entry-families", `{"strokes":[],"deletedPdfPages":[1],"pdfPageTemplates":[{"page":2,"template":"grid"}]}`, `{"strokes":[],"deletedPdfPages":[3],"permanentlyDeletedPdfPages":[1],"pdfPageTemplates":[{"page":2,"template":"ruled"}]}`},
		{"volatile-mask", `{"strokes":[],"updatedAt":"a","sourcePdf":{"path":"a.pdf","ctime":1,"mtime":2}}`, `{"strokes":[],"updatedAt":"b","sourcePdf":{"path":"a.pdf","ctime":3,"mtime":4}}`},
		{"quarantine", `{"strokes":[]}`, `{"strokes":[],"appendedPages":[{"id":"p"}]}`},
	} {
		ops, err := annot.Diff(parse(t, tc.base), parse(t, tc.cur))
		row := map[string]any{"name": tc.name, "base": tc.base, "current": tc.cur}
		if err != nil {
			row["error"] = errorName(err)
		} else {
			row["ops"] = opBytes(t, ops)
		}
		data["diff"] = append(data["diff"], row)
	}
	base := parse(t, `{"strokes":[{"id":"a"}]}`)
	key := annot.ElementKey{Kind: annot.Strokes, ID: "a"}
	dead := apply(t, base, annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: key}}}, 1).Sidecar
	for _, tc := range []struct {
		name string
		head annot.Sidecar
		ops  annot.Ops
		at   int64
	}{
		{"delete-wins", dead, annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", 9), put(annot.Strokes, "b", 1)}}, 2},
		{"kind-scoped-ids", dead, annot.Ops{Elements: []annot.ElementOp{put(annot.TextItems, "a", 1)}}, 2},
		{"replace-restores", dead, annot.ReplaceOps(dead, base), 2},
		{"meta-delete-versus-null", base, annot.Ops{MetaPut: map[string]json.RawMessage{"null": json.RawMessage("null")}, MetaDel: []string{"unknown"}}, 1},
	} {
		r := apply(t, tc.head, tc.ops, tc.at)
		data["apply"] = append(data["apply"], map[string]any{"name": tc.name, "state": stateBytes(t, tc.head), "ops": opBytes(t, tc.ops), "trusted_replace": tc.ops.Replace, "at": tc.at, "want_state": stateBytes(t, r.Sidecar), "canonical": string(materialize(t, r.Sidecar)), "effective": opBytes(t, r.Effective), "dropped": r.Dropped})
		data["state"] = append(data["state"], map[string]any{"name": tc.name, "input": stateBytes(t, r.Sidecar), "canonical": string(materialize(t, r.Sidecar)), "want_state": stateBytes(t, r.Sidecar)})
	}
	for _, tc := range []struct{ name, kind, raw string }{
		{"duplicate-key", "file", `{"strokes":[],"x":{"a":1,"a":2}}`},
		{"duplicate-id", "file", `{"strokes":[{"id":"a"},{"id":"a"}]}`},
		{"missing-id", "file", `{"strokes":[{}]}`},
		{"lone-surrogate", "file", `{"strokes":[],"x":"\ud800"}`},
		{"structural-edit", "ops", `{"meta":{"put":{"removedPages":[]}}}`},
		{"forged-replace", "ops", `{"replace":true}`},
		{"unordered-deletes", "ops", `{"meta":{"del":["z","a"]}}`},
		{"future-state", "state", `{"v":2,"version":0,"content":{"strokes":[]},"order":[],"tombstones":[]}`},
	} {
		var err error
		switch tc.kind {
		case "file":
			_, err = annot.Parse([]byte(tc.raw), annot.DefaultLimits())
		case "ops":
			_, err = annot.ParseOps([]byte(tc.raw), annot.DefaultLimits())
		case "state":
			_, err = annot.DecodeState([]byte(tc.raw), annot.DefaultLimits())
		}
		if err == nil {
			t.Fatalf("invalid fixture accepted: %s", tc.name)
		}
		data["invalid"] = append(data["invalid"], map[string]any{"name": tc.name, "kind": tc.kind, "input": tc.raw, "error": errorName(err)})
	}
	return data
}

// TestAnnotContractFixtures is both the generator and a drift check. These are
// porting contracts, not an independent correctness oracle; model tests supply it.
func TestAnnotContractFixtures(t *testing.T) {
	data := fixtureData(t)
	for _, name := range []string{"parse", "diff", "apply", "state", "invalid"} {
		envelope := map[string]any{"_comment": "Generated by TestAnnotContractFixtures; do not hand-edit. Raw JSON is carried in strings to preserve exact bytes. Correctness is established by model/property tests, not generation.", "cases": data[name]}
		want, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		path := filepath.Join(*annotFixtureDir, name+".json")
		if *updateAnnot {
			if err := os.MkdirAll(*annotFixtureDir, 0700); err != nil {
				t.Fatal(err)
			} // #nosec G703 -- explicit generator destination.
			if err := os.WriteFile(path, want, 0600); err != nil {
				t.Fatal(err)
			} // #nosec G703 -- fixed fixture names in explicit generator destination.
		}
		got, err := os.ReadFile(path) // #nosec G304 G703 -- fixed fixture names in explicit generator destination.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s drift; regenerate and review", name)
		}
	}
}
