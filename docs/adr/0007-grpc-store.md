# 0007. One BoltDB writer behind gRPC

- Status: Accepted
- Date: 2026-11-25
- Supersedes in part: [0001](0001-hexagonal-layout.md)

## Context

bbolt holds an exclusive lock on a read-write open. The API Deployment runs two replicas. Those replicas have to read and write one dataset, including idempotency keys.

## Decision

`store-server` is the only process that opens the BoltDB file. It serves `todo.Repository` over gRPC on port 50051. `api-server` always dials `STORE_ADDR` and implements the port with `internal/store/client`. The Kubernetes store Deployment has one replica and mounts the volume. The API Deployment has two replicas and mounts nothing.

`DB_PATH` belongs to the store process. The API process leaves the file closed.

## Consequences

- Both API replicas see the same items and the same idempotency keys.
- A store restart pauses writes until that pod is Ready again. The volume keeps the file across that restart.
- A second store replica against the same file brings the exclusive lock back.
- Local runs start `cmd/store-server`, then `cmd/api-server` with `STORE_ADDR` set.
