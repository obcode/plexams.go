---
name: build-binary-cleanup
description: "Always delete the built ./plexams.go binary after testing — it breaks gowatch"
metadata:
  node_type: memory
  type: feedback
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

The project's build convention is `go build -o plexams.go .`, so the binary is named
`plexams.go` (same `.go` extension as source). Oliver runs `gowatch`, which treats the
binary as a Go source file and fails with `plexams.go:1:1: illegal character U+007F`
(the ELF header).

**Always `rm -f plexams.go` after building/testing it.** Better: when I only need to run
the server or a command for a test, prefer `go run .` (no leftover artifact) over
`go build -o plexams.go .`. If I do build it, delete the binary as the last step before
finishing.

**Why:** a leftover `plexams.go` binary silently breaks Oliver's gowatch dev loop.

See [[git-workflow]].
