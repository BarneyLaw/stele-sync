package textop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Parse decodes  ot.js-style JSON, rejecting non-canonical components
func Parse(raw []byte) (Op, error) {
	return DefaultLimits().Parse(raw)
}

// Parse applies l's input, component and length limits before constructing an Op.
// Counts use integer JSON tokens; fractional and exponent spellings are rejected.
func (l Limits) Parse(raw []byte) (Op, error) {
	if err := l.check(); err != nil {
		return Op{}, err
	}
	if len(raw) > l.MaxJSONBytes {
		return Op{}, ErrLimit
	}
	if !utf8.Valid(raw) {
		return Op{}, ErrInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('[') {
		return Op{}, ErrInvalidJSON
	}
	b, err := NewBuilder(l)
	if err != nil {
		return Op{}, err
	}
	var prev kind
	count := 0
	for dec.More() {
		if count >= l.MaxComponents {
			return Op{}, ErrLimit
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return Op{}, ErrInvalidJSON
		}
		value = bytes.TrimSpace(value)
		var k kind
		if len(value) > 0 && value[0] == '"' {
			k = insert
			if err := validEscapes(value); err != nil {
				return Op{}, err
			}
			var s string
			if err := json.Unmarshal(value, &s); err != nil {
				return Op{}, ErrInvalidJSON
			}
			if s == "" {
				return Op{}, ErrNonCanonical
			}
			b.Insert(s)
		} else {
			// strconv.ParseInt deliberately rejects decimals, exponents,
			// null, booleans, objects, and nested arrays.
			n, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				return Op{}, ErrInvalidCount
			}
			if n == 0 {
				return Op{}, ErrNonCanonical
			}
			// Compare before negating: MinInt64 must never overflow.
			if n > int64(l.MaxUnits) || n < -int64(l.MaxUnits) {
				return Op{}, ErrLimit
			}
			if n > 0 {
				k = retain
				b.Retain(int(n))
			} else {
				k = deleteText
				b.Delete(int(-n))
			}
		}
		if count > 0 && (prev == k || (prev == deleteText && k == insert)) {
			return Op{}, ErrNonCanonical
		}
		if b.err != nil {
			return Op{}, b.err
		}
		prev = k
		count++
	}
	tok, err = dec.Token()
	if err != nil || tok != json.Delim(']') {
		return Op{}, ErrInvalidJSON
	}
	if _, err := dec.Token(); err != io.EOF {
		return Op{}, ErrInvalidJSON
	}
	return b.Op()
}

// validEscapes runs on a syntactically valid raw JSON string. The JSON decoder
// validates syntax, but this scan additionally rejects unpaired surrogate escapes.
func validEscapes(raw []byte) error {
	end := len(raw) - 1
	for i := 1; i < end; {
		if raw[i] != '\\' {
			i++
			continue
		}
		if i+1 >= end {
			return ErrInvalidJSON
		}
		if raw[i+1] != 'u' {
			i += 2
			continue
		}
		if i+6 > end {
			return ErrInvalidJSON
		}
		u, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return ErrInvalidJSON
		}
		i += 6
		if u >= 0xdc00 && u <= 0xdfff {
			return ErrInvalidUnicode
		}
		if u < 0xd800 || u > 0xdbff {
			continue
		}
		if i+6 > end || raw[i] != '\\' || raw[i+1] != 'u' {
			return ErrInvalidUnicode
		}
		v, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil || v < 0xdc00 || v > 0xdfff {
			return ErrInvalidUnicode
		}
		i += 6
	}
	return nil
}

// MarshalJSON encodes canonical components deterministically. It does not impose
// a transport frame limit: a composed local buffer can exceed one network frame.
func (op Op) MarshalJSON() ([]byte, error) {
	out := []byte{'['}
	for i, c := range op.comps {
		if i > 0 {
			out = append(out, ',')
		}
		switch c.k {
		case retain:
			out = strconv.AppendInt(out, int64(c.n), 10)
		case deleteText:
			out = strconv.AppendInt(out, -int64(c.n), 10)
		case insert:
			encoded, err := json.Marshal(string(utf16.Decode(c.u)))
			if err != nil {
				return nil, fmt.Errorf("encode insertion: %w", err)
			}
			out = append(out, encoded...)
		}

	}
	return append(out, ']'), nil
}
