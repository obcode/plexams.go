---
name: db-transactions
description: withTransaction makes multi-document writes atomic; falls back on a standalone mongod; replica set needs a keyfile in production
metadata:
  node_type: memory
  type: project
---

Three write paths touch several documents and were not atomic ([db/transaction.go](db/transaction.go), added 2026-07-30): `AddStudentReg`/`RemoveStudentReg` (registration + Primuss counter — the documented cause of the drift [db-indexes.md](db-indexes.md) and `ValidateDBReferences` report), `AddExamToSlot` (delete + insert the plan entry), `ReplacePlannedRooms` (clear + refill the room plan). They now go through `db.withTransaction`.

**The fallback is deliberate, do not turn it into an error.** Transactions require a replica set; a standalone mongod cannot run them. `detectTransactionSupport` checks `hello().setName` once at connect and logs `MongoDB is not a replica set: multi-document writes are not atomic`. Without the fallback, every standalone deployment — including production before the replica-set rollout, and any developer machine — would break.

**Inside `withTransaction`, fn MUST use the context it is handed.** That context carries the session; an operation using the outer context silently runs outside the transaction and is not rolled back. This is why the wrapped bodies were extracted into `addStudentReg`/`removeStudentReg` rather than left inline.

**`Drop` is not allowed inside a transaction.** `ReplacePlannedRooms` therefore empties with `DeleteMany({})`. That is better anyway: a drop also discarded the indexes `EnsureIndexes` created, which only came back on the next start. Watch out for the other ~23 `Drop(ctx)` calls in `db/` — any of them that should become transactional needs the same treatment. Also note `InsertMany` rejects an empty batch, hence the explicit empty-list guard.

**Production needs a keyfile.** mongod *refuses to start* with `--replSet` while authorization is enabled and no keyfile is present (`security.keyFile is required when authorization is enabled with replica sets`). Since a release deploys `deploy/docker-compose.yml` automatically, the lines there are intentionally commented out — activating them before the keyfile exists on the host stops Mongo entirely, not just writes. Runbook: "MongoDB Replica Set aktivieren" in [deploy/README.md](deploy/README.md). The DevContainer needs no keyfile (no auth) and already runs `--replSet rs0`, initiated idempotently by its healthcheck.

Tests ([db/transaction_test.go](db/transaction_test.go)) assert **both** topologies: against a replica set a failing insert rolls back and the original room plan survives; against a standalone the partial state is asserted explicitly, so the fallback is documented rather than silently tolerated. Note `InsertMany` is ordered, so the standalone case leaves the *first* document of the batch behind — not an empty collection. To run against a replica set locally, start a throwaway mongod with `--replSet` and `rs.initiate()` it (see [mongotest-without-docker.md](mongotest-without-docker.md)); `?directConnection=true` in the URI avoids host-name resolution of the member.
