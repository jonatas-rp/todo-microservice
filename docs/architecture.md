# Architecture

The to-do service runs as two processes. `api-server` serves the JSON HTTP API and always dials the store. `store-server` is the only process that opens the BoltDB file. Decisions are recorded in [adr/](adr/README.md).

## Packages

Dependencies point inward. `internal/todo` does not import HTTP, BoltDB, or gRPC. `cmd/api-server` and `cmd/store-server` wire the concrete types.

| Package | Role |
| --- | --- |
| `cmd/api-server` | Load config, dial the store, build the service, limiter, and handler, shut down on SIGINT or SIGTERM |
| `cmd/store-server` | Open BoltDB, serve the gRPC store, stop gracefully on SIGINT or SIGTERM |
| `internal/config` | API: `PORT` (default `8080`), `LOG_LEVEL`, `STORE_ADDR`, optional `PATH_PREFIX`. Store: `GRPC_ADDR` (default `:50051`), `DB_PATH` (default `todos.db`) |
| `internal/todo` | `Todo`, title checks, `Repository` port, use cases |
| `internal/storage` | BoltDB adapter for `todo.Repository`, and the in-memory adapter for `ratelimit.Store` |
| `internal/store/pb` | Generated gRPC messages |
| `internal/store/client` | gRPC client that implements `todo.Repository` |
| `internal/store/server` | gRPC server in front of the BoltDB adapter |
| `internal/httpapi` | Routes, JSON envelope, problem details, middleware |
| `internal/ratelimit` | Token bucket. Capacity 10, refill 1 token per second |
| `internal/logger` | One JSON line per call. A request logger lives only in that request's context |

```text
cmd/api-server
  → config, logger, store/client, ratelimit, todo, httpapi
cmd/store-server
  → config, logger, storage, store/server
store/client
  → store/pb, todo
store/server
  → store/pb, storage
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

1. `requestLog` assigns `X-Request-ID` with `uuid.NewString()`, stores that id on the request context under `"request_id"`, stores a new logger in the request context, and logs the matched route pattern. The store client copies the id onto each gRPC call.
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

`PATH_PREFIX` (`/dev` or `/prod` behind the ingress, empty for a direct client) is written into the Swagger UI asset URLs and the OpenAPI `servers` entry. The UI stays mounted at `/v1/docs/`. The ingress strips the prefix before the process sees the request, so the browser requests those assets with the prefix still on the URL.

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

`config.Load` reads the environment. The API process dials `STORE_ADDR`. The store process opens the BoltDB file with mode `0600`, waits up to one second for the file lock, and creates both buckets. The HTTP server uses a 5 second read-header timeout. On SIGINT or SIGTERM the API marks itself unready, stops accepting new connections, and waits up to 10 seconds for in-flight requests. The store process stops the gRPC server the same way.

BoltDB serializes writes inside the process: one read-write transaction at a time, and many read-only transactions. The rate-limit map uses one mutex, and the bucket update runs while that lock is held.

## Limitations

These follow from BoltDB and from state that lives in the process.

### One file, one writer

bbolt v1.5.0 takes an exclusive lock on a read-write `Open`. A second process that opens the same file for writing would update the meta pages and the free pages on its own and corrupt the file, so the library refuses that open. This service's `Open` times out after one second.

The Kubernetes install keeps that lock inside one process. `todo-store` is a single replica, mounts the PVC, and serves `todo.Repository` over gRPC. The API Deployment has two replicas and does not mount the database file. Both replicas call `STORE_ADDR`, so they read and write the same dataset, including idempotency keys.

`store-server` is still a single writer. A restart pauses writes until that pod is Ready again. The PVC keeps the file across that restart. Adding another store replica against the same file brings the exclusive lock back.

Read-only mode (`Options.ReadOnly`) uses a shared lock so several processes can read one frozen file. While they hold it, no process can open that file for writing, and a process that already has it open for writing blocks the readers. This API creates, updates, and deletes, so read-only mode does not produce one shared live dataset.

A lockfile in front of BoltDB only chooses which single process may open the file. The other replicas still cannot read or write it.

### Kubernetes objects

Shared API manifests live in `deployments/k8s/base/api/`. Shared store manifests live in `deployments/k8s/base/store/`. Allow rules that both environments share live in `deployments/k8s/components/`. Each overlay is one kustomization: the namespace, both bases, those components, and the values that differ (`PATH_PREFIX`, the ingress path, and resource requests). Resource names stay `todo-service` for the API and `todo-store` for the database.

| Overlay | Apply | Namespace |
| --- | --- | --- |
| Development | `kubectl apply -k deployments/k8s/overlays/development/` | `development` |
| Production | `kubectl apply -k deployments/k8s/overlays/production/` | `production` |

Build the API image as `todo-service:v1.0.0` from `Dockerfile.todoserver`, and the store image as `store-server:v1.0.0` from `Dockerfile.store`.

| Object | Purpose |
| --- | --- |
| Namespace (overlay) | `development` or `production`, from that overlay’s `namespace.yaml` |
| ServiceAccount `todo-service` | API pod identity with no Role or ClusterRole bindings; token not auto-mounted |
| ServiceAccount `todo-store` | Store pod identity, same constraints |
| ConfigMap `todo-service` | `PORT`, `LOG_LEVEL`, `STORE_ADDR`, and the overlay’s `PATH_PREFIX` |
| ConfigMap `todo-store` | `GRPC_ADDR`, `LOG_LEVEL`, and `DB_PATH` |
| PersistentVolume `todo-store-data-<namespace>` | Static hostPath `/run/desktop/mnt/host/c/Users/jorp5/arancia-todo-data/<namespace>`, class `manual`, reclaim `Retain`. WSL reads it through the gitignored `data` symlink |
| PersistentVolumeClaim `todo-store-data` | Binds that volume and mounts it at `/data` on the store pod |
| Deployment `todo-service` | Two replicas, no volume, liveness `/livez`, readiness `/readyz` |
| Deployment `todo-store` | One replica, the PVC, image `store-server` |
| Service `todo-service` | ClusterIP port 8080 → container port 8080. This Service does not strip `/dev` or `/prod` |
| Ingress `todo-service` | `/dev` (development) or `/prod` (production) is rewritten away before the API. The Service port is 8080 |
| Service `todo-store` | ClusterIP port 50051 → the store container |
| NetworkPolicy `todo-service-deny-all` | Base denies all traffic. `components/api-networkpolicy` allows TCP 8080 from the `ingress-nginx` namespace and TCP 50051 to `todo-store` |
| NetworkPolicy `todo-store` | Base denies all traffic. `components/store-networkpolicy` allows TCP 50051 from pods labeled `app: todo-service` |

`GET /readyz` on an API pod pings the store over gRPC. A store that cannot open BoltDB exits before it listens. An API that cannot reach the store fails readiness and stays running. Liveness stays on `/livez` for the API and on TCP 50051 for the store.

`kubectl delete -k` on an overlay deletes that namespace and both workloads. The BoltDB file stays in the host directory. The store claim is ReadWriteOnce, and the store Deployment rolls with `maxSurge: 1` and `maxUnavailable: 0`, so the new pod mounts the volume after the old pod releases it.

The base NetworkPolicies deny all ingress and egress. `components/api-networkpolicy` allows the nginx Ingress controller (`namespace` labeled `kubernetes.io/metadata.name=ingress-nginx`) to reach the API Pods on port 8080, and allows those Pods to dial the store on port 50051. `components/store-networkpolicy` allows TCP 50051 only from pods labeled `app: todo-service`. Other egress, including DNS, still needs extra rules if you rely on it. The CNI must enforce NetworkPolicy for this to apply. Development lowers the API and store container resources with patches; production uses the store resources in `deployments/k8s/base/store/` and sets the API resources in the overlay.

### Rate limit is per process

Each process has its own buckets of 10 tokens, refilling at 1 per second. A client spread across two replicas gets about twice the allowance. Restarting a process clears its buckets.

The key is the host of `RemoteAddr`. Forwarded headers are ignored. Behind a Service, an ingress, or a port-forward, that host is often the node, the proxy, or the port-forward client, so unrelated callers share one bucket on each pod.

### Idempotency keys do not expire

Keys remain in the `idempotency` bucket for the life of the store file. Title validation runs before the key lookup, so a replay whose title is blank is 422 and does not return the stored item.
