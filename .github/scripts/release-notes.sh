#!/bin/sh
set -eu

# Builds GitHub release notes; outputs roughly:
#
#   # Major/Minor/Patch: vX.Y.Z
#
#   <CHANGELOG.md contents for this version>
#
#   ## Links
#   ...

version=$1

notes=$(sh "$(dirname "$0")/changelog-extract.sh" "$version")
notes=$(printf '%s' "$notes" | sed '/./,$!d')

previous=$(git tag --list | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | awk -v cur="v$version" '
    { if ($0 == cur) { print prev; exit } prev = $0 }
')

if [ -z "$previous" ]; then
    type=Major
else
    type=$(awk -v a="v$version" -v b="$previous" '
        BEGIN {
            split(a, x, ".")
            split(b, y, ".")
            print (x[1] != y[1]) ? "Major" : (x[2] != y[2]) ? "Minor" : "Patch"
        }')
fi

year=$(date +%Y)

cat <<EOF
# $type: v$version

$notes

## Links
<!-- [Examples](https://examples.arbiterer.sonolink.dev) -->
- [Discord Server](https://discord.com/invite/tPHVWBPedt)

<i><code>© $year <a href="https://github.com/sonolink">SonoLink Development Team</a></code></i>
EOF
