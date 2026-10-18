package proto

import (
	"encoding/hex"
	"encoding/json/jsontext"
	"fmt"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
)

// EncodeServer validates locally constructed messages and bounds complete frames.
func EncodeServer(m ServerMsg, c Config) ([]byte, error) {
	w := wire{}
	var e error
	switch v := value(m).(type) {
	case Welcome:
		w = wire{"t": "welcome", "device_id": v.Device, "head_seq": v.HeadSeq, "limits": advertisedWire(v.Limits)}
	case Live:
		w, e = changeWire(v.Change)
		w["t"] = "change"
	case Changes:
		entries := make([]wire, 0, len(v.Entries))
		for _, ch := range v.Entries {
			var cw wire
			cw, e = changeWire(ch)
			if e != nil {
				return nil, e
			}
			entries = append(entries, cw)
		}
		w = wire{"t": "changes", "req": v.Req, "entries": entries, "more": v.More}
	case Files:
		files := make([]wire, 0, len(v.Files))
		for _, fi := range v.Files {
			files = append(files, fileWire(fi))
		}
		w = wire{"t": "files", "req": v.Req, "head_seq": v.HeadSeq, "files": files, "more": v.More}
	case Reject:
		w = wire{"t": "reject", "client_op_id": v.ID, "code": v.Code}
		if d := detailWire(v.Detail); len(d) > 0 {
			w["detail"] = d
		}
	case Error:
		w = wire{"t": "error", "req": v.Req, "code": v.Code}
		if d := detailWire(v.Detail); len(d) > 0 {
			w["detail"] = d
		}
	case Ping:
		w = wire{"t": "ping", "n": v.N}
	default:
		return nil, ErrInvalidMessage
	}
	if e != nil {
		return nil, e
	}
	raw, e := marshalFrame(w, c.MaxServerFrame)
	if e != nil {
		return nil, e
	}
	if _, e = DecodeServer(raw, c); e != nil {
		return nil, e
	}
	return raw, nil
}
func changeWire(ch Change) (wire, error) {
	w := wire{"seq": ch.Seq, "at": ch.At.Format(commitTime), "actor": actorWire(ch.Actor), "device_id": ch.Device, "client_op_id": ch.ID}
	// Reject accidental truncation or timezone rewriting of committed timestamps.
	if ch.At.Location().String() != "UTC" || ch.At.Nanosecond()%1000000 != 0 {
		return w, ErrInvalidMessage
	}
	members := make([]wire, 0, len(ch.Members))
	for _, m := range ch.Members {
		mw, e := memberWire(m)
		if e != nil {
			return w, e
		}
		members = append(members, mw)
	}
	w["members"] = members
	return w, nil
}
func memberWire(m Member) (wire, error) {
	w := wire{"file_id": m.File, "version": m.Version, "kind": m.Kind}
	if !m.Path.IsZero() {
		w["path"] = m.Path.String()
	}
	if m.Class != 0 {
		w["class"] = classWire(m.Class)
	}
	if m.Kind == KindCreate {
		w["owner"] = ownerWire(m.Owner)
	}
	if m.Origin != nil {
		w["origin"] = wire{"copy_of": m.Origin.CopyOf, "reason": m.Origin.Reason}
	}
	if m.Len != nil {
		w["len"] = *m.Len
	}
	if m.Hash != nil {
		w["hash"] = hex.EncodeToString(m.Hash[:])
	}
	p, e := payloadWire(m.Payload)
	if e != nil {
		return w, e
	}
	for k, v := range p {
		w[k] = v
	}
	return w, nil
}
func payloadWire(payload Payload) (wire, error) {
	switch v := value(payload).(type) {
	case nil:
		if payload != nil {
			return nil, ErrInvalidMessage
		}
		return wire{}, nil
	case TextPayload:
		b, e := v.Op.MarshalJSON()
		return wire{"op": jsontext.Value(b)}, e
	case AnnotPayload:
		b, e := v.Ops.MarshalJSON()
		return wire{"ops": string(b)}, e
	case BlobPayload:
		return wire{"blob": blobWire(v.Ref)}, nil
	case Omitted:
		return wire{"omitted": true}, nil
	default:
		return nil, ErrInvalidMessage
	}
}
func fileWire(v FileInfo) wire {
	w := wire{"file_id": v.File, "path": v.Path.String(), "class": classWire(v.Class), "owner": ownerWire(v.Owner), "version": v.Version, "size": v.Size}
	if v.Len != nil {
		w["len"] = *v.Len
	}
	if v.Hash != nil {
		w["hash"] = hex.EncodeToString(v.Hash[:])
	}
	if v.Blob != nil {
		w["blob"] = blobWire(*v.Blob)
	}
	return w
}
func detailWire(d Detail) wire {
	w := wire{}
	if d.Field != "" {
		w["field"] = d.Field
	}
	if d.Reason != "" {
		w["reason"] = d.Reason
	}
	if d.Component != nil {
		w["component"] = *d.Component
	}
	if d.Class != 0 {
		w["class"] = classWire(d.Class)
		w["demotion"] = d.Demotion.String()
	} else if d.Demotion != classify.Kept {
		w["demotion"] = d.Demotion.String()
	}
	if d.Limit != 0 {
		w["limit"] = d.Limit
	}
	if d.Scope != "" {
		w["scope"] = d.Scope
	}
	if d.RetryAfterMS != 0 {
		w["retry_after_ms"] = d.RetryAfterMS
	}
	return w
}
func advertisedWire(v Advertised) wire {
	return wire{"max_frame": v.MaxFrame, "max_server_frame": v.MaxServerFrame, "split_bytes": v.SplitBytes, "page_entries": v.PageEntries, "inflight_per_device": v.InflightPerDevice, "rate": wire{"per_sec": v.Rate.PerSec, "burst": v.Rate.Burst}, "classify": wire{"text_exts": v.Classify.TextExts, "max_text": v.Classify.MaxText, "max_sidecar": v.Classify.MaxSidecar}, "guard": wire{"window_ms": v.Guard.WindowMS, "max_count": v.Guard.MaxCount, "min_for_ratio": v.Guard.MinForRatio, "ratio": v.Guard.Ratio, "buckets": v.Guard.Buckets}}
}

// Inline measures encoded payload fields, including JSON quoting. Metadata and
// divergence checks survive omission; namespace and blob payloads never change.
func Inline(m Member, c Config) Member {
	switch value(m.Payload).(type) {
	case TextPayload, AnnotPayload:
		p, e := payloadWire(m.Payload)
		if e != nil {
			return m
		}
		if _, e = marshalFrame(p, c.MaxInline); e == ErrFrameTooLarge {
			m.Payload = Omitted{}
		}
	}
	return m
}

// Page builds a byte- and entry-bounded feed page in linear encoding work. It
// owns encoded entries so later caller mutations cannot change the byte budget.
type Page struct {
	req     Req
	c       Config
	entries []jsontext.Value
	bytes   int
	last    int64
	err     error
}

// NewPage starts a page. Invalid configuration or req is returned by Add/Encode.
func NewPage(req Req, c Config) *Page {
	p := &Page{req: req, c: c, entries: []jsontext.Value{}}
	if e := c.Validate(); e != nil {
		p.err = e
		return p
	}
	if req == 0 {
		p.err = ErrInvalidMessage
		return p
	}
	b, e := marshalFrame(wire{"t": "changes", "req": req, "entries": p.entries, "more": false}, c.MaxServerFrame)
	p.err = e
	p.bytes = len(b)
	return p
}

// Add appends a whole transaction or returns false without modifying the page.
// A transaction that cannot fit an empty page returns ErrFrameTooLarge.
func (p *Page) Add(ch Change) (bool, error) {
	if p.err != nil {
		return false, p.err
	}
	if len(p.entries) >= p.c.PageEntries {
		return false, nil
	}
	if len(p.entries) > 0 && int64(ch.Seq) != p.last+1 {
		return false, fmt.Errorf("%w: noncontiguous page", ErrInvalidMessage)
	}
	copyCh := ch
	copyCh.Members = append([]Member(nil), ch.Members...)
	for i, m := range copyCh.Members {
		copyCh.Members[i] = Inline(m, p.c)
	}
	if _, e := EncodeServer(Live{copyCh}, p.c); e != nil {
		return false, e
	}
	w, e := changeWire(copyCh)
	if e != nil {
		return false, e
	}
	raw, e := marshalFrame(w, p.c.MaxServerFrame)
	if e != nil {
		return false, e
	}
	size := p.bytes + len(raw)
	if len(p.entries) > 0 {
		size++
	}
	if size > p.c.MaxServerFrame {
		if len(p.entries) == 0 {
			return false, ErrFrameTooLarge
		}
		return false, nil
	}
	p.entries = append(p.entries, jsontext.Value(raw))
	p.bytes = size
	p.last = int64(ch.Seq)
	return true, nil
}

// Encode returns one bounded page. more=true requires at least one entry.
func (p *Page) Encode(more bool) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	if more && len(p.entries) == 0 {
		return nil, ErrInvalidMessage
	}
	return marshalFrame(wire{"t": "changes", "req": p.req, "entries": p.entries, "more": more}, p.c.MaxServerFrame)
}
