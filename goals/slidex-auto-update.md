# Goal: Slidex Bundle Auto Update

Slug: `slidex-auto-update`
Status: Ready for review
Activation: `/goal @goals/slidex-auto-update.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only
after the activation command is run.

## Goal Text

Implement a channel-aware slidex bundle updater that updates the CLI, runtime
files, bundled Codex plugin, and local plugin marketplace as one verified
release unit, while preserving slidex's CLI repository model and existing deck
delivery contracts. The updater must infer the immutable update channel from
the original installation package: `production` follows stable GitHub Releases,
`canary` follows canary/prerelease GitHub Releases, and `local-development`
disables automatic updates. Completion is verified by focused unit tests,
release-package smoke tests, `go test ./...`, `go run ./cmd/slidex doctor
--json`, and documented/manual verification of the Codex restart workflow,
including workbench UI status banners and post-restart plugin verification.
Between iterations, use evidence from failing tests, release metadata, doctor
findings, and Codex plugin state to choose the next smallest defensible change.
If release metadata, Codex plugin behavior, platform file replacement, or
checksum verification cannot be proven under current constraints, stop with the
attempted paths, evidence gathered, blocker, and the decision or external state
needed.

## Interview Summary

- Desired outcome: slidex can safely update itself from GitHub Releases without
  drifting the CLI, runtime templates/schemas, Codex plugin package, local
  marketplace, and Codex plugin cache/registration state.
- Non-goals: do not build a hosted update service; do not add a channel switch
  command; do not silently mutate legacy user Codex config selectors; do not
  directly edit undocumented Codex plugin cache internals; do not claim Codex
  plugin reload is complete until restart/post-restart verification has passed.
- Evidence: local tests for release discovery, asset naming, checksum/digest
  validation, install-mode/channel detection, local-development disablement,
  plugin restart-required state, workbench banner rendering, rollback/staging
  behavior, and Codex verification status; `go test ./...`; `go run
  ./cmd/slidex doctor --json`; package smoke with `scripts/package-release.sh`;
  docs checks for install/update instructions.
- Constraints: preserve the deck-local workspace model, existing render/QA/package
  contracts, exact runtime/dependency pinning rules, release package contents,
  Codex plugin manifest/version-lock invariants, and local CLI repository scope.
- Scope: changes may touch `cmd/slidex/`, release packaging scripts, GitHub
  Actions release workflow, install/update docs, plugin/workbench UI templates,
  schemas/fixtures/tests, and state files needed for update bookkeeping.
- Autonomy: Codex may implement and iterate through low-risk repository changes
  and tests, but should stop before destructive local Codex config cleanup,
  credential-dependent release publishing, or undocumented cache mutation.
- Budget: continue until the evidence checklist passes or no defensible path
  remains; avoid broad refactors unrelated to update behavior.
- Blocked condition: stop if GitHub release metadata cannot identify assets
  deterministically, Codex plugin reinstall behavior cannot be verified through
  documented commands/App Server surfaces, Windows replacement semantics require
  a user decision, or checksum verification policy needs a product decision.
- Reporting: final report should summarize implemented command surface,
  channel behavior, verification commands and outputs, restart workflow, files
  changed, residual risks, and any manual Codex App verification still required.

## Assumptions

- The first installed release package determines the update channel for that
  installation and the updater never changes channels.
- `production` packages are created from stable releases; `canary` packages are
  created from prerelease/canary releases; source checkouts and `go install`
  development binaries are treated as `local-development`.
- A release archive is the update unit. Updating only the binary is insufficient
  because slidex packages templates, schemas, protocol bundles, the Codex plugin,
  and the local plugin marketplace together.
- Codex local plugin updates require plugin directory/marketplace refresh plus
  Codex App or CLI session restart before new bundled skills are reliably loaded.
- Existing legacy marketplace selectors such as `slidex-local` may be reported
  as drift, but cleanup requires explicit opt-in outside the default updater.
- GitHub release asset SHA-256 verification is mandatory before unattended
  updates are considered complete.

## Design Contract

- Add update status/check/apply/verify behavior without introducing a channel
  switching command.
- Record build/install metadata sufficient to distinguish `production`,
  `canary`, and `local-development`, including at least version, channel, tag,
  commit, build time, install root, release asset name, and installed-at time
  where available.
- Fix the release naming contract so Git tag names such as `v0.1.0` and asset
  versions such as `0.1.0` are handled consistently by docs, scripts, workflow,
  installer, and updater.
- Discover `production` updates from stable GitHub Releases and `canary` updates
  from prerelease/canary releases only. A `production` install must not follow
  canary releases; a `canary` install must not silently downgrade or switch to
  production.
- Detect `local-development` and return a disabled update state with actionable
  guidance instead of applying release updates.
- Download the matching OS/architecture release archive and checksum/digest
  evidence, verify SHA-256 before extraction, then verify the candidate bundle
  before replacing the active install root.
- Replace the install root atomically where possible and use staging/rollback or
  next-run handoff where platform file locking requires it, especially on
  Windows.
- Re-register or refresh the Codex marketplace/plugin only through documented
  Codex commands or App Server surfaces such as plugin list/read/install and
  skills list with force reload.
- After an update that changes bundled plugin content, set a persisted
  restart-required state. CLI output, JSON output, doctor/update verify, and the
  workbench page must all report that Codex must be restarted and a new thread
  started before the new plugin skills are considered active.
- Provide a post-restart verification workflow that can prove `slidex@shiinamachi`
  is installed/enabled, the plugin manifest/cache version matches the active
  install root, and the bundled `slidex-start` skill is visible from the updated
  plugin path/version.
- Show workbench status banners for canary channel, local-development update
  disabled, CLI/plugin/cache drift, restart required, and verified states. The
  banner must not block deck work unnecessarily, but it must not overstate plugin
  verification.

## Verification Checklist

- [ ] Release asset naming contract is corrected and covered by tests/docs.
- [ ] Build/install metadata records channel, tag, commit, build time, asset, and
      install mode where available.
- [ ] `production`, `canary`, and `local-development` update states are detected
      and tested independently.
- [ ] `local-development` disables automatic updates and reports clear guidance.
- [ ] Update discovery selects only stable releases for production and only
      canary/prerelease releases for canary.
- [ ] SHA-256 verification fails closed for mismatched or missing digest/checksum
      evidence.
- [ ] Candidate bundle validation checks CLI version/build info, plugin manifest,
      version-lock, marketplace metadata, runtime paths, and doctor/package
      invariants before activation.
- [ ] Install-root replacement is staged with rollback/next-run behavior for
      platform constraints, including Windows executable locking.
- [ ] Codex plugin registration uses documented Codex CLI/App Server surfaces and
      does not directly mutate undocumented plugin cache internals.
- [ ] Update application persists a restart-required state when plugin content may
      have changed.
- [ ] CLI human output and JSON output consistently report channel, current
      version, target version, plugin status, restart requirement, and next
      verification command.
- [ ] Workbench UI displays the correct channel/update/restart/drift/verified
      banner states without overlapping or obscuring deck workflow controls.
- [ ] Post-restart verification proves the installed plugin and visible bundled
      skill match the active install root.
- [ ] `go test ./...` passes.
- [ ] `go run ./cmd/slidex doctor --json` passes for repository health.
- [ ] Relevant release package smoke command from `VERSIONING.md` passes.
- [ ] Install/update documentation and one-shot Codex prompt describe update,
      restart, and verification workflows accurately.

## Boundaries

- Allowed repository scope: `/home/kentakang/shiinamachi/Projects/slidex`.
- Allowed external sources: official OpenAI Codex docs, official GitHub docs,
  GitHub Releases API, GitHub CLI documentation, and the public
  `shiinamachi/slidex` release metadata required for verification.
- Do not publish GitHub Releases, delete local Codex configuration, remove
  legacy marketplaces, or alter user deck outputs unless explicitly requested.
- Do not version generated deck delivery artifacts as part of this Goal.

## Source Notes

- Codex local plugin install/update model: local marketplaces point at plugin
  directories, and changed local plugin directories require Codex restart before
  the updated plugin is reliably picked up.
  https://developers.openai.com/codex/plugins/build#install-a-local-plugin-manually
- Codex marketplace metadata supports local and Git-backed plugin sources with
  path/ref/sha selectors.
  https://developers.openai.com/codex/plugins/build#marketplace-metadata
- Codex App Server exposes plugin and skill surfaces, including plugin
  list/read/install and skills list with reload support.
  https://developers.openai.com/codex/app-server#api-overview
- GitHub Releases REST API exposes release and asset metadata suitable for
  deterministic update discovery.
  https://docs.github.com/en/rest/releases/releases
  https://docs.github.com/en/rest/releases/assets
- GitHub release assets expose immutable SHA-256 digests.
  https://github.blog/changelog/2025-06-03-releases-now-expose-digests-for-release-assets/
- GitHub immutable releases protect published tags/assets.
  https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases
- Current slidex release packaging already bundles CLI, runtime templates,
  schemas, plugin package, marketplace, and protocol bundle together:
  `scripts/package-release.sh`, `INSTALL.md`, `VERSIONING.md`.

## Expected Final Report Format

When the Goal completes, report:

- Final command surface and channel behavior.
- Release/package metadata contract and how it is tested.
- Codex plugin update, restart, workbench banner, and post-restart verification
  workflow.
- Verification commands run and key pass/fail evidence.
- Files changed and any generated artifacts intentionally left uncommitted.
- Residual risks, especially checksum evidence or platform-specific replacement
  limitations.
