#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

usage() {
  cat >&2 <<'USAGE'
usage: scripts/create-canary-release-note.sh [--base-version VERSION] [--timestamp YYYYMMDDHHMMSS] [--commit-sha SHA] [--release-version VERSION] [--notes-dir DIR]

Creates release-notes/canary/<base-version>/<timestamp>.md from
release-notes/canary/_template.md without overwriting an existing note.
When --timestamp is omitted, the timestamp is derived from the selected
commit's commit time in UTC.
USAGE
}

base_version=""
timestamp=""
commit_sha=""
release_version=""
notes_dir="${SLIDEX_RELEASE_NOTES_DIR:-release-notes}"

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --base-version)
      base_version="${2:-}"
      shift 2
      ;;
    --timestamp)
      timestamp="${2:-}"
      shift 2
      ;;
    --commit-sha|--commit)
      commit_sha="${2:-}"
      shift 2
      ;;
    --release-version)
      release_version="${2:-}"
      shift 2
      ;;
    --notes-dir)
      notes_dir="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$base_version" ]]; then
  base_version="$(tr -d '[:space:]' < VERSION)"
fi
if [[ ! "$base_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'base version must be an exact production version, got %s\n' "$base_version" >&2
  exit 2
fi

if [[ -z "$commit_sha" ]]; then
  commit_sha="$(git rev-parse --verify HEAD)"
fi

if [[ -z "$timestamp" ]]; then
  timestamp="$(TZ=UTC git show -s --format=%cd --date=format-local:%Y%m%d%H%M%S "$commit_sha")"
fi
if [[ ! "$timestamp" =~ ^[0-9]{14}$ ]]; then
  printf 'canary timestamp must be YYYYMMDDHHMMSS, got %s\n' "$timestamp" >&2
  exit 2
fi

expected_release_version="${base_version}-canary.${timestamp}"
if [[ -z "$release_version" ]]; then
  release_version="$expected_release_version"
fi
if [[ "$release_version" != "$expected_release_version" ]]; then
  printf 'release version must be %s for this canary note, got %s\n' "$expected_release_version" "$release_version" >&2
  exit 2
fi

template="${notes_dir}/canary/_template.md"
note="${notes_dir}/canary/${base_version}/${timestamp}.md"
if [[ ! -f "$template" ]]; then
  printf 'missing canary release note template: %s\n' "$template" >&2
  exit 1
fi
if [[ -e "$note" ]]; then
  printf 'canary release note already exists: %s\n' "$note" >&2
  exit 1
fi

mkdir -p "$(dirname "$note")"
while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line//\{\{BASE_VERSION\}\}/$base_version}"
  line="${line//\{\{TIMESTAMP\}\}/$timestamp}"
  line="${line//\{\{RELEASE_VERSION\}\}/$release_version}"
  line="${line//\{\{COMMIT_SHA\}\}/$commit_sha}"
  printf '%s\n' "$line"
done < "$template" > "$note"

if grep -Eq '\{\{(BASE_VERSION|TIMESTAMP|RELEASE_VERSION|COMMIT_SHA)\}\}' "$note"; then
  printf 'canary release note still contains unresolved placeholders: %s\n' "$note" >&2
  exit 1
fi

printf 'created %s\n' "$note"
