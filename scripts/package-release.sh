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
package_version="${release_version#v}"

tool_version="$(go run ./cmd/slidex version | awk '{print $2}')"
case "$package_version" in
  dev-*|ci-*)
    ;;
  *)
    canary_pattern="^${tool_version//./\\.}-canary\\.[0-9]{14}$"
    if [[ "$package_version" != "$tool_version" && ! "$package_version" =~ $canary_pattern ]]; then
      printf 'release version %s does not match slidex CLI version %s\n' "$release_version" "$tool_version" >&2
      exit 2
    fi
    ;;
esac

release_tag="${SLIDEX_RELEASE_TAG:-}"
if [[ -z "$release_tag" ]]; then
  case "$package_version" in
    dev-*|ci-*) release_tag="$release_version" ;;
    *) release_tag="v${package_version}" ;;
  esac
fi

build_channel="${SLIDEX_BUILD_CHANNEL:-}"
if [[ -z "$build_channel" ]]; then
  if [[ "$package_version" == dev-* || "$package_version" == ci-* ]]; then
    build_channel="local-development"
  elif [[ "$package_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-canary\.[0-9]{14}$ ]]; then
    build_channel="canary"
  else
    build_channel="production"
  fi
fi
case "$build_channel" in
  production|canary|local-development) ;;
  *)
    printf 'unsupported build channel: %s\n' "$build_channel" >&2
    exit 2
    ;;
esac

commit_sha="${SLIDEX_COMMIT_SHA:-$(git rev-parse --verify HEAD 2>/dev/null || echo unknown)}"
build_time="${SLIDEX_BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

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
  "VERSION"
  "commands.md"
  "decks/README.md"
  "decks/_template"
  "examples/sample_deck_spec.json"
  "go.mod"
  "go.sum"
  "internal/codex/protocol"
  "package.json"
  "pnpm-lock.yaml"
  "pnpm-workspace.yaml"
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

  package_name="slidex_${package_version}_${goos}_${goarch}"
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

  mkdir -p "$package_dir/.slidex"
  cat > "$package_dir/.slidex/install.json" <<EOF
{
  "schemaVersion": "slidex.install.v1",
  "toolName": "slidex",
  "version": "${package_version}",
  "channel": "${build_channel}",
  "tag": "${release_tag}",
  "commit": "${commit_sha}",
  "buildTime": "${build_time}",
  "installRoot": "",
  "releaseAssetName": "${package_name}.${archive_ext}",
  "installedAt": "",
  "installMode": "release-package",
  "os": "${goos}",
  "arch": "${goarch}"
}
EOF

  if [[ "$archive_ext" == "zip" ]]; then
    (cd "$work_dir" && zip -qr "$dist_abs/$package_name.zip" "$package_name")
  else
    tar -C "$work_dir" -czf "$dist_abs/$package_name.tar.gz" "$package_name"
  fi
done

checksum_name="slidex_${package_version}_checksums.txt"
(
  cd "$dist_abs"
  rm -f "$checksum_name"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum slidex_"${package_version}"_* > "$checksum_name"
  else
    shasum -a 256 slidex_"${package_version}"_* > "$checksum_name"
  fi
)

printf 'Release package artifacts written to %s\n' "$dist_abs"
