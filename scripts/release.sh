#!/usr/bin/env bash
#
# Cut a release: update the VERSION file, commit it, tag it and push.
#
# The VERSION file is what stamps the build, because the production stack is
# built by Komodo from compose.yaml and a compose build passes no build
# arguments. Keeping the file and the tag in step is the whole point of this
# script - a tag alone would still deploy as the previous version.
#
# Usage: scripts/release.sh v2.4.0 [--push]

set -euo pipefail

version="${1:-}"
push="${2:-}"

if [[ -z "$version" ]]; then
    echo "Usage: $0 <version> [--push]" >&2
    echo "Example: $0 v2.4.0 --push" >&2
    exit 1
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$ ]]; then
    echo "Version must look like v2.4.0 (got '$version')" >&2
    exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if [[ -n "$(git status --porcelain)" ]]; then
    echo "The working tree has uncommitted changes; commit or stash them first." >&2
    git status --short >&2
    exit 1
fi

if git rev-parse "$version" >/dev/null 2>&1; then
    echo "Tag $version already exists." >&2
    exit 1
fi

echo "$version" > VERSION
git add VERSION
git commit -m "Release $version"
git tag -a "$version" -m "$version"

echo "Tagged $version (VERSION file updated)."

if [[ "$push" == "--push" ]]; then
    branch="$(git rev-parse --abbrev-ref HEAD)"
    git push origin "$branch"
    git push origin "$version"
    echo "Pushed $branch and $version."
else
    echo "Not pushed. Run: git push origin HEAD && git push origin $version"
fi
