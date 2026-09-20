# 014: Mechanical phase 1 promotion during M0

Status: accepted

Date: 2026-09-20

## Context

M0 requires the inherited tests to pass after a mechanical package move.
The imported phase 1 code predates the stricter phase 2 lint, dependency, and
TypeScript rules. Rewriting its behavior during bootstrap would obscure that proof.
ADR 013 is reserved for the Freedraw verification required in M2.

## Decision

Promote the existing tracked phase 1 files into the root module, leaving the
history-preserved legacy snapshot intact. Change only module/import/package names,
formatting, and relative test-data paths. Keep published phase 1 command and plugin
identifiers. The objects directory keeps its inherited Go package name, store.

Apply explicit lint exceptions only to enumerated inherited Go filenames.
Keep vet, race detection, staticcheck (except the existing QF1001 suggestion in
s3.go), dependency checks, formatting, and audit enabled. Inherited GC alone keeps
its old manifest/store dependencies until M10; new GC files obey the layer table.
Standard-library imports are allowed by each layer; storage SDKs belong only to
adapters, and the WebSocket library belongs only to transport. Core production
code additionally rejects I/O package imports and direct clock reads.

The inherited plugin retains its compiler/lint baseline. New src/core code uses
strict-type-checked lint, no Obsidian/adapter imports, and tsconfig.core.json with
exact optional properties and explicit overrides. M13 owns the remaining plugin
restructure. A nested plugin/go.mod prevents npm packages containing Go source
from joining the server's ./... package set.

Pin Go 1.26.6 within phase 1's existing minor, update Vitest to 4.1.11, and refresh
affected transitive npm dependencies to satisfy M0's new security gates.
Keep all existing contract fixture content unchanged.

## Consequences

New files receive all enabled rules; inherited files retain recorded lint debt.
Remove their exceptions as those files are rewritten in their owning milestones.
This is a bootstrap compatibility exception, not permission to add new logic
to exempt files without addressing the relevant guidelines.

M0 does not claim phase 2 correctness: fuzzing starts in M1–M4, the engine simulator
in M7, and server end-to-end tests in M14. Their tasks explicitly report deferral
until implementations exist. All CI jobs run on every PR for now, so required
checks cannot disappear through workflow path filters.
