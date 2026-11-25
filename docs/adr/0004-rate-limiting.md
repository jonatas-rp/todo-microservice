# 0004. In-memory token-bucket rate limiting

- Status: Accepted
- Date: 2026-11-23

## Context

The API needs a per-client limit that is safe under concurrent requests. The limit is process-local for this version. Health probes must keep working when a client is limited.

## Decision

`internal/ratelimit` owns the token-bucket algorithm and a `Store` port. `storage.Memory` implements the port with one mutex. `Update` runs the callback while that lock is held, so the read-modify-write of a bucket cannot race.

A bucket starts full on first use. Later calls add `elapsedSeconds * perSecond` tokens, capped at capacity, then spend one token when at least one remains. The process is wired with capacity 10 and a refill of 1 token per second.

The client key is the host of `RemoteAddr`. Forwarded-header values are ignored. A denied request gets `429` problem details with detail `"rate limit exceeded"` and the handler is not called. `GET /livez` and `GET /readyz` skip the limiter.

## Consequences

- Two replicas each keep their own buckets, so the effective limit is per pod.
- Restarting the process clears every bucket.
- The limit follows the TCP peer. A reverse proxy that does not preserve the client address will collapse many clients into one key.
