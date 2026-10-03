package annot

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"unicode/utf8"
)

// OpKind distinguishes a complete replacement from a deletion.
type OpKind uint8

const (
	// Put replaces the entire value at a key.
	Put OpKind = iota + 1
	// Del deletes the value at a key.
	Del
)

// ElementOp changes one annotation. Del must not carry a Value.
type ElementOp struct {
	Op    OpKind
	Key   ElementKey
	Value json.RawMessage
}

// EntryOp changes one page entry. Entry deletions leave no tombstones.
type EntryOp struct {
	Op    OpKind
	Key   EntryKey
	Value json.RawMessage
}

// Ops is an ordered operation set. Replace is trusted server output only;
// ParseOps rejects it. Apply validates public fields and owns copies of bytes.
type Ops struct {
	Elements []ElementOp
	Entries  []EntryOp
	MetaPut  map[string]json.RawMessage
	MetaDel  []string
	Replace  bool
}

// ParseOps decodes a client edit and rejects unknown control fields, duplicate
// operation keys, noncanonical ordering and any replace field, even false.
func ParseOps(raw []byte, l Limits) (Ops, error) {
	l, err := l.normalized()
	if err != nil {
		return Ops{}, err
	}
	compact, err := strictJSON(raw, l, l.MaxOpBytes)
	if err != nil {
		return Ops{}, err
	}
	obj, err := object(compact)
	if err != nil {
		return Ops{}, ErrNonCanonical
	}
	if err := onlyFields(obj, "elements", "entries", "meta"); err != nil {
		return Ops{}, err
	}
	o := Ops{}
	for _, name := range []string{"elements", "entries"} {
		v, ok := obj[name]
		if !ok {
			continue
		}
		items, err := array(v)
		if err != nil {
			return Ops{}, ErrNonCanonical
		}
		for _, item := range items {
			fields, err := object(item)
			if err != nil {
				return Ops{}, ErrNonCanonical
			}
			if err := onlyFields(fields, "put", "del", "value"); err != nil {
				return Ops{}, err
			}
			key, isPut := fields["put"]
			del, isDel := fields["del"]
			if isPut == isDel {
				return Ops{}, ErrNonCanonical
			}
			op := Put
			if isDel {
				op = Del
				key = del
			}
			if isPut && fields["value"] == nil {
				return Ops{}, ErrNonCanonical
			}
			if isDel && fields["value"] != nil {
				return Ops{}, ErrNonCanonical
			}
			keyFields, err := object(key)
			if err != nil {
				return Ops{}, ErrNonCanonical
			}
			if name == "elements" {
				if err := onlyFields(keyFields, "kind", "id"); err != nil || len(keyFields) != 2 {
					return Ops{}, ErrNonCanonical
				}
				var k ElementKey
				if err := json.Unmarshal(key, &k); err != nil {
					return Ops{}, ErrNonCanonical
				}
				o.Elements = append(o.Elements, ElementOp{Op: op, Key: k, Value: fields["value"]})
			} else {
				if err := onlyFields(keyFields, "family", "page"); err != nil || len(keyFields) != 2 {
					return Ops{}, ErrNonCanonical
				}
				var k EntryKey
				if err := json.Unmarshal(key, &k); err != nil {
					return Ops{}, ErrNonCanonical
				}
				o.Entries = append(o.Entries, EntryOp{Op: op, Key: k, Value: fields["value"]})
			}
		}
	}
	if raw, ok := obj["meta"]; ok {
		meta, err := object(raw)
		if err != nil {
			return Ops{}, ErrNonCanonical
		}
		if err := onlyFields(meta, "put", "del"); err != nil {
			return Ops{}, err
		}
		if v, ok := meta["put"]; ok {
			o.MetaPut, err = object(v)
			if err != nil {
				return Ops{}, ErrNonCanonical
			}
		}
		if v, ok := meta["del"]; ok {
			if _, err := array(v); err != nil {
				return Ops{}, ErrNonCanonical
			}
			if err := json.Unmarshal(v, &o.MetaDel); err != nil {
				return Ops{}, ErrNonCanonical
			}
		}
	}
	return checkedOps(o, l, false)
}
func onlyFields(obj map[string]json.RawMessage, names ...string) error {
	for key := range obj {
		if !slices.Contains(names, key) {
			return ErrNonCanonical
		}
	}
	return nil
}
func validElementKey(k ElementKey, l Limits) error {
	if kindIndex(k.Kind) < 0 {
		return ErrNonCanonical
	}
	if k.ID == "" || !utf8.ValidString(k.ID) {
		return ErrMissingID
	}
	if len(k.ID) > l.MaxIDBytes {
		return ErrLimit
	}
	return nil
}
func validEntry(k EntryKey, v json.RawMessage, op OpKind) error {
	if k.Page <= 0 || int64(k.Page) > 2147483647 {
		return ErrNonCanonical
	}
	switch k.Family {
	case PDFPageState:
		if op == Put && !bytes.Equal(v, []byte(`"hidden"`)) && !bytes.Equal(v, []byte(`"permanent"`)) {
			return ErrNonCanonical
		}
	case PDFPageTemplates:
		if op == Put {
			obj, err := object(v)
			if err != nil {
				return ErrNonCanonical
			}
			page, err := pageNumber(obj["page"])
			if err != nil || page != k.Page {
				return ErrNonCanonical
			}
		}
	default:
		return ErrNonCanonical
	}
	return nil
}
func checkedOps(o Ops, l Limits, trusted bool) (Ops, error) {
	if o.Replace && !trusted {
		return Ops{}, ErrNonCanonical
	}
	count := len(o.Elements) + len(o.Entries) + len(o.MetaPut) + len(o.MetaDel)
	maxCount, maxBytes := l.MaxOps, l.MaxOpBytes
	if trusted {
		maxCount = l.MaxElements + l.MaxTombstones
		maxBytes = l.MaxStateBytes
	}
	if count > maxCount {
		return Ops{}, ErrLimit
	}
	// Count before copying or encoding potentially large RawMessages.
	budget := maxBytes
	consume := func(n int) bool {
		if n > budget {
			return false
		}
		budget -= n
		return true
	}
	out := Ops{Replace: o.Replace, MetaPut: map[string]json.RawMessage{}}
	seen := map[ElementKey]bool{}
	deleting := false
	var previous ElementKey
	for _, e := range o.Elements {
		if err := validElementKey(e.Key, l); err != nil {
			return Ops{}, err
		}
		if seen[e.Key] {
			return Ops{}, ErrNonCanonical
		}
		seen[e.Key] = true
		if !consume(len(e.Key.ID) + len(e.Value)) {
			return Ops{}, ErrLimit
		}
		switch e.Op {
		case Put:
			if deleting {
				return Ops{}, ErrNonCanonical
			}
			raw, err := strictJSON(e.Value, l, maxBytes)
			if err != nil {
				return Ops{}, err
			}
			id, err := getID(raw, l)
			if err != nil {
				return Ops{}, err
			}
			if id != e.Key.ID {
				return Ops{}, ErrMissingID
			}
			e.Value = raw
		case Del:
			if len(e.Value) != 0 || (deleting && compareElement(previous, e.Key) >= 0) {
				return Ops{}, ErrNonCanonical
			}
			deleting = true
			previous = e.Key
		default:
			return Ops{}, ErrNonCanonical
		}
		out.Elements = append(out.Elements, e)
	}
	for i, e := range o.Entries {
		if i > 0 && compareEntry(o.Entries[i-1].Key, e.Key) >= 0 {
			return Ops{}, ErrNonCanonical
		}
		if !consume(len(e.Value)) {
			return Ops{}, ErrLimit
		}
		switch e.Op {
		case Put:
			raw, err := strictJSON(e.Value, l, maxBytes)
			if err != nil {
				return Ops{}, err
			}
			e.Value = raw
		case Del:
			if len(e.Value) != 0 {
				return Ops{}, ErrNonCanonical
			}
		default:
			return Ops{}, ErrNonCanonical
		}
		if err := validEntry(e.Key, e.Value, e.Op); err != nil {
			return Ops{}, err
		}
		out.Entries = append(out.Entries, e)
	}
	for _, key := range slices.Sorted(maps.Keys(o.MetaPut)) {
		if !utf8.ValidString(key) || reserved(key) {
			return Ops{}, ErrNonCanonical
		}
		if structural(key) && !o.Replace {
			return Ops{}, ErrStructuralKey
		}
		v := o.MetaPut[key]
		if !consume(len(key) + len(v)) {
			return Ops{}, ErrLimit
		}
		raw, err := strictJSON(v, l, maxBytes)
		if err != nil {
			return Ops{}, err
		}
		if structural(key) {
			if _, err := array(raw); err != nil {
				return Ops{}, ErrNonCanonical
			}
		}
		out.MetaPut[key] = raw
	}
	for i, key := range o.MetaDel {
		if !utf8.ValidString(key) || reserved(key) || (i > 0 && o.MetaDel[i-1] >= key) {
			return Ops{}, ErrNonCanonical
		}
		if structural(key) && !o.Replace {
			return Ops{}, ErrStructuralKey
		}
		if _, ok := o.MetaPut[key]; ok {
			return Ops{}, ErrNonCanonical
		}
		if !consume(len(key)) {
			return Ops{}, ErrLimit
		}
		out.MetaDel = append(out.MetaDel, key)
	}
	if len(out.encode()) > maxBytes {
		return Ops{}, ErrLimit
	}
	return out, nil
}

// MarshalJSON encodes compact operations directly, retaining opaque bytes.
// This may encode trusted replacement results for the committed log; client
// ParseOps deliberately cannot read those. Use the method directly when storing
// bytes: an outer json.Marshal call may HTML-escape custom Marshaler output.
func (o Ops) MarshalJSON() ([]byte, error) {
	checked, err := checkedOps(o, DefaultLimits(), o.Replace)
	if err != nil {
		return nil, err
	}
	return checked.encode(), nil
}
func (o Ops) encode() []byte {
	out := []byte{'{'}
	var elements, entries []json.RawMessage
	for _, e := range o.Elements {
		key := []byte{'{'}
		key = appendMember(key, "kind", quoted(string(e.Key.Kind)))
		key = appendMember(key, "id", quoted(e.Key.ID))
		key = append(key, '}')
		item := []byte{'{'}
		if e.Op == Put {
			item = appendMember(item, "put", key)
			item = appendMember(item, "value", e.Value)
		} else {
			item = appendMember(item, "del", key)
		}
		elements = append(elements, append(item, '}'))
	}
	for _, e := range o.Entries {
		key := []byte{'{'}
		key = appendMember(key, "family", quoted(string(e.Key.Family)))
		key = appendMember(key, "page", []byte(strconv.Itoa(e.Key.Page)))
		key = append(key, '}')
		item := []byte{'{'}
		if e.Op == Put {
			item = appendMember(item, "put", key)
			item = appendMember(item, "value", e.Value)
		} else {
			item = appendMember(item, "del", key)
		}
		entries = append(entries, append(item, '}'))
	}
	out = appendMember(out, "elements", rawArray(elements))
	out = appendMember(out, "entries", rawArray(entries))
	meta := []byte{'{'}
	puts := []byte{'{'}
	for _, key := range slices.Sorted(maps.Keys(o.MetaPut)) {
		puts = appendMember(puts, key, o.MetaPut[key])
	}
	meta = appendMember(meta, "put", append(puts, '}'))
	var dels []json.RawMessage
	for _, key := range o.MetaDel {
		dels = append(dels, quoted(key))
	}
	meta = appendMember(meta, "del", rawArray(dels))
	out = appendMember(out, "meta", append(meta, '}'))
	if o.Replace {
		out = appendMember(out, "replace", []byte("true"))
	}
	return append(out, '}')
}
