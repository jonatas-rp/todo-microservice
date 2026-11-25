# 0003. Request-scoped structured logging

- Status: Accepted
- Date: 2026-11-23

## Context

Logs need a stable schema, the source location of the log call, and a request id that cannot leak from one request into another.

## Decision

Each line is one JSON object with fields in this order: `level`, `timestamp` (UTC, RFC 3339 with nanoseconds), `location` (`file:line`, trimmed at `todo-service/`), `request_id` when the logger belongs to a request, `message`, and `context` when the call passes a map.

`Info` and `Error` take `map[string]any`. A nil map omits `context`. `With` returns a copy, so the parent logger is unchanged.

The outermost middleware creates a new logger for the request, stores it only in that request's context, sets `X-Request-ID`, and logs the matched route pattern. The pattern comes from `ServeMux.Handler` before the handler runs. A process logger with no request id is used for startup, shutdown, and any log outside a request. Concurrent writes to the same writer are serialized around the line write only.

## Consequences

- A log line from one request cannot carry another request's id. The logger is not stored on a shared handler.
- Call sites that have nothing extra to record pass `nil`.
- `location` points at the caller of `Info` or `Error`, which makes the file path part of the log contract for anyone grepping it.
