# M3 namespace core

The [accepted proposal](../internal/core/vpath/M3-core-namespace-proposal.md)
is implemented by `internal/core/{vpath,classify,rules}` and the pure client ports
in `plugin/src/core`. This milestone supplies decisions and contracts; the M4/M5/M7
protocol, database and engine shells remain their existing documented skeletons.
It does not activate phase 2 sync in the phase 1 plugin.

## Implementation and component constraints

- Phase 1 uses SanitizePath and NFC. Its sanitizer, fallback, numbered Canvas names
  and CollisionKey keep their published outputs. Strict device parsing and Key
  are separate. The inherited portable tests still assert the same outputs.
- Parse validates UTF-8/NFC before structure, then character rules, then byte
  limits. PathError.Component is zero-based, or -1 for a whole-path error. Empty
  Path is an absent-path sentinel; production rule inputs come from Parse.
- KeyScheme pins algorithm, Go uppercase, x/text case folding and normalization
  versions. On Go 1.26.6 these tables are 15.0.0. The host's newer Go must not
  silently regenerate the golden: use the pinned toolchain. Key is server-only.
- The client pins Unicode 15 assignment boundaries and x/text stream-safe
  nonstarter counts through schema/vpath/normalization.json. Native JS NFC alone
  differs for newer assignments and long combining runs. Generated regressions
  cover both; the client treats future assignments as inert starters and inserts
  the same combining grapheme joiners as Go. The port needs a runtime supporting
  at least Unicode 15; supported app runtimes remain part of M13 platform testing.
- Classify uses complete bytes and the existing annot.Parse, including its
  materialized-byte ceiling. Limits.Validate exposes M2's existing limit validation
  without parsing a dummy document. The TS recognition-only port mirrors strict
  JSON syntax, raw scalar/key spelling, duplicate checks and annotation budgets;
  it does not replace the future annotation operation port.
- Config.Classify must equal the creation classifier. Otherwise rename class
  checks could disagree with configured code extensions or size limits. Every
  Decide call receives Config so a custom CanvasRoot also governs puts/deletes.
- RenameRequest.SidecarHolder closes an omission in the proposal: both group
  targets can collide. A sidecar member must pair with the primary and the target,
  be live, be authorized, and retain its name category. Reject the entire group
  before mutation if any member fails. Namespace lookup still belongs inside the
  engine transaction; a pure callback does not provide locking by itself.
- FreeName continues numeric suffixes without integer overflow, preserves the
  longest configured compound extension, sanitizes device labels, and returns
  lookup errors unchanged. Numbered PDFs reserve 11 bytes for their paired
  sidecars. Total-path overflow is an error; it does not silently move directories.
- M2's AnnotReplace remains head-only. Stale replacements become opaque archives;
  ordinary B edits require unpruned tombstones. The retention owner, not core,
  derives the pruning cutoff. A's gap age is measured from the server base commit.
- DeleteGuard stores a bounded array by value. The extra slot keeps a full rolling
  window plus the current bucket, expiring at bucket end plus Window. It never
  forgets early, mutates prior values, counts confirmations/system actions, or
  advances state on rejection. Negative counts fail closed; accepted time does
  not regress. Client nanoseconds are bigint, not imprecise floating timestamps.

## Corrections to the proposed interface and tables

The exported settlement constant is SettleClassChange: Go cannot define two
ClassChange identifiers in one package. DecideEdit/Put/Replace/Delete take Config,
and RenameRequest adds SidecarHolder. Config adds Classify, bounds MaxAttempts to
10,000, compound suffixes to 64 bytes, and guard buckets to 256. Negative/future
bases are InvalidBase; a content operation for the wrong class is ClassChange.

Microsoft also reserves superscript device aliases COM¹/²/³ and LPT¹/²/³,
console handles CONIN$/CONOUT$, and device stems with spaces before an extension.
Parse rejects these. Phase 1 cannot change output, so Sanitize may return a strict
error after successful legacy sanitization; the bridge must use Fallback. Removing
controls can also expose a non-NFC legacy string. These cases are explicit errors,
not silently changed historical manifests.

A non-pairing .annot.json is always C, including when json appears in TextExts.
Otherwise a conflict sidecar could accidentally become text. Settle also waits on
an existing A/B file newly classified C with Demotion=Kept: metadata size checks
already returned C, so the content stage no longer has a TooLarge reason. Existing
C remains C, regardless of later valid bytes. Default text extensions are only
md/txt/mdx; the spec's additional code-extension list remains configurable.

Both server and client derive the expected class from metadata before Confirm.
The server compares its result to the client's claim; passing a client-supplied C
directly into Confirm would bypass the authoritative classification check.

## Verification and CI

Tests were introduced before each new core implementation, then expanded with
boundary regressions. Shared fixtures are generated by Go tests; they establish
Go/TS agreement, not an independent oracle. The key golden is explicitly
hand-written. The guard property checks an independent timestamp event list,
and naming properties use occupied key sets.

```sh
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs contract
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs namespace-acceptance
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs fixtures
GOTOOLCHAIN=go1.26.6 node tools/tasks.mjs fixtures-check
```

The discovered core matrix includes all M3 coverage, property and fuzz checks.
Nightly uses 100,000 cases per M3 property and one hour per fuzz target; PRs use
1,000 property cases and 60 seconds. The local acceptance task uses 100,000 cases,
strictly greater than 90% coverage and ten minutes for each target. Contract and
fixture drift gates include all six M3 JSON files, and verify tracked fixtures.
A Windows NTFS exclusive-create probe is mandatory inside the existing stable
Core result gate, so no branch-protection context changes are needed. Logs are
uploaded on success or failure. The task's ordinary unit environment disables
optional local filesystem probes to avoid accidental host-dependent runs.

[Verification results](m3-verification.md) record what was actually run. The
Windows hosted check is configured, not claimed to have run locally. macOS/iOS
checks were deferred at the user's request. Android path-event behavior and the
FreeDraw partial-write observation still require a suitable application environment;
the provisional 10-second quiet default stays in place.

## Authoritative references

- [Unicode §3.13, canonical caseless matching](https://www.unicode.org/versions/Unicode17.0.0/core-spec/chapter-3/): normalization around folding. M3 deliberately adds simple uppercase and pins its older runtime tables; it is an over-approximation, not a claim to use Unicode 17 tables.
- [Unicode normalization process stability](https://www.unicode.org/reports/tr15/#Stability_Prior_Version): treat characters unassigned in the pinned version as inert; the stream-safe section also explains the bounded nonstarter policy.
- [Microsoft filename rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file): forbidden characters, trailing dot/space behavior, device aliases and long-path limitations.
- [Microsoft CreateFile console names](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilea#consoles): CONIN$ and CONOUT$ refer to consoles, so strict paths reject them too.
- [Go x/text/cases](https://pkg.go.dev/golang.org/x/text/cases): full folding and versioned tables. Construct the folder per call; no shared mutable caser.
- [Apple File System Reference](https://developer.apple.com/support/downloads/Apple-File-System-Reference.pdf): filesystem naming context; it does not substitute for the deferred actual-volume measurement.
- [Go fuzzing documentation](https://go.dev/doc/security/fuzz/): deterministic fuzz targets, native corpus retention and race-enabled execution.
- [M2's pinned FreeDraw evidence](annot-development.md): sidecar naming, structural quarantine, recovery writes and remote-write deferral. M3 reuses the measured format and does not relax those limits.
