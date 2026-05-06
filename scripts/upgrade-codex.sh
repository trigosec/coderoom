#!/bin/sh
# upgrade-codex: advance the pinned Codex version to latest, validating each
# intermediate version against the committed schemas and integration tests.
#
# Usage: make upgrade-codex
#
# On success: updates CODEX_VERSION to the latest version.
# On failure: exits with the version that introduced a breaking change.

set -e

CURRENT=$(grep -v '^\#' CODEX_VERSION | tr -d '[:space:]')
LATEST=$(npm view @openai/codex version 2>/dev/null)

if [ "$CURRENT" = "$LATEST" ]; then
    echo "codex is already at latest ($CURRENT)"
    exit 0
fi

echo "current: $CURRENT"
echo "latest:  $LATEST"

# Fetch all published versions between current and latest (exclusive of current).
VERSIONS=$(node -e "
const versions = $(npm view @openai/codex versions --json 2>/dev/null);
const current = '$CURRENT';
const latest = '$LATEST';
const idx = versions.indexOf(current);
const end = versions.indexOf(latest);
console.log(versions.slice(idx + 1, end + 1).join('\n'));
")

if [ -z "$VERSIONS" ]; then
    echo "no intermediate versions found"
    exit 0
fi

echo ""
echo "versions to validate:"
echo "$VERSIONS"
echo ""

for VERSION in $VERSIONS; do
    echo "--- testing @openai/codex@$VERSION ---"
    CODEX_VERSION_OVERRIDE=$VERSION go test -tags integration ./internal/agent/codex/...
    echo "ok"
done

# All versions passed — update the pinned version.
# Requires GNU sed (Linux). On macOS, install gnu-sed via Homebrew.
sed -i "s/^[0-9].*/$(echo "$LATEST")/" CODEX_VERSION
echo ""
echo "updated CODEX_VERSION to $LATEST"
echo "review schema changes (if any) and commit CODEX_VERSION + updated schemas."
