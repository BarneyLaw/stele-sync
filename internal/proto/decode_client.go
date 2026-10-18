package proto

import (
	"bytes"
	"errors"

	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/rules"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
)

// DecodeClient validates an entire frame before trusting any correlation value.
// Config must have passed Validate. Body failures never include raw input text.
func DecodeClient(frame []byte, c Config) (ClientMsg, error) {
	if e := framing(frame, c.MaxFrame); e != nil {
		return nil, e
	}
	f := &fault{}
	r := newObject(bytes.TrimSpace(frame), true, "", f)
	typ := r.str("t")
	if f.code != "" {
		return nil, &CloseError{CloseProtocol, "protocol"}
	}
	var m ClientMsg
	switch typ {
	case "submit":
		id, e := ids.ParseClientOpID(r.str("client_op_id"))
		if e != nil || f.code != "" {
			return nil, &CloseError{CloseProtocol, "protocol"}
		}
		body := readBody(r, c)
		if f.code != "" {
			return nil, &RejectError{id, f.code, f.detail}
		}
		return Submit{id, body}, nil
	case "pull_changes", "list_files":
		req := Req(r.integer("req", 1, 1<<32-1))
		if f.code != "" {
			return nil, &CloseError{CloseProtocol, "protocol"}
		}
		if typ == "pull_changes" {
			r.only("t", "req", "after_seq")
			m = PullChanges{req, ids.Seq(r.integer("after_seq", 0, ids.MaxSafe))}
		} else {
			r.only("t", "req", "after_file_id")
			var after *ids.FileID
			if r.has("after_file_id") {
				id := r.file("after_file_id")
				after = &id
			}
			m = ListFiles{req, after}
		}
		if f.code != "" {
			return nil, &QueryError{req, InvalidRequest, f.detail}
		}
	case "hello":
		r.only("t", "token")
		if f.code != "" {
			return nil, &CloseError{CloseProtocol, "protocol"}
		}
		token, e := ParseToken(r.str("token"))
		if e != nil || f.code != "" {
			return nil, &CloseError{CloseAuth, "bad_token"}
		}
		m = Hello{token}
	case "pong":
		r.only("t", "n")
		m = Pong{uint32(r.integer("n", 0, 1<<32-1))}
	default:
		return nil, &CloseError{CloseProtocol, "protocol"}
	}
	if f.code != "" {
		return nil, &CloseError{CloseProtocol, "protocol"}
	}
	return m, nil
}
func readBody(r object, c Config) Body {
	kind := Kind(r.str("kind"))
	only := func(names ...string) { r.only(append([]string{"t", "client_op_id", "kind"}, names...)...) }
	switch kind {
	case KindTextEdit:
		only("file_id", "base_version", "op")
		file := r.file("file_id")
		base := r.version()
		op, e := c.Text.Parse(r.raw("op"))
		opFault(r, "op", e, int64(c.Text.MaxJSONBytes))
		return TextEdit{file, base, op}
	case KindAnnotEdit:
		only("file_id", "base_version", "ops")
		file := r.file("file_id")
		base := r.version()
		raw := r.str("ops")
		ops, e := annot.ParseOps([]byte(raw), c.Annot)
		opFault(r, "ops", e, int64(c.Annot.MaxOpBytes))
		return AnnotEdit{file, base, ops}
	case KindAnnotReplace:
		only("file_id", "base_version", "content")
		return AnnotReplace{r.file("file_id"), r.version(), readContent(r.child("content"), classify.Sidecar, c)}
	case KindPutBlob:
		only("file_id", "base_version", "blob")
		return PutBlob{r.file("file_id"), r.version(), r.child("blob").blob()}
	case KindCreate:
		only("file_id", "path", "class", "content", "on_taken", "copy_of")
		v := Create{File: r.file("file_id"), Path: r.path("path"), Class: r.class("class"), OnTaken: readOnTaken(r)}
		v.Content = readContent(r.child("content"), v.Class, c)
		if r.has("copy_of") {
			b := r.child("copy_of")
			b.only("file_id", "reason")
			v.CopyOf = &CopyOf{b.file("file_id"), readCopyReason(b, false)}
		}
		return v
	case KindRename:
		only("file_id", "path", "on_taken")
		return Rename{r.file("file_id"), r.path("path"), readOnTaken(r)}
	case KindRenameGroup:
		only("pdf", "sidecar", "on_taken")
		read := func(k string) Target {
			v := r.child(k)
			v.only("file_id", "path")
			return Target{v.file("file_id"), v.path("path")}
		}
		v := RenameGroup{read("pdf"), read("sidecar"), readOnTaken(r)}
		if v.PDF.File == v.Sidecar.File {
			r.fail("sidecar.file_id", InvalidSubmission, Detail{})
		}
		return v
	case KindDelete:
		only("file_id", "base_version", "confirmed")
		v := Delete{File: r.file("file_id"), Base: r.version()}
		if r.has("confirmed") {
			v.Confirmed = r.boolean("confirmed")
		}
		return v
	default:
		r.fail("kind", InvalidSubmission, Detail{})
		return nil
	}
}
func readOnTaken(r object) rules.OnTaken {
	if !r.has("on_taken") {
		return rules.Number
	}
	switch r.str("on_taken") {
	case "number":
		return rules.Number
	case "reject":
		return rules.Reject
	default:
		r.fail("on_taken", InvalidSubmission, Detail{})
		return rules.Number
	}
}
func readCopyReason(r object, server bool) CopyReason {
	v := CopyReason(r.str("reason"))
	switch v {
	case CopyRebase, CopyDeleted, CopyClassChange, CopyFirstSync:
		return v
	case CopyStalePut, CopyStaleReplace:
		if server {
			return v
		}
	}
	r.fail("reason", InvalidSubmission, Detail{})
	return v
}
func readContent(r object, class classify.Class, c Config) Content {
	r.only("text", "sidecar", "blob")
	if len(r.fields) != 1 {
		r.fail("", InvalidSubmission, Detail{})
		return nil
	}
	if r.has("blob") {
		return BlobContent{r.child("blob").blob()}
	}
	key := "text"
	if class == classify.Sidecar {
		key = "sidecar"
	} else if class != classify.Text {
		r.fail("", InvalidSubmission, Detail{})
		return nil
	}
	raw := r.str(key)
	if len(raw) > c.MaxInlineContent {
		r.fail(key, TooLarge, Detail{Limit: int64(c.MaxInlineContent)})
	}
	if class == classify.Text {
		return InlineText{raw}
	}
	return InlineSidecar{[]byte(raw)}
}
func opFault(r object, field string, e error, limit int64) {
	if e == nil {
		return
	}
	if errors.Is(e, textop.ErrLimit) || errors.Is(e, annot.ErrLimit) {
		r.fail(field, TooLarge, Detail{Limit: limit})
		return
	}
	reason := "ErrInvalidJSON"
	for _, v := range []struct {
		e    error
		name string
	}{
		{textop.ErrInvalidUTF8, "ErrInvalidUTF8"}, {textop.ErrInvalidJSON, "ErrInvalidJSON"}, {textop.ErrInvalidCount, "ErrInvalidCount"}, {textop.ErrNonCanonical, "ErrNonCanonical"}, {textop.ErrLengthMismatch, "ErrLengthMismatch"}, {textop.ErrSplitsSurrogate, "ErrSplitsSurrogate"}, {textop.ErrInvalidUnicode, "ErrInvalidUnicode"}, {textop.ErrInvalidLimits, "ErrInvalidLimits"},
		{annot.ErrInvalidJSON, "ErrInvalidJSON"}, {annot.ErrNotSidecar, "ErrNotSidecar"}, {annot.ErrDuplicateKey, "ErrDuplicateKey"}, {annot.ErrDuplicateID, "ErrDuplicateID"}, {annot.ErrMissingID, "ErrMissingID"}, {annot.ErrStructuralKey, "ErrStructuralKey"}, {annot.ErrWholeDocument, "ErrWholeDocument"}, {annot.ErrNonCanonical, "ErrNonCanonical"}, {annot.ErrInvalidLimits, "ErrInvalidLimits"}, {annot.ErrInvalidState, "ErrInvalidState"},
	} {
		if errors.Is(e, v.e) {
			reason = v.name
			break
		}
	}
	r.fail(field, InvalidOp, Detail{Reason: reason})
}
