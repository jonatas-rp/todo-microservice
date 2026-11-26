# 0001. Hexagonal package layout

- Status: Accepted
- Date: 2026-11-23
- Superseded in part by [0007](0007-grpc-store.md). The API process dials the store. The store process opens BoltDB.

## Context

The service is a small Go HTTP API with one persistence adapter. The domain, the HTTP transport, and BoltDB need to stay replaceable without pulling each other in.

## Decision

Dependencies point inward. `internal/todo` owns the `Todo` model, the `Repository` port, and the use cases. `internal/storage` implements that port with BoltDB. `internal/httpapi` depends on a small `TodoService` interface and does not import the store. `cmd/api-server` and `cmd/store-server` are the composition roots: each loads config, wires concrete types, and owns process shutdown. Which process opens the file is decided in [0007](0007-grpc-store.md).

Constructors return concrete types (`New`, `NewHandler`, `Open`, `Load`). Interfaces stay next to the package that calls them.

## Consequences

- A new storage engine implements `todo.Repository` and is wired in `main`. The use cases stay unchanged.
- HTTP tests can pass a fake service. Domain tests can pass an in-memory repository.
- `internal/todo` cannot import `net/http` or `bbolt`.
