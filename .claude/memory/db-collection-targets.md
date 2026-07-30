---
name: db-collection-targets
description: ReplaceAll takes a typed ReplaceTarget; the old untyped collection name in context.Value is gone
metadata:
  node_type: memory
  type: project
---

`DropAndSave` used to take its target collection as an **untyped string smuggled through `context.Value`** (`db.CollectionName("collectionName")`). Two failure modes: a typo silently created and wrote into a different collection with no compile error, and a missing value panicked on an unchecked type assertion inside `getCollectionSemesterFromContext`.

Replaced on 2026-07-30 by `db.ReplaceAll(ctx, target, objects)` with a typed `db.ReplaceTarget` and one constant per allowed collection (`TargetZPAStudents`, `TargetInvigilatorRequirements`, `TargetSelfInvigilations`, `TargetOtherInvigilations`) — so callers outside `db` still pick a target without naming a collection, but cannot invent one by accident. Adding a target means adding a constant in [db/save.go](db/save.go), which is the point.

`CollectionName`, `getCollectionSemesterFromContext` and `collectionNameFromContext` are gone. So is `Save`, which had no callers.

Because the delete and insert now sit in one place, `ReplaceAll` also runs through `withTransaction` (see [db-transactions.md](db-transactions.md)) and clears with `DeleteMany` so indexes survive.

**Do not reintroduce the pattern.** If a new consumer needs a collection it cannot name, add a `ReplaceTarget` constant — never a string parameter and never a context value.
