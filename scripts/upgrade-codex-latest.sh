#!/bin/sh
# upgrade-codex-latest: repeatedly advance to the next stable Codex release
# until CODEX_VERSION reaches the latest published stable version.

set -e

ALREADY_LATEST_EXIT=10

while true; do
    if ./scripts/upgrade-codex-next.sh; then
        continue
    fi
    status=$?
    if [ "$status" -eq "$ALREADY_LATEST_EXIT" ]; then
        exit 0
    fi
    exit "$status"
done
