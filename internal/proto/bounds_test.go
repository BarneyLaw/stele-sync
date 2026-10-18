package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

func TestInlineAndPage(t *testing.T) {
	_, servers := samples(t)
	ch := servers[3].msg.(Live).Change
	m := ch.Members[0]
	c := DefaultConfig()
	p, e := payloadWire(m.Payload, c)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := marshalFrame(p, c.MaxServerFrame)
	if e != nil {
		t.Fatal(e)
	}
	c.MaxInline = len(raw)
	if !reflect.DeepEqual(Inline(m, c), m) {
		t.Fatal("threshold omitted")
	}
	c.MaxInline--
	if _, ok := Inline(m, c).Payload.(Omitted); !ok {
		t.Fatal("above threshold stayed inline")
	}
	if _, ok := m.Payload.(TextPayload); !ok {
		t.Fatal("mutated member")
	}
	c = DefaultConfig()
	c.PageEntries = 2
	page := NewPage(1, c)
	for i := 1; i <= 2; i++ {
		ch.Seq = ids.Seq(i)
		ok, e := page.Add(ch)
		if e != nil || !ok {
			t.Fatal(e)
		}
	}
	ch.Seq = 3
	if ok, e := page.Add(ch); e != nil || ok {
		t.Fatal("entry cap", e)
	}
	raw, e = page.Encode(true)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodeServer(raw, c)
	if e != nil || len(decoded.(Changes).Entries) != 2 {
		t.Fatal(e)
	}
	// Small but valid budgets make the byte cap fire before the entry cap.
	c = DefaultConfig()
	c.MaxInline = 100
	c.EnvelopeReserve = 1
	c.MaxFrame = 2048
	c.Text.MaxJSONBytes = 2047
	c.Annot.MaxOpBytes = 2047
	c.MaxInlineContent = 2047
	c.MaxServerFrame = 500
	if e = c.Validate(); e != nil {
		t.Fatal(e)
	}
	page = NewPage(1, c)
	ch.Seq = 1
	if ok, e := page.Add(ch); e != nil || !ok {
		t.Fatal(e)
	}
	ch.Seq = 2
	if ok, e := page.Add(ch); e != nil || ok {
		t.Fatal("byte cap", e)
	}
	for _, more := range []bool{false, true} {
		raw, e = page.Encode(more)
		if e != nil || len(raw) > c.MaxServerFrame {
			t.Fatal("frame cap", e)
		}
	}
	c.MaxServerFrame = 201
	page = NewPage(1, c)
	if _, e := page.Add(ch); !errors.Is(e, ErrFrameTooLarge) {
		t.Fatal("single too big", e)
	}
	if _, e := NewPage(0, DefaultConfig()).Encode(false); e == nil {
		t.Fatal("zero request")
	}
	if _, e := NewPage(1, Config{}).Add(ch); e == nil {
		t.Fatal("bad config")
	}
	if _, e := NewPage(1, DefaultConfig()).Encode(true); e == nil {
		t.Fatal("empty more")
	}
	page = NewPage(1, DefaultConfig())
	ch.Seq = 1
	if _, e = page.Add(ch); e != nil {
		t.Fatal(e)
	}
	ch.Seq = 3
	if _, e = page.Add(ch); e == nil {
		t.Fatal("gap accepted")
	}
}
func TestMaxPayloadFitsFrame(t *testing.T) {
	c := DefaultConfig()
	op, e := c.Text.Parse([]byte(`["` + strings.Repeat("a", c.Text.MaxJSONBytes-4) + `"]`))
	if e != nil {
		t.Fatal(e)
	}
	clients, _ := samples(t)
	s := clients[5].msg.(Submit)
	body := s.Body.(TextEdit)
	body.Op = op
	s.Body = body
	raw, e := EncodeClient(s, c)
	if e != nil || len(raw) > c.MaxFrame {
		t.Fatal(e)
	}
	// Measure raw annotation syntax overhead before filling its bounded value.
	empty := `{"elements":[],"entries":[],"meta":{"put":{"x":""},"del":[]}}`
	ann := []byte(strings.Replace(empty, `"x":""`, `"x":"`+strings.Repeat("a", c.Annot.MaxOpBytes-len(empty))+`"`, 1))
	raw = append([]byte(submitPrefix+`"kind":"annot_edit","file_id":"`+testID+`","base_version":1,"ops":`), mustJSON(t, string(ann))...)
	raw = append(raw, '}')
	if len(raw) > c.MaxFrame {
		t.Fatal("ordinary annotation payload did not fit")
	}
	if _, e = DecodeClient(raw, c); e != nil {
		t.Fatal(e)
	}
	// Quoting expansion cannot bypass the authoritative whole-frame ceiling.
	s = clients[10].msg.(Submit)
	create := s.Body.(Create)
	create.Content = InlineText{strings.Repeat("\x00", c.MaxInlineContent)}
	s.Body = create
	if _, e = EncodeClient(s, c); !errors.Is(e, ErrFrameTooLarge) {
		t.Fatal("escaped frame overflow", e)
	}
	c.MaxInlineContent = 4
	raw = []byte(submitPrefix + `"kind":"create","file_id":"` + testID + `","path":"x.md","class":"A","content":{"text":"12345"}}`)
	_, e = DecodeClient(raw, c)
	var reject *RejectError
	if !errors.As(e, &reject) || reject.Code != TooLarge || reject.Detail.Limit != 4 {
		t.Fatal(e)
	}
}
func mustJSON(t testing.TB, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestSafeIntegerMembers(t *testing.T) {
	for _, name := range []string{"base_version", "size", "after_seq", "req", "n"} {
		for _, n := range []string{"null", "1.0", "1e3", "-1", "9007199254740992", "9223372036854775808"} {
			raw := ""
			switch name {
			case "base_version":
				raw = submitPrefix + `"kind":"delete","file_id":"` + testID + `","base_version":` + n + `}`
			case "size":
				raw = submitPrefix + `"kind":"put_blob","file_id":"` + testID + `","base_version":1,"blob":{"sha256":"` + strings.Repeat("0", 64) + `","size":` + n + `}}`
			case "after_seq":
				raw = `{"t":"pull_changes","req":1,"after_seq":` + n + `}`
			case "req":
				raw = `{"t":"list_files","req":` + n + `}`
			case "n":
				raw = `{"t":"pong","n":` + n + `}`
			}
			if _, e := DecodeClient([]byte(raw), DefaultConfig()); e == nil {
				t.Fatal(name, n)
			}
		}
	}
	for _, n := range []string{"0", "4294967295"} {
		if _, e := DecodeClient([]byte(`{"t":"pong","n":`+n+`}`), DefaultConfig()); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := DecodeClient([]byte(`{"t":"pull_changes","req":4294967295,"after_seq":9007199254740991}`), DefaultConfig()); e != nil {
		t.Fatal(e)
	}
}
func TestUnknownServerFields(t *testing.T) {
	_, servers := samples(t)
	for _, s := range servers {
		raw, e := EncodeServer(s.msg, DefaultConfig())
		if e != nil {
			t.Fatal(e)
		}
		var obj map[string]any
		if e = json.Unmarshal(raw, &obj); e != nil {
			t.Fatal(e)
		}
		addUnknown(obj)
		raw = mustJSON(t, obj)
		m, e := DecodeServer(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(m, s.msg) {
			t.Fatal(s.name, e)
		}
	}
}
func addUnknown(v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, v := range x {
			addUnknown(v)
		}
		x["future"] = map[string]any{"value": "😀"}
	case []any:
		for _, v := range x {
			addUnknown(v)
		}
	}
}
func TestInvalidLocalValues(t *testing.T) {
	c := DefaultConfig()
	clients, servers := samples(t)
	for _, v := range []ClientMsg{nil, (*Hello)(nil), Hello{}, Submit{}, PullChanges{}, ListFiles{}} {
		if _, e := EncodeClient(v, c); e == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
	for _, v := range []ServerMsg{nil, (*Ping)(nil), Welcome{}, Live{}, Changes{}, Files{}, Reject{}, Error{}} {
		if _, e := EncodeServer(v, c); e == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
	for _, s := range clients {
		v := reflect.New(reflect.TypeOf(s.msg))
		v.Elem().Set(reflect.ValueOf(s.msg))
		if _, e := EncodeClient(v.Interface().(ClientMsg), c); e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range servers {
		v := reflect.New(reflect.TypeOf(s.msg))
		v.Elem().Set(reflect.ValueOf(s.msg))
		if _, e := EncodeServer(v.Interface().(ServerMsg), c); e != nil {
			t.Fatal(e)
		}
	}
	w := servers[0].msg.(Welcome)
	w.Limits.Guard.Ratio = math.NaN()
	if _, e := EncodeServer(w, c); e == nil {
		t.Fatal("NaN")
	}
	s := clients[5].msg.(Submit)
	s.Body = (*TextEdit)(nil)
	if _, e := EncodeClient(s, c); e == nil {
		t.Fatal("nil body")
	}
	if _, e := marshalFrame(wire{"bad": func() {}}, 100); e == nil {
		t.Fatal("unsupported type")
	}
	a := annot.Ops{Replace: true}
	s = clients[6].msg.(Submit)
	b := s.Body.(AnnotEdit)
	b.Ops = a
	s.Body = b
	if _, e := EncodeClient(s, c); e == nil {
		t.Fatal("trusted client")
	}
}
func TestFrameSyntax(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{} {}`), []byte(`{"t":"pong","n":0,"x":{"a":0,"a":1}}`), []byte("{\"t\":\"pong\",\"n\":0,\"x\":\"\xff\"}"), bytes.Repeat([]byte("x"), DefaultConfig().MaxServerFrame+1)} {
		if _, e := DecodeServer(raw, DefaultConfig()); e == nil {
			t.Fatal("accepted")
		}
	}
}

func TestTextWireDoesNotExpandHTML(t *testing.T) {
	c := DefaultConfig()
	raw := []byte(submitPrefix + `"kind":"text_edit","file_id":"` + testID + `","base_version":1,"op":["` + strings.Repeat("<", c.Text.MaxJSONBytes-4) + `"]}`)
	m, e := DecodeClient(raw, c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = EncodeClient(m, c); e != nil {
		t.Fatal(e)
	}
}

func TestInlineTransformedComponentOverflow(t *testing.T) {
	// Transform can split deletes into more components than client ingress allows,
	// even when the resulting encoded bytes fit the inline threshold.
	c := DefaultConfig()
	l := c.Text
	l.MaxComponents *= 2
	b, e := textop.NewBuilder(l)
	if e != nil {
		t.Fatal(e)
	}
	for range c.Text.MaxComponents/2 + 1 {
		b.Retain(1).Insert("x")
	}
	op, e := b.Op()
	if e != nil {
		t.Fatal(e)
	}
	_, servers := samples(t)
	ch := servers[3].msg.(Live).Change
	n := op.TargetLen()
	ch.Members[0].Payload = TextPayload{op}
	ch.Members[0].Len = &n
	ch.Members[0] = Inline(ch.Members[0], c)
	if _, ok := ch.Members[0].Payload.(Omitted); !ok {
		t.Fatal("unparseable transformed output stayed inline")
	}
	if _, e := EncodeServer(Live{ch}, c); e != nil {
		t.Fatal(e)
	}
}
