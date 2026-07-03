---
name: mongotest-without-docker
description: How to run the mongotest-based integration tests when Docker/testcontainers is unavailable
metadata:
  node_type: memory
  type: reference
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

`internal/mongotest.NewDB(t)` uses testcontainers (needs Docker) OR `PLEXAMS_TEST_MONGO_URI`, else the test skips. This sandbox has **no Docker**, so integration tests skip by default.

To verify them green anyway: download a standalone `mongod` (network works, arch aarch64), run it, point the env var at it:

```
curl -sL -o mongo.tgz https://fastdl.mongodb.org/linux/mongodb-linux-aarch64-ubuntu2204-7.0.14.tgz
tar xzf mongo.tgz
./mongodb-*/bin/mongod --dbpath <scratch>/dbdata --port 27099 --nounixsocket --bind_ip 127.0.0.1 &
PLEXAMS_TEST_MONGO_URI="mongodb://127.0.0.1:27099" go test ./...
```

**How to apply:** do this in the scratchpad dir; `pkill -9 -f "mongod --dbpath"` when done. Beware: `pgrep -f "mongod --dbpath"` also matches the pgrep shell wrapper itself — confirm with `ps aux | grep mongod` instead.
