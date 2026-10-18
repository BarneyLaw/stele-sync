# 012: Fan out from the committed change feed

Status: accepted

Refined by [024: one transaction change](024-transaction-changes.md), including own-change recognition during catch-up.

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Independent document actors can commit and then publish out of sequence. A client persisting a live cursor could skip an earlier change after a crash.

## Decision

Use the committed change feed as a transactional outbox, with NOTIFY only as a wake-up. Reject actors sending acknowledgements and broadcasts directly after commit.

## Consequences

A single hub reads committed rows in sequence order for gap-free delivery and resolves ambiguous commits. Acknowledgements still require durable commit before delivery.
