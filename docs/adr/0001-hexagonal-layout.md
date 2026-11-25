# 0001. Hexagonal package layout

- Status: Accepted
- Date: 2026-11-23

## Context

The service is a small Go HTTP API with one persistence adapter. The domain, the HTTP transport, and BoltDB need to stay replaceable without pulling each other in.

## Decision

Dependencies point inward. `internal/todo` owns the `Todo` model, the `Repository` port, and the use cases. `internal/storage` implements that port with BoltDB. `internal/httpapi` depends on a small `TodoService` interface and does not import the store. `cmd/api-server` is the composition root: it loads config, opens the database, builds the service, the limiter, and the handler, and owns process shutdown.

Constructors return concrete types (`New`, `NewHandler`, `Open`, `Load`). Interfaces stay next to the package that calls them.

## Consequences

- A new storage engine implements `todo.Repository` and is wired in `main`. The use cases stay unchanged.
- HTTP tests can pass a fake service. Domain tests can pass an in-memory repository.
- `internal/todo` cannot import `net/http` or `bbolt`.
