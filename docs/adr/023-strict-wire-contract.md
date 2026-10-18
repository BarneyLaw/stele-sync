# 023: Strict directional frames and attributable errors

Status: accepted

Date: 2026-10-18

Source: [Accepted M4 proposal](../../internal/proto/M4%20proposal%20proto.md).

## Context

Go v1 JSON semantics could repair invalid Unicode, accept duplicate names and
case-fold fields differently from JavaScript. Reconnecting on every bad body
also prevents clients from freeing their in-flight slots.

## Decision

Validate complete UTF-8 JSON objects with jsontext defaults before trusting
correlation. Reject client extensions; ignore server extensions. Missing or
untrustworthy correlation closes 4400; an invalid attributable submission returns
reject; an invalid attributable query returns error. Required fields distinguish
absence and null from legitimate zero/false values. Go uses presence-aware raw
object readers and closed per-variant field tables, rather than duplicating
pointer wire structs for each nested shape. Errors contain field names and
sentinel names, never raw values. Limit nesting to 64 containers before parsing.

Keep separate sealed client/server interfaces and all four Go codecs. Generate
directional fixtures and the code catalogue; TypeScript encodes clients and
validates server values. UUIDs, hashes and safe integer counters are canonical.

## Consequences

Roll out the server first. Unknown server fields remain forward-compatible;
unknown client fields expose mistakes immediately. TypeScript server validation
is value-level: duplicate keys and original number spelling are guaranteed by
the Go encoder, not recoverable after JSON.parse. Framing depth bounds the
allocation amplification measured in docs/m4-verification.md.
