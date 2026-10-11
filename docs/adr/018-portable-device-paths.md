# 018: Strict device paths and a versioned filesystem key

Status: accepted

Date: 2026-10-11

## Context

Published phase 1 manifests depend on the original sanitizer and simple lowercase
collision key. Full Unicode folding alone misses dotless-i uppercase collisions.
Device sidecars can exceed the old 120-byte component ceiling.

## Decision

Freeze phase 1 output. Rename its Path function to SanitizePath and mutable
Normalize function variable to NFC; new Path values come from strict Parse.
Device paths are valid UTF-8, NFC, relative, slash-separated and portable, with
200 UTF-8 bytes per component and 512 in total. Reasons and zero-based component
indexes (or -1 for whole-path failures) are cross-language fixtures. Structural,
character and length checks run in separate phases for stable precedence.

Use `NFC(Fold(NFD(SimpleUpper(path))))` on the server only. Pin the algorithm and
Go uppercase, x/text folding and normalization Unicode versions in KeyScheme and
a hand-written golden. M5 stores the scheme and refuses mismatches until an
explicit rekey checks collisions. Never use phase 1 CollisionKey as path_key.

The strict API additionally rejects COM¹/²/³ and LPT¹/²/³, console names
CONIN$/CONOUT$, and device stems with spaces before an extension, which Windows
reserves but phase 1 omitted. Sanitize can therefore fail after SanitizePath;
the bridge must fall back. Normalization before character removal can also expose
a non-NFC legacy result. Do not silently alter existing manifests to fix either.

The TS validator uses a generated assignment/stream-safe table from the same Go
version. Native String.normalize alone disagrees for future-assigned scripts and
for x/text's inserted grapheme joiners after long nonstarter runs. Freeze both
behaviors as contract data; the client does not compute uniqueness keys.

## Consequences

Over-collisions are intentional. Windows requires long-path support; filesystem
aliases such as generated 8.3 names and symlinks are shell concerns, not Unicode
key equivalences. Passing golden cases is not a proof of every filesystem's
behavior. Actual volume measurements and their limits live in
[the verification record](../m3-verification.md). macOS/iOS measurements were
explicitly deferred by the user for this implementation.

Sources: [Unicode canonical caseless matching](https://www.unicode.org/versions/Unicode17.0.0/core-spec/chapter-3/),
[Microsoft filename rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file),
[Go case folding](https://pkg.go.dev/golang.org/x/text/cases).
