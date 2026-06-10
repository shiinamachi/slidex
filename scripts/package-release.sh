#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

release_version="${SLIDEX_RELEASE_VERSION:-}"
if [[ -z "$release_version" ]]; then
  if release_version="$(git describe --tags --exact-match 2>/dev/null)"; then
    :
  else
    release_version="dev-$(git rev-parse --short HEAD 2>/dev/null || echo local)"
  fi
fi
release_version="${release_version#refs/tags/}"

tool_version="$(go run ./cmd/slidex version | awk '{print $2}')"
case "$release_version" in
  dev-*|ci-*)
    ;;
  *)
    release_base="${release_version#v}"
    if [[ "$release_base" != "$tool_version" ]]; then
      printf 'release version %s does not match slidex CLI version %s\n' "$release_version" "$tool_version" >&2
      exit 2
    fi
    ;;
esac

dist_dir="${SLIDEX_DIST_DIR:-dist}"
targets="${SLIDEX_TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}"

runtime_paths=(
  ".agents/plugins/marketplace.json"
  ".agents/skills/slidex"
  ".mise.toml"
  "CODEX_INSTALL_PROMPT.md"
  "INSTALL.md"
  "LICENSE"
  "README.ko.md"
  "README.md"
  "VERSIONING.md"
  "cmd/slidex/VERSION"
  "commands.md"
  "decks/README.md"
  "decks/_template"
  "examples/sample_deck_spec.json"
  "go.mod"
  "go.sum"
  "internal/codex/protocol"
  "plugins/slidex"
  "schemas"
  "slidex.toml"
)

missing=()
for path in "${runtime_paths[@]}"; do
  if [[ ! -e "$path" ]]; then
    missing+=("$path")
  fi
done
if (( ${#missing[@]} > 0 )); then
  printf 'missing release runtime path: %s\n' "${missing[@]}" >&2
  exit 1
fi

rm -rf "$dist_dir"
mkdir -p "$dist_dir"
dist_abs="$(cd "$dist_dir" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

for target in $targets; do
  if [[ "$target" != */* ]]; then
    printf 'invalid target %q, expected goos/goarch\n' "$target" >&2
    exit 2
  fi
  goos="${target%%/*}"
  goarch="${target##*/}"

  binary="slidex"
  archive_ext="tar.gz"
  if [[ "$goos" == "windows" ]]; then
    binary="slidex.exe"
    archive_ext="zip"
  fi

  package_name="slidex_${release_version}_${goos}_${goarch}"
  package_dir="$work_dir/$package_name"
  mkdir -p "$package_dir"

  env CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$package_dir/$binary" ./cmd/slidex
  if [[ "$goos" != "windows" ]]; then
    chmod 0755 "$package_dir/$binary"
  fi

  for path in "${runtime_paths[@]}"; do
    mkdir -p "$package_dir/$(dirname "$path")"
    cp -R "$path" "$package_dir/$path"
  done

  if [[ "$archive_ext" == "zip" ]]; then
    (cd "$work_dir" && zip -qr "$dist_abs/$package_name.zip" "$package_name")
  else
    tar -C "$work_dir" -czf "$dist_abs/$package_name.tar.gz" "$package_name"
  fi
done

checksum_name="slidex_${release_version}_checksums.txt"
(
  cd "$dist_abs"
  rm -f "$checksum_name"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum slidex_"${release_version}"_* > "$checksum_name"
  else
    shasum -a 256 slidex_"${release_version}"_* > "$checksum_name"
  fi
)

printf 'Release package artifacts written to %s\n' "$dist_abs"
