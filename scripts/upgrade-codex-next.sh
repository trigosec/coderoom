#!/bin/sh
# upgrade-codex-next: advance the pinned Codex version by one stable release,
# re-record transcripts, compare broad behavioral signals, and run replay plus
# live integration validation.
#
# Exit code 10 means CODEX_VERSION is already at the latest stable version.

set -e

TRANSCRIPT_ROOT=internal/agent/codex/testdata/transcripts
ALREADY_LATEST_EXIT=10

CURRENT=$(grep -v '^\#' CODEX_VERSION | tr -d '[:space:]')
LATEST=$(npm view @openai/codex version 2>/dev/null)

if [ "$CURRENT" = "$LATEST" ]; then
    echo "codex is already at latest ($CURRENT)"
    exit "$ALREADY_LATEST_EXIT"
fi

NEXT=$(node -e "
const versions = $(npm view @openai/codex versions --json 2>/dev/null);
const current = '$CURRENT';
const stable = versions.filter(v => /^\\d+\\.\\d+\\.\\d+$/.test(v));
const idx = stable.indexOf(current);
if (idx === -1) throw new Error('current version not found in stable list: ' + current);
if (idx + 1 >= stable.length) throw new Error('no stable version after current: ' + current);
console.log(stable[idx + 1]);
")

echo "current: $CURRENT"
echo "next:    $NEXT"

copy_transcript_inputs() {
    src_version=$1
    dst_version=$2
    src_dir="$TRANSCRIPT_ROOT/$src_version"
    dst_dir="$TRANSCRIPT_ROOT/$dst_version"

    rm -rf "$dst_dir"
    mkdir -p "$dst_dir"
    for case_dir in "$src_dir"/*; do
        [ -d "$case_dir" ] || continue
        case_name=$(basename "$case_dir")
        mkdir -p "$dst_dir/$case_name"
        find "$case_dir" -maxdepth 1 -type f \
            \( -name 'prompt.md' -o -name 'conversation.md' -o -name 'conversation-*.md' \) \
            -exec cp {} "$dst_dir/$case_name/" \;
    done
}

run_replay_suite() {
    go test ./internal/transcript ./cmd/codex-upgrade-compare ./cmd/codex-record ./cmd/codex-replay ./internal/agent/codex
}

run_live_suite() {
    CODEX_VERSION_OVERRIDE=$1 go test -tags integration ./internal/agent/codex -run 'TestClientLiveContextRetention|TestClientLiveTranscriptScenarios'
}

echo "--- upgrading Codex transcripts to @$NEXT ---"
echo "phase: scaffold transcript inputs"
copy_transcript_inputs "$CURRENT" "$NEXT"

echo "phase: record transcripts"
go run ./cmd/codex-record "$NEXT"

echo "phase: compare transcript expectations"
go run ./cmd/codex-upgrade-compare \
    "$TRANSCRIPT_ROOT/$CURRENT" \
    "$TRANSCRIPT_ROOT/$NEXT"

echo "phase: replay test suite"
run_replay_suite

echo "phase: live integration suite"
run_live_suite "$NEXT"

sed -i "s/^[0-9].*/$(echo "$NEXT")/" CODEX_VERSION
echo ""
echo "updated CODEX_VERSION to $NEXT"
echo "review transcript changes and commit CODEX_VERSION + updated transcript fixtures."
