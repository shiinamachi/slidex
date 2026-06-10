# slidex Version Management

This repository uses one release base version for the slidex CLI and bundled
Codex Plugin. The root `VERSION` file is the single canonical version file.

## Canonical Values

| Field | Source | Required value |
|---|---|---|
| CLI name | `cmd/slidex/main.go` `toolName` | `slidex` |
| CLI version | `VERSION` | release base version |
| CLI developer | `cmd/slidex/main.go` `toolDeveloperName` | `shiinamachi` |
| CLI license | `cmd/slidex/main.go` `toolLicenseIdentifier` | `MIT` |
| Plugin manifest version | generated into `plugins/slidex/.codex-plugin/plugin.json` | `<VERSION>+codex.<cachebuster>` |
| Plugin lock | generated into `plugins/slidex/.codex-plugin/version-lock.json` | `pluginVersion` and `slidexCliVersion` equal `VERSION` |
| Marketplace name | `.agents/plugins/marketplace.json` | `shiinamachi` |

## Bump Procedure

1. Update only `VERSION`.
2. Run `go run ./cmd/slidex sync-version-metadata` to regenerate duplicated
   plugin metadata from that file. The command preserves the existing
   `+codex.<cachebuster>` suffix in `plugin.json`.
3. Refresh only the plugin manifest build metadata cachebuster after plugin
   metadata changes. Do not increment the base version just to force Codex to
   reinstall a local plugin.
4. Release tags must be `v<VERSION>`. `scripts/package-release.sh` rejects
   release package names whose version does not match the CLI version, except
   `dev-*` and `ci-*` smoke packages.

## Verification

Run these checks before releasing or handing off plugin metadata changes:

```bash
gofmt -w cmd/slidex
go run ./cmd/slidex sync-version-metadata
go test ./...
go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json
go run ./cmd/slidex doctor --json
SLIDEX_RELEASE_VERSION="v$(cat VERSION)" SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh
```

`slidex doctor` validates `VERSION`, the embedded CLI version,
plugin manifest, plugin version lock, bundled Marketplace metadata, exact
Go/Codex pins, and local protocol bundle. A version or identity drift in those
files is a repository-health failure.
