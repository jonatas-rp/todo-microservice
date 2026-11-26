# API usage

The HTTP API is served by `api-server`. Locally it listens on `http://localhost:8080`. Through the development and production ingresses the same routes are under `http://localhost/dev` and `http://localhost/prod`. The ingress strips that prefix before the process sees the path.

Set `BASE` to the root you are calling, then use the examples below.

```bash
BASE=http://localhost:8080
# BASE=http://localhost/dev
# BASE=http://localhost/prod
```

A successful body is `application/json` with `success`, `message`, and `data`. An error is `application/problem+json` with `type`, `title`, `status`, `detail`, and `instance`. Every response includes `X-Request-ID`.

`GET /livez` and `GET /readyz` are not rate limited. Every other route shares a bucket of 10 requests that refills one token per second. The bucket is keyed by the host of the client connection. A limited call returns `429`.

Create and replace bodies accept only `title` and `completed`. `title` is trimmed and must not be blank. A body larger than 1 MiB is rejected. The public id is a UUID. The interactive reference is `GET /v1/docs/`.

## List to-dos

`GET /v1/todos` returns every item in public-id order. `data` is an empty array when the store has none.

```bash
curl -sS "$BASE/v1/todos"
```

## Create a to-do

`POST /v1/todos` requires an `Idempotency-Key` header that is a UUID. The first call stores the item and returns `201` with a `Location` header of `/v1/todos/{id}`. A later call with the same key returns `200` and the original item, even when the body differs. A missing or non-UUID key returns `400`. A blank title returns `422` and does not consume the key.

```bash
curl -sS -D - -X POST "$BASE/v1/todos" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 6f1c0b3e-2a4d-4e7a-9c1b-8d5e0a7f2b14" \
  -d '{"title":"buy milk","completed":false}'
```

The `Idempotency-Key` is not the item id. Replace and delete use `data.id` from this response.

## Replace a to-do

`PUT /v1/todos/{id}` replaces `title` and `completed`. The public id stays the same. An unknown id returns `404`.

```bash
curl -sS -X PUT "$BASE/v1/todos/0bcdbae1-c2e2-40f7-8ee3-d11fdf286055" \
  -H "Content-Type: application/json" \
  -d '{"title":"buy oats","completed":true}'
```

## Delete a to-do

`DELETE /v1/todos/{id}` removes the item and returns `200` with `data` set to `null`. An unknown id returns `404`.

```bash
curl -sS -X DELETE "$BASE/v1/todos/0bcdbae1-c2e2-40f7-8ee3-d11fdf286055"
```

## OpenAPI document

`GET /v1/openapi.json` returns the contract in `docs/openapi.json`.

```bash
curl -sS "$BASE/v1/openapi.json"
```

## Swagger UI

`GET /v1/docs/` returns the HTML UI. It loads the OpenAPI document from this same API.

```bash
curl -sS "$BASE/v1/docs/"
```

## Liveness

`GET /livez` reports that the process is running. It does not check the store.

```bash
curl -sS "$BASE/livez"
```

## Readiness

`GET /readyz` reports that this process can reach the store. A store that cannot be pinged returns `503`.

```bash
curl -sS "$BASE/readyz"
```

## Health

`GET /health` reports that the process is running. Unlike the probes, this route is rate limited.

```bash
curl -sS "$BASE/health"
```
