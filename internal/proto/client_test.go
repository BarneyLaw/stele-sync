package proto

import (
	"errors"
	"strings"
	"testing"
)

const testID = "019a1b2c-0000-7000-8000-000000000abc"
const submitPrefix = `{"t":"submit","client_op_id":"` + testID + `",`

func TestTierTable(t *testing.T) {
	cases := []struct{ raw, tier, code, field string }{
		{`{`, "close", "4400", ""}, {`[]`, "close", "4400", ""}, {`null`, "close", "4400", ""}, {`{}`, "close", "4400", ""},
		{`{"t":"ping","n":0}`, "close", "4400", ""}, {`{"t":"pong","n":0,"N":1}`, "close", "4400", ""},
		{`{"t":"hello","token":"bad"}`, "close", "4401", ""}, {`{"t":"hello"}`, "close", "4401", ""},
		{`{"t":"submit","client_op_id":null}`, "close", "4400", ""},
		{submitPrefix + `"kind":"delete","file_id":"` + testID + `","base_version":1,"x":"\ud800"}`, "close", "4400", ""},
		{submitPrefix + `"kind":"delete","kind":"delete"}`, "close", "4400", ""},
		{submitPrefix + `"kind":"unknown"}`, "reject", "invalid_submission", "kind"},
		{submitPrefix + `"kind":"delete","file_id":"` + testID + `"}`, "reject", "invalid_submission", "base_version"},
		{submitPrefix + `"kind":"delete","file_id":"` + testID + `","base_version":9007199254740992}`, "reject", "invalid_submission", "base_version"},
		{submitPrefix + `"kind":"text_edit","file_id":"` + testID + `","base_version":1,"op":[0]}`, "reject", "invalid_op", "op"},
		{submitPrefix + `"kind":"annot_edit","file_id":"` + testID + `","base_version":1,"ops":"{\"replace\":true}"}`, "reject", "invalid_op", "ops"},
		{submitPrefix + `"kind":"rename","file_id":"` + testID + `","path":"../bad"}`, "reject", "invalid_path", "path"},
		{`{"t":"pull_changes","req":0,"after_seq":0}`, "close", "4400", ""},
		{`{"t":"pull_changes","req":1,"after_seq":-1}`, "query", "invalid_request", "after_seq"},
		{`{"t":"list_files","req":1,"after_file_id":null}`, "query", "invalid_request", "after_file_id"},
		{`{"t":"list_files","req":1,"unknown":0}`, "query", "invalid_request", "unknown"},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			_, e := DecodeClient([]byte(c.raw), DefaultConfig())
			if e == nil {
				t.Fatal("accepted")
			}
			switch c.tier {
			case "close":
				var v *CloseError
				if !errors.As(e, &v) || fmtCode(v.Code) != c.code {
					t.Fatal(e)
				}
			case "reject":
				var v *RejectError
				if !errors.As(e, &v) || string(v.Code) != c.code || v.Detail.Field != c.field || v.ID.String() != testID {
					t.Fatalf("%+v", e)
				}
			case "query":
				var v *QueryError
				if !errors.As(e, &v) || string(v.Code) != c.code || v.Detail.Field != c.field || v.Req != 1 {
					t.Fatalf("%+v", e)
				}
			}
		})
	}
}
func TestContentNotClassified(t *testing.T) {
	raw := submitPrefix + `"kind":"create","file_id":"` + testID + `","path":"x.pdf.annot.json","class":"B","content":{"sidecar":"not a document"}}`
	m, e := DecodeClient([]byte(raw), DefaultConfig())
	if e != nil {
		t.Fatal(e)
	}
	if string(m.(Submit).Body.(Create).Content.(InlineSidecar).Raw) != "not a document" {
		t.Fatal(m)
	}
}
func TestStrictness(t *testing.T) {
	for _, raw := range []string{
		submitPrefix + `"kind":"create","file_id":"` + testID + `","path":"x.md","class":"A","content":{"text":"x","blob":null}}`,
		submitPrefix + `"kind":"rename_group","pdf":{"file_id":"` + testID + `","path":"x.pdf","x":0},"sidecar":{"file_id":"` + testID + `","path":"x.pdf.annot.json"}}`,
		submitPrefix + `"kind":"delete","file_id":"` + testID + `","base_version":1,"confirmed":null}`,
		submitPrefix + `"kind":"delete","file_id":"` + testID + `","base_version":1,"Confirmed":true}`,
	} {
		if _, e := DecodeClient([]byte(raw), DefaultConfig()); e == nil {
			t.Fatal("accepted", raw)
		}
	}
	raw := `{"t":"pong","n":0,"unknown":` + strings.Repeat(`[`, 10000) + `0` + strings.Repeat(`]`, 10000) + `}`
	if _, e := DecodeClient([]byte(raw), DefaultConfig()); e == nil {
		t.Fatal("deep JSON accepted")
	}
}
