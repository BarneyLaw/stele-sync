package textop_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

const maxRecordBytes = 64 << 20

type oracleHeader struct {
	SchemaVersion  int    `json:"schema_version"`
	GeneratorHash  string `json:"generator_hash"`
	OTVersion      string `json:"ot_version"`
	TieBreak       string `json:"tie_break"`
	Kind           string `json:"kind"`
	Profile        string `json:"profile"`
	Seed           uint64 `json:"seed"`
	Count          int    `json:"count"`
	LargePerKind   int    `json:"large_per_kind"`
	ComplexPerKind int    `json:"complex_per_kind"`
}

type oracleCase struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Index    int             `json:"i"`
	Category string          `json:"category"`
	Doc      string          `json:"doc"`
	Op       json.RawMessage `json:"op,omitempty"`
	A        json.RawMessage `json:"a,omitempty"`
	B        json.RawMessage `json:"b,omitempty"`
	Want     json.RawMessage `json:"want,omitempty"`
	WantAP   json.RawMessage `json:"want_ap,omitempty"`
	WantBP   json.RawMessage `json:"want_bp,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("missing source location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}

func generatorHash() (string, error) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "tools/oracle/generate.mjs"))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")))), nil
}

func exactFields(raw []byte, required, optional string) error {
	var obj map[string]json.RawMessage
	// This helper checks field presence only. Every caller separately applies
	// strictJSON to the typed value for duplicates, Unicode, nulls and unknowns.
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("expected object")
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields(required) {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("missing %s", key)
		}
		allowed[key] = true
	}
	for _, key := range strings.Fields(optional) {
		allowed[key] = true
	}
	for key := range obj {
		if !allowed[key] {
			return fmt.Errorf("unexpected %s", key)
		}
	}
	return nil
}

func readHeader(raw []byte, hash string) (oracleHeader, error) {
	var h oracleHeader
	if err := exactFields(raw, "schema_version generator_hash ot_version tie_break kind profile seed count large_per_kind complex_per_kind", ""); err != nil {
		return h, err
	}
	if err := strictJSON(raw, &h); err != nil {
		return h, err
	}
	if h.SchemaVersion != 1 || h.OTVersion != "0.0.15" || h.TieBreak != "a-first" || len(h.GeneratorHash) != 64 || h.GeneratorHash != hash {
		return h, fmt.Errorf("unsupported oracle provenance: %+v", h)
	}
	if _, err := hex.DecodeString(h.GeneratorHash); err != nil {
		return h, fmt.Errorf("invalid generator hash: %w", err)
	}
	if h.Kind != "all" && h.Kind != "apply" && h.Kind != "compose" && h.Kind != "transform" {
		return h, fmt.Errorf("invalid kind")
	}
	quota := 0
	minimum := 6
	switch h.Profile {
	case "scale":
		quota = 100
		minimum = 206
	case "fixtures":
	default:
		return h, fmt.Errorf("invalid profile")
	}
	if h.Kind == "all" {
		minimum *= 3
	}
	if h.Count < minimum || h.Count > 10000000 || h.Seed > 4294967295 || h.LargePerKind != quota || h.ComplexPerKind != quota {
		return h, fmt.Errorf("invalid count, seed or mandatory quotas")
	}
	return h, nil
}

func readCase(raw []byte) (oracleCase, error) {
	var c oracleCase
	if err := strictJSON(raw, &c); err != nil {
		return c, err
	}
	fields := "id kind i category doc "
	switch c.Kind {
	case "apply":
		fields += "op "
		if c.Error != "" {
			fields += "error"
		} else {
			fields += "want"
		}
	case "compose":
		fields += "a b want"
	case "transform":
		fields += "a b want_ap want_bp"
	default:
		return c, fmt.Errorf("unknown case kind %q", c.Kind)
	}
	if err := exactFields(raw, fields, ""); err != nil {
		return c, err
	}
	if c.ID == "" || c.Index < 0 {
		return c, fmt.Errorf("invalid case identity")
	}
	switch c.Category {
	case "identity", "unicode", "tie-cancel", "overlap", "delete", "replace", "large", "complex", "random":
	default:
		return c, fmt.Errorf("unknown category")
	}
	if c.Error != "" && c.Error != "length_mismatch" {
		return c, fmt.Errorf("unknown positive-fixture error")
	}
	return c, nil
}

// compareCase is shared by golden, streamed, and replay tests. It reports errors
// instead of calling testing.T so malformed fixtures and failure paths are testable.
func compareCase(c oracleCase) error {
	d, err := textop.DocFromString(c.Doc)
	if err != nil {
		return fmt.Errorf("document: %w", err)
	}
	parse := func(raw []byte) (textop.Op, error) {
		if _, _, _, err := components(raw); err != nil {
			return textop.Op{}, fmt.Errorf("invalid oracle input: %w", err)
		}
		l := textop.DefaultLimits()
		l.MaxJSONBytes = max(l.MaxJSONBytes, len(raw))
		return l.Parse(raw)
	}
	if c.Kind == "apply" {
		op, err := parse(c.Op)
		if err != nil {
			return err
		}
		if c.Error != "" {
			// Independently prove the premise, rather than trusting the label.
			if op.BaseLen() != ulen(c.Doc)+1 {
				return fmt.Errorf("negative case has no one-unit length mismatch")
			}
			if _, err := referenceApply(c.Doc+"x", c.Op); err != nil {
				return fmt.Errorf("negative case not otherwise valid: %w", err)
			}
			_, err := textop.Apply(d, op)
			if !errors.Is(err, textop.ErrLengthMismatch) {
				return fmt.Errorf("Apply error got %s want length_mismatch", fmt.Sprint(err))
			}
			if err := textop.ValidateAgainst(d, op); !errors.Is(err, textop.ErrLengthMismatch) {
				return fmt.Errorf("Validate error got %s want length_mismatch", fmt.Sprint(err))
			}
			return nil
		}
		var want string
		if err := strictJSON(c.Want, &want); err != nil {
			return fmt.Errorf("apply want: %w", err)
		}
		got, err := applyChecked(d, op)
		if err != nil {
			return err
		}
		if got.String() != want {
			return fmt.Errorf("apply document differs: got %q want %q", got.String(), want)
		}
		return nil
	}
	a, err := parse(c.A)
	if err != nil {
		return err
	}
	b, err := parse(c.B)
	if err != nil {
		return err
	}
	as, bs := encoded(a), encoded(b)
	da, err := applyChecked(d, a)
	if err != nil {
		return fmt.Errorf("a application: %w", err)
	}
	if c.Kind == "compose" {
		combined, err := textop.Compose(a, b)
		if err != nil {
			return err
		}
		if err := equalOp(combined, c.Want); err != nil {
			return fmt.Errorf("compose: %w", err)
		}
		got, err := applyChecked(d, combined)
		if err != nil {
			return err
		}
		sequential, err := applyChecked(da, b)
		if err != nil {
			return err
		}
		if got.String() != sequential.String() {
			return fmt.Errorf("compose application paths differ")
		}
		if combined.BaseLen() != a.BaseLen() || combined.TargetLen() != b.TargetLen() {
			return fmt.Errorf("compose lengths")
		}
	} else {
		ap, bp, err := textop.Transform(a, b)
		if err != nil {
			return err
		}
		if err := equalOp(ap, c.WantAP); err != nil {
			return fmt.Errorf("transform ap: %w", err)
		}
		if err := equalOp(bp, c.WantBP); err != nil {
			return fmt.Errorf("transform bp: %w", err)
		}
		db, err := applyChecked(d, b)
		if err != nil {
			return err
		}
		left, err := applyChecked(da, bp)
		if err != nil {
			return err
		}
		right, err := applyChecked(db, ap)
		if err != nil {
			return err
		}
		if left.String() != right.String() {
			return fmt.Errorf("transform application paths differ")
		}
		if ap.BaseLen() != b.TargetLen() || bp.BaseLen() != a.TargetLen() || ap.TargetLen() != bp.TargetLen() {
			return fmt.Errorf("transform lengths")
		}
	}
	if !bytes.Equal(as, encoded(a)) || !bytes.Equal(bs, encoded(b)) {
		return fmt.Errorf("algebra mutated operand")
	}
	return nil
}

func validateFixture(raw []byte, kind, hash string, count int) error {
	var f struct {
		Header json.RawMessage   `json:"header"`
		Cases  []json.RawMessage `json:"cases"`
	}
	if err := exactFields(raw, "header cases", ""); err != nil {
		return err
	}
	if err := strictJSON(raw, &f); err != nil {
		return err
	}
	h, err := readHeader(f.Header, hash)
	if err != nil {
		return err
	}
	if h.Kind != kind || h.Profile != "fixtures" || h.Count != count || len(f.Cases) != count {
		return fmt.Errorf("fixture count/kind mismatch")
	}
	categories := map[string]int{}
	for i, raw := range f.Cases {
		c, err := readCase(raw)
		if err != nil {
			return fmt.Errorf("case %d: %w", i, err)
		}
		if c.Kind != kind || c.Index != i || c.ID != fmt.Sprintf("%s-%d", kind, i) {
			return fmt.Errorf("case %d identity mismatch", i)
		}
		categories[c.Category]++
		if err := compareCase(c); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
	}
	for _, category := range []string{"identity", "unicode", "tie-cancel", "overlap", "delete", "replace"} {
		if categories[category] < 1 {
			return fmt.Errorf("missing category %s", category)
		}
	}
	return nil
}

func golden(t *testing.T, kind string, count int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "schema/textop", kind+".json")) // #nosec G304 -- kind is one of three test literals.
	if err != nil {
		t.Fatal(err)
	}
	hash, err := generatorHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateFixture(raw, kind, hash, count); err != nil {
		t.Fatal(err)
	}
}
func TestOracleApply(t *testing.T)     { golden(t, "apply", 2000) }
func TestOracleCompose(t *testing.T)   { golden(t, "compose", 2000) }
func TestOracleTransform(t *testing.T) { golden(t, "transform", 3000) }

var errorCodes = map[string]error{
	"invalid_utf8": textop.ErrInvalidUTF8, "invalid_json": textop.ErrInvalidJSON, "invalid_unicode": textop.ErrInvalidUnicode,
	"invalid_count": textop.ErrInvalidCount, "noncanonical": textop.ErrNonCanonical, "length_mismatch": textop.ErrLengthMismatch,
	"splits_surrogate": textop.ErrSplitsSurrogate, "limit": textop.ErrLimit, "invalid_limits": textop.ErrInvalidLimits,
}

func invalidCase(raw []byte) error {
	var c struct {
		ID     string          `json:"id"`
		Stage  string          `json:"stage"`
		Raw    *string         `json:"raw"`
		Base64 *string         `json:"raw_base64"`
		Doc    *string         `json:"doc"`
		B      json.RawMessage `json:"b"`
		Error  string          `json:"error"`
		Limits *textop.Limits  `json:"limits"`
	}
	if err := strictJSON(raw, &c); err != nil {
		return err
	}
	want, ok := errorCodes[c.Error]
	if !ok || c.ID == "" {
		return fmt.Errorf("invalid negative fixture identity/error")
	}
	fields := "id stage error "
	if (c.Raw == nil) == (c.Base64 == nil) {
		return fmt.Errorf("require exactly one raw representation")
	}
	var input []byte
	if c.Raw != nil {
		input = []byte(*c.Raw)
		fields += "raw "
	} else {
		var err error
		input, err = base64.StdEncoding.Strict().DecodeString(*c.Base64)
		if err != nil {
			return err
		}
		fields += "raw_base64 "
	}
	l := textop.DefaultLimits()
	if c.Limits != nil {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		if err := exactFields(fields["limits"], "MaxUnits MaxDocBytes MaxComponents MaxJSONBytes", ""); err != nil {
			return err
		}
		l = *c.Limits
	}
	var got error
	switch c.Stage {
	case "parse":
		_, got = l.Parse(input)
	case "doc":
		_, got = l.DocFromString(string(input))
	case "apply":
		fields += "doc "
		if c.Doc == nil {
			return fmt.Errorf("missing document")
		}
		d, err := textop.DocFromString(*c.Doc)
		if err != nil {
			return err
		}
		op, err := textop.Parse(input)
		if err != nil {
			return fmt.Errorf("apply fixture failed during parse: %w", err)
		}
		_, got = l.Apply(d, op)
		if e := l.ValidateAgainst(d, op); !errors.Is(e, want) {
			return fmt.Errorf("Validate: %s want %s", fmt.Sprint(e), want.Error())
		}
	case "compose", "transform":
		fields += "b "
		a, err := textop.Parse(input)
		if err != nil {
			return err
		}
		b, err := textop.Parse(c.B)
		if err != nil {
			return err
		}
		if c.Stage == "compose" {
			_, got = l.Compose(a, b)
		} else {
			_, _, got = l.Transform(a, b)
		}
	default:
		return fmt.Errorf("unknown stage %q", c.Stage)
	}
	if err := exactFields(raw, fields, "limits"); err != nil {
		return err
	}
	if !errors.Is(got, want) {
		return fmt.Errorf("%s: got %s want %s", c.ID, fmt.Sprint(got), want.Error())
	}
	return nil
}

func TestInvalid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(), "schema/textop/invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Header struct {
			Version     int    `json:"schema_version"`
			Kind        string `json:"kind"`
			HandWritten bool   `json:"hand_written"`
		} `json:"header"`
		Cases []json.RawMessage `json:"cases"`
	}
	if err := exactFields(raw, "header cases", ""); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if err := exactFields(fields["header"], "schema_version kind hand_written", ""); err != nil {
		t.Fatal(err)
	}
	if err := strictJSON(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Header.Version != 1 || f.Header.Kind != "invalid" || !f.Header.HandWritten || len(f.Cases) == 0 {
		t.Fatal("invalid negative fixture header/count")
	}
	seen := map[string]bool{}
	for _, raw := range f.Cases {
		var id struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &id); err != nil {
			t.Fatal(err)
		}
		if seen[id.ID] {
			t.Fatal("duplicate id")
		}
		seen[id.ID] = true
		t.Run(id.ID, func(t *testing.T) {
			if err := invalidCase(raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOracleRejectsCorruption(t *testing.T) {
	valid := `{"id":"apply-0","kind":"apply","i":0,"category":"identity","doc":"","op":[],"want":""}`
	for _, bad := range []string{
		strings.Replace(valid, `"want":""`, `"want":"","error":"length_mismatch"`, 1),
		strings.Replace(valid, `"want":""`, `"want":null`, 1), strings.Replace(valid, `"want":""`, `"wat":""`, 1),
		strings.Replace(valid, `"doc":""`, `"doc":"","doc":"x"`, 1), strings.Replace(valid, `"doc":""`, `"doc":"\ud800"`, 1),
		valid + ` {}`, strings.Replace(valid, `"op":[]`, `"op":[0]`, 1), strings.Replace(valid, `"op":[]`, `"op":[1.0]`, 1),
		strings.Replace(valid, `"want":""`, `"want":"wrong"`, 1),
	} {
		c, err := readCase([]byte(bad))
		if err == nil {
			err = compareCase(c)
		}
		if err == nil {
			t.Fatalf("accepted corrupt case: %s", bad)
		}
	}
	for _, raw := range []string{`{"id":"x","stage":"parse","raw":"[]","error":"typo"}`, `{"id":"x","stage":"parse","raw_base64":"!!","error":"invalid_utf8"}`, `{"id":"x","stage":"parse","raw":"[]","raw_base64":"W10=","error":"invalid_json"}`} {
		if invalidCase([]byte(raw)) == nil {
			t.Fatal("accepted bad negative fixture")
		}
	}
	if invalidCase([]byte(`{"id":"x","stage":"parse","raw":"[]","error":"invalid_limits","limits":{"MaxUnits":0,"maxunits":0,"MaxDocBytes":1,"MaxComponents":1,"MaxJSONBytes":1}}`)) == nil {
		t.Fatal("accepted ambiguous limits fields")
	}
	// Exact components catch semantic aliases on repetitive text; final text alone
	// would miss a wrong retain/delete position.
	if equalOp(mustOp(t, `[1,-1]`), []byte(`[-1,1]`)) == nil {
		t.Fatal("comparison normalized distinct operations")
	}
	if equalOp(mustOp(t, `["<>&"]`), []byte(`["\u003c\u003e\u0026"]`)) != nil {
		t.Fatal("comparison confused spelling with text")
	}
}

func TestOracleFixtureEnvelope(t *testing.T) {
	h, stream := syntheticStream(t)
	lines := bytes.Split(bytes.TrimSpace(stream), []byte{'\n'})
	hraw, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(`{"header":`), hraw...)
	raw = append(raw, []byte(`,"cases":[`)...)
	raw = append(raw, bytes.Join(lines[1:], []byte{','})...)
	raw = append(raw, ']', '}')
	if err := validateFixture(raw, "apply", h.GeneratorHash, 6); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]byte{
		"empty-cases":       append(append([]byte(`{"header":`), hraw...), []byte(`,"cases":[]}`)...),
		"count-mismatch":    bytes.Replace(raw, []byte(`"count":6`), []byte(`"count":7`), 1),
		"version":           bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":2`), 1),
		"tie":               bytes.Replace(raw, []byte(`"a-first"`), []byte(`"b-first"`), 1),
		"ot-version":        bytes.Replace(raw, []byte(`"0.0.15"`), []byte(`"0.0.14"`), 1),
		"duplicate-id":      bytes.Replace(raw, []byte(`"apply-1"`), []byte(`"apply-0"`), 1),
		"missing-case-list": append(append([]byte(`{"header":`), hraw...), '}'),
		"trailing":          append(bytes.Clone(raw), []byte(` []`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if validateFixture(bad, "apply", h.GeneratorHash, 6) == nil {
				t.Fatal("accepted corrupt fixture envelope")
			}
		})
	}
	// A first mismatch must preserve the exact complete case for replay.
	badStream := bytes.Replace(stream, []byte(`"want":""`), []byte(`"want":"WRONG"`), 1)
	_, failure := consumeOracle(bytes.NewReader(badStream), h, maxRecordBytes)
	if failure == nil || !bytes.Equal(failure.Case, bytes.Split(badStream, []byte{'\n'})[1]) || len(failure.Header) == 0 {
		t.Fatal("lost replay evidence")
	}
}
