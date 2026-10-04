package annot

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
)

// Result contains the new immutable state, the operations to persist/broadcast,
// and tombstoned puts suppressed by ordinary delete-wins semantics.
type Result struct {
	Sidecar   Sidecar
	Effective Ops
	Dropped   []ElementKey
}

// Apply atomically applies an ordered server submission. at must exceed the
// state's previous version. The shell owns idempotency and base/retention checks.
// Invalid operations reject the whole batch, including otherwise dropped puts.
func Apply(head Sidecar, ops Ops, at int64) (Result, error) {
	head = head.configured()
	if at <= head.version {
		return Result{}, ErrInvalidState
	}
	if !ops.Replace && RequiresWholeDocument(head) {
		return Result{}, ErrWholeDocument
	}
	checked, err := checkedOps(ops, head.limits, ops.Replace)
	if err != nil {
		return Result{}, err
	}
	next := head
	next.elements = maps.Clone(head.elements)
	next.entries = maps.Clone(head.entries)
	next.meta = maps.Clone(head.meta)
	next.tombstones = maps.Clone(head.tombstones)
	next.version = at
	if checked.Replace {
		// Full replacement is authorized at head. Reset ordering to submitted order
		// and retain evidence of every removed key, including unknown deletes below.
		for _, key := range orderedElements(head) {
			next.tombstones[key] = at
		}
		next.elements = map[ElementKey]element{}
		next.entries = map[EntryKey]json.RawMessage{}
		next.meta = map[string]json.RawMessage{}
	}
	effective := checked
	effective.Elements = nil
	var dropped []ElementKey
	for _, e := range checked.Elements {
		switch e.Op {
		case Put:
			if _, dead := next.tombstones[e.Key]; dead && !checked.Replace {
				dropped = append(dropped, e.Key)
				continue
			}
			order := orderKey{Version: at, Ordinal: len(effective.Elements)}
			if old, ok := next.elements[e.Key]; ok {
				order = old.order
			}
			next.elements[e.Key] = element{value: e.Value, order: order}
			delete(next.tombstones, e.Key)
		case Del:
			delete(next.elements, e.Key)
			next.tombstones[e.Key] = at
		}
		effective.Elements = append(effective.Elements, e)
	}
	for _, e := range checked.Entries {
		switch e.Op {
		case Put:
			next.entries[e.Key] = e.Value
		case Del:
			delete(next.entries, e.Key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(checked.MetaPut)) {
		next.meta[key] = checked.MetaPut[key]
	}
	for _, key := range checked.MetaDel {
		delete(next.meta, key)
	}
	if len(next.elements)+len(next.entries) > next.limits.MaxElements || len(next.tombstones) > next.limits.MaxTombstones {
		return Result{}, ErrLimit
	}
	raw, err := Marshal(next)
	if err != nil {
		return Result{}, err
	}
	// Per-value checks cannot see the enclosing document's depth or node count.
	if _, err := strictJSON(raw, next.limits, next.limits.MaxBytes); err != nil {
		return Result{}, err
	}
	if _, err := EncodeState(next); err != nil {
		return Result{}, err
	}
	return Result{Sidecar: next, Effective: cloneOps(effective), Dropped: dropped}, nil
}
func cloneOps(o Ops) Ops {
	out := o
	out.Elements = slices.Clone(o.Elements)
	out.Entries = slices.Clone(o.Entries)
	for i := range out.Elements {
		out.Elements[i].Value = bytes.Clone(out.Elements[i].Value)
	}
	for i := range out.Entries {
		out.Entries[i].Value = bytes.Clone(out.Entries[i].Value)
	}
	out.MetaPut = make(map[string]json.RawMessage, len(o.MetaPut))
	for key, value := range o.MetaPut {
		out.MetaPut[key] = bytes.Clone(value)
	}
	out.MetaDel = slices.Clone(o.MetaDel)
	return out
}
