package proto

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
)

// Replacing every present member with null checks presence at each nesting level,
// independently of the decoder's field tables (including false/zero values).
func TestEveryFieldRejectsNull(t *testing.T) {
	clients, servers := samples(t)
	run := func(name string, raw []byte, decode func([]byte) error) {
		var root any
		if e := json.Unmarshal(raw, &root); e != nil {
			t.Fatal(e)
		}
		var walk func(any)
		walk = func(v any) {
			switch obj := v.(type) {
			case map[string]any:
				for key, old := range obj {
					obj[key] = nil
					if e := decode(mustJSON(t, root)); e == nil {
						t.Errorf("%s accepted null %s", name, key)
					}
					obj[key] = old
					walk(old)
				}
			case []any:
				for _, child := range obj {
					walk(child)
				}
			}
		}
		walk(root)
	}
	for _, s := range clients {
		raw, e := EncodeClient(s.msg, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		run(s.name, raw, func(b []byte) error { _, e := DecodeClient(b, DefaultConfig()); return e })
	}
	for _, s := range servers {
		raw, e := EncodeServer(s.msg, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		run(s.name, raw, func(b []byte) error { _, e := DecodeServer(b, DefaultConfig()); return e })
	}
}
func TestServerSemanticFailures(t *testing.T) {
	_, servers := samples(t)
	for _, s := range servers {
		raw, e := EncodeServer(s.msg, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		var root any
		if e = json.Unmarshal(raw, &root); e != nil {
			t.Fatal(e)
		}
		var walk func(any)
		walk = func(v any) {
			switch obj := v.(type) {
			case map[string]any:
				for k, old := range obj {
					if _, ok := old.(float64); ok && k != "ratio" {
						for _, bad := range []any{float64(9007199254740992), 1.5, "1"} {
							obj[k] = bad
							if _, e := DecodeServer(mustJSON(t, root), DefaultConfig()); e == nil {
								t.Fatalf("%s accepted %s=%v", s.name, k, bad)
							}
						}
						obj[k] = old
					}
					walk(old)
				}
			case []any:
				for _, child := range obj {
					walk(child)
				}
			}
		}
		walk(root)
	}
	for _, mutation := range []func(map[string]any){
		func(m map[string]any) { m["kind"] = "annot_replace" }, func(m map[string]any) { m["kind"] = "rename_group" }, func(m map[string]any) { m["hash"] = strings.Repeat("f", 64) }, func(m map[string]any) { m["omitted"] = false; delete(m, "op") }, func(m map[string]any) { m["len"] = 100 }, func(m map[string]any) { m["class"] = "A" }, func(m map[string]any) { m["path"] = "x.md" },
	} {
		raw, e := EncodeServer(servers[3].msg, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		var root map[string]any
		if e = json.Unmarshal(raw, &root); e != nil {
			t.Fatal(e)
		}
		m := root["members"].([]any)[0].(map[string]any)
		mutation(m)
		if _, e := DecodeServer(mustJSON(t, root), DefaultConfig()); e == nil {
			t.Fatal("accepted member", m)
		}
	}
}
func TestClientVariantsAndDetails(t *testing.T) {
	for _, v := range []struct{ suffix, field string }{
		{`"kind":"create","file_id":"` + testID + `","path":"x.md","class":"Z","content":{"text":"x"}}`, "class"},
		{`"kind":"create","file_id":"` + testID + `","path":"x.bin","class":"C","content":{"text":"x"}}`, "content."},
		{`"kind":"rename","file_id":"` + testID + `","path":"x.md","on_taken":"other"}`, "on_taken"},
		{`"kind":"create","file_id":"` + testID + `","path":"x.md","class":"A","content":{"text":"x"},"copy_of":{"file_id":"` + testID + `","reason":"stale_put"}}`, "copy_of.reason"},
		{`"kind":"put_blob","file_id":"` + testID + `","base_version":1,"blob":{"sha256":"` + strings.Repeat("g", 64) + `","size":0}}`, "blob.sha256"},
		{`"kind":"put_blob","file_id":"` + testID + `","base_version":1,"blob":{"sha256":"` + strings.Repeat("F", 64) + `","size":0}}`, "blob.sha256"},
	} {
		_, e := DecodeClient([]byte(submitPrefix+v.suffix), DefaultConfig())
		var r *RejectError
		if !errors.As(e, &r) || r.Code != InvalidSubmission || r.Detail.Field != v.field {
			t.Fatal(v.field, e)
		}
	}
	_, e := DecodeClient([]byte(submitPrefix+`"kind":"rename","file_id":"`+testID+`","path":"ok/../bad"}`), DefaultConfig())
	var r *RejectError
	if !errors.As(e, &r) || r.Detail.Reason != "traversal" || r.Detail.Component == nil || *r.Detail.Component != 1 {
		t.Fatal(e)
	}
	for _, reason := range []CopyReason{CopyDeleted, CopyClassChange, CopyFirstSync} {
		raw := submitPrefix + `"kind":"create","file_id":"` + testID + `","path":"x.md","class":"A","content":{"text":"x"},"copy_of":{"file_id":"` + testID + `","reason":"` + string(reason) + `"}}`
		if _, e := DecodeClient([]byte(raw), DefaultConfig()); e != nil {
			t.Fatal(e)
		}
	}
	list := Subprotocols()
	list[0] = "changed"
	if Subprotocols()[0] != Subprotocol {
		t.Fatal("mutable protocol list")
	}
}
func TestAllDetailValues(t *testing.T) {
	_, servers := samples(t)
	r := servers[len(servers)-5].msg.(Reject)
	for _, d := range []Detail{{Reason: "pruned"}, {Class: classify.Text, Demotion: classify.Kept}, {Demotion: classify.InvalidUTF8}, {Scope: "mailbox", RetryAfterMS: 1}, {Component: ptrInt(2)}, {Class: classify.Opaque, Demotion: classify.TooLarge}, {Class: classify.Opaque, Demotion: classify.ContainsNUL}} {
		r.Detail = d
		raw, e := EncodeServer(r, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		got, e := DecodeServer(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(got, r) {
			t.Fatal(e)
		}
	}
	for _, d := range []Detail{{Scope: "unknown"}, {Class: classify.Class('Z')}, {Demotion: classify.Demotion(255)}, {Limit: -1}, {RetryAfterMS: -1}, {Component: ptrInt(-2)}} {
		r.Detail = d
		if _, e := EncodeServer(r, DefaultConfig()); e == nil {
			t.Fatal("invalid detail")
		}
	}
}
func ptrInt(n int) *int { return &n }

func TestAcceptedEscapeHeavyFrameReencodes(t *testing.T) {
	c := DefaultConfig()
	// Omitted optional fields leave room for one maximally escaped inline string.
	prefix := submitPrefix + `"kind":"create","file_id":"` + testID + `","path":"x.md","class":"A","content":{"text":"`
	suffix := `"}}`
	raw := []byte(prefix + strings.Repeat(`\\`, (c.MaxFrame-len(prefix)-len(suffix))/2) + suffix)
	m, e := DecodeClient(raw, c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = EncodeClient(m, c); e != nil {
		t.Fatal("accepted content must remain sendable", e)
	}
}

func TestAnnotationCanonicalFrameGrowth(t *testing.T) {
	c := DefaultConfig()
	prefix := submitPrefix + `"kind":"annot_edit","file_id":"` + testID + `","base_version":1,"ops":`
	opsPrefix := `{"meta":{"put":{"x":"`
	opsSuffix := `"}}}`
	makeRaw := func(n int) []byte {
		return append(append([]byte(prefix), mustJSON(t, opsPrefix+strings.Repeat(`\\`, n)+opsSuffix)...), '}')
	}
	raw := makeRaw((c.MaxFrame - len(makeRaw(0))) / 4)
	_, e := DecodeClient(raw, c)
	var rejection *RejectError
	if !errors.As(e, &rejection) || rejection.Code != TooLarge || rejection.Detail.Field != "frame" {
		t.Fatal("canonical growth must be attributed", e)
	}
	_, servers := samples(t)
	encoded, e := EncodeServer(servers[4].msg, c)
	if e != nil {
		t.Fatal(e)
	}
	var root map[string]any
	if e = json.Unmarshal(encoded, &root); e != nil {
		t.Fatal(e)
	}
	member := root["members"].([]any)[0].(map[string]any)
	member["ops"] = opsPrefix + opsSuffix
	empty := mustJSON(t, root)
	member["ops"] = opsPrefix + strings.Repeat(`\\`, (c.MaxServerFrame-len(empty))/4) + opsSuffix
	raw = mustJSON(t, root)
	if _, e := DecodeServer(raw, c); e == nil {
		t.Fatal("server canonical growth must remain bounded")
	}
}
