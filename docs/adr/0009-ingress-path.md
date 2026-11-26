# 0009. One ingress, two path prefixes

- Status: Accepted
- Date: 2026-11-25

## Context

Development and production run in one cluster. Each API has to be reachable from the workstation. The docs UI loads CSS, JavaScript, and the OpenAPI document from URLs in the HTML.

## Decision

The API Service is ClusterIP on port 8080. External traffic enters through the ingress-nginx controller on port 80. Development matches `/dev(/|$)(.*)` and production matches `/prod(/|$)(.*)`. The annotation `nginx.ingress.kubernetes.io/rewrite-target: /$2` strips that prefix before the process sees the request.

`PATH_PREFIX` is `/dev` or `/prod` in that environment, and empty when a client calls the process directly. The docs UI stays mounted at `/v1/docs/`. The prefix is written into the asset URLs (`BasePath`, `SwaggerJSON`) and into the OpenAPI `servers` entry. `InternalBasePath` stays `/v1/docs/` so the handler still matches the rewritten path.

## Consequences

- Both environments share port 80 and are told apart by path: `http://localhost/dev/v1/todos` and `http://localhost/prod/v1/todos`.
- The docs UI is `http://localhost/dev/v1/docs/` or `http://localhost/prod/v1/docs/`.
- A request that reaches the process at `/v1/docs/` with an empty `PATH_PREFIX` still serves the UI. The asset URLs then have no prefix.
