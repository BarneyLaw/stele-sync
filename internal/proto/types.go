package proto

import (
	"github.com/BarneyLaw/stele-sync/internal/core/annot"
	"github.com/BarneyLaw/stele-sync/internal/core/classify"
	"github.com/BarneyLaw/stele-sync/internal/core/ids"
	"github.com/BarneyLaw/stele-sync/internal/core/rules"
	"github.com/BarneyLaw/stele-sync/internal/core/textop"
	"github.com/BarneyLaw/stele-sync/internal/core/vpath"
	"time"
)

// Req correlates a query and answer; zero is invalid.
type Req uint32

// ClientMsg seals the client-to-server catalogue.
type ClientMsg interface{ clientMsg() }

// ServerMsg seals the server-to-client catalogue.
type ServerMsg interface{ serverMsg() }

// Body seals the submission variants.
type Body interface{ body() }

// Content seals complete-content transport variants.
type Content interface{ content() }

// Payload seals committed member payloads.
type Payload interface{ payload() }

// Hello authenticates after subprotocol negotiation.
type Hello struct{ Token Token }

// PullChanges requests the ordered feed after a persisted cursor.
type PullChanges struct {
	Req   Req
	After ids.Seq
}

// ListFiles requests a page in file-identifier order.
type ListFiles struct {
	Req   Req
	After *ids.FileID
}

// Submit identifies exactly one retryable transaction.
type Submit struct {
	ID   ids.ClientOpID
	Body Body
}

// Pong responds to the heartbeat nonce.
type Pong struct{ N uint32 }

// TextEdit submits a canonical text operation.
type TextEdit struct {
	File ids.FileID
	Base ids.Version
	Op   textop.Op
}

// AnnotEdit submits untrusted annotation operations.
type AnnotEdit struct {
	File ids.FileID
	Base ids.Version
	Ops  annot.Ops
}

// AnnotReplace submits complete sidecar content, whose base is checked by the engine.
type AnnotReplace struct {
	File    ids.FileID
	Base    ids.Version
	Content Content
}

// PutBlob submits an uploaded blob reference.
type PutBlob struct {
	File ids.FileID
	Base ids.Version
	Blob BlobRef
}

// Create claims an identity, path and content class.
type Create struct {
	File    ids.FileID
	Path    vpath.Path
	Class   classify.Class
	Content Content
	OnTaken rules.OnTaken
	CopyOf  *CopyOf
}

// Rename requests one namespace change.
type Rename struct {
	File    ids.FileID
	Path    vpath.Path
	OnTaken rules.OnTaken
}

// RenameGroup requests an atomic PDF and sidecar rename.
type RenameGroup struct {
	PDF, Sidecar Target
	OnTaken      rules.OnTaken
}

// Delete explicitly removes content at a known base version.
type Delete struct {
	File      ids.FileID
	Base      ids.Version
	Confirmed bool
}

// InlineText carries complete class A text.
type InlineText struct{ Text string }

// InlineSidecar carries unparsed FreeDraw file bytes for engine classification.
type InlineSidecar struct{ Raw []byte }

// BlobContent carries uploaded complete content for any class.
type BlobContent struct{ Ref BlobRef }

// BlobRef identifies immutable content by hash and byte size.
type BlobRef struct {
	SHA256 [32]byte
	Size   int64
}

// Target identifies a rename destination.
type Target struct {
	File ids.FileID
	Path vpath.Path
}

// CopyReason explains a requested or server-produced conflict copy.
type CopyReason string

const (
	// CopyRebase preserves edits outside the reconciliation window.
	CopyRebase CopyReason = "rebase"
	// CopyDeleted preserves edits to a deleted identity.
	CopyDeleted CopyReason = "deleted"
	// CopyClassChange preserves content whose class changed.
	CopyClassChange CopyReason = "class_change"
	// CopyFirstSync preserves a first-sync conflict.
	CopyFirstSync CopyReason = "first_sync"
	// CopyStalePut preserves an outdated whole-blob write.
	CopyStalePut CopyReason = "stale_put"
	// CopyStaleReplace preserves an outdated sidecar replacement.
	CopyStaleReplace CopyReason = "stale_replace"
)

// CopyOf requests a server-named conflict copy.
type CopyOf struct {
	File   ids.FileID
	Reason CopyReason
}

// Origin records why a committed conflict copy exists.
type Origin struct {
	CopyOf ids.FileID
	Reason CopyReason
}

// Kind identifies a submission or committed member variant.
type Kind string

const (
	// KindTextEdit is a text operation.
	KindTextEdit Kind = "text_edit"
	// KindAnnotEdit is an annotation operation set.
	KindAnnotEdit Kind = "annot_edit"
	// KindAnnotReplace is a whole sidecar submission.
	KindAnnotReplace Kind = "annot_replace"
	// KindPutBlob is a whole blob update.
	KindPutBlob Kind = "put_blob"
	// KindCreate creates an identity.
	KindCreate Kind = "create"
	// KindRename changes a path.
	KindRename Kind = "rename"
	// KindRenameGroup atomically renames two identities.
	KindRenameGroup Kind = "rename_group"
	// KindDelete tombstones an identity.
	KindDelete Kind = "delete"
)

// Welcome authenticates the device and publishes authoritative configuration.
type Welcome struct {
	Device  ids.DeviceID
	HeadSeq ids.Seq
	Limits  Advertised
}

// Changes returns a page of complete transaction records.
type Changes struct {
	Req     Req
	Entries []Change
	More    bool
}

// Live carries the same record used in catch-up, flattened on the wire.
type Live struct{ Change Change }

// Files returns a file listing with a resync cursor.
type Files struct {
	Req     Req
	HeadSeq ids.Seq
	Files   []FileInfo
	More    bool
}

// Reject frees a submission's in-flight slot with a stable error code.
type Reject struct {
	ID     ids.ClientOpID
	Code   Code
	Detail Detail
}

// Error answers a failed query.
type Error struct {
	Req    Req
	Code   QueryCode
	Detail Detail
}

// Ping asks for a heartbeat reply.
type Ping struct{ N uint32 }

// Change is one committed transaction, delivered identically live and in catch-up.
type Change struct {
	Seq     ids.Seq
	At      time.Time
	Actor   rules.Actor
	Device  ids.DeviceID
	ID      ids.ClientOpID
	Members []Member
}

// Member is one file change; two members are reserved for an atomic rename group.
type Member struct {
	File    ids.FileID
	Version ids.Version
	Kind    Kind
	Path    vpath.Path
	Class   classify.Class
	Owner   rules.Owner
	Origin  *Origin
	Payload Payload
	Len     *int
	Hash    *[32]byte
}

// TextPayload contains a transformed text operation.
type TextPayload struct{ Op textop.Op }

// AnnotPayload contains effective operations, including trusted replacements.
type AnnotPayload struct{ Ops annot.Ops }

// BlobPayload contains a small immutable blob reference.
type BlobPayload struct{ Ref BlobRef }

// Omitted directs the client to fetch state over HTTPS.
type Omitted struct{}

// FileInfo describes a live identity during full resync.
type FileInfo struct {
	File    ids.FileID
	Path    vpath.Path
	Class   classify.Class
	Owner   rules.Owner
	Version ids.Version
	Size    int64
	Blob    *BlobRef
	Len     *int
	Hash    *[32]byte
}

// Advertised is the configuration clients must adopt from welcome.
type Advertised struct {
	MaxFrame, MaxServerFrame, SplitBytes, PageEntries, InflightPerDevice int
	Rate                                                                 Rate
	Classify                                                             Classification
	Guard                                                                Guard
}

// Rate advertises the inbound token bucket.
type Rate struct{ PerSec, Burst int }

// Classification advertises complete-content classification policy.
type Classification struct {
	TextExts            []string
	MaxText, MaxSidecar int
}

// Guard advertises the bulk-delete guard policy.
type Guard struct {
	WindowMS              int64
	MaxCount, MinForRatio int
	Ratio                 float64
	Buckets               int
}

func (Hello) clientMsg()       {}
func (PullChanges) clientMsg() {}
func (ListFiles) clientMsg()   {}
func (Submit) clientMsg()      {}
func (Pong) clientMsg()        {}
func (Welcome) serverMsg()     {}
func (Changes) serverMsg()     {}
func (Live) serverMsg()        {}
func (Files) serverMsg()       {}
func (Reject) serverMsg()      {}
func (Error) serverMsg()       {}
func (Ping) serverMsg()        {}
func (TextEdit) body()         {}
func (AnnotEdit) body()        {}
func (AnnotReplace) body()     {}
func (PutBlob) body()          {}
func (Create) body()           {}
func (Rename) body()           {}
func (RenameGroup) body()      {}
func (Delete) body()           {}
func (InlineText) content()    {}
func (InlineSidecar) content() {}
func (BlobContent) content()   {}
func (TextPayload) payload()   {}
func (AnnotPayload) payload()  {}
func (BlobPayload) payload()   {}
func (Omitted) payload()       {}
