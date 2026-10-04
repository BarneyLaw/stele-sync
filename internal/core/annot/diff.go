package annot

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
)

// Diff computes a local edit against its on-disk baseline, not the newer shadow.
// Added pages or trash on either side require a full document submission.
func Diff(base, cur Sidecar) (Ops, error) {
	base, cur = base.configured(), cur.configured()
	if RequiresWholeDocument(base) || RequiresWholeDocument(cur) {
		return Ops{}, ErrWholeDocument
	}
	o := difference(base, cur, false)
	return checkedOps(o, base.limits, false)
}

// ReplaceOps constructs a trusted full replacement. The caller MUST first
// authorize base == head. ParseOps never accepts Replace from client bytes.
func ReplaceOps(head, doc Sidecar) Ops { return difference(head.configured(), doc.configured(), true) }

func difference(base, cur Sidecar, replace bool) Ops {
	o := Ops{Replace: replace, MetaPut: map[string]json.RawMessage{}}
	for _, key := range orderedElements(cur) {
		e := cur.elements[key]
		old, ok := base.elements[key]
		if replace || !ok || !bytes.Equal(old.value, e.value) {
			o.Elements = append(o.Elements, ElementOp{Op: Put, Key: key, Value: bytes.Clone(e.value)})
		}
	}
	dels := []ElementKey{}
	for key := range base.elements {
		if _, ok := cur.elements[key]; !ok {
			dels = append(dels, key)
		}
	}
	slices.SortFunc(dels, compareElement)
	for _, key := range dels {
		o.Elements = append(o.Elements, ElementOp{Op: Del, Key: key})
	}
	keys := maps.Clone(base.entries)
	for key, value := range cur.entries {
		keys[key] = value
	}
	for _, key := range sortedEntries(keys) {
		value, ok := cur.entries[key]
		old, had := base.entries[key]
		if !ok {
			o.Entries = append(o.Entries, EntryOp{Op: Del, Key: key})
		} else if replace || !had || !bytes.Equal(old, value) {
			o.Entries = append(o.Entries, EntryOp{Op: Put, Key: key, Value: bytes.Clone(value)})
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cur.meta)) {
		value := cur.meta[key]
		old, ok := base.meta[key]
		if !replace && (key == "updatedAt" || structural(key)) {
			continue
		}
		if replace || !ok || !bytes.Equal(maskedMeta(key, old), maskedMeta(key, value)) {
			o.MetaPut[key] = bytes.Clone(value)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(base.meta)) {
		if !replace && (key == "updatedAt" || structural(key)) {
			continue
		}
		if _, ok := cur.meta[key]; !ok {
			o.MetaDel = append(o.MetaDel, key)
		}
	}
	return o
}
func maskedMeta(key string, raw json.RawMessage) []byte {
	if key != "sourcePdf" {
		return raw
	}
	obj, err := object(raw)
	if err != nil {
		return raw
	}
	delete(obj, "ctime")
	delete(obj, "mtime")
	out := []byte{'{'}
	for _, name := range slices.Sorted(maps.Keys(obj)) {
		out = appendMember(out, name, obj[name])
	}
	return append(out, '}')
}
