# 0011. Liveness and readiness

- Status: Accepted
- Date: 2026-11-25

## Context

A pod that cannot serve traffic has to leave the Service. A process that is still running has to stay up while it waits for a dependency. Restarting on a failed dependency turns a short outage into a crash loop.

## Decision

API liveness is `GET /livez`, which checks that the process is running. API readiness is `GET /readyz`, which returns 503 when the process is shutting down or the store ping fails. Store liveness and readiness are TCP connections to port 50051. The store process exits if it cannot open BoltDB, so that port opens only after the file is open.

`terminationGracePeriodSeconds` is 30. Shutdown of either process waits up to 10 seconds.

## Consequences

- An API pod that cannot reach the store stays unready and keeps failing readiness. Kubernetes leaves it out of the Service and keeps the process running.
- A dead process fails liveness and is restarted.
- During shutdown, readiness fails first, then the server finishes in-flight calls.
