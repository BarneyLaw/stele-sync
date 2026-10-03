package annot

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

// strictJSON validates without rebuilding values: whitespace is the only
// discarded information. encoding/json alone repairs lone surrogates and
// silently collapses duplicate keys, so neither is entrusted to Unmarshal.
func strictJSON(raw []byte, l Limits, maxBytes int) (json.RawMessage, error) {
	if len(raw) > maxBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(raw) || !json.Valid(raw) || !validUnicodeEscapes(raw) {
		return nil, ErrInvalidJSON
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	nodes := 0
	if err := walkJSON(d, l, 1, &nodes); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalidJSON
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return nil, ErrInvalidJSON
	}
	return out.Bytes(), nil
}
func walkJSON(d *json.Decoder, l Limits, depth int, nodes *int) error {
	*nodes++
	if depth > l.MaxDepth || *nodes > l.MaxNodes {
		return ErrLimit
	}
	tok, err := d.Token()
	if err != nil {
		return ErrInvalidJSON
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return ErrInvalidJSON
			}
			name, ok := key.(string)
			if !ok {
				return ErrInvalidJSON
			}
			if seen[name] {
				return ErrDuplicateKey
			}
			seen[name] = true
			if err := walkJSON(d, l, depth+1, nodes); err != nil {
				return err
			}
		}
		if _, err := d.Token(); err != nil {
			return ErrInvalidJSON
		}
	case json.Delim('['):
		for d.More() {
			if err := walkJSON(d, l, depth+1, nodes); err != nil {
				return err
			}
		}
		if _, err := d.Token(); err != nil {
			return ErrInvalidJSON
		}
	}
	return nil
}
func validUnicodeEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		for i++; i < len(raw) && raw[i] != '"'; i++ {
			if raw[i] != '\\' {
				continue
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			u, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			i += 4
			if u >= 0xdc00 && u <= 0xdfff {
				return false
			}
			if u < 0xd800 || u > 0xdbff {
				continue
			}
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			v, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || v < 0xdc00 || v > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
func object(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, ErrNotSidecar
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ErrInvalidJSON
	}
	return out, nil
}
func array(raw []byte) ([]json.RawMessage, error) {
	if len(raw) == 0 || raw[0] != '[' {
		return nil, ErrNotSidecar
	}
	var out []json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ErrInvalidJSON
	}
	return out, nil
}
func quoted(s string) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s) // A Go string has no unsupported JSON value.
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

// appendMember embeds raw values directly, avoiding json.Marshal's HTML escape
// pass through RawMessage. Callers supply strictly validated compact values.
func appendMember(out []byte, name string, value []byte) []byte {
	if len(out) > 1 {
		out = append(out, ',')
	}
	out = append(out, quoted(name)...)
	out = append(out, ':')
	return append(out, value...)
}
func rawArray(values []json.RawMessage) []byte {
	out := []byte{'['}
	for i, v := range values {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, v...)
	}
	return append(out, ']')
}
