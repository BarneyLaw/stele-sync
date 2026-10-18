package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/rules"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
)

var updateProtocol = flag.Bool("update-protocol", false, "regenerate M4 protocol fixtures")

type clientSample struct {
	name string
	msg  ClientMsg
}
type serverSample struct {
	name string
	msg  ServerMsg
}
type fixture struct {
	Name  string          `json:"name"`
	Frame json.RawMessage `json:"frame"`
}

func samples(t testing.TB) ([]clientSample, []serverSample) {
	t.Helper()
	f, e := ids.ParseFileID(testID)
	if e != nil {
		t.Fatal(e)
	}
	f2, e := ids.ParseFileID(strings.Replace(testID, "abc", "abd", 1))
	if e != nil {
		t.Fatal(e)
	}
	dev, e := ids.ParseDeviceID(testID)
	if e != nil {
		t.Fatal(e)
	}
	id, e := ids.ParseClientOpID(testID)
	if e != nil {
		t.Fatal(e)
	}
	path := func(s string) vpath.Path {
		v, e := vpath.Parse(s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	op, e := textop.Parse([]byte(`[2,"hi",-1]`))
	if e != nil {
		t.Fatal(e)
	}
	ops, e := annot.ParseOps([]byte(`{"meta":{"put":{"raw":{"n":1.0,"s":"\u0061<>&"}}}}`), annot.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	replacement, e := annot.ParseCommitted([]byte(`{"replace":true,"meta":{"put":{"appendedPages":[{"n":1.0}]}}}`), annot.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	token, e := ParseToken("stele_" + strings.Repeat("A", 43))
	if e != nil {
		t.Fatal(e)
	}
	blob := BlobRef{SHA256: [32]byte{1, 2, 3}, Size: 123}
	clients := []clientSample{{"hello", Hello{token}}, {"pull_changes", PullChanges{1, 0}}, {"list_files", ListFiles{1, nil}}, {"list_files_after", ListFiles{2, &f}}, {"pong", Pong{0}}}
	bodies := []struct {
		name string
		body Body
	}{
		{"text_edit", TextEdit{f, 1, op}}, {"annot_edit", AnnotEdit{f, 1, ops}}, {"annot_replace_inline", AnnotReplace{f, 1, InlineSidecar{[]byte(`{"a":1.0,"s":"\u0061<>&"}`)}}}, {"annot_replace_blob", AnnotReplace{f, 1, BlobContent{blob}}},
		{"put_blob", PutBlob{f, 1, blob}}, {"create_a", Create{File: f, Path: path("notes/a.md"), Class: classify.Text, Content: InlineText{"hello 😀\r\n<>&"}}},
		{"create_b", Create{File: f, Path: path("a.pdf.annot.json"), Class: classify.Sidecar, Content: InlineSidecar{[]byte(`{"strokes":[],"raw":1.0}`)}}},
		{"create_c", Create{File: f, Path: path("a.pdf"), Class: classify.Opaque, Content: BlobContent{blob}, OnTaken: rules.Reject}},
		{"create_copy", Create{File: f, Path: path("a.md"), Class: classify.Text, Content: BlobContent{blob}, CopyOf: &CopyOf{f2, CopyRebase}}},
		{"rename", Rename{f, path("b.md"), rules.Number}}, {"rename_group", RenameGroup{Target{f, path("b.pdf")}, Target{f2, path("b.pdf.annot.json")}, rules.Reject}}, {"delete", Delete{f, 1, true}},
	}
	for _, v := range bodies {
		clients = append(clients, clientSample{v.name, Submit{id, v.body}})
	}
	length := 4
	hash := [32]byte{9, 8, 7}
	a := Member{File: f, Version: 2, Kind: KindTextEdit, Payload: TextPayload{op}, Len: &length}
	b := Member{File: f, Version: 2, Kind: KindAnnotEdit, Payload: AnnotPayload{ops}, Hash: &hash}
	change := func(members ...Member) Change {
		return Change{Seq: 1, At: time.Date(2026, 10, 18, 9, 30, 0, 123000000, time.UTC), Actor: rules.Device, Device: dev, ID: id, Members: members}
	}
	advertised := Advertised{MaxFrame: 1 << 20, MaxServerFrame: 2 << 20, SplitBytes: 512 << 10, PageEntries: 500, InflightPerDevice: 64, Rate: Rate{50, 200}, Classify: Classification{[]string{"md", "txt", "mdx"}, 8 << 20, 16 << 20}, Guard: Guard{300000, 25, 5, .1, 30}}
	servers := []serverSample{{"welcome", Welcome{dev, 0, advertised}}, {"ping", Ping{0}}, {"changes_empty", Changes{Req: 1, Entries: []Change{}}}, {"change_text", Live{change(a)}}, {"change_annot", Live{change(b)}}, {"changes", Changes{Req: 1, Entries: []Change{change(a)}, More: true}}}
	for _, v := range []struct {
		name string
		m    Member
	}{
		{"create_a", Member{File: f, Version: 1, Kind: KindCreate, Path: path("a.md"), Class: classify.Text, Payload: TextPayload{op}, Len: &length, Origin: &Origin{f2, CopyFirstSync}}},
		{"create_b", Member{File: f, Version: 1, Kind: KindCreate, Path: path("a.pdf.annot.json"), Class: classify.Sidecar, Payload: AnnotPayload{replacement}, Hash: &hash}},
		{"create_c", Member{File: f, Version: 1, Kind: KindCreate, Path: path("a.pdf"), Class: classify.Opaque, Owner: rules.Canvas, Payload: BlobPayload{blob}, Origin: &Origin{f2, CopyStalePut}}},
		{"omitted_text", Member{File: f, Version: 2, Kind: KindTextEdit, Payload: Omitted{}, Len: &length}},
		{"omitted_annot", Member{File: f, Version: 2, Kind: KindAnnotEdit, Payload: Omitted{}, Hash: &hash}},
		{"put_blob", Member{File: f, Version: 2, Kind: KindPutBlob, Payload: BlobPayload{blob}}},
		{"rename", Member{File: f, Version: 2, Kind: KindRename, Path: path("new.md")}},
		{"delete", Member{File: f, Version: 2, Kind: KindDelete}},
	} {
		ch := change(v.m)
		if v.name == "put_blob" {
			ch.Actor = rules.Bridge
		}
		if v.name == "delete" {
			ch.Actor = rules.Admin
		}
		servers = append(servers, serverSample{v.name, Live{ch}})
	}
	servers = append(servers, serverSample{"rename_group", Live{change(Member{File: f, Version: 2, Kind: KindRename, Path: path("b.pdf")}, Member{File: f2, Version: 3, Kind: KindRename, Path: path("b.pdf.annot.json")})}})
	servers = append(servers, serverSample{"files", Files{Req: 1, HeadSeq: 1, Files: []FileInfo{{File: f, Path: path("a.md"), Class: classify.Text, Version: 1, Size: 4, Len: &length}, {File: f2, Path: path("a.pdf"), Class: classify.Opaque, Owner: rules.Canvas, Version: 1, Size: 123, Blob: &blob}}, More: true}}, serverSample{"files_b", Files{Req: 1, HeadSeq: 1, Files: []FileInfo{{File: f, Path: path("a.pdf.annot.json"), Class: classify.Sidecar, Version: 1, Size: 10, Hash: &hash}}}}, serverSample{"files_empty", Files{Req: 1, Files: []FileInfo{}}})
	component := -1
	for _, code := range Codes() {
		detail := Detail{}
		switch code {
		case InvalidSubmission:
			detail.Field = "base_version"
		case InvalidOp:
			detail = Detail{Field: "ops", Reason: "ErrNonCanonical"}
		case InvalidPath:
			detail = Detail{Field: "path", Reason: "absolute", Component: &component}
		case TooLarge:
			detail = Detail{Field: "op", Limit: 1000}
		case RateLimited:
			detail = Detail{Scope: "device", RetryAfterMS: 100}
		case ClassMismatch:
			detail = Detail{Class: classify.Opaque, Demotion: classify.NotFreedraw}
		case BulkDeleteGuard:
			detail.Limit = 25
		case UnknownFile, FileExists, FileDeleted, PathTaken, StaleDelete, InvalidBase, ReadOnlyPath, ClassChange, GroupRequired, RebaseRequired, WholeDocumentRequired, BlobMissing, Internal:
			// These codes carry no detail.
		}
		servers = append(servers, serverSample{"reject_" + string(code), Reject{id, code, detail}})
	}
	for _, code := range QueryCodes() {
		detail := Detail{}
		if code == ResyncRequired {
			detail.Reason = "ahead"
		}
		if code == InvalidRequest {
			detail.Field = "after_seq"
		}
		if code == QueryRateLimited {
			detail.RetryAfterMS = 100
		}
		servers = append(servers, serverSample{"error_" + string(code), Error{1, code, detail}})
	}
	return clients, servers
}
func golden(t *testing.T, path string, v any) {
	t.Helper()
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	b = append(b, '\n')
	p := filepath.Join("../../schema/protocol", path)
	if *updateProtocol {
		if e = os.MkdirAll(filepath.Dir(p), 0750); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	want, e := os.ReadFile(p) // #nosec G304 -- path is a fixed test fixture destination.
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("fixture drift: %s", p)
	}
}
func TestProtocolFixtures(t *testing.T) {
	clients, servers := samples(t)
	c2s := []fixture{}
	s2c := []fixture{}
	for _, s := range clients {
		raw, e := EncodeClient(s.msg, DefaultConfig())
		if e != nil {
			t.Fatalf("%s: %v", s.name, e)
		}
		got, e := DecodeClient(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(got, s.msg) {
			t.Fatalf("client %s round trip: %v\ngot=%#v\nwant=%#v", s.name, e, got, s.msg)
		}
		c2s = append(c2s, fixture{s.name, raw})
	}
	for _, s := range servers {
		raw, e := EncodeServer(s.msg, DefaultConfig())
		if e != nil {
			t.Fatalf("%s: %v", s.name, e)
		}
		got, e := DecodeServer(raw, DefaultConfig())
		if e != nil || !reflect.DeepEqual(got, s.msg) {
			t.Fatalf("server %s round trip: %v\ngot=%#v\nwant=%#v", s.name, e, got, s.msg)
		}
		s2c = append(s2c, fixture{s.name, raw})
	}
	golden(t, "c2s/valid.json", c2s)
	golden(t, "s2c/valid.json", s2c)
	golden(t, "codes.json", struct {
		Subprotocol string              `json:"subprotocol"`
		Reject      []Code              `json:"reject"`
		Query       []QueryCode         `json:"query"`
		Close       map[string][]string `json:"close"`
	}{Subprotocol, Codes(), QueryCodes(), map[string][]string{"1000": {"normal"}, "1001": {"shutting_down"}, "1009": {"frame_too_large"}, "1011": {"internal"}, "4400": {"protocol"}, "4401": {"hello_timeout", "bad_token", "revoked"}, "4409": {"replaced"}, "4426": {"unsupported_protocol"}, "4429": {"slow_consumer"}}})
	golden(t, "limits.json", map[string]int{"max_frame": 1 << 20, "max_server_frame": 2 << 20, "envelope_reserve": 16 << 10, "max_inline": 512 << 10, "max_inline_content": (1 << 20) - (16 << 10), "max_text_op": (1 << 20) - (16 << 10), "max_annot_op": (1 << 20) - (16 << 10), "page_entries": 500, "max_members": 2, "max_depth": MaxDepth})
}

type invalidFixture struct{ Name, Frame, Tier, Code, Field, Reason string }

func TestInvalidFixtures(t *testing.T) {
	for _, direction := range []string{"c2s", "s2c"} {
		t.Run(direction, func(t *testing.T) {
			raw, e := os.ReadFile("../../schema/protocol/" + direction + "/invalid.json") // #nosec G304 -- direction is a closed literal list.
			if e != nil {
				t.Fatal(e)
			}
			var v struct {
				Source string
				Cases  []invalidFixture
			}
			if e = json.Unmarshal(raw, &v); e != nil {
				t.Fatal(e)
			}
			if v.Source != "hand-written" || len(v.Cases) == 0 {
				t.Fatal("invalid fixture provenance")
			}
			for _, f := range v.Cases {
				t.Run(f.Name, func(t *testing.T) {
					if direction == "s2c" {
						if _, e := DecodeServer([]byte(f.Frame), DefaultConfig()); e == nil {
							t.Fatal("accepted")
						}
						return
					}
					_, e := DecodeClient([]byte(f.Frame), DefaultConfig())
					var cl *CloseError
					var re *RejectError
					var qe *QueryError
					switch {
					case errors.As(e, &cl):
						if f.Tier != "close" || f.Code != fmtCode(cl.Code) {
							t.Fatal(e)
						}
					case errors.As(e, &re):
						if f.Tier != "reject" || f.Code != string(re.Code) || f.Field != re.Detail.Field {
							t.Fatal(e)
						}
					case errors.As(e, &qe):
						if f.Tier != "query" || f.Code != string(qe.Code) || f.Field != qe.Detail.Field {
							t.Fatal(e)
						}
					default:
						t.Fatal("missing tier", e)
					}
				})
			}
		})
	}
}

// Catalogue coverage discovers sealed implementers from their marker methods;
// adding a new type without a fixture must fail even if a switch still compiles.
func TestCatalogueComplete(t *testing.T) {
	clients, servers := samples(t)
	seen := map[string]map[string]bool{"clientMsg": {}, "serverMsg": {}, "body": {}}
	for _, s := range clients {
		seen["clientMsg"][reflect.TypeOf(s.msg).Name()] = true
		if submit, ok := s.msg.(Submit); ok {
			seen["body"][reflect.TypeOf(submit.Body).Name()] = true
		}
	}
	for _, s := range servers {
		seen["serverMsg"][reflect.TypeOf(s.msg).Name()] = true
	}
	source, e := parser.ParseFile(token.NewFileSet(), "types.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range source.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		group, ok := seen[fn.Name.Name]
		if !ok {
			continue
		}
		receiver, ok := fn.Recv.List[0].Type.(*ast.Ident)
		if !ok {
			t.Fatal("unexpected sealed receiver")
		}
		if !group[receiver.Name] {
			t.Errorf("missing %s fixture: %s", fn.Name.Name, receiver.Name)
		}
	}
	catalogue, e := parser.ParseFile(token.NewFileSet(), "codes.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range catalogue.Decls {
		decl, ok := d.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			continue
		}
		for _, spec := range decl.Specs {
			v := spec.(*ast.ValueSpec)
			typ, ok := v.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for _, value := range v.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok {
					continue
				}
				if typ.Name == "Code" {
					s, e := strconv.Unquote(lit.Value)
					if e != nil {
						t.Fatal(e)
					}
					if !slices.Contains(Codes(), Code(s)) {
						t.Error("missing rejection code", s)
					}
				} else if typ.Name == "QueryCode" {
					s, e := strconv.Unquote(lit.Value)
					if e != nil {
						t.Fatal(e)
					}
					if !slices.Contains(QueryCodes(), QueryCode(s)) {
						t.Error("missing query code", s)
					}
				}
			}
		}
	}
}
