package textop_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{`[]`, `[1,"😀",-2]`, `["a"]`, `["\ud800x"]`, `["\ud83d\ude00"]`, `["\\ud800"]`, `[1e0]`, `[-9223372036854775808]`, `{}`, `["<>&\u2028\u2029\u0000"]`, "[\"\xff\"]"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		op, err := textop.Parse(raw)
		if err != nil {
			return
		}
		if err := checkOp(op); err != nil {
			t.Fatal(err)
		}
		if err := equalOp(op, raw); err != nil {
			t.Fatal(err)
		}
	})
}

func FuzzInsertEscapes(f *testing.F) {
	for _, seed := range []string{`"a"`, `"\ud800x"`, `"\ud800"`, `"\udc00"`, `"\ud83d\ude00"`, `"\\ud800"`, `"�"`, `"\ud800\u0041"`, `"\u0000"`, `"\q"`, `"\uD83D\uDE00"`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, literal string) {
		// Only syntactically valid JSON string literals belong to this focused
		// property. FuzzParse owns all other JSON shapes and invalid UTF-8.
		raw := bytes.TrimSpace([]byte(literal))
		if len(raw)+2 > textop.DefaultLimits().MaxJSONBytes {
			return
		}
		if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' || !utf8.Valid(raw) || !json.Valid(raw) {
			return
		}
		op, err := textop.Parse(append(append([]byte{'['}, raw...), ']'))
		if scalarString(raw) != nil {
			if !errors.Is(err, textop.ErrInvalidUnicode) {
				t.Fatalf("malformed surrogate became %s: %v", encoded(op), err)
			}
			return
		}
		var want string
		if e := json.Unmarshal(raw, &want); e != nil {
			t.Fatal(e)
		}
		if want == "" {
			if !errors.Is(err, textop.ErrNonCanonical) {
				t.Fatal(err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		got, e := applyChecked(textop.Doc{}, op)
		if e != nil || got.String() != want {
			t.Fatalf("literal changed: %v", e)
		}
	})
}

func FuzzApply(f *testing.F) {
	for _, seed := range [][2]string{{"A😀B", `[1,-2,1]`}, {"😀", `[1,-1]`}, {"ab", `[1,"<>&",1]`}, {"", `[]`}, {"\xff", `[]`}, {"e\u0301", `[1,-1]`}} {
		f.Add([]byte(seed[0]), []byte(seed[1]))
	}
	f.Fuzz(func(t *testing.T, docBytes, raw []byte) {
		d, err := textop.DocFromString(string(docBytes))
		if err != nil {
			return
		}
		op, err := textop.Parse(raw)
		if err != nil {
			return
		}
		want, modelErr := referenceApply(string(docBytes), raw)
		got, applyErr := textop.Apply(d, op)
		validErr := textop.ValidateAgainst(d, op)
		if modelErr != nil {
			if !errors.Is(applyErr, modelErr) || !errors.Is(validErr, modelErr) {
				t.Fatalf("model %v Apply %v Validate %v", modelErr, applyErr, validErr)
			}
			return
		}
		if len(want) > textop.DefaultLimits().MaxDocBytes {
			if !errors.Is(applyErr, textop.ErrLimit) {
				t.Fatal(applyErr)
			}
			return
		}
		if applyErr != nil || validErr != nil || got.String() != want || got.Len() != op.TargetLen() || !utf8.ValidString(got.String()) {
			t.Fatalf("Apply %v Validate %v model disagrees", applyErr, validErr)
		}
		if d.String() != string(docBytes) {
			t.Fatal("document mutated")
		}
	})
}
