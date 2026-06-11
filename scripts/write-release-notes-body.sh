#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

usage() {
  cat >&2 <<'USAGE'
usage: scripts/write-release-notes-body.sh OUTPUT_FILE

Required environment:
  BASE_VERSION       production base version without leading v
  BUILD_CHANNEL      production or canary
  COMMIT_SHA         selected source commit
  RELEASE_VERSION    package version without leading v
  SOURCE_REF         selected source branch

Required for canary:
  RELEASE_TIMESTAMP  YYYYMMDDHHMMSS suffix used in RELEASE_VERSION
USAGE
}

if [[ "$#" != "1" ]]; then
  usage
  exit 2
fi

notes_file="$1"
base_version="${BASE_VERSION:-}"
build_channel="${BUILD_CHANNEL:-}"
commit_sha="${COMMIT_SHA:-}"
release_version="${RELEASE_VERSION:-}"
release_timestamp="${RELEASE_TIMESTAMP:-}"
source_ref="${SOURCE_REF:-}"
notes_dir="${SLIDEX_RELEASE_NOTES_DIR:-release-notes}"

for name in BASE_VERSION BUILD_CHANNEL COMMIT_SHA RELEASE_VERSION SOURCE_REF; do
  if [[ -z "${!name:-}" ]]; then
    printf 'missing required environment: %s\n' "$name" >&2
    exit 2
  fi
done
if [[ ! "$base_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'BASE_VERSION must be an exact production version, got %s\n' "$base_version" >&2
  exit 2
fi

case "$build_channel" in
  production)
    if [[ "$release_version" != "$base_version" ]]; then
      printf 'production RELEASE_VERSION must equal BASE_VERSION %s, got %s\n' "$base_version" "$release_version" >&2
      exit 2
    fi
    release_notes_path="${notes_dir}/${base_version}.md"
    ;;
  canary)
    if [[ -z "$release_timestamp" ]]; then
      printf 'missing required environment for canary: RELEASE_TIMESTAMP\n' >&2
      exit 2
    fi
    if [[ ! "$release_timestamp" =~ ^[0-9]{14}$ ]]; then
      printf 'RELEASE_TIMESTAMP must be YYYYMMDDHHMMSS, got %s\n' "$release_timestamp" >&2
      exit 2
    fi
    expected_release_version="${base_version}-canary.${release_timestamp}"
    if [[ "$release_version" != "$expected_release_version" ]]; then
      printf 'canary RELEASE_VERSION must be %s, got %s\n' "$expected_release_version" "$release_version" >&2
      exit 2
    fi
    release_notes_path="${notes_dir}/canary/${base_version}/${release_timestamp}.md"
    ;;
  *)
    printf 'unsupported build channel: %s\n' "$build_channel" >&2
    exit 2
    ;;
esac

if [[ ! -f "$release_notes_path" ]]; then
  printf 'missing %s release notes for %s: %s\n' "$build_channel" "$release_version" "$release_notes_path" >&2
  exit 2
fi
if grep -Eq 'TODO:|\{\{[A-Z0-9_]+\}\}' "$release_notes_path"; then
  printf '%s release notes still contain template placeholders: %s\n' "$build_channel" "$release_notes_path" >&2
  exit 2
fi

{
  cat "$release_notes_path"
  printf '\n## Release metadata\n\n'
  printf 'Release packages are built by GitHub Actions for Linux, macOS, and Windows on amd64 and arm64.\n'
  printf 'Each archive includes the slidex CLI binary plus the runtime templates, schemas, Codex plugin package, repo marketplace, and install guide required for local use.\n'
  printf '\n'
  printf 'Build channel: %s\n' "$build_channel"
  printf 'Source branch: %s\n' "$source_ref"
  printf 'Source commit: %s\n' "$commit_sha"
  printf 'Version: %s\n' "$release_version"
  printf 'Release notes source: %s\n' "$release_notes_path"
  printf 'Release package assets are SHA-256 checksummed by this workflow.\n'
} > "$notes_file"
