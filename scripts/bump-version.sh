#!/usr/bin/env bash
# Print the version to publish. If v$(cat VERSION) is already a git tag, bump patch.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
current="$(tr -d ' \n' < VERSION)"
if [[ ! "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "invalid VERSION: ${current}" >&2
	exit 1
fi
if git rev-parse "v${current}" >/dev/null 2>&1; then
	IFS=. read -r major minor patch <<<"$current"
	current="${major}.${minor}.$((patch + 1))"
	printf '%s\n' "$current" >VERSION
fi
printf '%s\n' "$current"
