#!/bin/sh
set -eu

# Prints the body of the '## [Unreleased]' section of CHANGELOG.md, keeping
# only "### Category" headings that actually have notes under them
notes=$(awk -v heading="## [Unreleased]" '
    /^## \[/ {
        if (found) exit
        if (index($0, heading) == 1) { found = 1; next }
    }
    /^\[[^]]+\]: / { if (found) exit }
    found {
        if ($0 ~ /^### /) {
            if (h != "" && b != "") printf "%s\n\n%s\n\n", h, b
            h = $0; b = ""
        } else if (h != "" && $0 != "") {
            b = (b == "" ? $0 : b "\n" $0)
        }
    }
    END {
        if (h != "" && b != "") printf "%s\n\n%s\n", h, b
    }
' CHANGELOG.md)

if [ -z "$notes" ]; then
    echo "CHANGELOG.md has no notes under '## [Unreleased]'" >&2
    exit 1
fi

printf '%s\n' "$notes"
