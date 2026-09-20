# 002: ot.js encoding and UTF-16 positions

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Go and JavaScript must apply identical operations at identical text positions.

## Decision

Use ot.js JSON operation arrays and UTF-16 code units. Reject CodeMirror ChangeSet JSON and code-point indexing.

## Consequences

The wire format is compact, documented, and has an independent oracle. Editor positions require no per-keystroke conversion; Go must count UTF-16 units and reject split surrogate pairs.
