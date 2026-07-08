#!/bin/sh
set -eu

status=0

dirs=$(
    find cmd internal -type f -name '*_test.go' -exec dirname {} \; | sort -u
)

for dir in $dirs; do
    main_test="$dir/main_test.go"
    if [ ! -f "$main_test" ]; then
        echo "missing goleak TestMain: $dir has tests but no main_test.go" >&2
        status=1
        continue
    fi
    if ! grep -q 'goleak\.VerifyTestMain(m)' "$main_test"; then
        echo "missing goleak VerifyTestMain: $main_test" >&2
        status=1
    fi
done

exit "$status"
