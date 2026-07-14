---
name: git-workflow
description: "Oliver's git workflow for plexams.go — semantic-release, feature branches, commit in steps"
metadata:
  node_type: memory
  type: feedback
  originSessionId: 6285039b-3933-4bb1-a8f3-24a7355c4a1d
---

Oliver uses **semantic-release** and **feature branches** for plexams.go. He works on a
feature branch (not main) and wants work **committed in steps as it is completed**, not
piled into one big commit at the end.

**Why:** semantic-release derives version bumps + changelog from Conventional Commits;
clean stepwise history matters.

**How to apply:**
- Use Conventional Commit messages (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:` …).
- Commit each completed logical step right away (don't batch unrelated work).
- **"Committen und pushen" = the FULL flow by default, no need to ask each time** (Oliver
  2026-07-14: "auf main mergen und pushen. Wie immer. Das will ich nicht immer sagen müssen"):
  when work is on a feature branch, finish by `git checkout main && git merge --ff-only <branch>
  && git push origin main`, then delete the branch (local + remote). Don't stop at "pushed the
  branch, want a PR?" — merge to main and push unless he explicitly says otherwise. No PR needed.
- Committing **directly to main** is also fine (Oliver 2026-07-06: "direkt auf main ist wunderbar"),
  e.g. small follow-up fixes — branch only when it adds value or he asks.
- Do NOT commit session noise like `.claude/settings.json` permission-allowlist changes.
- **DO always commit AND push `.claude/memory/`** (Oliver 2026-07-08: "committen und pushen"). The home memory dir symlinks to the repo's tracked `.claude/memory/`, so memory edits are versioned; keep them in sync on the remote. Commit them (own commit is fine) and `git push` whenever memory changes.
- End commit messages with the Co-Authored-By trailer.
