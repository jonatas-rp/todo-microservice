# 0002. HTTP success envelope and problem details

- Status: Accepted
- Date: 2026-11-23

## Context

Callers need one shape for success and one shape for failure. `net/http` ServeMux answers unknown routes and wrong methods with plain text, which would break that contract.

## Decision

Every successful response is `application/json`:

```json
{"success": true, "message": "todo created", "data": {}}
```

`success` is a boolean. `data` is the payload, or `null` when there is nothing to return. Delete is `200` with `"data": null` and message `"todo deleted"`.

Every error is `application/problem+json` (RFC 9457) with `type` `about:blank`, `title`, `status`, `detail`, and `instance`. The mapping is:

| Condition | Status |
| --- | --- |
| Malformed JSON or unknown fields | 400 |
| Missing `Idempotency-Key` | 400 |
| Unknown route or id | 404 |
| Wrong method on a known route | 405, `Allow` kept |
| Body larger than 1 MiB | 413 |
| Blank title | 422 |
| Rate limit exceeded | 429 |
| Unexpected failure or panic | 500, detail `"internal error"` |
| Readiness check failed | 503 |

Unexpected errors are logged with the request id. The response detail stays generic.

Middleware wraps the mux from the outside in: `requestLog`, `rateLimit`, `recoverPanic`, `problemFallback`, then ServeMux. `problemFallback` replaces the mux's plain-text 404 and 405 bodies and keeps the `Allow` header. `recoverPanic` re-panics `http.ErrAbortHandler` and otherwise writes a 500 problem when the handler has not already written a status.

## Consequences

- Clients branch on `success` and on the HTTP status. They do not parse plain-text errors.
- A delete that succeeds still has a JSON body, so clients that only accept `204` need to follow this contract.
- Panic stacks stay in the log. They are not copied into the problem body.
