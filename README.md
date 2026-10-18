# stele-sync
Phase 2 of the stele project family. This iteration features fault tolerant server-side sync actions over all connected devices.

M0 bootstraps the monorepo and promotes the existing phase 1 worker and plugin.
The phase 2 server shell remains a documented package skeleton. M1 implements
the text-operation core; M2 adds the [FreeDraw annotation core](docs/annot-development.md)
with [compatibility and verification evidence](docs/annot-verification.md).
M3 adds [portable namespaces, classification and safety rules](docs/m3-development.md)
with [verification results and remaining platform checks](docs/m3-verification.md).
M4 adds the `stele.v1` [protocol contract](internal/proto/M4%20proposal%20proto.md),
shared UUIDv7 types, bounded Go codecs and the TypeScript protocol port; see
[M4 verification](docs/m4-verification.md). The server/session/HTTP runtime remains
for later milestones. [M1 performance was rerun](docs/textop-verification.md#2026-10-18-linux-rerun) without changing its thresholds.

See [development setup and M0 verification](docs/development.md), the
[implementation guide](architecture/obsync-single%20Implementation%20Guide.md),
and [architecture decisions](docs/adr/).

With Go 1.27.2, Node 24, Docker Compose, Git Bash (Windows), and kubectl or
kustomize available:

```sh
npm ci --prefix plugin
node tools/tasks.mjs tools
node tools/dev.mjs up
node tools/tasks.mjs ci
```

`make ci` runs the same command. Development services bind only to localhost;
Postgres uses port 55432, Garage S3 uses 3900, and the phase 1 web endpoint uses 3902.
