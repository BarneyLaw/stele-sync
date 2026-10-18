package proto

import (
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"reflect"

	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/rules"
)

type wire map[string]any

// Dereference sealed message pointers too; reject nil pointers without panics.
func value(v any) any {
	if v == nil {
		return nil
	}
	r := reflect.ValueOf(v)
	if r.Kind() == reflect.Pointer {
		if r.IsNil() {
			return nil
		}
		return r.Elem().Interface()
	}
	return v
}
func blobWire(b BlobRef) wire { return wire{"sha256": hex.EncodeToString(b.SHA256[:]), "size": b.Size} }
func contentWire(v Content) (wire, error) {
	switch c := value(v).(type) {
	case InlineText:
		return wire{"text": c.Text}, nil
	case InlineSidecar:
		return wire{"sidecar": string(c.Raw)}, nil
	case BlobContent:
		return wire{"blob": blobWire(c.Ref)}, nil
	default:
		return nil, ErrInvalidMessage
	}
}
func takenWire(v rules.OnTaken) string {
	switch v {
	case rules.Number:
		return "number"
	case rules.Reject:
		return "reject"
	default:
		return "invalid"
	}
}
func classWire(c classify.Class) string { return string([]byte{byte(c)}) }
func ownerWire(v rules.Owner) string {
	switch v {
	case rules.User:
		return "user"
	case rules.Canvas:
		return "canvas"
	default:
		return "invalid"
	}
}
func actorWire(v rules.Actor) string {
	switch v {
	case rules.Device:
		return "device"
	case rules.Bridge:
		return "bridge"
	case rules.Admin:
		return "admin"
	default:
		return "invalid"
	}
}

// EncodeClient encodes and validates a reference-client submission before send.
// Complete encoded size is authoritative even when string quoting expands data.
func EncodeClient(m ClientMsg, c Config) ([]byte, error) {
	var w wire
	var e error
	switch v := value(m).(type) {
	case Hello:
		w = wire{"t": "hello", "token": v.Token.secret}
	case PullChanges:
		w = wire{"t": "pull_changes", "req": v.Req, "after_seq": v.After}
	case ListFiles:
		w = wire{"t": "list_files", "req": v.Req}
		if v.After != nil {
			w["after_file_id"] = v.After
		}
	case Pong:
		w = wire{"t": "pong", "n": v.N}
	case Submit:
		w, e = bodyWire(v.Body, c)
		if e != nil {
			return nil, e
		}
		w["t"] = "submit"
		w["client_op_id"] = v.ID
	default:
		return nil, ErrInvalidMessage
	}
	raw, e := marshalFrame(w, c.MaxFrame)
	if e != nil {
		return nil, e
	}
	if _, e = DecodeClient(raw, c); e != nil {
		return nil, e
	}
	return raw, nil
}
func bodyWire(body Body, c Config) (wire, error) {
	var w wire
	var e error
	content := func(v Content) { w["content"], e = contentWire(v) }
	switch v := value(body).(type) {
	case TextEdit:
		w = wire{"kind": KindTextEdit, "file_id": v.File, "base_version": v.Base}
		var raw []byte
		raw, e = v.Op.MarshalJSON()
		w["op"] = jsontext.Value(raw)
	case AnnotEdit:
		w = wire{"kind": KindAnnotEdit, "file_id": v.File, "base_version": v.Base}
		var raw []byte
		raw, e = c.Annot.MarshalOps(v.Ops)
		w["ops"] = string(raw)
	case AnnotReplace:
		w = wire{"kind": KindAnnotReplace, "file_id": v.File, "base_version": v.Base}
		content(v.Content)
	case PutBlob:
		w = wire{"kind": KindPutBlob, "file_id": v.File, "base_version": v.Base, "blob": blobWire(v.Blob)}
	case Create:
		w = wire{"kind": KindCreate, "file_id": v.File, "path": v.Path.String(), "class": classWire(v.Class)}
		if v.OnTaken != rules.Number {
			w["on_taken"] = takenWire(v.OnTaken)
		}
		content(v.Content)
		if v.CopyOf != nil {
			w["copy_of"] = wire{"file_id": v.CopyOf.File, "reason": v.CopyOf.Reason}
		}
	case Rename:
		w = wire{"kind": KindRename, "file_id": v.File, "path": v.Path.String()}
		if v.OnTaken != rules.Number {
			w["on_taken"] = takenWire(v.OnTaken)
		}
	case RenameGroup:
		w = wire{"kind": KindRenameGroup, "pdf": wire{"file_id": v.PDF.File, "path": v.PDF.Path.String()}, "sidecar": wire{"file_id": v.Sidecar.File, "path": v.Sidecar.Path.String()}}
		if v.OnTaken != rules.Number {
			w["on_taken"] = takenWire(v.OnTaken)
		}
	case Delete:
		w = wire{"kind": KindDelete, "file_id": v.File, "base_version": v.Base}
		if v.Confirmed {
			w["confirmed"] = true
		}
	default:
		return nil, ErrInvalidMessage
	}
	return w, e
}
func marshalFrame(v any, maximum int) ([]byte, error) {
	raw, e := json.Marshal(v, json.Deterministic(true))
	if e != nil {
		return nil, fmt.Errorf("%w: cannot encode wire value", ErrInvalidMessage)
	}
	if len(raw) > maximum {
		return nil, ErrFrameTooLarge
	}
	return raw, nil
}
