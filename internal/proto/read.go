package proto

import (
	"bytes"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

type fault struct {
	code   Code
	detail Detail
}
type object struct {
	fields map[string]jsontext.Value
	strict bool
	prefix string
	err    *fault
}

func (r object) fail(field string, code Code, detail Detail) {
	if r.err.code == "" {
		detail.Field = r.prefix + field
		r.err.code = code
		r.err.detail = detail
	}
}
func newObject(raw []byte, strict bool, prefix string, f *fault) object {
	r := object{strict: strict, prefix: prefix, err: f}
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &r.fields) != nil {
		r.fail("", InvalidSubmission, Detail{})
		r.fields = map[string]jsontext.Value{}
	}
	return r
}
func (r object) only(names ...string) {
	if !r.strict {
		return
	}
	for _, k := range sortedKeys(r.fields) {
		if !slices.Contains(names, k) {
			r.fail(k, InvalidSubmission, Detail{})
		}
	}
}
func sortedKeys(m map[string]jsontext.Value) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
func (r object) has(k string) bool { _, ok := r.fields[k]; return ok }
func (r object) raw(k string) jsontext.Value {
	v, ok := r.fields[k]
	if !ok || bytes.Equal(v, []byte("null")) {
		r.fail(k, InvalidSubmission, Detail{})
	}
	return v
}
func (r object) str(k string) string {
	var s string
	v := r.raw(k)
	if len(v) == 0 || v[0] != '"' || json.Unmarshal(v, &s) != nil {
		r.fail(k, InvalidSubmission, Detail{})
	}
	return s
}
func (r object) integer(k string, min, max int64) int64 {
	v := r.raw(k)
	n, e := strconv.ParseInt(string(v), 10, 64)
	if e != nil || n < min || n > max {
		r.fail(k, InvalidSubmission, Detail{})
		return 0
	}
	return n
}
func (r object) boolean(k string) bool {
	var b bool
	v := r.raw(k)
	if !bytes.Equal(v, []byte("true")) && !bytes.Equal(v, []byte("false")) {
		r.fail(k, InvalidSubmission, Detail{})
	} else {
		b = bytes.Equal(v, []byte("true"))
	}
	return b
}
func (r object) child(k string) object { return newObject(r.raw(k), r.strict, r.prefix+k+".", r.err) }
func (r object) file(k string) ids.FileID {
	v, e := ids.ParseFileID(r.str(k))
	if e != nil {
		r.fail(k, InvalidSubmission, Detail{})
	}
	return v
}
func (r object) version() ids.Version { return ids.Version(r.integer("base_version", 1, ids.MaxSafe)) }
func (r object) path(k string) vpath.Path {
	v, e := vpath.Parse(r.str(k))
	if e != nil {
		var pe *vpath.PathError
		if errors.As(e, &pe) {
			n := pe.Component
			r.fail(k, InvalidPath, Detail{Reason: pe.Reason.String(), Component: &n})
		} else {
			r.fail(k, InvalidPath, Detail{})
		}
	}
	return v
}
func (r object) hash(k string) [32]byte {
	var h [32]byte
	s := r.str(k)
	if len(s) != 64 || strings.ToLower(s) != s {
		r.fail(k, InvalidSubmission, Detail{})
		return h
	}
	b, e := hex.DecodeString(s)
	if e != nil {
		r.fail(k, InvalidSubmission, Detail{})
		return h
	}
	copy(h[:], b)
	return h
}
func (r object) class(k string) classify.Class {
	s := r.str(k)
	if s != "A" && s != "B" && s != "C" {
		r.fail(k, InvalidSubmission, Detail{})
		return 0
	}
	return classify.Class(s[0])
}
func (r object) array(k string, max int) []jsontext.Value {
	v := r.raw(k)
	var out []jsontext.Value
	if len(v) == 0 || v[0] != '[' || json.Unmarshal(v, &out) != nil {
		r.fail(k, InvalidSubmission, Detail{})
		return nil
	}
	if len(out) > max {
		r.fail(k, InvalidSubmission, Detail{})
		return nil
	}
	return out
}
func (r object) blob() BlobRef {
	r.only("sha256", "size")
	return BlobRef{SHA256: r.hash("sha256"), Size: r.integer("size", 0, ids.MaxSafe)}
}
func fmtCode(c CloseCode) string { return strconv.FormatUint(uint64(c), 10) }

// Check depth without allocating a stack or interpreting strings. The strict
// JSON pass below then verifies every token, including ignored unknown fields.
func framing(frame []byte, max int) error {
	if len(frame) > max {
		return &CloseError{CloseTooLarge, "frame_too_large"}
	}
	depth := 0
	quoted := false
	escaped := false
	for _, b := range frame {
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '[', '{':
			depth++
			if depth > MaxDepth {
				return &CloseError{CloseProtocol, "protocol"}
			}
		case ']', '}':
			depth--
		}
	}
	raw := bytes.TrimSpace(frame)
	if len(raw) == 0 || raw[0] != '{' || !jsontext.Value(raw).IsValid() {
		return &CloseError{CloseProtocol, "protocol"}
	}
	return nil
}
