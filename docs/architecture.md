# Architecture

The to-do service is one Go process. It serves a JSON HTTP API and stores items in a BoltDB file opened by that process. Decisions are recorded in [adr/](adr/README.md).

## Packages

Dependencies point inward. `internal/todo` does not import HTTP or BoltDB. `cmd/api-server` is the only place that wires concrete types.

| Package | Role |
| --- | --- |
| `cmd/api-server` | Load config, open the database, build the service, limiter, and handler, shut down on SIGINT or SIGTERM |
| `internal/config` | `PORT` (default `8080`) and `DB_PATH` (default `todos.db`). The process listens on all interfaces |
| `internal/todo` | `Todo`, title checks, `Repository` port, use cases |
| `internal/storage` | BoltDB adapter for `todo.Repository`, and the in-memory adapter for `ratelimit.Store` |
| `internal/httpapi` | Routes, JSON envelope, problem details, middleware |
| `internal/ratelimit` | Token bucket. Capacity 10, refill 1 token per second |
| `internal/logger` | One JSON line per call. A request logger lives only in that request's context |

```text
cmd/api-server
  → config, logger, storage, ratelimit, todo, httpapi
httpapi
  → todo, ratelimit, logger
storage
  → todo, ratelimit
ratelimit
  → (no application packages)
todo
  → (no application packages)
```

## Request path

`NewHandler` registers routes on `net/http` ServeMux, then wraps the mux from the outside in:

1. `requestLog` assigns `X-Request-ID` with `uuid.NewString()`, stores a new logger in the request context, and logs the matched route pattern.
2. `rateLimit` charges the token bucket for the host of `RemoteAddr`. `GET /livez`, `GET /readyz`, and `/v1/docs/` are not charged.
3. `recoverPanic` turns a panic into a 500 problem response. `http.ErrAbortHandler` is re-panicked.
4. `problemFallback` replaces ServeMux's plain-text 404 and 405 bodies and keeps the `Allow` header.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/v1/todos` | 200, every item, empty array when none exist. Order is BoltDB key order, which is the public id, not the sequence |
| POST | `/v1/todos` | Requires a UUID `Idempotency-Key`. 201 `todo created` on first insert, 200 `todo already created` on replay. `Location` is `/v1/todos/{id}` |
| PUT | `/v1/todos/{id}` | 200 full replace. Both `title` and `completed` are required |
| DELETE | `/v1/todos/{id}` | 200, `"data": null` |
| GET | `/v1/openapi.json` | 200, the OpenAPI document in `docs/openapi.json` |
| GET | `/v1/docs/` | 200, Swagger UI for that document. Assets under this path are not rate limited |
| GET | `/livez` | 200 when the process is running |
| GET | `/readyz` | 200 when both BoltDB buckets exist. 503 when storage is missing or the ping fails |
| GET | `/health` | 200 process check. This route is rate limited |

Success bodies are `application/json`:

```json
{"success": true, "message": "todo created", "data": {}}
```

Errors are `application/problem+json` (RFC 9457) with `type` `about:blank`. Unexpected failures log the real error and respond with detail `internal error`.

## Data

A to-do has a public UUID and a hidden sequence:

| Field | Where it is visible | How it is assigned |
| --- | --- | --- |
| `PublicID` (`json:"id"`) | Response, URL, BoltDB key | UUID v4 in the domain, before the write |
| `ID` (`json:"-"`) | BoltDB record only | `NextSequence` inside the create transaction |
| `Title` | Response and record | Trimmed, rejected when blank (422) |
| `Completed` | Response and record | Taken from the request body |

The file has two buckets. `todos` is keyed by the public id and stores `id`, `public_id`, `title`, and `completed`. `idempotency` stores the same encoded item under the `Idempotency-Key`. The key check and the insert commit in one transaction. A known key returns the original item and does not take another sequence number. Update loads the row, changes title and completed, and writes it back so the sequence stays on that row.

## Process

`config.Load` reads the environment. `storage.Open` opens the BoltDB file with mode `0600` and waits up to one second for the file lock, then creates both buckets. The HTTP server uses a 5 second read-header timeout. On SIGINT or SIGTERM the process stops accepting new connections and waits up to 10 seconds for in-flight requests.

BoltDB serializes writes inside the process: one read-write transaction at a time, and many read-only transactions. The rate-limit map uses one mutex, and the bucket update runs while that lock is held.

## Limitations

These follow from BoltDB and from state that lives in the process. They show up as soon as more than one replica is running.

### One file, one writer

bbolt v1.5.0 takes an exclusive lock on a read-write `Open`. A second process that opens the same file for writing would update the meta pages and the free pages on its own and corrupt the file, so the library refuses that open. This service's `Open` times out after one second.

Read-only mode (`Options.ReadOnly`) uses a shared lock so several processes can read one frozen file. While they hold it, no process can open that file for writing, and a process that already has it open for writing blocks the readers. This API creates, updates, and deletes on every replica, so read-only mode does not produce one shared live dataset.

A lockfile in front of BoltDB only chooses which single process may open the file. The other replicas still cannot read or write it. Sharing the path through a volume does not remove the lock. Each replica needs its own file. The files are not replicated and are not merged.

### Replicas diverge

The Service can send each request to a different pod.

- A create stored on one pod is missing from a list on the other.
- An update or delete can return 404 because that pod never stored the id.
- The same `Idempotency-Key` on two pods inserts two items, with two public ids. A replay returns the original item only when it reaches the pod that stored the key.
- `NextSequence` starts at 1 in each file. Clients do not see those numbers. The two files are still separate sequences.

### Deployment with a shared PVC

The challenge asks for a Deployment with two replicas. The manifests also mount one ReadWriteOnce PVC (`todo-service-data`) at `/data` for `DB_PATH=/data/todos.db`.

That combination does not give two healthy writers:

- BoltDB allows only one process to open the file for writing. The second Pod's `Open` times out after one second.
- Readiness then fails on that Pod (`/readyz` → 503), so it stays out of the Service endpoints or flaps while the Deployment restarts it.
- On a single-node cluster such as Docker Desktop, both Pods can often mount the same RWO volume. The file lock still blocks the second opener. On a multi-node cluster, RWO usually schedules only one Pod on the node that holds the volume; the other Pod may stay Pending.
- The PVC does keep the database file across a Pod restart for whichever replica successfully opens it. It does not create a shared, consistent dataset for two running replicas.

Honest options for a later version: one replica, a StatefulSet with one PVC per Pod (still divergent state), or a client/server database that accepts many writers. This install keeps `replicas: 2` as required and documents the limitation instead of silently dropping to one replica.

### Kubernetes objects

Shared manifests live in `deployments/k8s/base/`. Each overlay owns its `Namespace` (`namespace.yaml`) and sets `namespace:` to the same name, plus an `environment` label and resource patches (development is smaller than production). Resource names stay `todo-service` (and friends); environments are separated by namespace:

| Overlay | Apply | Namespace |
| --- | --- | --- |
| Development | `kubectl apply -k deployments/k8s/overlays/development/` | `development` |
| Production | `kubectl apply -k deployments/k8s/overlays/production/` | `production` |

Build the image as `todo-service:dev` for Docker Desktop before applying.

| Object (base name) | Purpose |
| --- | --- |
| Namespace (overlay) | `development` or `production`, created by that overlay’s `namespace.yaml` |
| ServiceAccount `todo-service` | Pod identity with no Role or ClusterRole bindings; token not auto-mounted |
| ConfigMap `todo-service` | `PORT`, `LOG_LEVEL`, and `DB_PATH` |
| PersistentVolumeClaim `todo-service-data` | 1Gi RWO volume mounted at `/data` |
| Deployment `todo-service` | Two replicas, resource requests/limits, liveness `/livez`, readiness `/readyz`, shared PVC |
| Service `todo-service` | LoadBalancer port 80 → container port 8080 |
| Ingress `todo-service` | HTTP under `/dev` (development) or `/prod` (production); nginx rewrites to the app paths |
| NetworkPolicy `todo-service-deny-all` | Base denies all traffic; overlays allow TCP 8080 from the `ingress-nginx` namespace |

`GET /readyz` pings the local BoltDB open in that process. A Pod that cannot open the shared file is not ready.

Base NetworkPolicy denies all ingress and egress. Each overlay patches it so only the nginx Ingress controller (`namespace` labeled `kubernetes.io/metadata.name=ingress-nginx`) may reach the Pods on port 8080. Egress stays denied. LoadBalancer traffic that does not come from that controller, and DNS egress, still need extra rules if you rely on them. The CNI must enforce NetworkPolicy for this to apply.

### Rate limit is per process

Each process has its own buckets of 10 tokens, refilling at 1 per second. A client spread across two replicas gets about twice the allowance. Restarting a process clears its buckets.

The key is the host of `RemoteAddr`. Forwarded headers are ignored. Behind a Service, an ingress, or a port-forward, that host is often the node, the proxy, or the port-forward client, so unrelated callers share one bucket on each pod.

### Idempotency keys do not expire

Keys remain in the `idempotency` bucket for the life of that pod's file. Title validation runs before the key lookup, so a replay whose title is blank is 422 and does not return the stored item.
