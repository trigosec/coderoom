#!/bin/sh
set -eu

spaced_title='Code'' Room'
capitalized='Code''room'
upper_spaced='CODE'' ROOM'
sentence_case='Code'' room'

if matches=$(git grep -n \
    -e "$spaced_title" \
    -e "$capitalized" \
    -e "$upper_spaced" \
    -e "$sentence_case" \
    -- .); then
    echo "error: invalid coderoom branding" >&2
    echo 'Use lowercase "coderoom" for the product name.' >&2
    echo "$matches" >&2
    exit 1
fi
