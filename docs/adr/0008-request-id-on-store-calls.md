# 0008. Request id on store calls

- Status: Accepted
- Date: 2026-11-25
- See also: [0003](0003-request-logging.md)

## Context

A store log line has to name the HTTP request that caused it. The id already exists for the API log. The store process is a different binary, so it cannot read the API request context.

## Decision

`requestLog` stores the id on the request context under the key `"request_id"`, beside the request logger. `internal/store/client` reads that value and sets `request_id` on List, Create, Update, and Delete. The store server builds its logger with `WithRequestID` from that field. A missing value is an empty id.

## Consequences

- The same UUID appears on the API line and the store line for that call.
- Call sites pass the request context through. There is no request-id package.
- Startup and shutdown logs stay on the process logger and omit `request_id`.
