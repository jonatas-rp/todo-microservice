# 0005. Idempotent create

- Status: Accepted
- Date: 2026-11-23

## Context

A retried `POST /v1/todos` must not insert a second item. The check and the insert have to commit together, because a crash between them would either lose the key or lose the item.

## Decision

`POST /v1/todos` requires an `Idempotency-Key` header whose value is a UUID. A missing key or a value that is not a UUID is `400`.

BoltDB keeps a second bucket, `idempotency`, beside `todos`. Inside the create transaction, a known key is decoded and returned with `created=false`. The sequence is not advanced and nothing is written. A new key stores the encoded item under the public id and stores the same bytes under the idempotency key.

The first response is `201` with message `"todo created"` and a `Location` header. A replay is `200` with message `"todo already created"` and the original item, including when the retry body differs.

## Consequences

- The key and the item are visible together or not at all.
- Replays do not allocate a new public id or a new sequence number.
- Title validation runs before the repository lookup. A replay whose title is blank is `422` and does not return the stored item.
- Keys are kept for the life of the database file. There is no expiry.
