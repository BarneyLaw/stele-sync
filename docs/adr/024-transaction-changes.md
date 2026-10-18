# 024: One committed transaction, one delivered change

Status: accepted

Date: 2026-10-18

Source: [Accepted M4 proposal](../../internal/proto/M4%20proposal%20proto.md).

## Context

An idempotent retry does not prevent a crash-before-ack client from treating
its own committed edit as remote during catch-up. Independent ack/remote records
also complicate atomic rename groups and sequence cursors.

## Decision

Store one change record with seq, actor, device_id, client_op_id and commit time
per transaction, plus one or two ordered members. Live change and catch-up changes
carry identical records. Two-member records contain distinct rename members.
The originating client matches device_id/client_op_id against in-flight bytes
persisted before sending. Sequence allocation, member rows and the idempotency
record commit together. The feed is the only delivery path.

Carry resulting UTF-16 length for text and CanonicalHash for annotations. Derive
Canvas and conflict-copy notices from actor/origin in the feed. Revocation uses
close 4401, not an ephemeral notice.

## Consequences

M5 needs change_members and submission correlation on changes. M7/M13 must
persist in-flight identifiers and recognize own catch-up entries before any
remote transformation. M7/M14 must plant and catch the lost-own-change bug.
The codecs alone do not implement this durable client state machine.
