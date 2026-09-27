package textop_test

// This model deliberately has no OT cursor or builder. It edits Unicode scalar
// slices using independently counted UTF-16 offsets. Oracle expectations are
// decoded here without using textop.Parse, and compared before any normalization.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

type fataler interface{ Fatalf(string, ...any) }

func mustOp(t fataler, raw string) textop.Op {
	op, err := textop.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse(%s): %v", raw, err)
	}
	return op
}

func mustDoc(t fataler, s string) textop.Doc {
	d, err := textop.DocFromString(s)
	if err != nil {
		t.Fatalf("DocFromString: %v", err)
	}
	return d
}

func encoded(op textop.Op) []byte {
	raw, err := op.MarshalJSON()
	if err != nil {
		panic(err)
	} // no fallible data sources in immutable Op
	return raw
}

func ulen(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// strictJSON rejects duplicates, trailing values, nulls, invalid UTF-8 and lone
// surrogate escapes before encoding/json can silently repair any string.
func strictJSON(raw []byte, dst any) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid UTF-8 fixture")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		start := i
		for i++; i < len(raw); i++ {
			if raw[i] == '\\' {
				i++
				continue
			}
			if raw[i] == '"' {
				break
			}
		}
		if i == len(raw) {
			return fmt.Errorf("unterminated string")
		}
		if err := scalarString(raw[start : i+1]); err != nil {
			return err
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if tok == nil {
			return fmt.Errorf("null is not a fixture value")
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		seen := map[string]bool{}
		for dec.More() {
			if d == '{' {
				key, err := dec.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return fmt.Errorf("duplicate/invalid key %v", key)
				}
				seen[s] = true
			}
			if err := walk(); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON: %s", fmt.Sprint(err))
	}
	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

// A state machine over escaped UTF-16 values, separate from the production
// look-ahead validator. Ordinary bytes/escapes flush a pending high surrogate.
func scalarString(raw []byte) error {
	if !json.Valid(raw) {
		return fmt.Errorf("invalid JSON string")
	}
	pending := false
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] != '\\' || raw[i+1] != 'u' {
			if pending {
				return fmt.Errorf("unpaired high surrogate")
			}
			if raw[i] == '\\' {
				i++
			}
			continue
		}
		u, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return err
		}
		i += 5
		switch {
		case u >= 0xd800 && u <= 0xdbff:
			if pending {
				return fmt.Errorf("two high surrogates")
			}
			pending = true
		case u >= 0xdc00 && u <= 0xdfff:
			if !pending {
				return fmt.Errorf("unpaired low surrogate")
			}
			pending = false
		default:
			if pending {
				return fmt.Errorf("unpaired high surrogate")
			}
		}
	}
	if pending {
		return fmt.Errorf("unpaired high surrogate")
	}
	return nil
}

type part struct {
	kind byte
	n    int
	text string
}

func components(raw []byte) ([]part, int, int, error) {
	var items []json.RawMessage
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[' {
		return nil, 0, 0, fmt.Errorf("operation must be an array")
	}
	// An operation is an array of primitives, so object duplicate detection is
	// unnecessary here: every object/array/null component fails integer parsing.
	// Check raw UTF-8 and string surrogates before any decoder can repair them.
	if !utf8.Valid(raw) {
		return nil, 0, 0, fmt.Errorf("invalid UTF-8 operation")
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, 0, 0, err
	}
	out := make([]part, 0, len(items))
	base, target := 0, 0
	var prev byte
	for _, item := range items {
		item = bytes.TrimSpace(item)
		p := part{}
		if item[0] == '"' {
			if err := scalarString(item); err != nil {
				return nil, 0, 0, err
			}
			if err := json.Unmarshal(item, &p.text); err != nil {
				return nil, 0, 0, err
			}
			p.kind = 'i'
			p.n = ulen(p.text)
			target += p.n
		} else {
			n, err := strconv.ParseInt(string(item), 10, 32)
			if err != nil {
				return nil, 0, 0, err
			}
			p.kind = 'r'
			if n < 0 {
				p.kind = 'd'
				n = -n
			}
			p.n = int(n)
			base += p.n
			if p.kind == 'r' {
				target += p.n
			}
		}
		if p.n == 0 || prev == p.kind || prev == 'd' && p.kind == 'i' {
			return nil, 0, 0, fmt.Errorf("noncanonical components")
		}
		prev = p.kind
		out = append(out, p)
	}
	return out, base, target, nil
}

func checkOp(op textop.Op) error {
	raw := encoded(op)
	_, base, target, err := components(raw)
	if err != nil {
		return err
	}
	if base != op.BaseLen() || target != op.TargetLen() {
		return fmt.Errorf("stored lengths disagree with components")
	}
	// Algebraic codec budget is distinct from network ingress.
	l := textop.DefaultLimits()
	l.MaxJSONBytes = max(l.MaxJSONBytes, len(raw))
	round, err := l.Parse(raw)
	if err != nil {
		return err
	}
	if !bytes.Equal(encoded(round), raw) {
		return fmt.Errorf("round-trip changed operation")
	}
	return nil
}

func equalOp(got textop.Op, want []byte) error {
	w, wb, wt, err := components(want)
	if err != nil {
		return fmt.Errorf("invalid expectation: %w", err)
	}
	g, gb, gt, err := components(encoded(got))
	if err != nil {
		return fmt.Errorf("invalid actual operation: %w", err)
	}
	if !reflect.DeepEqual(g, w) || gb != wb || gt != wt || got.BaseLen() != wb || got.TargetLen() != wt {
		return fmt.Errorf("components differ: got %s want %s", encoded(got), want)
	}
	return nil
}

func referenceApply(doc string, raw []byte) (string, error) {
	ps, base, _, err := components(raw)
	if err != nil {
		return "", err
	}
	if base != ulen(doc) {
		return "", textop.ErrLengthMismatch
	}
	runes := []rune(doc)
	out := make([]rune, 0, len(runes))
	pos, scalarPos := 0, 0
	for _, p := range ps {
		if p.kind == 'i' {
			out = append(out, []rune(p.text)...)
			continue
		}
		start, end := scalarPos, pos+p.n
		for pos < end && scalarPos < len(runes) {
			pos += utf16.RuneLen(runes[scalarPos])
			scalarPos++
		}
		if pos != end {
			return "", textop.ErrSplitsSurrogate
		}
		if p.kind == 'r' {
			out = append(out, runes[start:scalarPos]...)
		}
	}
	return string(out), nil
}

func applyChecked(d textop.Doc, op textop.Op) (textop.Doc, error) {
	before := encoded(op)
	original := d.String()
	want, refErr := referenceApply(original, before)
	got, err := textop.Apply(d, op)
	if refErr != nil {
		return textop.Doc{}, fmt.Errorf("model rejected valid-case input: %w", refErr)
	}
	if err != nil {
		return got, err
	}
	if got.String() != want || !utf8.ValidString(got.String()) || got.Len() != op.TargetLen() || got.Len() != ulen(want) {
		return got, fmt.Errorf("apply differs from scalar model")
	}
	if !bytes.Equal(before, encoded(op)) || d.String() != original {
		return got, fmt.Errorf("Apply mutated input")
	}
	if err := textop.ValidateAgainst(d, op); err != nil {
		return got, err
	}
	return got, nil
}
