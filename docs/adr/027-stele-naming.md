# 027: Stele names and subprotocol versioning

Status: accepted

Date: 2026-10-18

Source: [Accepted M4 proposal](../../internal/proto/M4%20proposal%20proto.md).

## Context

The phase 2 protocol must freeze its external name before clients ship.
Phase 1 already ships as stele-pull, while deployed infrastructure still uses
obsync names that cannot be changed without moving data.

## Decision

The project is stele; phase 1 is stele-pull, phase 2 stele-sync. Use
cmd/stele-server, cmd/stelectl, stele.v1, stele_ device credentials and metrics,
stele_changes NOTIFY, stele-sync Garage bucket/plugin id, stele database/role and
Stele-* state headers. Token formatting, generic JSON and structured logs redact
credentials; only EncodeClient emits a hello credential.

Version negotiation uses Sec-WebSocket-Protocol. No common version means accept
then immediately close 4426 unsupported_protocol so browsers can surface it.
Hello carries only a token; welcome returns the token's device identity.

Keep deployed phase 1 bucket/namespace obsync, Argo CD apps garage-obsync and
obsync-worker, and the obsync:pull-scope rule key until deliberate migration.
The existing phase 1 plugin retains its published stele-pull id.

## Consequences

Rename unused command placeholders and development phase 2 defaults now.
Existing developer Postgres volumes need deliberate role/database migration or
a fresh volume; changing environment variables does not migrate stored data.
No deployed resources are renamed by this change.
