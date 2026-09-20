# 001: Server-side operational transform

Status: proposed

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

Several devices can submit edits against old versions of a single-user vault.

## Decision

Use Jupiter / ot.js style server transformation, with our own transform, compose, and apply implementations in Go and TypeScript. Reject the alternative where the authority only accepts operations at head and clients rebase/retry, as in CodeMirror collab.

## Consequences

The core algorithm is owned and property-tested by the project. ot.js remains an independent test oracle, never a runtime dependency. M1 must derive the case tables and demonstrate convergence at scale (S1, S2, S16).

M0 explicitly requests proposed status until the transform question is settled. The architecture decision table and M1 already say accepted; this record preserves M0's requested review checkpoint without changing the chosen design.
