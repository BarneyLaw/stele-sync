# 005: WebSocket for operations and HTTPS for blobs

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Large binary transfers must not block keystroke propagation, and mobile clients need resumable downloads.

## Decision

Carry operations over WebSocket and blobs over HTTPS with Range support. Reject sending everything over WebSocket or replacing it with SSE plus POST.

## Consequences

Control messages remain responsive and blob downloads can resume. The WebSocket transport can also support phase 3 without implementing phase 3 features now.
