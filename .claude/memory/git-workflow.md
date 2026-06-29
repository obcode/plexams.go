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
- He's already on the feature branch — don't create a new branch unless asked.
- Do NOT commit session noise like `.claude/settings.json` permission-allowlist changes.
- End commit messages with the Co-Authored-By trailer.
