# Architecture decision records

The system overview and its limits are in [architecture.md](../architecture.md). Decisions are listed here in the order they were made.

| ADR | Title |
| --- | --- |
| [0001](0001-hexagonal-layout.md) | Hexagonal package layout |
| [0002](0002-http-responses.md) | HTTP success envelope and problem details |
| [0003](0003-request-logging.md) | Request-scoped structured logging |
| [0004](0004-rate-limiting.md) | In-memory token-bucket rate limiting |
| [0005](0005-create-idempotency.md) | Idempotent create |
| [0006](0006-public-identifiers.md) | Public UUID and internal sequence |
| [0007](0007-grpc-store.md) | One BoltDB writer behind gRPC |
| [0008](0008-request-id-on-store-calls.md) | Request id on store calls |
| [0009](0009-ingress-path.md) | One ingress, two path prefixes |
| [0010](0010-host-store-volume.md) | BoltDB file on the Windows host |
| [0011](0011-probes.md) | Liveness and readiness |
