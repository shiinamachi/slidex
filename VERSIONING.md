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
| Install metadata | packaged into `.slidex/install.json` | immutable `channel`, `tag`, `version`, commit, build time, asset name, and install mode |

## Bump Procedure

1. Run `mise run version:bump patch`, `mise run version:bump minor`,
   `mise run version:bump major`, or `mise run version:bump <version>`.
   The task updates `VERSION`, runs
   `go run ./cmd/slidex sync-version-metadata`, and creates
   `release-notes/<VERSION>.md` from `release-notes/_template.md`.
2. Fill in the new release note before publishing. Production releases fail if
   the matching `release-notes/<VERSION>.md` file is missing or still contains
   template placeholders. Canary releases use the same base-version note as
   draft release context and append build metadata in the GitHub Release body.
3. Refresh only the plugin manifest build metadata cachebuster after plugin
   metadata changes. Do not increment the base version just to force Codex to
   reinstall a local plugin.
4. Production release tags must be `v<VERSION>`. Canary release tags are
   `v<VERSION>-canary.<YYYYMMDDHHMMSS>`, where the timestamp is the selected
   source commit time normalized to UTC. Release asset names always use the
   package version without the leading tag `v`, for example
   `slidex_0.1.0_linux_amd64.tar.gz` or
   `slidex_0.1.0-canary.20260611032635_linux_amd64.tar.gz`.
   `scripts/package-release.sh` rejects release package names whose version
   does not match the CLI version or that canary pattern, except `dev-*` and
   `ci-*` smoke packages.
5. GitHub Actions releases are dispatched manually. Choose `canary` to build
   the current `develop` branch commit, or `production` to build the current
   `main` branch commit.
6. Release packages include `.slidex/install.json`. The updater treats its
   `channel` as immutable: `production` follows stable releases, `canary`
   follows canary prereleases, and `local-development` disables automatic
   release updates.
7. Release assets are not overwritten. If a release tag already exists, the
   release job fails instead of uploading with `--clobber`. The release job
   generates SHA-256 checksums for release assets before publishing.

## Verification

Run these checks before releasing or handing off plugin metadata changes:

```bash
gofmt -w cmd/slidex
go run ./cmd/slidex sync-version-metadata
go test ./...
go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json
go run ./cmd/slidex doctor --json
go run ./cmd/slidex update status --json
SLIDEX_RELEASE_VERSION="v$(cat VERSION)" SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh
SLIDEX_RELEASE_VERSION="$(cat VERSION)-canary.20260611032635" SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh
```

`slidex doctor` validates `VERSION`, the embedded CLI version,
plugin manifest, plugin version lock, bundled Marketplace metadata, exact
Go/Codex pins, and local protocol bundle. A version or identity drift in those
files is a repository-health failure.

`slidex update status --json` reports the immutable install channel from
`.slidex/install.json` when running from a release package. Source checkouts and
`go install` development binaries report `local-development` and disable
automatic release updates. `slidex update apply` validates the candidate bundle,
verifies release archives with SHA-256 checksums, stages activation, preserves a
backup or Windows pending handoff, and marks Codex plugin restart verification
as required when bundled plugin content may have changed. Direct `--candidate`
application bypasses release archive download and checksum verification, so it
is for explicit manual repair or development use only.
When a Windows handoff is pending, `slidex update status --json` reports
`pendingActivation: true` and a `pendingActivationCommand` that runs from an
activator binary outside the old install root and staged candidate. That command
validates and activates the staged bundle before the normal Codex restart
verification workflow continues.
