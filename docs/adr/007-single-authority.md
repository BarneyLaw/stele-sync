# 007: One replica with an advisory-lock fence

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Two active authorities could fork history; a single user does not require high availability.

## Decision

Run one replica and acquire a Postgres advisory lock on a dedicated connection. Reject HA with leader election.

## Consequences

A second instance exits. Losing the lock connection stops writes and exits; clients work offline while the authority restarts.
