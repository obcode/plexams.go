#!/usr/bin/env bash
# Link Claude Code's per-project memory directory to the copy kept in this repo, so it
# survives DevContainer rebuilds (the harness reads/writes user-level ~/.claude, which is
# ephemeral; the repo's .claude/memory is committed and persistent).
#
# Run once after a container (re)build, e.g. from the devcontainer postCreateCommand:
#   "postCreateCommand": "bash .claude/link-memory.sh"
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$SCRIPT_DIR/.." && pwd)"
SOURCE="$REPO/.claude/memory"

# Claude Code derives the project slug from the workspace path (every non-alphanumeric
# character becomes a dash), e.g. /workspace/plexams.go -> -workspace-plexams-go.
SLUG="$(printf '%s' "$REPO" | sed 's/[^a-zA-Z0-9]/-/g')"
TARGET="$HOME/.claude/projects/$SLUG/memory"

mkdir -p "$SOURCE" "$(dirname "$TARGET")"

# If a real (non-symlink) memory dir already exists, fold its files into the repo copy
# and back it up, so nothing is lost the first time this runs.
if [ -e "$TARGET" ] && [ ! -L "$TARGET" ]; then
	cp -an "$TARGET"/. "$SOURCE"/ 2>/dev/null || true
	mv "$TARGET" "$TARGET.bak.$(date +%s)"
fi

ln -sfn "$SOURCE" "$TARGET"
echo "linked $TARGET -> $SOURCE"
