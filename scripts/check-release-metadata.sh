#!/bin/sh

set -eu

if ! grep -Fq '## [Unreleased]' CHANGELOG.md; then
    echo "CHANGELOG.md must contain an Unreleased section" >&2
    exit 1
fi

versions=$(sed -n 's/^## \[\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)\] - .*/\1/p' CHANGELOG.md)

if [ -z "$versions" ]; then
    echo "CHANGELOG.md contains no stable release sections" >&2
    exit 1
fi

duplicates=$(printf '%s\n' "$versions" | sort | uniq -d)
if [ -n "$duplicates" ]; then
    echo "duplicate changelog versions:" >&2
    printf '%s\n' "$duplicates" >&2
    exit 1
fi

for version in $versions; do
    notes=$(./scripts/release-notes.sh "$version")
    if [ -z "$notes" ]; then
        echo "release notes for $version are empty" >&2
        exit 1
    fi
done

echo "release metadata is valid"
