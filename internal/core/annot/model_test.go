package annot_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"pgregory.net/rapid"
)

type reference struct {
	live  map[annot.ElementKey]json.RawMessage
	dead  map[annot.ElementKey]bool
	order map[annot.ElementKey]int
	next  int
}

func newReference() *reference {
	return &reference{live: map[annot.ElementKey]json.RawMessage{}, dead: map[annot.ElementKey]bool{}, order: map[annot.ElementKey]int{}}
}
func (m *reference) apply(o annot.Ops) {
	for _, e := range o.Elements {
		if e.Op == annot.Del {
			delete(m.live, e.Key)
			delete(m.order, e.Key)
			m.dead[e.Key] = true
			continue
		}
		if m.dead[e.Key] {
			continue
		}
		if _, ok := m.live[e.Key]; !ok {
			m.order[e.Key] = m.next
			m.next++
		}
		m.live[e.Key] = bytes.Clone(e.Value)
	}
}
func (m *reference) file() []byte {
	root := map[string][]json.RawMessage{"strokes": {}, "textItems": {}}
	keys := make([]annot.ElementKey, 0, len(m.live))
	for key := range m.live {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return m.order[keys[i]] < m.order[keys[j]] })
	for _, key := range keys {
		root[string(key.Kind)] = append(root[string(key.Kind)], m.live[key])
	}
	raw, err := json.Marshal(root)
	if err != nil {
		panic(err)
	}
	return raw
}
func sameModel(sc annot.Sidecar, m *reference) bool {
	raw, err := annot.Marshal(sc)
	if err != nil {
		return false
	}
	var got map[string]json.RawMessage
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	var want map[string]json.RawMessage
	if json.Unmarshal(m.file(), &want) != nil {
		return false
	}
	for _, kind := range []string{"strokes", "textItems"} {
		var a, b bytes.Buffer
		if json.Compact(&a, got[kind]) != nil || json.Compact(&b, want[kind]) != nil || !bytes.Equal(a.Bytes(), b.Bytes()) {
			return false
		}
	}
	return true
}

// Each client edits a stale local file and sometimes refreshes it. The oracle
// uses plain maps plus insertion ranks, not annot state, Apply or Marshal.
func modelRun(seed uint64, steps int, forgetTombstones bool) error {
	rng := rand.New(rand.NewSource(int64(seed & ((1 << 63) - 1)))) // #nosec G404 -- replayable test scheduling, not security.
	server := annot.Empty()
	model := newReference()
	clients := []annot.Sidecar{annot.Empty(), annot.Empty(), annot.Empty()}
	for n := 1; n <= steps; n++ {
		device := rng.Intn(len(clients))
		if rng.Intn(3) == 0 {
			clients[device] = server
		}
		base := clients[device]
		raw, err := annot.Marshal(base)
		if err != nil {
			return err
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		kind := []annot.Kind{annot.Strokes, annot.TextItems}[rng.Intn(2)]
		id := fmt.Sprintf("k%d", rng.Intn(6))
		var arr []json.RawMessage
		if err := json.Unmarshal(doc[string(kind)], &arr); err != nil {
			return err
		}
		index := -1
		for i, v := range arr {
			var e struct{ ID string }
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			if e.ID == id {
				index = i
			}
		}
		if rng.Intn(3) == 0 {
			if index >= 0 {
				arr = append(arr[:index], arr[index+1:]...)
			}
		} else {
			value := put(kind, id, rng.Intn(5)).Value
			if index >= 0 {
				arr[index] = value
			} else {
				arr = append(arr, value)
			}
		}
		if arr == nil {
			arr = []json.RawMessage{}
		}
		doc[string(kind)], err = json.Marshal(arr)
		if err != nil {
			return err
		}
		raw, err = json.Marshal(doc)
		if err != nil {
			return err
		}
		cur, err := annot.Parse(raw, annot.DefaultLimits())
		if err != nil {
			return err
		}
		ops, err := annot.Diff(base, cur)
		if err != nil {
			return err
		}
		clients[device] = cur
		model.apply(ops)
		head := server
		if forgetTombstones {
			raw, err = annot.Marshal(head)
			if err != nil {
				return err
			}
			head, err = annot.Parse(raw, annot.DefaultLimits())
			if err != nil {
				return err
			}
		}
		r, err := annot.Apply(head, ops, int64(n))
		if err != nil {
			return err
		}
		if !sameModel(r.Sidecar, model) {
			return fmt.Errorf("seed %d step %d: content/order differs", seed, n)
		}
		replay, err := annot.Apply(head, r.Effective, int64(n))
		if err != nil {
			return err
		}
		if !sameModel(replay.Sidecar, model) || len(replay.Dropped) != 0 {
			return fmt.Errorf("seed %d step %d: replay differs", seed, n)
		}
		server = r.Sidecar
	}
	return nil
}
func TestModelAgreement(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if err := modelRun(rapid.Uint64().Draw(t, "seed"), 12, false); err != nil {
			t.Fatal(err)
		}
	})
}
func TestModelFindsPlantedBug(t *testing.T) {
	for seed := uint64(0); seed < 1000; seed++ {
		if err := modelRun(seed, 40, true); err != nil {
			t.Logf("planted tombstone bug caught: %v", err)
			return
		}
	}
	t.Fatal("model did not detect the planted bug")
}
func TestPropNoLostAckedPut(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		if err := modelRun(rapid.Uint64().Draw(t, "seed"), 8, false); err != nil {
			t.Fatal(err)
		}
	})
}
func TestPropDeleteWins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := annot.ElementKey{Kind: annot.Strokes, ID: "x"}
		r, err := annot.Apply(annot.Empty(), annot.Ops{Elements: []annot.ElementOp{{Op: annot.Del, Key: key}}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		r, err = annot.Apply(r.Sidecar, annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "x", rapid.Int().Draw(t, "value"))}}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if !sameModel(r.Sidecar, newReference()) || len(r.Dropped) != 1 {
			t.Fatal("resurrection")
		}
	})
}
func TestPropCommutesOnDisjointKeys(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base, err := annot.Parse([]byte(`{"strokes":[{"id":"a"},{"id":"b"}]}`), annot.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		a := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "a", rapid.IntRange(0, 5).Draw(t, "a"))}}
		b := annot.Ops{Elements: []annot.ElementOp{put(annot.Strokes, "b", rapid.IntRange(0, 5).Draw(t, "b"))}}
		ab, err := annot.Apply(base, a, 1)
		if err != nil {
			t.Fatal(err)
		}
		ab, err = annot.Apply(ab.Sidecar, b, 2)
		if err != nil {
			t.Fatal(err)
		}
		ba, err := annot.Apply(base, b, 1)
		if err != nil {
			t.Fatal(err)
		}
		ba, err = annot.Apply(ba.Sidecar, a, 2)
		if err != nil {
			t.Fatal(err)
		}
		if annot.CanonicalHash(ab.Sidecar) != annot.CanonicalHash(ba.Sidecar) {
			t.Fatal("disjoint existing keys did not commute")
		}
	})
}
