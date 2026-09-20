# 010: No end-to-end encryption in phase 2

Status: accepted

Date: 2026-09-20

Source: [Architecture decision log](../../architecture/obsync-single%20Architecture%20Specification.md#10-decision-log-and-open-questions).

## Context

The server must read text to transform operations.

## Decision

Accept a trusted server that sees plaintext; do not implement end-to-end encryption.

## Consequences

TLS protects transport and storage-layer encryption is a separate decision. The operator can read the vault, and revoked devices retain content already downloaded.
