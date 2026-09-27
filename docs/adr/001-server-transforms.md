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

## M1 derivation and evidence

The complete Compose/Transform component tables, residual-length rules,
termination argument and per-pair TP1 rationale are in
[the package documentation](../../internal/core/textop/doc.go). Their 54 named
test rows cover all nine pairs at shorter/equal/longer lengths, with literal
expected operations. They were derived from consumption and preservation of
source/inserted text, not copied from the oracle implementation.

The independent Go property model edits rune slices and converts scalar lengths
with the standard UTF-16 library. It never asks the implementation to supply
the next generated operation's reference document. The separate pinned ot.js
oracle compares components as well as effects. Both models are fallible;
deterministic boundary cases and strict harness failure tests check assumptions
that differential agreement alone cannot establish.

The [oracle guide](../../tools/oracle/README.md) records reproducible commands,
budgets, limits of the evidence and replay/reporting behavior. M1 does not claim
the later engine delivery, persistence or complete S16 system milestones.
