---
name: mongotest-without-docker
description: How to run the mongotest-based integration tests when Docker/testcontainers is unavailable
metadata:
  node_type: memory
  type: reference
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

`internal/mongotest.NewDB(t)` uses testcontainers (needs Docker) OR `PLEXAMS_TEST_MONGO_URI`, else the test skips. This sandbox has **no Docker**, so integration tests skip by default.

Set **`PLEXAMS_TEST_MONGO_REQUIRED=1`** to turn that skip into a failure. CI does both: `.github/workflows/ci.yml` runs a `mongo:8` service container, points `PLEXAMS_TEST_MONGO_URI` at it and sets `REQUIRED`, so a MongoDB that fails to come up breaks the build instead of quietly skipping every integration test. Without it "green" and "never ran" are indistinguishable.

To verify them green anyway: download a standalone `mongod` (network works, arch aarch64), run it, point the env var at it:

```
curl -sL -o mongo.tgz https://fastdl.mongodb.org/linux/mongodb-linux-aarch64-ubuntu2204-7.0.14.tgz
tar xzf mongo.tgz
./mongodb-*/bin/mongod --dbpath <scratch>/dbdata --port 27099 --nounixsocket --bind_ip 127.0.0.1 &
PLEXAMS_TEST_MONGO_URI="mongodb://127.0.0.1:27099" go test ./...
```

**How to apply:** do this in the scratchpad dir; `pkill -9 -f "mongod --dbpath"` when done. Beware: `pgrep -f "mongod --dbpath"` also matches the pgrep shell wrapper itself — confirm with `ps aux | grep mongod` instead.

## Prefer a throwaway mongod over the dev MongoDB — history

Pointing `PLEXAMS_TEST_MONGO_URI` at the **dev MongoDB** (`$MONGODB_URI`, port 27013) used to corrupt real data, because `mongotest` isolated only the *semester* database while the master data (rooms, NTAs, study programs, users, templates) lived in a hardcoded global one. `AddRoom` in `plexams/anny_integration_test.go` added four rooms per run without cleanup and `SetAnnyConfig` overwrote the real config; ~50 runs left 200 junk rooms in `plexams.rooms` and wiped the real `personalizationnames`. It also made `TestAnnyBookedBySlot` flaky (~1 in 20) because the test read the previous runs' leftover rooms.

Fixed on 2026-07-30: the global database name is a field on `db.DB`, reached via `db.globalDatabase()`, and `mongotest` points it at `<testdb>_global` and drops it in cleanup. `TestGlobalDatabaseIsIsolated` in `internal/mongotest/mongotest_test.go` guards it (verified to fail when the isolation is removed). See [db-indexes.md](db-indexes.md) for the other place that touches the global DB.

Running against the dev MongoDB is safe again, but a throwaway mongod is still the better default — it also keeps `plexams_test_*` databases off the dev server.
