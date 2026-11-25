# 0006. Public UUID and internal sequence

- Status: Accepted
- Date: 2026-11-23

## Context

Clients need a stable id in URLs and JSON. The store also needs a monotonic sequence from BoltDB. Publishing that sequence would expose record order and the size of the collection.

## Decision

`Todo` has two identifiers. `PublicID` is a UUID v4 from `github.com/google/uuid`, created in the domain when the item is built. It is the JSON field `id`, the `todos` bucket key, and the `{id}` path value for update and delete. `ID` is a `uint64` assigned with `NextSequence` inside the create transaction, after the idempotency check misses. Its JSON tag is `-`, so HTTP responses omit it. The stored record in the `todos` bucket keeps both fields (`id` and `public_id`).

Update loads the existing record, replaces title and completed, and writes it back so the sequence stays on the row. There is no second bucket mapping the public id to the sequence. The public id is already the key.

## Consequences

- A response looks like `{"id":"<uuid>","title":"...","completed":false}`. The sequence exists only in the database file.
- Put and delete resolve in one bucket lookup.
- The sequence is assigned only for a create that inserts. A replay returns the original sequence.
- Listing order follows BoltDB key order, which is the UUID byte order, not the sequence order.
