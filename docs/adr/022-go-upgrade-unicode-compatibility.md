# 022: Upgrade Go without changing published path spelling

Status: accepted; supplements 018 and updates 014's toolchain pin

Date: 2026-10-11

## Context

The Go 1.26.6 audit reports reachable standard-library vulnerabilities. The user
explicitly requested the latest stable Go release on m3/namespace, authorizing
this upgrade here instead of the separate PR normally required by guidelines 3.1.
The official download list identifies Go 1.27.2 as that release.

Go 1.27 also selects Unicode 17 in both the standard library and the existing
x/text dependency. An unqualified version bump changes legacy CollisionKey,
Canvas normalization, the server key scheme and the client path contract.
Requiring Unicode 17 on clients would exclude older Obsidian runtimes needlessly.
The existing govulncheck and golangci-lint releases also fail to analyze the new
toolchain's syntax/export data.

## Decision

Pin Go 1.27.2 in go.mod and the worker build image. Both CI Go setup steps read
go.mod. Pin govulncheck v1.9.0 and golangci-lint v2.14.0 and build tools with the
selected Go version. Their existing cache key includes both version files.

Keep WireUnicodeVersion at 15.0.0. NFC treats scalars unassigned in that version
as inert boundaries, following UAX #15 normalization process stability. Use
x/text's versioned Assigned table; the six Go/TS contracts retain their semantics
and the client still requires only Unicode 15. Legacy CollisionKey also retains
Unicode 15 simple lowercasing by leaving newer assignments/mapping targets inert.
An independently measured pre-upgrade checksum covers every Unicode scalar key;
multi-scalar regressions and the existing normalization fixture cover composition,
ordering and stream-safe behavior. Future casing corrections must pass this guard.

Server-only keys use Unicode 17 uppercase, folding and normalization. Explicitly
review the new KeyScheme, and extend the hand-written golden with newly assigned
Cyrillic, Latin, Garay, Tolong Siki and canonical-composition pairs. Wire spelling
and key normalization are separately versioned contracts. M5 still must reject a
stored key-scheme mismatch until an explicit rekey checks collisions. No M5
database or persisted phase 2 namespace exists in this milestone.

## Consequences

Security/toolchain upgrades can proceed without changing phase 1 manifest output
or raising client Unicode requirements. Server keys intentionally identify more
names as equivalent. The key golden continues to fail on an unreviewed Unicode
upgrade, independently of the frozen wire fixture.

Go 1.27's JSON encoder emits a literal replacement character for an invalid UTF-8
fixture label where the old encoder escaped it. Regeneration changes that label's
encoding only; input_base64, parse reason and component remain identical.

Sources: [Go downloads](https://go.dev/dl/),
[Go 1.27 release notes](https://go.dev/doc/go1.27),
[UAX #15 process stability](https://www.unicode.org/reports/tr15/#Stability_Prior_Version),
[x/text Assigned](https://pkg.go.dev/golang.org/x/text/unicode/rangetable#Assigned),
[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck),
[golangci-lint Go support](https://golangci-lint.run/docs/welcome/faq/).
