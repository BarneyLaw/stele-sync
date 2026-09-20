# 004: Order the vault feed with a single-row lock

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

A reconnect cursor must never skip a transaction that commits after a higher sequence number.

## Decision

Increment vault.head_seq while holding the single vault row lock in the same transaction as the change. Reject bigserial as the feed order.

## Consequences

Sequence order equals commit order and rollback removes the increment. Commits serialize briefly on one row, acceptable at single-user write rates.
