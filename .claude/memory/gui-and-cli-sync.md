---
name: gui-and-cli-sync
description: "After any backend/GraphQL/model change, always give GUI-client instructions AND keep the CLI in sync"
metadata:
  node_type: memory
  type: feedback
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Standing rule for every change to plexams.go (Oliver, 2026-06-21):

1. **Always output plexams.gui-agent instructions together with the change** — not only when
   asked. Whenever a GraphQL schema/field/mutation/subscription (or a REST route) changes, or a
   backend change can break an existing GUI query (e.g. a removed field), include the concrete GUI
   adjustment in the same answer. Oliver drives a separate agent in the plexams.gui repo and pastes
   these.

**UPDATE 2026-07-08:** rule 2 ("keep the CLI in sync") is obsolete — the CLI (`cmd/`) was removed;
plexams.go is now server-only (see [[cli-to-gui-migration]]). The GUI is the single facade, so rule
1 is the whole rule now: there is no CLI left to adjust/remove.

**Why:** the GUI is the only facade over the Plexams logic; a backend change that doesn't update the
GUI leaves it broken.

See [[git-workflow]] and [[emails-over-graphql]].
