# stele-sync
Phase 2 of applications in the obsync project family. This iteration features fault tolerant server-side sync actions over all connected devices.

M0 bootstraps the monorepo and promotes the existing phase 1 worker and plugin.
The phase 2 server shell remains a documented package skeleton. M1 implements
the text-operation core; M2 adds the [FreeDraw annotation core](docs/annot-development.md)
with [compatibility and verification evidence](docs/annot-verification.md).

See [development setup and M0 verification](docs/development.md), the
[implementation guide](architecture/obsync-single%20Implementation%20Guide.md),
and [architecture decisions](docs/adr/).

With Go 1.26.6, Node 24, Docker Compose, Git Bash (Windows), and kubectl or
kustomize available:

```sh
npm ci --prefix plugin
node tools/tasks.mjs tools
node tools/dev.mjs up
node tools/tasks.mjs ci
```

`make ci` runs the same command. Development services bind only to localhost;
Postgres uses port 55432, Garage S3 uses 3900, and the phase 1 web endpoint uses 3902.
