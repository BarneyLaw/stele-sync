# 009: Bound reconciliation by a transform window

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Very old overlapping edits can converge mathematically while producing unreadable content.

## Decision

Transform only inside both a 24-hour and 5,000-operation window. Outside it require a conflict copy. Reject always transforming.

## Consequences

Work and semantic damage are bounded, while both versions of acknowledged content survive. These initial limits are configurable and must be revisited after the device soak.
