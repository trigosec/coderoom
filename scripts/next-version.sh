#!/usr/bin/env bash
set -euo pipefail

year=$(date +%Y)
latest=$(git tag --list "v0.${year}.*" --sort=-version:refname | head -n1)

if [ -z "$latest" ]; then
    echo "v0.${year}.1"
else
    n=$(echo "$latest" | awk -F. '{print $3}')
    echo "v0.${year}.$((n + 1))"
fi
