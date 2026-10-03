package annot

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Parse reads the pinned FreeDraw file form. It preserves compact JSON values,
// original array order, and opaque structural metadata; it cannot restore
// tombstones or creation history. Use DecodeState for server snapshots.
func Parse(b []byte, l Limits) (Sidecar, error) {
	l, err := l.normalized()
	if err != nil {
		return Sidecar{}, err
	}
	raw, err := strictJSON(b, l, l.MaxBytes)
	if err != nil {
		return Sidecar{}, err
	}
	root, err := object(raw)
	if err != nil {
		return Sidecar{}, err
	}
	if _, ok := root["strokes"]; !ok {
		return Sidecar{}, ErrNotSidecar
	}
	sc := newSidecar(l)
	ordinal := 0
	for _, kind := range kinds() {
		v, exists := root[string(kind)]
		if !exists {
			continue
		}
		values, err := array(v)
		if err != nil {
			return Sidecar{}, err
		}
		for _, value := range values {
			id, err := getID(value, l)
			if err != nil {
				return Sidecar{}, err
			}
			key := ElementKey{Kind: kind, ID: id}
			if _, exists := sc.elements[key]; exists {
				return Sidecar{}, ErrDuplicateID
			}
			if len(sc.elements) >= l.MaxElements {
				return Sidecar{}, ErrLimit
			}
			sc.elements[key] = element{value: value, order: orderKey{Ordinal: ordinal}}
			ordinal++
		}
		delete(root, string(kind))
	}
	if err := parseEntries(root, &sc); err != nil {
		return Sidecar{}, err
	}
	for _, key := range []string{"appendedPages", "removedPages"} {
		if v, ok := root[key]; ok {
			if _, err := array(v); err != nil {
				return Sidecar{}, err
			}
		}
	}
	sc.meta = root
	if _, err := Marshal(sc); err != nil {
		return Sidecar{}, err
	}
	return sc, nil
}
func getID(raw []byte, l Limits) (string, error) {
	obj, err := object(raw)
	if err != nil {
		return "", ErrMissingID
	}
	var id string
	if err := json.Unmarshal(obj["id"], &id); err != nil || id == "" || !utf8.ValidString(id) {
		return "", ErrMissingID
	}
	if len(id) > l.MaxIDBytes {
		return "", ErrLimit
	}
	return id, nil
}
func pageNumber(raw []byte) (int, error) {
	// Page indexes must be portable between Go and JavaScript.
	n, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil || n <= 0 {
		return 0, ErrNotSidecar
	}
	return int(n), nil
}
func parseEntries(root map[string]json.RawMessage, sc *Sidecar) error {
	for _, name := range []string{"deletedPdfPages", "permanentlyDeletedPdfPages", "pdfPageTemplates"} {
		raw, ok := root[name]
		if !ok {
			continue
		}
		values, err := array(raw)
		if err != nil {
			return err
		}
		seen := map[int]bool{}
		for _, value := range values {
			pageRaw := value
			family := PDFPageState
			entry := json.RawMessage(`"hidden"`)
			if name == "permanentlyDeletedPdfPages" {
				entry = json.RawMessage(`"permanent"`)
			}
			if name == "pdfPageTemplates" {
				obj, err := object(value)
				if err != nil {
					return err
				}
				pageRaw = obj["page"]
				family = PDFPageTemplates
				entry = value
			}
			page, err := pageNumber(pageRaw)
			if err != nil {
				return err
			}
			if seen[page] {
				return ErrDuplicateKey
			}
			seen[page] = true
			key := EntryKey{Family: family, Page: page}
			// FreeDraw treats permanent as dominant even if an old file lists both.
			sc.entries[key] = entry
			if len(sc.entries)+len(sc.elements) > sc.limits.MaxElements {
				return ErrLimit
			}
		}
		delete(root, name)
	}
	return nil
}

// Marshal writes stable known keys, sorted unknown keys and creation-ordered
// annotations, indented with two spaces like FreeDraw. Opaque values are embedded
// directly; numbers, field order and string escape spellings survive.
func Marshal(sc Sidecar) ([]byte, error) {
	sc = sc.configured()
	root := maps.Clone(sc.meta)
	values := map[Kind][]json.RawMessage{}
	for _, key := range orderedElements(sc) {
		values[key.Kind] = append(values[key.Kind], sc.elements[key].value)
	}
	for _, kind := range kinds() {
		root[string(kind)] = rawArray(values[kind])
	}
	var hidden, permanent, templates []json.RawMessage
	for _, key := range sortedEntries(sc.entries) {
		v := sc.entries[key]
		switch key.Family {
		case PDFPageState:
			if bytes.Equal(v, []byte(`"hidden"`)) {
				hidden = append(hidden, []byte(strconv.Itoa(key.Page)))
			} else {
				permanent = append(permanent, []byte(strconv.Itoa(key.Page)))
			}
		case PDFPageTemplates:
			templates = append(templates, v)
		}
	}
	root["deletedPdfPages"] = rawArray(hidden)
	root["permanentlyDeletedPdfPages"] = rawArray(permanent)
	root["pdfPageTemplates"] = rawArray(templates)
	out := []byte{'{'}
	for _, key := range topLevelKeys() {
		if v, ok := root[key]; ok {
			out = appendMember(out, key, v)
			delete(root, key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(root)) {
		out = appendMember(out, key, root[key])
	}
	out = append(out, '}')
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return nil, ErrInvalidState
	}
	if pretty.Len() > sc.limits.MaxBytes {
		return nil, ErrLimit
	}
	return pretty.Bytes(), nil
}

// CanonicalHash is SHA-256 of Marshal(sc), the exact bytes used by the audit.
// Construction and Apply enforce all invariants, so Marshal cannot fail here.
func CanonicalHash(sc Sidecar) [32]byte { raw, _ := Marshal(sc); return sha256.Sum256(raw) }

func orderedElements(sc Sidecar) []ElementKey {
	keys := slices.Collect(maps.Keys(sc.elements))
	slices.SortFunc(keys, func(a, b ElementKey) int {
		if a.Kind != b.Kind {
			return kindIndex(a.Kind) - kindIndex(b.Kind)
		}
		ao, bo := sc.elements[a].order, sc.elements[b].order
		if ao.Version < bo.Version {
			return -1
		}
		if ao.Version > bo.Version {
			return 1
		}
		if ao.Ordinal != bo.Ordinal {
			return ao.Ordinal - bo.Ordinal
		}
		return compareElement(a, b)
	})
	return keys
}
func sortedEntries(m map[EntryKey]json.RawMessage) []EntryKey {
	keys := slices.Collect(maps.Keys(m))
	slices.SortFunc(keys, compareEntry)
	return keys
}
