# M4 proposal: proto

2026-10-18 · Status: accepted (D1 to D9 agreed 2026-10-18; D10 to D12 added while writing, open for objection)

M4 is the wire contract between the stele-sync server and its clients: a strict decoder for client frames, an encoder for server frames, the reverse pair for the Go reference client, a closed catalogue of codes, and fixtures both languages run. One committed transaction becomes exactly one change record, delivered the same way live and during catch-up, so a crash between commit and acknowledgement takes the ordinary code path. Large payloads never block the WebSocket: clients split them, and anything still too large is sent by reference and fetched over HTTPS.

This document replaces the M4 section of the Implementation Guide. It follows the M3 proposal's layout: background and findings, decisions, step 0, then Goal, Interface, Semantics, Implementation, Proof and Exit criteria, followed by wiring notes for later milestones.

## 1. Background: what the repository hands M4

Read at `21f30d8` (the M3 merge).

- **`internal/proto` is a one-line `doc.go`.** Depguard lets it import only the standard library and `internal/core/`. The adapters (`storage`, `blob`, `compact`, `gc`) may not import `proto`, and `go.mod` has no UUID library.
- **The payload parsers exist.** `textop.Limits.Parse` takes ot.js JSON up to 1 MiB and 65,536 components and rejects lone surrogate escapes. `annot.ParseOps` takes element operations up to 1 MiB and rejects any `replace` field. `annot.Parse` takes a FreeDraw file up to 16 MiB. `vpath.Parse` returns a `*PathError` with a typed `Reason` and a zero-based component index.
- **M1 to M3 left wire requirements.** Reject codes `whole_document_required`, `invalid_path {reason, component}` and `class_mismatch {class, demotion}`. An `AnnotReplace` body. `on_taken` on create and rename, `copy_of {file_id, reason}` on create, `confirmed` on delete, and `rename_group`. The final path and class travel with the change.
- **The repository pins Go 1.27.2,** where `encoding/json/v2` and `encoding/json/jsontext` ship as standard packages. v2 rejects invalid UTF-8 and duplicate object names by default and matches member names case-sensitively. The v1 API now runs on the v2 engine but keeps v1 semantics.
- **The project name is final: stele.** Phase 1 already ships as `stele-pull` (plugin id, `cmd/stele-pull`, `cmd/stele-pull-worker`). Phase 2 still carries `obsync-server`, `obsyncctl` and the subprotocol `obsync.v1` in the specification. M4 freezes the subprotocol string, so the rename lands here (D9).

## 2. Findings

F1 to F6 came out of the debate. F7 to F12 came up while writing this proposal.

**F1. v1 `encoding/json` would let Go and TypeScript read different messages.** v1 matches names case-insensitively, so `"On_Taken":"reject"` is read by Go and ignored by JavaScript. It accepts duplicate names, last one winning. It replaces invalid UTF-8 and lone surrogate escapes with U+FFFD, so a path `"a\ud800.md"` arrives as `a�.md`, passes `vpath.Parse`, and names the server's file differently from the client's. M1 and M2 already guard their own payload bytes; every other string in a frame is unguarded.

**F2. A crash between commit and acknowledgement duplicates content.** The server commits a text insert "hi"; the client crashes before the ack arrives. On reconnect, catch-up delivers that change before the client has resent anything. If the client cannot recognise the change as its own, it treats it as remote, transforms its still-pending "hi" against it, and the document reads "hihi". The idempotent retry never runs. The specification's `changes` table has no `client_op_id`, and its client regenerates pending edits by diffing against the shadow, which mints a new id and reopens the hole even with the column.

**F3. The size limits do not compose.** `textop` and `annot` each accept 1 MiB payloads, equal to the 1 MiB frame, so a maximum payload cannot fit once the envelope is added. A transformed text operation can be larger than its input because concurrent inserts split its deletes. An `AnnotReplace` can be 16 MiB and a text head 8 MiB. The specification routes large data around the WebSocket only from client to server.

**F4. Queries have no correlation id.** Two concurrent `fetch_head` requests cannot be matched to their answers.

**F5. Identifier types cannot live in `proto`.** `storage/pg` needs `FileID` and `Version` but may not import `proto`.

**F6. The subprotocol still says `obsync.v1`.**

**F7. Committed annotation operations cannot be read back.** `annot.Ops.MarshalJSON` writes `"replace":true` for trusted replacement output, and `annot.ParseOps` rejects any `replace` field. The server must send effective operations to clients and read them from its own log, so a trusted parser for committed operations is missing.

**F8. Five rejections have no wire code.** M3's `ClassChange`, `InvalidBase` and `GroupRequired` decisions have none. Neither does an edit naming a file id the server has never seen, nor a create reusing a live file id under a new `client_op_id`.

**F9. JavaScript changes the spelling of nested JSON.** `JSON.parse` followed by `JSON.stringify` turns `1.0` into `1` and rewrites escapes. M2's contract preserves raw number spelling, key order and escapes inside sidecar values and hashes the exact bytes. If a sidecar or annotation operations travel as nested JSON objects, a TypeScript client cannot forward them byte for byte without a hand-written raw-preserving serializer. Text operations are safe: they hold only integers and strings compared by value (ADR 002).

**F10. Configuration skew produces rejections, not errors.** The server is the classification authority (ADR 019). A client configured with a different text-extension list or size limit gets `class_mismatch` on every affected create. The client-side bulk-delete pre-check (ADR 020) has the same exposure to guard configuration.

**F11. Some catalogue entries duplicate others or vanish offline.** `hello.last_seq` repeats `pull_changes.after_seq`. `hello.device_id` repeats what the token identifies. `notice` frames are not stored, so a phone offline for a day never learns that a Canvas file was removed, although the removal itself is in the change feed.

**F12. A cursor can be ahead of the server.** Restoring Postgres from the WAL archive can lose up to five minutes (RPO). A device that saw those changes reconnects with `last_seq` greater than the restored `head_seq`. The specification handles only a cursor older than retention.

## 3. Decisions

| # | Decision | ADR |
| --- | --- | --- |
| D1 | **Strict client frames, lenient server frames.** Client frames are decoded with `jsontext` and v2 defaults: no duplicate names, no invalid UTF-8, case-sensitive names, no lone surrogates in any string, and unknown members rejected. Clients ignore unknown members in server frames. The server always ships first, so it never legitimately receives a member it does not know. | 023 |
| D2 | **Three error tiers.** Untrustworthy frames close the session with 4400. A well-formed submission with an invalid body gets `reject {client_op_id, code, detail}`. An invalid query gets `error {req, code}`. `invalid_op` narrows to operation payloads; structural faults get `invalid_submission`. | 023 |
| D3 | **One change record per committed transaction.** Each carries `seq`, `actor`, `device_id`, `client_op_id` and one or two members, and is delivered identically live (`change`) and during catch-up (`changes`). There is no separate `ack` or `remote`. A client recognises its own changes by device id and matches them to persisted in-flight submissions by `client_op_id`. | 024 |
| D4 | **Bounded frames; large data by reference.** Client frames 1 MiB with nested payload limits strictly below them. Server frames 2 MiB. Clients split edits to an advertised size. Create and replace content is inline or a blob reference. A member whose payload exceeds the inline threshold is sent with `omitted: true` and the client fetches state over HTTPS. Pages are bounded by bytes as well as entries. | 025 |
| D5 | **One body per submit kind; correlated queries.** No shared `file_id`/`base_version` header. `confirmed` exists only on delete. Queries carry `req`, echoed in the answer. Go has separate sealed `ClientMsg` and `ServerMsg` interfaces and implements all four codecs; TypeScript implements client encoding and server decoding. | 023 |
| D6 | **`internal/core/ids`.** `FileID`, `DeviceID` and `ClientOpID` are 16-byte values with canonical lowercase text, checked as UUIDv7 with the RFC 9562 variant. `Version` and `Seq` are int64 capped at 2^53 − 1. | 026 |
| D7 | **Fixtures split by direction.** Go's client decoder is authoritative for client frames; TypeScript proves its encoder produces the same values. Go's server encoder produces golden frames TypeScript must accept, plus a hand-written invalid set TypeScript must reject. | 023 |
| D8 | **Per-change divergence checks.** Class A members carry the resulting length in UTF-16 units; Class B members carry `CanonicalHash`. No per-change hash for Class A. | 024 |
| D9 | **Versioning through the subprotocol; handshake cleanup; the stele name.** Negotiate `stele.v1` through `Sec-WebSocket-Protocol`; an unsupported version closes with 4426. `hello` carries only the token. `Token` redacts itself in every formatting path. Phase 2 identifiers not yet deployed are renamed to stele. | 027 |
| D10 | **Byte-exact payloads travel as JSON strings.** Annotation operations and inline sidecar content are strings containing JSON text. Text operations stay nested arrays. | 025 |
| D11 | **Notices come from the feed.** There is no `notice` message. Canvas changes and removals are changes with `actor: "bridge"`; conflict copies are creates with an `origin`. Revocation is close 4401 with reason `revoked`. | 024 |
| D12 | **Heads only over HTTPS; the server advertises shared configuration.** The WebSocket loses `fetch_head`. `GET /v1/files/{file_id}/state` serves A and B heads. `welcome` advertises the classification and guard configuration clients must use. A cursor ahead of `head_seq` gets `resync_required`. | 025 |

## 4. Step 0: measurements before code (about half a day)

Run on Go 1.27.2 and record the results in `docs/m4-verification.md`.

1. **Lone surrogates.** Decode JSON strings containing `\ud800`, `\udc00`, `\ud800A`, `\udc00\ud800` and a valid pair through `jsontext.Decoder` with default options. If any lone surrogate is accepted or replaced rather than rejected, `proto` runs a surrogate scan over every string token, extracted from M1's scanner into an unexported helper rather than duplicated.
2. **v2 unmarshal semantics.** For an int64 field: `1.0`, `1e3`, `-0`, `01`, `9007199254740992`, `9223372036854775808` and `null`. Record which are rejected. `null` into a non-pointer field is expected to leave the zero value, which is why required members are decoded through presence-tracking wire structs (section 7.1).
3. **Unknown and case-variant members.** Confirm `RejectUnknownMembers(true)` rejects an unknown member and a case variant of a known one at every nesting level used by the wire structs.
4. **Allocation.** Decode a 1 MiB frame consisting of one string, one array of 65,536 integers, and 10,000 nested objects; record allocations and bytes per decode. The decoder's memory must stay within a small constant multiple of the frame.

## 5. Goal

- **`internal/core/ids`**: identifier and counter types every layer shares.
- **`annot.ParseCommitted`**: the trusted counterpart of `ParseOps` for committed operations (F7).
- **`internal/proto`**: typed messages for the whole catalogue in section 7, `DecodeClient`, `EncodeServer`, `EncodeClient`, `DecodeServer`, the inline and paging helpers, and the code catalogue.
- **`schema/protocol/`**: generated codes, limits and golden frames, and hand-written invalid frames.
- **`plugin/src/core/protocol/`**: the TypeScript client encoder and server decoder, passing the fixtures.

## 6. Interface

### ids

```go
package ids

type FileID [16]byte     // client-minted (ADR 008); server-minted for conflict copies
type DeviceID [16]byte   // server-minted at enrollment
type ClientOpID [16]byte // client-minted, one per submission
type Version int64       // per file, 1 .. MaxSafe
type Seq int64           // vault-wide, 0 .. MaxSafe; 0 means before the first change

const MaxSafe = 1<<53 - 1

func ParseFileID(s string) (FileID, error) // 36-char lowercase canonical, version 7, variant 10
func NewFileID(unixMilli int64, entropy io.Reader) (FileID, error)
func (id FileID) String() string
func (id FileID) MarshalText() ([]byte, error)
func (id *FileID) UnmarshalText(b []byte) error
// The same five functions exist for DeviceID and ClientOpID.

func (v Version) Valid() bool // 1 <= v <= MaxSafe
func (s Seq) Valid() bool     // 0 <= s <= MaxSafe

var ErrInvalidID, ErrUnsafeInt error
```

The time and entropy for a new id are parameters, so the package stays pure. Distinct types mean a `ClientOpID` cannot be passed where a `FileID` is expected.

### annot (addition)

```go
// ParseCommitted decodes operations from the committed log or a server frame.
// It accepts a trusted replace field. Only server-produced bytes reach it.
func ParseCommitted(raw []byte, l Limits) (Ops, error)
```

### proto

```go
package proto

const Subprotocol = "stele.v1"
func Subprotocols() []string // supported versions, newest first; a function, not a mutable variable

type Config struct {
    MaxFrame        int           // client to server, 1 MiB
    EnvelopeReserve int           // 16 KiB kept free for the envelope
    MaxServerFrame  int           // server to client, 2 MiB
    MaxInline       int           // per-member inline payload, 512 KiB; also the client split size
    PageEntries     int           // 500
    Text            textop.Limits // MaxJSONBytes = MaxFrame - EnvelopeReserve
    Annot           annot.Limits  // MaxOpBytes = MaxFrame - EnvelopeReserve
    MaxInlineContent int          // decoded inline content, MaxFrame - EnvelopeReserve
}
func DefaultConfig() Config
func (c Config) Validate() error // all violations at once; see section 7.7

// Token is a device credential. Every formatting path prints "stele_[redacted]".
type Token struct{ /* unexported */ }
func ParseToken(s string) (Token, error)
func (t Token) Hash() [32]byte // SHA-256; what auth looks up and compares
func (t Token) String() string
func (t Token) GoString() string
func (t Token) Format(f fmt.State, verb rune)
func (t Token) LogValue() slog.Value

type Req uint32 // query correlation id, 1 .. 2^32 - 1

// Client to server.
type ClientMsg interface{ clientMsg() }
type Hello struct{ Token Token }
type PullChanges struct{ Req Req; After ids.Seq }
type ListFiles struct{ Req Req; After *ids.FileID }
type Submit struct{ ID ids.ClientOpID; Body Body }
type Pong struct{ N uint32 }

type Body interface{ body() }
type TextEdit struct{ File ids.FileID; Base ids.Version; Op textop.Op }
type AnnotEdit struct{ File ids.FileID; Base ids.Version; Ops annot.Ops }
type AnnotReplace struct{ File ids.FileID; Base ids.Version; Content Content }
type PutBlob struct{ File ids.FileID; Base ids.Version; Blob BlobRef }
type Create struct {
    File    ids.FileID
    Path    vpath.Path
    Class   classify.Class
    Content Content
    OnTaken rules.OnTaken
    CopyOf  *CopyOf
}
type Rename struct{ File ids.FileID; Path vpath.Path; OnTaken rules.OnTaken }
type RenameGroup struct{ PDF, Sidecar Target; OnTaken rules.OnTaken }
type Delete struct{ File ids.FileID; Base ids.Version; Confirmed bool }

type Content interface{ content() }
type InlineText struct{ Text string }   // A
type InlineSidecar struct{ Raw []byte } // B; FreeDraw file bytes, unparsed (section 7.3)
type BlobContent struct{ Ref BlobRef }  // A, B or C
type BlobRef struct{ SHA256 [32]byte; Size int64 }
type Target struct{ File ids.FileID; Path vpath.Path }
type CopyOf struct{ File ids.FileID; Reason CopyReason }

// Server to client.
type ServerMsg interface{ serverMsg() }
type Welcome struct{ Device ids.DeviceID; HeadSeq ids.Seq; Limits Advertised }
type Changes struct{ Req Req; Entries []Change; More bool }
type Live struct{ Change Change } // wire type "change"
type Files struct{ Req Req; HeadSeq ids.Seq; Files []FileInfo; More bool }
type Reject struct{ ID ids.ClientOpID; Code Code; Detail Detail }
type Error struct{ Req Req; Code QueryCode; Detail Detail }
type Ping struct{ N uint32 }

type Change struct {
    Seq     ids.Seq
    At      time.Time // server commit time
    Actor   rules.Actor
    Device  ids.DeviceID
    ID      ids.ClientOpID
    Members []Member // one, or two for a rename group
}
type Member struct {
    File    ids.FileID
    Version ids.Version
    Kind    Kind
    Path    vpath.Path     // create and rename
    Class   classify.Class // create
    Owner   rules.Owner    // create
    Origin  *Origin        // create of a conflict copy
    Payload Payload        // nil for rename and delete
    Len     *int           // Class A content: resulting length in UTF-16 units
    Hash    *[32]byte      // Class B content: annot.CanonicalHash after apply
}
type Payload interface{ payload() }
type TextPayload struct{ Op textop.Op }
type AnnotPayload struct{ Ops annot.Ops } // effective operations
type BlobPayload struct{ Ref BlobRef }
type Omitted struct{}                     // fetch state over HTTPS

// Codecs.
func DecodeClient(frame []byte, c Config) (ClientMsg, error) // *CloseError, *RejectError or *QueryError
func EncodeServer(m ServerMsg, c Config) ([]byte, error)      // ErrFrameTooLarge
func EncodeClient(m ClientMsg, c Config) ([]byte, error)      // reference client and loadgen
func DecodeServer(frame []byte, c Config) (ServerMsg, error)  // ignores unknown members

// Helpers for the hub.
func Inline(m Member, c Config) Member // replaces an oversized payload with Omitted
type Page struct{ /* unexported */ }
func NewPage(req Req, c Config) *Page
func (p *Page) Add(ch Change) (bool, error) // false: page full, start the next one
func (p *Page) Encode(more bool) ([]byte, error)

// Errors.
type CloseError struct{ Code CloseCode; Reason string }
type RejectError struct{ ID ids.ClientOpID; Code Code; Detail Detail }
type QueryError struct{ Req Req; Code QueryCode; Detail Detail }

type Code string      // closed set, section 7.6
type QueryCode string // closed set, section 7.6
type CloseCode uint16 // closed set, section 7.6

type Detail struct {
    Field        string
    Reason       string
    Component    *int
    Class        classify.Class
    Demotion     classify.Demotion
    Limit        int64
    Scope        string
    RetryAfterMS int64
}
```

`rules.OnTaken`, `rules.Actor`, `rules.Owner` and `classify.Class` are reused rather than redeclared, so there is one definition of each enum. Mapping engine decisions to codes still happens once, in `session` (guidelines 3.2); `proto` produces codes only for faults it detects while decoding.

## 7. Semantics

### 7.1 Framing, decoding and versioning

- Every message is one UTF-8 JSON text frame whose top level is an object with a string member `t`. A binary frame closes with 4400.
- The client offers `Sec-WebSocket-Protocol: stele.v1`. The server selects the newest version it shares with the offer. If none is shared, it accepts the upgrade and immediately closes with 4426 `unsupported_protocol`, because browsers cannot read the body of a refused upgrade. The plugin turns 4426 into "update the plugin".
- `DecodeClient` reads a frame in three passes over the same bytes, all with v2 defaults. Pass 1 decodes `{t, client_op_id, kind, req}` ignoring other members. Pass 2 decodes the concrete wire struct for `t` (and `kind`) with unknown members rejected. Pass 3 hands raw payload values to the core parsers. Each wire struct uses pointer fields so that a missing required member is distinguishable from a zero value, then converts to the domain type.
- Payload values are captured as `jsontext.Value`, which keeps the exact bytes for `textop.Parse` and `annot.ParseOps`.
- `DecodeServer` decodes the same way but ignores unknown members, matching the TypeScript client.

### 7.2 Client messages

| `t` | Members | Answer |
| --- | --- | --- |
| `hello` | `token` | `welcome`, or close 4401 |
| `pull_changes` | `req`, `after_seq` | `changes`, or `error` |
| `list_files` | `req`, optional `after_file_id` | `files`, or `error` |
| `submit` | `client_op_id`, `kind`, kind members (7.3) | a `change` carrying the same `client_op_id`, or `reject` |
| `pong` | `n` | none |

```json
{"t":"hello","token":"stele_9mQ2…"}
{"t":"pull_changes","req":1,"after_seq":1000}
{"t":"submit","client_op_id":"019a1b2c-…","kind":"text_edit","file_id":"019a0f00-…","base_version":17,"op":[5,"hi",-3,10]}
```

`hello` must be the first frame and arrive within 5 s; anything else first closes 4401. A token must match `stele_` followed by 43 base64url characters; a malformed token closes 4401 without a database lookup. `after_seq` is the client's persisted cursor; it is no longer repeated in `hello`.

### 7.3 Submit kinds

| `kind` | Members | Notes |
| --- | --- | --- |
| `text_edit` | `file_id`, `base_version`, `op` (ot.js array) | `op` parsed by `textop.Parse` with `Config.Text` |
| `annot_edit` | `file_id`, `base_version`, `ops` (string of JSON) | the string's contents parsed by `annot.ParseOps` |
| `annot_replace` | `file_id`, `base_version`, `content` | `content` is `sidecar` or `blob` |
| `put_blob` | `file_id`, `base_version`, `blob {sha256, size}` | |
| `create` | `file_id`, `path`, `class`, `content`, optional `on_taken`, optional `copy_of {file_id, reason}` | content shape must match class |
| `rename` | `file_id`, `path`, optional `on_taken` | |
| `rename_group` | `pdf {file_id, path}`, `sidecar {file_id, path}`, optional `on_taken` | |
| `delete` | `file_id`, `base_version`, optional `confirmed` | |

- `on_taken` is `"number"` (default) or `"reject"`. `class` is `"A"`, `"B"` or `"C"`. `confirmed` defaults to false.
- `content` is exactly one of `{"text": string}` (class A), `{"sidecar": string}` (class B) or `{"blob": {"sha256": hex, "size": int}}` (any class). Class C accepts only `blob`.
- `copy_of.reason` is one of `rebase`, `deleted`, `class_change` and `first_sync`: the client-requested conflict copies of M3 and M13.
- **Content is not classified in `proto`.** Inline text is bounded and must be well-formed; inline sidecar bytes are bounded and passed through unparsed. The engine runs `classify.Confirm` and `annot.Parse` on inline and blob-sourced content alike, so a malformed sidecar gets `class_mismatch` whichever way it arrived (ADR 019).
- `base_version` must be a valid `Version` (at least 1). Whether it is stale is the engine's decision.
- A `rename_group` has distinct file ids; that its targets pair (`SidecarOf`) is checked by `rules`, not here.

### 7.4 Server messages

| `t` | Members |
| --- | --- |
| `welcome` | `device_id`, `head_seq`, `limits` (7.7) |
| `changes` | `req`, `entries` (array of change records), `more` |
| `change` | one change record, flattened into the frame |
| `files` | `req`, `head_seq`, `files` (array of file records), `more` |
| `reject` | `client_op_id`, `code`, optional `detail` |
| `error` | `req`, `code`, optional `detail` |
| `ping` | `n` |

**Catch-up then live.** The client pulls until `more` is false; from then on the session is live and the server sends `change` frames. `seq` strictly increases with no gaps across every `changes` entry and `change` frame a session receives, whichever type carries it. A client applies entries in order and persists its cursor after each.

**Full resync.** The client pages `list_files` from the start. It takes `head_seq` from the first page as its cursor and later skips any change member whose `version` is at or below the version it listed for that file.

**File record:** `file_id`, `path`, `class`, `owner` (`user` or `canvas`), `version`, `size`, and `blob` (C), `len` (A) or `hash` (B).

### 7.5 Change records

```json
{"t":"change","seq":1043,"at":"2026-10-18T09:30:00.123Z","actor":"device",
 "device_id":"019a00aa-…","client_op_id":"019a1b2c-…",
 "members":[{"file_id":"019a0f00-…","version":18,"kind":"text_edit","op":[5,"hi",-3,10],"len":2048}]}
```

| Member `kind` | Members beyond `file_id`, `version`, `kind` |
| --- | --- |
| `create` | `path`, `class`, `owner`, optional `origin {copy_of, reason}`, and the class payload: `op` and `len` (A), `ops` and `hash` (B), `blob` (C) |
| `text_edit` | `op` (transformed), `len` |
| `annot_edit` | `ops` (effective operations as a string, possibly a trusted replace), `hash` |
| `put_blob` | `blob` |
| `rename` | `path` (final, possibly server-numbered) |
| `delete` | none |

- **One transaction, one record, one `seq`.** A `rename_group` is one record with two members, applied by the client together. A stale `put_blob` or `annot_replace` that became a server-made conflict copy is one record whose member is the copy's `create`, with `origin.reason` `stale_put` or `stale_replace`, carrying the submitter's `client_op_id`.
- **`actor`** is `device`, `bridge` or `admin`. **`device_id`** is always present; the bridge and admin act as system devices. **`client_op_id`** is always present, because every change originates from a submission.
- **Own changes.** A record whose `device_id` is the client's own and whose `client_op_id` matches a persisted in-flight submission is that submission's acknowledgement, live or during catch-up. This closes F2 only if the client persists each in-flight submission with its id before sending it (M13).
- **Divergence checks (D8).** `len` and `hash` describe the file after this member is applied. A mismatch on the client is a convergence failure: the client counts it, fetches state and reports through the audit.
- **Omitted payloads (D4).** A member may carry `"omitted": true` instead of its payload; `len` and `hash` remain. The client fetches `GET /v1/files/{file_id}/state` and adopts it if its version is at or above the member's, then skips that file's later members at or below the adopted version. Class C members are never omitted; a blob reference is small.
- **Derived notices (D11).** A Canvas file changed is a `put_blob` with actor `bridge`; removed, a `delete` with actor `bridge`; a conflict copy, a `create` with `origin`. Notices survive offline periods because they are part of the feed.
- `at` is RFC 3339 in UTC with millisecond precision. It is informational; clients never order by it.

### 7.6 Error tiers and codes

**Tier 1, close the session.** The frame cannot be trusted, so nothing in it is attributed.

| Close | Reason | When |
| --- | --- | --- |
| 1000 | `normal` | Clean client or server close |
| 1001 | `shutting_down` | Server graceful shutdown |
| 1009 | (library) | Frame above `MaxFrame`, enforced by `SetReadLimit` |
| 1011 | `internal` | Recovered panic in the session |
| 4400 | `protocol` | Binary frame, malformed JSON, invalid UTF-8, lone surrogate, duplicate name, top level not an object, missing or unknown `t`, a server-only `t`, an unknown member outside a submit body, a submit without a valid `client_op_id`, a query without a valid `req` |
| 4401 | `hello_timeout`, `bad_token`, `revoked` | Authentication |
| 4409 | `replaced` | A newer session for the same device |
| 4426 | `unsupported_protocol` | No shared subprotocol |
| 4429 | `slow_consumer` | Outbound queue full |

**Tier 2, reject the submission.** The frame is well formed and carries a valid `client_op_id`, so the client can free that file's in-flight slot and surface the failure instead of looping through reconnects.

| Code | Detail | Raised by |
| --- | --- | --- |
| `invalid_submission` | `field` | proto: unknown kind, unknown or missing member, wrong type, unsafe integer, invalid id, content shape not matching class |
| `invalid_op` | `field`, `reason` | proto: `textop` or `annot` payload rejected for any reason other than a size limit; `reason` is the core sentinel name |
| `invalid_path` | `field`, `reason`, `component` | proto: `vpath.Parse` failed |
| `too_large` | `field`, `limit` | proto (`textop.ErrLimit`, an `annot` limit error, or inline content above its limit), engine (result above the class ceiling) |
| `rate_limited` | `scope` (`rate`, `file`, `device`, `mailbox`), `retry_after_ms` | session, engine |
| `unknown_file` | | engine: file id never existed |
| `file_exists` | | engine: create with a live file id under a new `client_op_id` |
| `file_deleted` | | engine |
| `path_taken` | | engine (`on_taken = reject`, or names exhausted) |
| `stale_delete` | | engine |
| `invalid_base` | | engine: base above head |
| `read_only_path` | | engine |
| `class_change` | | engine: rename across name categories, or a content op for the wrong class |
| `group_required` | | engine: PDF with a live sidecar renamed alone, or a mismatched sidecar target |
| `class_mismatch` | `class`, `demotion` | engine |
| `bulk_delete_guard` | `limit` | engine |
| `rebase_required` | | engine |
| `whole_document_required` | | engine |
| `blob_missing` | | engine |
| `internal` | | engine: ambiguous commit or dependency failure; retry with the same id |

**Tier 3, answer the query with `error`.**

| Code | Detail |
| --- | --- |
| `resync_required` | `reason`: `pruned` (cursor older than retention) or `ahead` (cursor above `head_seq`, F12) |
| `invalid_request` | `field` |
| `rate_limited` | `retry_after_ms` |
| `internal` | |

All codes, close codes and reasons are exported to `schema/protocol/codes.json`. Clients branch on codes, never on text.

### 7.7 Limits, inline threshold and advertised configuration

| Setting | Default | Constraint checked by `Validate` |
| --- | --- | --- |
| `MaxFrame` | 1 MiB (1,048,576) | |
| `EnvelopeReserve` | 16 KiB | |
| `Text.MaxJSONBytes`, `Annot.MaxOpBytes`, `MaxInlineContent` | 1,032,192 | at most `MaxFrame − EnvelopeReserve` |
| `MaxServerFrame` | 2 MiB | at least `2 × MaxInline + EnvelopeReserve` |
| `MaxInline` | 512 KiB | at most each payload limit above, so anything inline also parses on the client |
| `PageEntries` | 500 | |
| Members per change | 2 | constant |

- `Inline` measures each member's encoded payload and replaces it with `Omitted` above `MaxInline`. Every encoded server frame is at most `MaxServerFrame`, and `Page.Add` closes a page before it would exceed it.
- **Client split rule.** Clients keep every submitted payload at or below the advertised `split_bytes` (`MaxInline`): large inserts become consecutive text operations, large annotation edits become several element batches, and create or replace content above it goes through the blob path. The server enforces only the hard limits, so a client slightly above the advisory value is not rejected.
- **`welcome.limits`** carries what clients must agree with the server on:

```json
{"max_frame":1048576,"max_server_frame":2097152,"split_bytes":524288,"page_entries":500,
 "inflight_per_device":64,"rate":{"per_sec":50,"burst":200},
 "classify":{"text_exts":["md","txt","mdx"],"max_text":8388608,"max_sidecar":16777216},
 "guard":{"window_ms":300000,"max_count":25,"min_for_ratio":5,"ratio":0.1,"buckets":30}}
```

`proto` defines and encodes `Advertised`; `session` fills it from the server configuration. `ratio` is the only non-integer number on the wire.

### 7.8 Cross-language rules

- **Strings** are well-formed Unicode in both directions. Go rejects lone surrogates in client frames (step 0 decides how). TypeScript checks every decoded server string with a lone-surrogate regular expression rather than `isWellFormed`, so no newer runtime is required.
- **Integers** are JSON integers without fraction or exponent, within 0 to 2^53 − 1 unless narrower. TypeScript checks `Number.isSafeInteger`.
- **Identifiers** are 36-character lowercase canonical UUIDv7. TypeScript mints UUIDv7 itself; `crypto.randomUUID` produces version 4 and is rejected.
- **Hashes** are 64 lowercase hex characters.
- **Byte-exact payloads (D10).** `annot_edit.ops`, `content.sidecar` and annotation members' `ops` are JSON strings whose contents are JSON text. `JSON.stringify` of a string preserves its contents exactly, so the client forwards FreeDraw bytes unchanged.
- **TypeScript's server-frame validation is value-level.** `JSON.parse` cannot see duplicate names or number spelling. The Go encoder never emits either, and the invalid server fixtures test only what a value-level check can see.

## 8. Implementation guide

Build in this order, each step green before the next.

1. **Step 0** (section 4), recorded before any decoder code.
2. **`core/ids`.** Write the round-trip property and the canonical-form table first.
3. **`annot.ParseCommitted`.** Property first: for random committed results, including trusted replaces, `ParseCommitted(Marshal(Effective))` equals `Effective` and replays to the same state.
4. **`proto` foundations.** `Config` and `Validate`, `Token` and its redaction, the code catalogue and the `codes.json` generator.
5. **`DecodeClient`.** Write the tier table tests first: one case per row of section 7.6 for tiers 1 and 3, and one per proto-raised row of tier 2.
6. **`EncodeServer`, `Inline` and `Page`.**
7. **`EncodeClient` and `DecodeServer`.**
8. **Fixtures.** `make fixtures` generates `schema/protocol/{codes,limits}.json`, `c2s/valid.json` and `s2c/valid.json` from Go. `c2s/invalid.json` and `s2c/invalid.json` are hand-written and marked so in their headers.
9. **TypeScript.** `plugin/src/core/protocol/` with the client encoder, the server decoder, UUIDv7 minting and the codes imported from `codes.json`, passing every protocol fixture in Vitest. It may be drafted by an agent once the fixtures exist (guidelines 9.2).
10. **Documents.** ADRs 023 to 027 and the updates in section 13.

Expected effort: about two part-time weeks.

## 9. Proof

| Test | Asserts |
| --- | --- |
| `TestIDCanonical` | Uppercase, braces, missing hyphens, wrong version and wrong variant are rejected; `ParseFileID(id.String()) == id` |
| `TestPropIDRoundTrip` | For random ids: text and JSON round trips are identities, and `NewFileID` output parses |
| `TestSafeIntegers` | 2^53 − 1 accepted; 2^53, negative values and non-integers rejected at every integer member |
| `TestPropCommittedOpsRoundTrip` | `ParseCommitted(Marshal(Effective))` equals `Effective`, including replaces; `ParseOps` still rejects them |
| `TestTierTable` | Every proto-detected row of section 7.6 produces its tier, code, reason and detail |
| `TestPropTierSoundness` | Mutating a valid submit's body yields a reject with the same `client_op_id`; adding a duplicate name or a lone surrogate anywhere yields close 4400 |
| `TestStrictness` | Case-variant names, unknown members at every depth and `null` for required members are rejected in client frames and ignored or rejected as documented in server frames |
| `TestPropClientRoundTrip` | `DecodeClient(EncodeClient(m)) == m` for random messages of every type and kind |
| `TestPropServerRoundTrip` | `DecodeServer(EncodeServer(m)) == m`, including omitted members and two-member changes |
| `TestCatalogueComplete` | Every message type and submit kind has an encoder, a decoder case and at least one valid fixture; every code appears in `codes.json` and in a fixture |
| `TestConfigValidate` | Each constraint in 7.7 fails alone and all failures are reported together; defaults validate |
| `TestInlineAndPage` | Payloads at `MaxInline` stay inline and one byte more are omitted; no encoded frame exceeds `MaxServerFrame`; pages close before the byte budget and at `PageEntries` |
| `TestMaxPayloadFitsFrame` | A text operation and an annotation operation at their parse limits, wrapped in their submit envelopes, fit in `MaxFrame` |
| `TestTokenRedaction` | `%v`, `%+v`, `%#v`, `%s`, `%q`, slog text and JSON handlers and `errors.New(fmt.Sprint(hello))` never contain the token |
| `TestContentNotClassified` | A malformed inline sidecar decodes; `class_mismatch` is the engine's answer, not `proto`'s |
| `TestGolden{C2S,S2C}` | Every fixture; Go regeneration has no diff |
| `FuzzDecodeClient`, `FuzzDecodeServer`, `FuzzParseIDs` | No panic; accepted input re-encodes and decodes to the same value; every error has exactly one tier; allocation stays within the step 0 bound |
| Vitest contract suite | TypeScript-encoded client messages equal the valid client fixtures by value; every valid server fixture decodes; every invalid server fixture is rejected with its reason |

**Cross-language fixtures.** As in M2 and M3, the fixtures prove TypeScript matches Go, not that Go is right. Go's correctness rests on the properties, the tier table and fuzzing.

## 10. Exit criteria

- Properties pass at 100,000 cases; each fuzz target runs 10 minutes with no finding.
- Package coverage above 90% in `ids` and `proto`.
- Fixtures committed and passing in both languages; regeneration produces no diff.
- Step 0 measurements recorded in `docs/m4-verification.md`.
- ADRs 023 to 027 merged; the Architecture Specification and Implementation Guide updated (section 13).

## 11. How it wires into later milestones

| Milestone | Uses from M4 | What M4 requires of it |
| --- | --- | --- |
| M5 storage/pg | `ids`, the change record shape | One `changes` row per transaction (`seq`, `actor`, `device_id`, `client_op_id`, `at`) and a `change_members` table (`seq`, ordinal, `file_id`, `version`, `kind`, path and class where present). The hub must rebuild every member's payload from committed rows; the NOTIFY channel is `stele_changes`. |
| M6 blob | `BlobRef` | Blob-sourced create and replace content is readable by the engine outside the transaction; transient content blobs are left for GC. |
| M7 engine | Every submit body; `Code` values | Map every rejecting `rules.Decision` and engine failure to a code, exhaustively checked in `session`. Server-made conflict copies are one change carrying the submitter's `client_op_id`. Text results above the class ceiling reject `too_large`. The reference client persists in-flight submissions with their ids before sending, recognises own changes in catch-up, follows the split rule, adopts omitted members by fetching state and rebases pending local edits. The simulator gains the crash-between-commit-and-ack case from F2 and fails on a planted version that drops own-change recognition. |
| M8 hub and sessions | Codecs, `Inline`, `Page`, `Advertised` | Subprotocol negotiation and 4426; tiers to close, reject and error; `welcome` filled from server configuration; no `notice` sender. `GET /v1/files/{file_id}/state` in `httpapi` serves A as UTF-8 text and B as `annot.EncodeState` bytes, with `Stele-Version`, `Stele-Class` and `Stele-Len` or `Stele-Hash` headers. |
| M9 auth | `Token` | Tokens are `stele_` plus 43 base64url characters. Revocation closes with 4401 `revoked`. The CLI is `stelectl`. |
| M11 Canvas bridge | Actor `bridge` | No notices: the plugin derives them from bridge changes. |
| M12 assembly | | `cmd/stele-server`, bucket `stele-sync`, NOTIFY channel `stele_changes`. |
| M13 plugin | The TypeScript protocol port | Persist each in-flight submission with its `client_op_id` before sending, in the same IndexedDB transaction as the buffer change. Mint UUIDv7. Use `welcome.limits` for classification, guard and split sizes instead of local settings. Fetch state for omitted members and for first sync. Read file bytes before decoding text so invalid UTF-8 classifies as C instead of being replaced. Close 4426 prompts a plugin update. |
| M14 verification | Metrics | Phase 2 metrics use the `stele_` prefix. Add `stele_frames_rejected_total{tier,code}`, `stele_omitted_members_total{class}` and `stele_divergence_detected_total{class}` (client-reported, expected 0). S3 covers a kill between commit and ack followed by catch-up. |

## 12. Accepted limitations

- **Half a paste is briefly visible.** A split edit reaches other devices as several changes.
- **Own changes are echoed.** The submitter receives its own payloads back; negligible at single-user rates.
- **Omitted members cost a round trip and a local rebase.** The rebase of pending edits against a fetched head uses a diff, so intent can be approximated in that rare case; convergence is unaffected.
- **First sync is one HTTPS request per A or B file.** A batch state endpoint can come later if first sync is measured to be slow.
- **TypeScript validation of server frames is value-level.** It cannot detect duplicate names or number spelling; it relies on the Go encoder emitting neither.
- **Strict client frames require server-first rollout,** which section 5.6 of the specification already mandates.
- **Deployed phase 1 identifiers keep their names** (section 13).

## 13. Documents to update

| Document | Change |
| --- | --- |
| Architecture Specification, section 3 | `changes` row per transaction plus `change_members`; `client_op_id` on `changes`; bucket `stele-sync` |
| Architecture Specification, sections 5.1 to 5.6 | Subprotocol `stele.v1` and 4426; `hello` with token only; the catalogue of section 7; no `fetch_head`, `head`, `ack`, `remote` or `notice`; the state endpoint; tiers and codes; limits and the advertised configuration |
| Architecture Specification, section 8 | Metric prefix `stele_`; `stelectl` in runbooks |
| Architecture Specification, title and section 10 | Project name resolved as stele; decision rows for ADRs 023 to 027 |
| Implementation Guide | Naming section; M4 replaced by this document; M5, M7, M8, M9, M12, M13 and M14 requirements from section 11 |
| ADRs | Write 023 (strict frames, tiers, directional fixtures), 024 (one change record, derived notices, divergence checks), 025 (bounded frames, by-reference state, string payloads), 026 (core identifiers), 027 (stele naming and subprotocol versioning); amend 005 and 012 to point at them |

**Naming (ADR 027).** Phase 2 identifiers that are not yet deployed or frozen take the stele name now: `cmd/stele-server`, `cmd/stelectl`, subprotocol `stele.v1`, token prefix `stele_`, phase 2 metric prefix `stele_`, NOTIFY channel `stele_changes`, Garage bucket `stele-sync`, plugin id `stele-sync`, Postgres database and role `stele`, and `Stele-*` HTTP headers. Deployed phase 1 resources keep their names until a deliberate migration: Garage bucket `obsync`, Kubernetes namespace `obsync` (phase 2 deploys into it), the `garage-obsync` and `obsync-worker` Argo CD applications, and the plugin's `obsync:pull-scope` rule key.

## Sources

- [Go 1.27 release notes](https://go.dev/doc/go1.27): `encoding/json/v2` and `jsontext` become standard; v1 backed by v2
- [encoding/json: Migrating to v2](https://pkg.go.dev/encoding/json@go1.27.2#hdr-Migrating_to_v2): duplicate names, invalid UTF-8 and case-sensitive matching in v2
- [RFC 6455](https://www.rfc-editor.org/rfc/rfc6455): subprotocol negotiation (section 1.9), close codes (section 7.4)
- [RFC 9562](https://www.rfc-editor.org/rfc/rfc9562): UUID version 7 layout and variant
- [RFC 8259, section 6](https://www.rfc-editor.org/rfc/rfc8259#section-6): interoperable integer range
- [Transactional outbox](https://microservices.io/patterns/data/transactional-outbox.html): the change feed that D3 makes the only delivery path
