#!/usr/bin/env bash
# Build and run the todo-service image for end-to-end tests.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${E2E_IMAGE:-todo-service:e2e}"
STORE_IMAGE="${E2E_STORE_IMAGE:-store-server:e2e}"
NAME="${E2E_NAME:-todo-service-e2e}"
STORE_NAME="${E2E_STORE_NAME:-${NAME}-store}"
NETWORK="${E2E_NETWORK:-${NAME}-net}"
STATE_DIR="${E2E_STATE_DIR:-$ROOT/test/e2e/.state}"
STATE_FILE="$STATE_DIR/$NAME.env"

usage() {
  cat <<'EOF'
Usage: scripts/e2e.sh <command>

Commands:
  up            Build both images, start the store and the API, wait until
                /readyz succeeds, and write BASE_URL to the state file.
  down          Stop and remove the containers and the network.
  stop          Send SIGTERM via docker stop and print EXIT_CODE=<n>.
  run-bad-port  Run a one-shot container with PORT=nope; exit with its status.
EOF
}

require_docker() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker is required" >&2
    exit 1
  }
}

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_ready() {
  local url="$1"
  local deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  echo "timed out waiting for $url" >&2
  docker logs "$STORE_NAME" >&2 || true
  docker logs "$NAME" >&2 || true
  return 1
}

build_images() {
  docker build -f "$ROOT/Dockerfile.store" -t "$STORE_IMAGE" "$ROOT"
  docker build -f "$ROOT/Dockerfile.todoserver" -t "$IMAGE" "$ROOT"
}

cmd_up() {
  require_docker
  mkdir -p "$STATE_DIR"
  build_images
  docker rm -f "$NAME" "$STORE_NAME" >/dev/null 2>&1 || true
  docker network create "$NETWORK" >/dev/null 2>&1 || true

  docker run -d --name "$STORE_NAME" --network "$NETWORK" \
    --tmpfs /data:uid=65532,gid=65532,mode=0770 \
    -e DB_PATH=/data/todos.db \
    "$STORE_IMAGE" >/dev/null

  local port
  port="$(free_port)"
  docker run -d --name "$NAME" --network "$NETWORK" \
    -e "STORE_ADDR=${STORE_NAME}:50051" \
    -p "127.0.0.1:${port}:8080" \
    "$IMAGE" >/dev/null

  local base="http://127.0.0.1:${port}"
  wait_ready "$base/readyz"

  cat >"$STATE_FILE" <<EOF
BASE_URL=$base
CONTAINER_NAME=$NAME
STORE_CONTAINER=$STORE_NAME
NETWORK=$NETWORK
IMAGE=$IMAGE
EOF
  echo "BASE_URL=$base"
}

cmd_down() {
  require_docker
  if [[ -f "$STATE_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$STATE_FILE"
  fi
  docker rm -f "${CONTAINER_NAME:-$NAME}" "${STORE_CONTAINER:-$STORE_NAME}" >/dev/null 2>&1 || true
  docker network rm "${NETWORK_NAME:-$NETWORK}" >/dev/null 2>&1 || true
  rm -f "$STATE_FILE"
}

cmd_stop() {
  require_docker
  if [[ -f "$STATE_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$STATE_FILE"
  fi
  local container="${CONTAINER_NAME:-$NAME}"
  docker stop -t 10 "$container" >/dev/null
  local code
  code="$(docker inspect -f '{{.State.ExitCode}}' "$container")"
  echo "EXIT_CODE=$code"
}

cmd_run_bad_port() {
  require_docker
  docker build -f "$ROOT/Dockerfile.todoserver" -t "$IMAGE" "$ROOT" >/dev/null
  local tmp="todo-e2e-badport-$$"
  set +e
  docker run --rm --name "$tmp" -e PORT=nope "$IMAGE"
  local code=$?
  set -e
  exit "$code"
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    up) cmd_up ;;
    down) cmd_down ;;
    stop) cmd_stop ;;
    run-bad-port) cmd_run_bad_port ;;
    -h|--help|help|"") usage; [[ -n "$cmd" ]] || exit 1 ;;
    *) echo "unknown command: $cmd" >&2; usage; exit 1 ;;
  esac
}

main "$@"
