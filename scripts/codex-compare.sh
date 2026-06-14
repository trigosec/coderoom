#!/bin/sh
# codex-compare: diff transcript `expect` front matter between two Codex
# transcript version directories.
#
# Usage:
#   ./scripts/codex-compare.sh <from-version> <to-version>

set -eu

TRANSCRIPT_ROOT=internal/agent/codex/testdata/transcripts

if [ "$#" -ne 2 ]; then
    echo "usage: ./scripts/codex-compare.sh <from-version> <to-version>" >&2
    exit 2
fi

if ! command -v yq >/dev/null 2>&1; then
    echo "codex-compare: yq is required but was not found in PATH" >&2
    exit 2
fi

FROM_VERSION=$1
TO_VERSION=$2
FROM_DIR="$TRANSCRIPT_ROOT/$FROM_VERSION"
TO_DIR="$TRANSCRIPT_ROOT/$TO_VERSION"

if [ ! -d "$FROM_DIR" ]; then
    echo "codex-compare: missing source version directory $FROM_DIR" >&2
    exit 2
fi
if [ ! -d "$TO_DIR" ]; then
    echo "codex-compare: missing target version directory $TO_DIR" >&2
    exit 2
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

extract_expect() {
    transcript_path=$1
    output_path=$2

    awk '
        /^---$/ { delimiters++; next }
        delimiters == 1 { print }
        delimiters == 2 { exit }
    ' "$transcript_path" | yq '.expect' > "$output_path"
}

DIFF_FOUND=0

for case_dir in "$FROM_DIR"/*; do
    [ -d "$case_dir" ] || continue
    case_name=$(basename "$case_dir")
    from_transcript="$FROM_DIR/$case_name/output.transcript"
    to_transcript="$TO_DIR/$case_name/output.transcript"

    echo "=== $case_name ==="

    if [ ! -f "$from_transcript" ]; then
        echo "missing source transcript: $from_transcript"
        DIFF_FOUND=1
        continue
    fi
    if [ ! -f "$to_transcript" ]; then
        echo "missing target transcript: $to_transcript"
        DIFF_FOUND=1
        continue
    fi

    from_expect="$TMP_DIR/$case_name.from.expect.yaml"
    to_expect="$TMP_DIR/$case_name.to.expect.yaml"
    extract_expect "$from_transcript" "$from_expect"
    extract_expect "$to_transcript" "$to_expect"

    if ! diff -u "$from_expect" "$to_expect"; then
        DIFF_FOUND=1
    fi
done

for case_dir in "$TO_DIR"/*; do
    [ -d "$case_dir" ] || continue
    case_name=$(basename "$case_dir")
    if [ ! -d "$FROM_DIR/$case_name" ]; then
        echo "=== $case_name ==="
        echo "extra target case: $case_name"
        DIFF_FOUND=1
    fi
done

exit "$DIFF_FOUND"
