#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: $0 VERSION" >&2
    exit 2
fi

version=${1#v}

awk -v version="$version" '
    index($0, "## [" version "] - ") == 1 {
        found = 1
        next
    }
    found && /^---$/ {
        exit
    }
    found {
        print
        content = 1
    }
    END {
        if (!found || !content) {
            exit 1
        }
    }
' CHANGELOG.md
