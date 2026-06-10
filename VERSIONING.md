# slidex Version Management

This repository uses one release base version for the slidex CLI and bundled
Codex Plugin. The CLI `toolVersion` is the canonical release base version.

## Canonical Values

| Field | Source | Required value |
|---|---|---|
| CLI name | `cmd/slidex/main.go` `toolName` | `slidex` |
| CLI version | `cmd/slidex/main.go` `toolVersion` | release base version |
| CLI developer | `cmd/slidex/main.go` `toolDeveloperName` | `shiinamachi` |
| CLI license | `cmd/slidex/main.go` `toolLicenseIdentifier` | `MIT` |
| Plugin manifest version | `plugins/slidex/.codex-plugin/plugin.json` | `<toolVersion>+codex.<cachebuster>` |
| Plugin lock | `plugins/slidex/.codex-plugin/version-lock.json` | `pluginVersion` and `slidexCliVersion` equal `toolVersion` |
| Marketplace name | `.agents/plugins/marketplace.json` | `shiinamachi` |

## Bump Procedure

1. Update `toolVersion` in `cmd/slidex/main.go`.
2. Update `plugins/slidex/.codex-plugin/plugin.json` so the version base before
   `+` equals `toolVersion`.
3. Refresh only the plugin manifest build metadata cachebuster after plugin
   metadata changes. Do not increment the base version just to force Codex to
   reinstall a local plugin.
4. Update `plugins/slidex/.codex-plugin/version-lock.json` so
   `pluginVersion`, `slidexCliVersion`, `developerName`, `license`, and
   `marketplaceName` match the canonical values.
5. Release tags must be `v<toolVersion>`. `scripts/package-release.sh` rejects
   release package names whose version does not match the CLI version, except
   `dev-*` and `ci-*` smoke packages.

## Verification

Run these checks before releasing or handing off plugin metadata changes:

```bash
gofmt -w cmd/slidex
go test ./...
go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json
go run ./cmd/slidex doctor --json
SLIDEX_RELEASE_VERSION=v0.1.0 SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh
```

`slidex doctor` validates the plugin manifest, plugin version lock, bundled
Marketplace metadata, exact Go/Codex pins, and local protocol bundle. A version
or identity drift in those files is a repository-health failure.
