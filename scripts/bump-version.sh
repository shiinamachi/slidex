#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

usage() {
  cat >&2 <<'USAGE'
usage: scripts/bump-version.sh patch|minor|major|<version>

Bumps VERSION, syncs duplicated version metadata, and creates
release-notes/<version>.md from release-notes/_template.md.
USAGE
}

if [[ "$#" != "1" ]]; then
  usage
  exit 2
fi

target="$1"
current="$(tr -d '[:space:]' < VERSION)"
if [[ ! "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'VERSION must contain a production base version, got %s\n' "$current" >&2
  exit 2
fi

IFS=. read -r major minor patch <<<"$current"
case "$target" in
  patch)
    patch=$((patch + 1))
    next="${major}.${minor}.${patch}"
    ;;
  minor)
    minor=$((minor + 1))
    next="${major}.${minor}.0"
    ;;
  major)
    major=$((major + 1))
    next="${major}.0.0"
    ;;
  *)
    next="$target"
    if [[ ! "$next" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf 'target version must be patch, minor, major, or an exact production version, got %s\n' "$target" >&2
      exit 2
    fi
    ;;
esac

if [[ "$next" == "$current" ]]; then
  printf 'target version must differ from current VERSION %s\n' "$current" >&2
  exit 2
fi

template="release-notes/_template.md"
note="release-notes/${next}.md"
if [[ ! -f "$template" ]]; then
  printf 'missing release note template: %s\n' "$template" >&2
  exit 1
fi
if [[ -e "$note" ]]; then
  printf 'release note already exists: %s\n' "$note" >&2
  exit 1
fi

printf '%s\n' "$next" > VERSION
go run ./cmd/slidex sync-version-metadata
mkdir -p release-notes
while IFS= read -r line || [[ -n "$line" ]]; do
  printf '%s\n' "${line//\{\{VERSION\}\}/$next}"
done < "$template" > "$note"

printf 'bumped VERSION from %s to %s\n' "$current" "$next"
printf 'created %s\n' "$note"
