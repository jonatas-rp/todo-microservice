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

The API Service in each environment is a ClusterIP. Nothing outside the cluster can reach it until an ingress controller publishes port 80 and rewrites `/dev` and `/prod` away. These overlays use the `nginx` IngressClass and allow traffic only from the `ingress-nginx` namespace.

Use the `docker-desktop` context. Install the controller once per cluster:

```bash
kubectl config use-context docker-desktop
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.15.1/deploy/static/provider/cloud/deploy.yaml
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s
kubectl get svc -n ingress-nginx ingress-nginx-controller
```

On Docker Desktop the Service's external address is `localhost`, so the API is reached at `http://localhost/dev` and `http://localhost/prod`.

Set `spec.hostPath.path` in `deployments/k8s/overlays/development/store-pv.yaml` and `deployments/k8s/overlays/production/store-pv.yaml` before applying. Docker Desktop cannot mount an Ubuntu home path. Use a separate directory per environment under `/run/desktop/mnt/host/`: a Windows drive (`.../c/Users/<you>/...`) survives a restart, and `/run/desktop/mnt/host/wsl` (Ubuntu's `/mnt/wsl`) is cleared when WSL stops.

The store runs as user `65532`, and a hostPath is not made writable by `fsGroup`. Create the directory first or `store-server` exits with `open /data/todos.db: permission denied`:

```bash
mkdir -p /mnt/wsl/todo-service/development /mnt/wsl/todo-service/production
chmod 0777 /mnt/wsl/todo-service/development /mnt/wsl/todo-service/production
```

Set the volume paths to `/run/desktop/mnt/host/wsl/todo-service/development` and `.../production`. If the pod already created the directory, `chmod` it and delete the pod.

Then apply one overlay per environment:

```bash
kubectl apply -k deployments/k8s/overlays/development
kubectl apply -k deployments/k8s/overlays/production
```

| Environment | API | Docs |
| --- | --- | --- |
| Development | `http://localhost/dev/v1/todos` | `http://localhost/dev/v1/docs/` |
| Production | `http://localhost/prod/v1/todos` | `http://localhost/prod/v1/docs/` |


## Documentation

| Doc | What it covers |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | Processes, request path, data layout, Kubernetes objects, and the limits of BoltDB |
| [docs/adr](docs/adr/README.md) | Decisions, in the order they were made |
| [docs/openapi.json](docs/openapi.json) | The HTTP contract |
