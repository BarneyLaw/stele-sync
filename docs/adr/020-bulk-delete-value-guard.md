# 020: Conservative deletion guard with value semantics

Status: accepted

Date: 2026-10-11

## Context

A percentage-only guard makes small vaults unusable. A mutable slice-backed
counter can corrupt a caller's prior state and make rejection non-transactional.

## Decision

Allow at most `min(25, max(5, floor(0.10 * live)))` unconfirmed device deletes
inside five minutes. Confirmed deletes and Bridge/Admin actions do not count.
A rejected batch returns the exact previous value. The plugin checks the whole
batch before sending; the server checks each submission and remains authoritative.

Use a bounded value array and configured bucket widths (30 ten-second buckets
by default, at most 256). Keep one additional boundary slot, because an event
can remain live through the following full window. Expire at the *end* of its
bucket plus Window: never early, at most one bucket late. Clamp regressing time
to the last accepted instant. Invalid counts/configuration fail closed.
TypeScript uses bigint nanoseconds, preserving Go's truncation origin and exact
boundary behavior without reading the clock.

## Consequences

One guard belongs to each device in engine memory. It resets on restart.
A client/server history disagreement may require another confirmation for the
remaining batch. The minimum floor is intentional even for very small vaults.
The ring is fixed and bounded; it does not retain a growing event log.
