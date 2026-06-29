---
name: gui-and-cli-sync
description: "After any backend/GraphQL/model change, always give GUI-client instructions AND keep the CLI in sync"
metadata:
  node_type: memory
  type: feedback
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Two standing rules for every change to plexams.go (Oliver, 2026-06-21):

1. **Always output plexams.gui-agent instructions together with the change** — not only when
   asked. Whenever a GraphQL schema/field/mutation/subscription changes (or a backend change can
   break an existing GUI query, e.g. a removed field), include the concrete GUI adjustment in the
   same answer. Oliver drives a separate agent in the plexams.gui repo and pastes these.

2. **Keep the CLI (`cmd/`) in sync — adjust or remove affected commands.** When the data model or
   logic changes, update the matching CLI command or delete it if obsolete. The goal: Oliver must
   not be able to break/corrupt data later by running a CLI command that no longer matches the new
   model. Treat the CLI as a first-class consumer, like the GUI.

**Why:** GUI and CLI are both facades over the same Plexams logic; a backend change that updates
only one leaves the other broken or dangerous.

See [[git-workflow]] and [[emails-over-graphql]].
