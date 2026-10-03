package annot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

type stateOrder struct {
	Key     ElementKey `json:"key"`
	Version int64      `json:"version"`
	Ordinal int        `json:"ordinal"`
}
type stateTombstone struct {
	Key     ElementKey `json:"key"`
	Version int64      `json:"version"`
}

// EncodeState writes a versioned server snapshot. This is the durable format
// for Postgres heads, snapshots and client shadows, never the PDF sidecar.
func EncodeState(sc Sidecar) ([]byte, error) {
	sc = sc.configured()
	content, err := Marshal(sc)
	if err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, content); err != nil {
		return nil, ErrInvalidState
	}
	out := []byte{'{'}
	out = appendMember(out, "v", []byte("1"))
	out = appendMember(out, "version", []byte(strconv.FormatInt(sc.version, 10)))
	out = appendMember(out, "content", compact.Bytes())
	var orders, tombs []json.RawMessage
	keys := slices.Collect(maps.Keys(sc.elements))
	slices.SortFunc(keys, compareElement)
	for _, key := range keys {
		e := sc.elements[key]
		item := []byte{'{'}
		item = appendMember(item, "key", encodeElementKey(key))
		item = appendMember(item, "version", []byte(strconv.FormatInt(e.order.Version, 10)))
		item = appendMember(item, "ordinal", []byte(strconv.Itoa(e.order.Ordinal)))
		orders = append(orders, append(item, '}'))
	}
	keys = slices.Collect(maps.Keys(sc.tombstones))
	slices.SortFunc(keys, compareElement)
	for _, key := range keys {
		item := []byte{'{'}
		item = appendMember(item, "key", encodeElementKey(key))
		item = appendMember(item, "version", []byte(strconv.FormatInt(sc.tombstones[key], 10)))
		tombs = append(tombs, append(item, '}'))
	}
	out = appendMember(out, "order", rawArray(orders))
	out = appendMember(out, "tombstones", rawArray(tombs))
	out = append(out, '}')
	if len(out) > sc.limits.MaxStateBytes {
		return nil, ErrLimit
	}
	return out, nil
}
func encodeElementKey(key ElementKey) []byte {
	out := []byte{'{'}
	out = appendMember(out, "kind", quoted(string(key.Kind)))
	out = appendMember(out, "id", quoted(key.ID))
	return append(out, '}')
}

// DecodeState validates the envelope, document, every order key and tombstone.
// A malformed state never degrades to an empty document or a tombstone-free one.
func DecodeState(b []byte, l Limits) (Sidecar, error) {
	sc, err := decodeState(b, l)
	if err != nil {
		return Sidecar{}, fmt.Errorf("%w: %w", ErrInvalidState, err)
	}
	return sc, nil
}
func decodeState(b []byte, l Limits) (Sidecar, error) {
	l, err := l.normalized()
	if err != nil {
		return Sidecar{}, err
	}
	envelopeLimits := l
	// One envelope level surrounds the document. Bookkeeping values also consume
	// the state byte budget; its token budget must allow that deterministic data.
	envelopeLimits.MaxDepth += 2
	envelopeLimits.MaxNodes = l.MaxStateBytes
	compact, err := strictJSON(b, envelopeLimits, l.MaxStateBytes)
	if err != nil {
		return Sidecar{}, err
	}
	root, err := object(compact)
	if err != nil {
		return Sidecar{}, err
	}
	if len(root) != 5 {
		return Sidecar{}, ErrInvalidState
	}
	if err := onlyFields(root, "v", "version", "content", "order", "tombstones"); err != nil {
		return Sidecar{}, err
	}
	if string(root["v"]) != "1" {
		return Sidecar{}, ErrInvalidState
	}
	version, err := strconv.ParseInt(string(root["version"]), 10, 64)
	if err != nil || version < 0 {
		return Sidecar{}, ErrInvalidState
	}
	sc, err := Parse(root["content"], l)
	if err != nil {
		return Sidecar{}, err
	}
	sc.version = version
	orders, err := array(root["order"])
	if err != nil || len(orders) != len(sc.elements) {
		return Sidecar{}, ErrInvalidState
	}
	seenOrder := map[orderKey]bool{}
	var previous ElementKey
	for i, raw := range orders {
		fields, err := object(raw)
		if err != nil || len(fields) != 3 {
			return Sidecar{}, ErrInvalidState
		}
		if err := onlyFields(fields, "key", "version", "ordinal"); err != nil {
			return Sidecar{}, err
		}
		if err := checkStateKey(fields["key"], l); err != nil {
			return Sidecar{}, err
		}
		var o stateOrder
		if err := json.Unmarshal(raw, &o); err != nil {
			return Sidecar{}, ErrInvalidState
		}
		if i > 0 && compareElement(previous, o.Key) >= 0 {
			return Sidecar{}, ErrInvalidState
		}
		previous = o.Key
		e, ok := sc.elements[o.Key]
		if !ok || o.Version < 0 || o.Version > version || o.Ordinal < 0 || o.Ordinal >= max(l.MaxOps, l.MaxElements) {
			return Sidecar{}, ErrInvalidState
		}
		order := orderKey{Version: o.Version, Ordinal: o.Ordinal}
		if seenOrder[order] {
			return Sidecar{}, ErrInvalidState
		}
		seenOrder[order] = true
		e.order = order
		sc.elements[o.Key] = e
	}
	tombs, err := array(root["tombstones"])
	if err != nil {
		return Sidecar{}, err
	}
	if len(tombs) > l.MaxTombstones {
		return Sidecar{}, ErrLimit
	}
	for i, raw := range tombs {
		fields, err := object(raw)
		if err != nil || len(fields) != 2 {
			return Sidecar{}, ErrInvalidState
		}
		if err := onlyFields(fields, "key", "version"); err != nil {
			return Sidecar{}, err
		}
		if err := checkStateKey(fields["key"], l); err != nil {
			return Sidecar{}, err
		}
		var tomb stateTombstone
		if err := json.Unmarshal(raw, &tomb); err != nil {
			return Sidecar{}, ErrInvalidState
		}
		if i > 0 && compareElement(previous, tomb.Key) >= 0 {
			return Sidecar{}, ErrInvalidState
		}
		previous = tomb.Key
		if tomb.Version <= 0 || tomb.Version > version {
			return Sidecar{}, ErrInvalidState
		}
		if _, live := sc.elements[tomb.Key]; live {
			return Sidecar{}, ErrInvalidState
		}
		sc.tombstones[tomb.Key] = tomb.Version
	}
	// The order table must describe the file it accompanies, not silently change
	// its render order. This also detects swapped or forged order assignments.
	original, err := Parse(root["content"], l)
	if err != nil {
		return Sidecar{}, err
	}
	if CanonicalHash(original) != CanonicalHash(sc) {
		return Sidecar{}, ErrInvalidState
	}
	if _, err := EncodeState(sc); err != nil {
		return Sidecar{}, err
	}
	return sc, nil
}
func checkStateKey(raw []byte, l Limits) error {
	fields, err := object(raw)
	if err != nil || len(fields) != 2 {
		return ErrInvalidState
	}
	if err := onlyFields(fields, "kind", "id"); err != nil {
		return err
	}
	var key ElementKey
	if err := json.Unmarshal(raw, &key); err != nil {
		return ErrInvalidState
	}
	return validElementKey(key, l)
}

// PruneTombstones removes tombstones strictly older than before, without changing
// content. The caller must first fence submissions older than the retained op
// window and persist a covering state snapshot. This function has no clock.
func PruneTombstones(sc Sidecar, before int64) Sidecar {
	sc = sc.configured()
	out := sc
	out.tombstones = maps.Clone(sc.tombstones)
	for key, version := range sc.tombstones {
		if version < before {
			delete(out.tombstones, key)
		}
	}
	return out
}
