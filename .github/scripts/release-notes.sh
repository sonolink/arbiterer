#!/bin/sh
set -eu

notes=$(awk -v heading="## [$1]" '
    /^## \[/ { if (found) exit; if (index($0, heading) == 1) { found = 1; next } }
    /^\[[^]]+\]: / { if (found) exit }
    found { print }
' CHANGELOG.md)

if [ -z "$(printf '%s' "$notes" | grep -v '^#' | tr -d '[:space:]')" ]; then
    echo "CHANGELOG.md has no notes under '## [$1]'" >&2
    exit 1
fi

printf '%s\n' "$notes"
