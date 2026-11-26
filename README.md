# To-do service

A Go HTTP API for to-do items. Two API replicas call one gRPC process, and that process is the only writer of the BoltDB file. Development and production each run from a kustomize overlay on Docker Desktop Kubernetes, reached through ingress-nginx on port 80 at `/dev` and `/prod`.

The HTTP API is under `/v1/todos`. Creates require an `Idempotency-Key` UUID. Success bodies use a JSON envelope. Errors use `application/problem+json`.

## Run locally

From this directory, start the store, then the API:

```bash
go run ./cmd/store-server
STORE_ADDR=127.0.0.1:50051 go run ./cmd/api-server
```

The API listens on `:8080`. The store listens on `:50051` and opens `todos.db` in the working directory. Docs are at `http://localhost:8080/v1/docs/`.

```bash
go test ./...
```

## Images

```bash
docker build -f Dockerfile.todoserver -t todo-service:v1.0.0 .
docker build -f Dockerfile.store -t store-server:v1.0.0 .
```

## Kubernetes

Use the `docker-desktop` context. Apply one overlay per environment:

```bash
kubectl apply -k deployments/k8s/overlays/development
kubectl apply -k deployments/k8s/overlays/production
```

| Environment | API | Docs |
| --- | --- | --- |
| Development | `http://localhost/dev/v1/todos` | `http://localhost/dev/v1/docs/` |
| Production | `http://localhost/prod/v1/todos` | `http://localhost/prod/v1/docs/` |

The store file for each environment is on the Windows host and is visible in this repo at `data/development` and `data/production`. That directory is gitignored.

## Documentation

| Doc | What it covers |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | Processes, request path, data layout, Kubernetes objects, and the limits of BoltDB |
| [docs/adr](docs/adr/README.md) | Decisions, in the order they were made |
| [docs/openapi.json](docs/openapi.json) | The HTTP contract |
