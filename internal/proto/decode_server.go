package proto

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"time"

	json "encoding/json/v2"
	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/rules"
)

const commitTime = "2006-01-02T15:04:05.000Z"

// DecodeServer checks server frames while ignoring unknown object members.
func DecodeServer(frame []byte, c Config) (ServerMsg, error) {
	if e := framing(frame, c.MaxServerFrame); e != nil {
		return nil, e
	}
	f := &fault{}
	r := newObject(bytes.TrimSpace(frame), false, "", f)
	var m ServerMsg
	switch r.str("t") {
	case "welcome":
		m = Welcome{readDevice(r), ids.Seq(r.integer("head_seq", 0, ids.MaxSafe)), readAdvertised(r.child("limits"))}
	case "change":
		m = Live{readChange(r, c)}
	case "changes":
		v := Changes{Req: Req(r.integer("req", 1, 1<<32-1)), More: r.boolean("more"), Entries: []Change{}}
		for _, raw := range r.array("entries", c.PageEntries) {
			ch := readChange(newObject(raw, false, "entries.", f), c)
			if len(v.Entries) > 0 && ch.Seq != v.Entries[len(v.Entries)-1].Seq+1 {
				r.fail("entries", InvalidSubmission, Detail{})
			}
			v.Entries = append(v.Entries, ch)
		}
		if v.More && len(v.Entries) == 0 {
			r.fail("entries", InvalidSubmission, Detail{})
		}
		m = v
	case "files":
		v := Files{Req: Req(r.integer("req", 1, 1<<32-1)), HeadSeq: ids.Seq(r.integer("head_seq", 0, ids.MaxSafe)), More: r.boolean("more"), Files: []FileInfo{}}
		for _, raw := range r.array("files", c.PageEntries) {
			fi := readFile(newObject(raw, false, "files.", f))
			if len(v.Files) > 0 && fi.File.String() <= v.Files[len(v.Files)-1].File.String() {
				r.fail("files", InvalidSubmission, Detail{})
			}
			v.Files = append(v.Files, fi)
		}
		if v.More && len(v.Files) == 0 {
			r.fail("files", InvalidSubmission, Detail{})
		}
		m = v
	case "reject":
		id := readOpID(r)
		code := Code(r.str("code"))
		if !slices.Contains(Codes(), code) {
			r.fail("code", InvalidSubmission, Detail{})
		}
		m = Reject{id, code, readDetail(r)}
	case "error":
		req := Req(r.integer("req", 1, 1<<32-1))
		code := QueryCode(r.str("code"))
		if !slices.Contains(QueryCodes(), code) {
			r.fail("code", InvalidSubmission, Detail{})
		}
		m = Error{req, code, readDetail(r)}
	case "ping":
		m = Ping{uint32(r.integer("n", 0, 1<<32-1))}
	default:
		r.fail("t", InvalidSubmission, Detail{})
	}
	if f.code != "" {
		return nil, ErrInvalidMessage
	}
	return m, nil
}
func readDevice(r object) ids.DeviceID {
	v, e := ids.ParseDeviceID(r.str("device_id"))
	if e != nil {
		r.fail("device_id", InvalidSubmission, Detail{})
	}
	return v
}
func readOpID(r object) ids.ClientOpID {
	v, e := ids.ParseClientOpID(r.str("client_op_id"))
	if e != nil {
		r.fail("client_op_id", InvalidSubmission, Detail{})
	}
	return v
}
func readOwner(r object) rules.Owner {
	switch r.str("owner") {
	case "user":
		return rules.User
	case "canvas":
		return rules.Canvas
	default:
		r.fail("owner", InvalidSubmission, Detail{})
		return rules.User
	}
}
func readChange(r object, c Config) Change {
	v := Change{Seq: ids.Seq(r.integer("seq", 1, ids.MaxSafe)), Device: readDevice(r), ID: readOpID(r), Members: []Member{}}
	s := r.str("at")
	at, e := time.Parse(commitTime, s)
	if e != nil || at.Format(commitTime) != s {
		r.fail("at", InvalidSubmission, Detail{})
	}
	v.At = at
	switch r.str("actor") {
	case "device":
		v.Actor = rules.Device
	case "bridge":
		v.Actor = rules.Bridge
	case "admin":
		v.Actor = rules.Admin
	default:
		r.fail("actor", InvalidSubmission, Detail{})
	}
	for _, raw := range r.array("members", 2) {
		v.Members = append(v.Members, readMember(newObject(raw, false, "members.", r.err), c))
	}
	if len(v.Members) == 0 {
		r.fail("members", InvalidSubmission, Detail{})
	}
	if len(v.Members) == 2 && (v.Members[0].Kind != KindRename || v.Members[1].Kind != KindRename || v.Members[0].File == v.Members[1].File) {
		r.fail("members", InvalidSubmission, Detail{})
	}
	return v
}
func readMember(r object, c Config) Member {
	v := Member{File: r.file("file_id"), Version: ids.Version(r.integer("version", 1, ids.MaxSafe)), Kind: Kind(r.str("kind"))}
	class := classify.Class(0)
	switch v.Kind {
	case KindCreate:
		v.Path = r.path("path")
		v.Class = r.class("class")
		v.Owner = readOwner(r)
		class = v.Class
		if r.has("origin") {
			o := r.child("origin")
			v.Origin = &Origin{o.file("copy_of"), readCopyReason(o, true)}
		}
	case KindRename:
		v.Path = r.path("path")
	case KindDelete:
	case KindTextEdit:
		class = classify.Text
	case KindAnnotEdit:
		class = classify.Sidecar
	case KindPutBlob:
		class = classify.Opaque
	default:
		r.fail("kind", InvalidSubmission, Detail{})
	}
	if v.Kind != KindCreate {
		for _, key := range []string{"class", "owner", "origin"} {
			if r.has(key) {
				r.fail(key, InvalidSubmission, Detail{})
			}
		}
		if v.Kind != KindRename && r.has("path") {
			r.fail("path", InvalidSubmission, Detail{})
		}
	}
	count := 0
	for _, key := range []string{"op", "ops", "blob", "omitted"} {
		if r.has(key) {
			count++
		}
	}
	if class == 0 {
		if count != 0 || r.has("len") || r.has("hash") {
			r.fail("payload", InvalidSubmission, Detail{})
		}
		return v
	}
	if count != 1 {
		r.fail("payload", InvalidSubmission, Detail{})
	}
	switch class {
	case classify.Text:
		n := int(r.integer("len", 0, ids.MaxSafe))
		v.Len = &n
		if r.has("hash") {
			r.fail("hash", InvalidSubmission, Detail{})
		}
	case classify.Sidecar:
		h := r.hash("hash")
		v.Hash = &h
		if r.has("len") {
			r.fail("len", InvalidSubmission, Detail{})
		}
	case classify.Opaque:
		if r.has("hash") || r.has("len") {
			r.fail("payload", InvalidSubmission, Detail{})
		}
	}
	if r.has("omitted") {
		if !r.boolean("omitted") || class == classify.Opaque {
			r.fail("omitted", InvalidSubmission, Detail{})
		}
		v.Payload = Omitted{}
		return v
	}
	switch class {
	case classify.Text:
		op, e := c.Text.Parse(r.raw("op"))
		opFault(r, "op", e, int64(c.Text.MaxJSONBytes))
		if v.Len != nil && op.TargetLen() != *v.Len {
			r.fail("len", InvalidSubmission, Detail{})
		}
		v.Payload = TextPayload{op}
	case classify.Sidecar:
		ops, e := annot.ParseCommitted([]byte(r.str("ops")), c.Annot)
		opFault(r, "ops", e, int64(c.Annot.MaxStateBytes))
		v.Payload = AnnotPayload{ops}
	case classify.Opaque:
		v.Payload = BlobPayload{r.child("blob").blob()}
	}
	return v
}
func readFile(r object) FileInfo {
	v := FileInfo{File: r.file("file_id"), Path: r.path("path"), Class: r.class("class"), Owner: readOwner(r), Version: ids.Version(r.integer("version", 1, ids.MaxSafe)), Size: r.integer("size", 0, ids.MaxSafe)}
	count := 0
	for _, k := range []string{"blob", "len", "hash"} {
		if r.has(k) {
			count++
		}
	}
	if count != 1 {
		r.fail("payload", InvalidSubmission, Detail{})
	}
	switch v.Class {
	case classify.Text:
		n := int(r.integer("len", 0, ids.MaxSafe))
		v.Len = &n
	case classify.Sidecar:
		h := r.hash("hash")
		v.Hash = &h
	case classify.Opaque:
		b := r.child("blob").blob()
		v.Blob = &b
		if b.Size != v.Size {
			r.fail("size", InvalidSubmission, Detail{})
		}
	}
	return v
}
func readDetail(parent object) Detail {
	if !parent.has("detail") {
		return Detail{}
	}
	r := parent.child("detail")
	d := Detail{}
	if r.has("field") {
		d.Field = r.str("field")
	}
	if r.has("reason") {
		d.Reason = r.str("reason")
	}
	if r.has("component") {
		n := int(r.integer("component", -1, ids.MaxSafe))
		d.Component = &n
	}
	if r.has("class") {
		d.Class = r.class("class")
	}
	if r.has("demotion") {
		s := r.str("demotion")
		found := false
		for _, v := range []classify.Demotion{classify.Kept, classify.TooLarge, classify.InvalidUTF8, classify.ContainsNUL, classify.NotFreedraw} {
			if s == v.String() {
				d.Demotion = v
				found = true
			}
		}
		if !found {
			r.fail("demotion", InvalidSubmission, Detail{})
		}
	}
	if r.has("limit") {
		d.Limit = r.integer("limit", 0, ids.MaxSafe)
	}
	if r.has("scope") {
		d.Scope = r.str("scope")
		if !slices.Contains([]string{"rate", "file", "device", "mailbox"}, d.Scope) {
			r.fail("scope", InvalidSubmission, Detail{})
		}
	}
	if r.has("retry_after_ms") {
		d.RetryAfterMS = r.integer("retry_after_ms", 0, ids.MaxSafe)
	}
	return d
}
func readAdvertised(r object) Advertised {
	positive := func(o object, k string) int { return int(o.integer(k, 1, ids.MaxSafe)) }
	v := Advertised{MaxFrame: positive(r, "max_frame"), MaxServerFrame: positive(r, "max_server_frame"), SplitBytes: positive(r, "split_bytes"), PageEntries: positive(r, "page_entries"), InflightPerDevice: positive(r, "inflight_per_device")}
	rate := r.child("rate")
	v.Rate = Rate{positive(rate, "per_sec"), positive(rate, "burst")}
	cl := r.child("classify")
	v.Classify.MaxText = positive(cl, "max_text")
	v.Classify.MaxSidecar = positive(cl, "max_sidecar")
	v.Classify.TextExts = []string{}
	seen := map[string]bool{}
	for _, raw := range cl.array("text_exts", 1024) {
		var s string
		if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil || s == "" || strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789") != "" || seen[s] {
			cl.fail("text_exts", InvalidSubmission, Detail{})
		}
		seen[s] = true
		v.Classify.TextExts = append(v.Classify.TextExts, s)
	}
	g := r.child("guard")
	v.Guard = Guard{WindowMS: g.integer("window_ms", 1, ids.MaxSafe), MaxCount: positive(g, "max_count"), MinForRatio: positive(g, "min_for_ratio"), Buckets: positive(g, "buckets")}
	raw := g.raw("ratio")
	if json.Unmarshal(raw, &v.Guard.Ratio) != nil || v.Guard.Ratio <= 0 || v.Guard.Ratio > 1 || math.IsNaN(v.Guard.Ratio) {
		g.fail("ratio", InvalidSubmission, Detail{})
	}
	if v.SplitBytes > v.MaxFrame || v.SplitBytes > (v.MaxServerFrame/2) || v.Guard.MinForRatio > v.Guard.MaxCount || v.Guard.Buckets > 10000 || v.Guard.WindowMS < int64(v.Guard.Buckets) {
		r.fail("limits", InvalidSubmission, Detail{})
	}
	return v
}
