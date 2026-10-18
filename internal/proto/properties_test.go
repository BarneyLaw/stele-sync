package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
	"pgregory.net/rapid"
)

func TestPropClientRoundTrip(t *testing.T) {
	clients, _ := samples(t)
	rapid.Check(t, func(t *rapid.T) {
		m := clients[rapid.IntRange(0, len(clients)-1).Draw(t, "type")].msg
		switch v := m.(type) {
		case PullChanges:
			v.Req = Req(rapid.Uint32Range(1, ^uint32(0)).Draw(t, "req"))
			v.After = ids.Seq(rapid.Int64Range(0, ids.MaxSafe).Draw(t, "seq"))
			m = v
		case Pong:
			v.N = rapid.Uint32().Draw(t, "nonce")
			m = v
		case Submit:
			entropy := rapid.SliceOfN(rapid.Byte(), 10, 10).Draw(t, "entropy")
			id, e := ids.NewClientOpID(rapid.Int64Range(0, 1<<48-1).Draw(t, "time"), bytes.NewReader(entropy))
			if e != nil {
				t.Fatal(e)
			}
			v.ID = id
			if edit, ok := v.Body.(TextEdit); ok {
				s := rapid.StringMatching(`[a-z😀中]{1,50}`).Draw(t, "insert")
				var b textop.Builder
				b.Retain(rapid.IntRange(0, 100).Draw(t, "retain")).Insert(s)
				op, e := b.Op()
				if e != nil {
					t.Fatal(e)
				}
				edit.Op = op
				v.Body = edit
			}
			m = v
		}
		raw, e := EncodeClient(m, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		got, e := DecodeClient(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(m, got) {
			t.Fatal("round trip", e)
		}
	})
}
func TestPropServerRoundTrip(t *testing.T) {
	_, servers := samples(t)
	rapid.Check(t, func(t *rapid.T) {
		m := servers[rapid.IntRange(0, len(servers)-1).Draw(t, "type")].msg
		switch v := m.(type) {
		case Live:
			v.Change.Seq = ids.Seq(rapid.Int64Range(1, ids.MaxSafe).Draw(t, "seq"))
			m = v
		case Ping:
			v.N = rapid.Uint32().Draw(t, "nonce")
			m = v
		case Error:
			v.Req = Req(rapid.Uint32Range(1, ^uint32(0)).Draw(t, "req"))
			m = v
		}
		raw, e := EncodeServer(m, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		got, e := DecodeServer(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(m, got) {
			t.Fatal("round trip", e)
		}
	})
}
func TestPropTierSoundness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		field := "unknown_" + rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "field")
		value := rapid.Int64().Draw(t, "value")
		raw, e := json.Marshal(map[string]any{"t": "submit", "client_op_id": testID, "kind": "delete", "file_id": testID, "base_version": 1, field: value})
		if e != nil {
			t.Fatal(e)
		}
		_, e = DecodeClient(raw, DefaultConfig())
		var reject *RejectError
		if !errors.As(e, &reject) || reject.ID.String() != testID || reject.Detail.Field != field {
			t.Fatal("lost attribution", e)
		}
		for _, bad := range []string{string(raw[:len(raw)-1]) + `,"t":"submit"}`, string(raw[:len(raw)-1]) + `,"bad":"\ud800"}`} {
			_, e = DecodeClient([]byte(bad), DefaultConfig())
			var closeErr *CloseError
			if !errors.As(e, &closeErr) || closeErr.Code != CloseProtocol {
				t.Fatal("attributed corrupt frame", e)
			}
		}
	})
}
func FuzzDecodeClient(f *testing.F) {
	clients, _ := samples(f)
	for _, v := range clients {
		raw, e := EncodeClient(v.msg, DefaultConfig())
		if e != nil {
			f.Fatal(e)
		}
		f.Add(raw)
	}
	f.Add([]byte(`null`))
	f.Add([]byte(submitPrefix + `"kind":"unknown"}`))
	f.Add([]byte(`{"t":"list_files","req":1,"after_file_id":null}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		m, e := DecodeClient(raw, DefaultConfig())
		if e != nil {
			var c *CloseError
			var r *RejectError
			var q *QueryError
			n := 0
			for _, ok := range []bool{errors.As(e, &c), errors.As(e, &r), errors.As(e, &q)} {
				if ok {
					n++
				}
			}
			if n != 1 {
				t.Fatal("missing error tier", e)
			}
			return
		}
		encoded, e := EncodeClient(m, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		again, e := DecodeClient(encoded, DefaultConfig())
		if e != nil || !reflect.DeepEqual(m, again) {
			t.Fatal("unstable round trip", e)
		}
	})
}
func FuzzDecodeServer(f *testing.F) {
	_, servers := samples(f)
	for _, v := range servers {
		raw, e := EncodeServer(v.msg, DefaultConfig())
		if e != nil {
			f.Fatal(e)
		}
		f.Add(raw)
	}
	f.Add([]byte(`{"t":"ping","n":0,"future":{"x":null}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		m, e := DecodeServer(raw, DefaultConfig())
		if e != nil {
			return
		}
		encoded, e := EncodeServer(m, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		again, e := DecodeServer(encoded, DefaultConfig())
		if e != nil || !reflect.DeepEqual(m, again) {
			t.Fatal("unstable round trip", e)
		}
	})
}
func TestFramingBounded(t *testing.T) {
	for _, raw := range [][]byte{bytes.Repeat([]byte("x"), DefaultConfig().MaxFrame+1), []byte(`{"t":"pong","n":0,"x":` + strings.Repeat("[", 10000) + `0` + strings.Repeat("]", 10000) + `}`)} {
		allocations := testing.AllocsPerRun(100, func() {
			if _, e := DecodeClient(raw, DefaultConfig()); e == nil {
				t.Fatal("accepted")
			}
		})
		if allocations > 3 {
			t.Fatalf("preflight allocated %.0f objects", allocations)
		}
	}
}
