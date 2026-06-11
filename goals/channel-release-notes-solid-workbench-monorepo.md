# Goal: Channel Release Notes And Solid Workbench Monorepo

Slug: `channel-release-notes-solid-workbench-monorepo`
Status: Ready for review
Activation: `/goal @goals/channel-release-notes-solid-workbench-monorepo.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only
after the activation command is run.

## Goal Text

Implement channel-specific release-note management and migrate the slidex
workbench frontend into a repo-managed SolidStart CSR monorepo package, verified
by release workflow contract tests, canary and production release-note smoke
checks, exact runtime/dependency pin checks, frontend build/test evidence,
workbench HTTP and browser smoke evidence, `go test ./...`, and repository
doctor, while preserving slidex's CLI repository model, immutable release asset
semantics, channel-aware update behavior, deck-local output boundaries, and
release-package runtime contents. The production channel must keep a canonical
base-version markdown note, canary release notes must be managed separately
under base-version folders with timestamped markdown files, and the current
embedded React workbench assets must be replaced by SolidStart-generated local
assets served by the Go CLI without requiring Node at runtime. Between iterations,
inspect the strongest failing evidence, make the smallest coherent repository
change that advances the contract, rerun the relevant narrow check, then broaden
to the full verification set. If no defensible implementation path remains or a
required product decision is missing, stop with attempted paths, evidence, the
blocker, and the exact decision needed.

## Interview Summary

- Desired outcome: Release notes are separated by build channel, canary builds
  have timestamped markdown notes under their base version folder, and the
  workbench frontend source is managed as a SolidStart CSR package in a root
  monorepo rather than as hand-authored embedded React JavaScript.
- Non-goals: Do not publish a GitHub Release, do not change production tag
  semantics away from `v<VERSION>`, do not add a hosted service or SaaS runtime,
  do not require Node/pnpm in installed release packages at workbench runtime,
  do not commit generated deck outputs, and do not weaken update checksum or
  immutable asset checks.
- Evidence: Repository tests cover the new release-note path contract,
  production and canary notes are generated/read from separate locations,
  `.github/workflows/release.yml` uses the correct note for each channel,
  `.mise.toml` pins Go, Node, and pnpm exactly, frontend package manifests and
  lockfiles pin dependencies exactly, SolidStart builds CSR assets, Go embeds
  only generated local assets, workbench save/browser smoke checks pass, and
  `go test ./...` plus `go run ./cmd/slidex doctor --json` pass.
- Constraints: Preserve the deck workspace contract under `decks/<deck_id>/`,
  existing CLI command behavior unless explicitly migrated, exact version
  pinning policy, local-only workbench serving, Korean-capable UI copy and
  wrapping, channel-aware update semantics, release package checksums, and
  Conventional Commit discipline.
- Scope: Allowed scope is this repository, especially `release-notes/`,
  `.github/workflows/release.yml`, `.mise.toml`, `VERSIONING.md`, `INSTALL.md`,
  `scripts/`, `cmd/slidex/`, `cmd/slidex/workbench_assets/`, a new root
  workbench frontend package such as `workbench/`, root `package.json`,
  `pnpm-workspace.yaml`, `pnpm-lock.yaml`, `README.md`, `README.ko.md`,
  `commands.md`, plugin skills/docs that mention the React workbench,
  `.agents/skills/slidex/`, `plugins/slidex/skills/`, `cmd/slidex/workflow.go`,
  and tests/schemas needed to verify the migration.
- Autonomy: Codex may implement repo-pattern-aligned changes directly and keep
  iterating through failing tests, but should stop before release publication,
  destructive local Codex configuration edits, or a broad product redesign of
  deck intake.
- Budget: Continue until the verification checklist passes or until no
  defensible path remains under the repository constraints.
- Blocked condition: Stop if channel release-note naming, canary timestamp
  source, Workbench source folder name, SolidStart CSR output strategy, or
  dependency/version policy requires a maintainer decision not already captured
  below.
- Reporting: Final report should list changed files, channel release-note paths,
  canary note naming examples, frontend package/build commands, pinned versions,
  verification commands and results, visual/browser evidence status, residual
  risks, and the commit hash if committed.

## Assumptions

- Production release notes should keep the current root-level
  `release-notes/<VERSION>.md` convention as the production channel's canonical
  note path.
- Canary release notes should use a canary channel directory containing a
  base-version folder:
  `release-notes/canary/<BASE_VERSION>/<YYYYMMDDHHMMSS>.md`, for example
  `release-notes/canary/0.1.0/20260611032635.md`.
- Canary release notes should be created from a dedicated template at
  `release-notes/canary/_template.md`.
- The canary template should include placeholders for at least
  `{{BASE_VERSION}}`, `{{TIMESTAMP}}`, `{{RELEASE_VERSION}}`, and
  `{{COMMIT_SHA}}`, and helper/workflow tests should verify that placeholders
  are resolved before publication.
- The canary timestamp should keep the existing UTC compact numeric format
  `YYYYMMDDHHMMSS` and should match the canary release version suffix
  `<VERSION>-canary.<YYYYMMDDHHMMSS>`.
- Existing root-level release note files are production notes, not shared
  canary notes. The release workflow should stop using them as canary draft
  context after this Goal is complete.
- The Workbench source package should live in a root-level folder named
  `workbench/` unless implementation evidence shows a different root folder is
  clearly more maintainable.
- The monorepo should use pnpm workspaces with a root `package.json`,
  `pnpm-workspace.yaml`, and `pnpm-lock.yaml`; Go remains the CLI/runtime owner.
- SolidStart should be configured for CSR/no SSR, for example through
  `app.config.ts` with `ssr: false`, and the release package should serve static
  built assets through the existing loopback Go workbench server.
- As of 2026-06-12 KST, current external version evidence is Node.js
  `24.16.0` LTS, pnpm `11.5.3`, and `@solidjs/start` `1.3.2`; if implementation
  starts later, re-check official sources and update exact pins before editing.

## Verification Checklist

- [ ] Release-note files are separated by channel:
      `release-notes/<VERSION>.md` for production and
      `release-notes/canary/<BASE_VERSION>/<YYYYMMDDHHMMSS>.md` for canary.
- [ ] A version bump or release-note helper creates the correct production note
      from a template without overwriting existing notes.
- [ ] A canary release-note helper creates the correct timestamped canary note
      from a template and can derive or accept the same timestamp used by the
      canary release version.
- [ ] `.mise.toml` exposes a canary release-note task, such as
      `release-notes:canary`, that calls a tested script/helper capable of
      deriving the timestamp from a selected ref or accepting an explicit
      timestamp that matches the release workflow.
- [ ] `release-notes/canary/_template.md` exists and its
      `{{BASE_VERSION}}`, `{{TIMESTAMP}}`, `{{RELEASE_VERSION}}`, and
      `{{COMMIT_SHA}}` placeholders are resolved in generated canary notes.
- [ ] `.github/workflows/release.yml` resolves the release-note path by
      `build_channel`, uses `release-notes/${BASE_VERSION}.md` for production,
      uses `release-notes/canary/${BASE_VERSION}/${release_timestamp}.md` for
      canary, fails clearly when the required channel note is missing, and
      rejects placeholder-only notes for production and canary releases.
- [ ] GitHub Release bodies include the channel-specific release-note source path
      and preserve existing release metadata, checksum, and no-clobber behavior.
- [ ] Repository tests cover production note lookup, canary note lookup, missing
      note failures, placeholder failures, timestamp/path validation, and the
      absence of the old single-note canary behavior.
- [ ] Documentation in `VERSIONING.md` and install/update docs explains the new
      channel release-note authoring workflow, production path, canary path,
      canary timestamp source, and examples.
- [ ] Any existing Goal or docs that still say canary releases may reuse the
      base-version production note are revised or marked superseded so they do
      not conflict with the new channel-specific release-note contract.
- [ ] Root `.mise.toml` pins Go, Node, and pnpm to exact versions, including the
      verified implementation-time latest Node LTS and latest pnpm.
- [ ] Root JS package files exist for the monorepo and avoid floating/range
      versions in all committed version surfaces, including `dependencies`,
      `devDependencies`, `peerDependencies`, `optionalDependencies`,
      `overrides`, `pnpm.overrides`, `packageManager`, `engines`, scripts, and
      workspace metadata.
- [ ] `pnpm-lock.yaml` is committed and `pnpm install --frozen-lockfile` passes
      under `mise exec`.
- [ ] A root-level Workbench frontend package exists, uses SolidStart at the
      verified implementation-time latest exact version, and is configured for
      CSR/no SSR.
- [ ] The current React UMD asset dependency and hand-authored
      `workbench-app.js` runtime source are replaced by generated local assets
      from the SolidStart build, with source code maintained under the Workbench
      package.
- [ ] Go embedding and package-release runtime paths include the generated
      Workbench assets needed at runtime and exclude unnecessary Node dependency
      folders such as `node_modules`.
- [ ] Frontend build output freshness is enforced: after `pnpm build`, generated
      Workbench assets are copied or emitted to the Go embed location, and tests
      or `doctor` detect drift between Workbench source, build metadata, and
      embedded assets.
- [ ] Release workflow test jobs run the frontend install/check/build path
      through mise, including `mise exec -- pnpm install --frozen-lockfile` and
      the Workbench package's check/build command before Go package smoke.
- [ ] Workbench UI behavior remains equivalent or better for deck intake:
      seeded fields, autosaved draft recovery, required-field validation,
      save brief, complete-and-generate, generation status polling, path display,
      loopback token handling, and Korean UI copy still work.
- [ ] Workbench status/doctor snapshots and plugin skills/docs no longer claim a
      React wizard when the shipped frontend is Solid-based.
- [ ] Workbench visual/browser evidence remains valid: HTTP save smoke passes,
      screenshot evidence is nonblank, DOM/API smoke covers field input, draft
      recovery, save, `Complete & generate`, generation status polling, and
      token/origin protection, and browser evidence commands continue to record
      and verify deck-local artifacts.
- [ ] `gofmt` has been run on edited Go files.
- [ ] Frontend format/typecheck/test/build commands pass through pnpm.
- [ ] `go test ./...` passes.
- [ ] `go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json`
      passes if any schema or CLI contract surface is touched.
- [ ] `go run ./cmd/slidex doctor --json` passes and validates the new runtime
      pins plus Workbench asset contract.
- [ ] Production package smoke passes with the current release package command
      from `VERSIONING.md`.
- [ ] Canary package smoke passes with a representative timestamped version such
      as `$(cat VERSION)-canary.20260611032635`.

## Constraints And Boundaries

- Keep implementation scoped to release-note workflow, versioning docs/scripts,
  Workbench frontend source/build/embedding, package contents, doctor/tests, and
  plugin/docs references that must change because the Workbench is no longer
  React.
- Do not build or require a Node server for installed slidex release packages;
  the Workbench must remain served by the local Go loopback server.
- Do not introduce unpinned runtime or library versions. Avoid version ranges or
  floating labels such as `latest`, `main`, `^`, `~`, `>=`, or `*` in committed
  dependency/version surfaces.
- Do not vendor remote browser assets without SHA-256 and exact immutable source
  evidence. Prefer generated local build artifacts from the pinned Workbench
  package.
- Do not loosen release asset checksum verification, updater fail-closed
  behavior, production tag format, canary tag format, or channel immutability.
- Do not alter generated deck outputs or commit ignored delivery artifacts.
- Preserve the repository's CLI-first purpose; frontend changes support the
  local Workbench only and do not turn slidex into a hosted app.

## Iteration Policy

- Start by mapping all current references to `release-notes/<VERSION>.md`,
  canary release metadata, React Workbench wording, embedded Workbench assets,
  package-release runtime paths, and doctor/test snapshots.
- Implement one coherent slice at a time: channel release-note paths and
  templates, release workflow/script support, docs/tests, Workbench monorepo
  scaffolding, SolidStart CSR migration, Go asset embedding, package/doctor
  updates, plugin/docs wording, then full verification.
- For each slice, run the narrowest test or command that proves the changed
  contract before broadening to package smoke, workbench smoke, `go test ./...`,
  and doctor.
- If evidence contradicts the planned contract, update the implementation or the
  Goal assumptions only after deciding which behavior should be authoritative.
- Keep commits small, coherent, and Conventional Commit-formatted.

## Blocked Stop Condition

Stop and report instead of continuing if:

- A maintainer rejects the assumption that root-level
  `release-notes/<VERSION>.md` is the production channel's canonical note path.
- The canary timestamp cannot be made identical across release version, tag, and
  note path without changing release workflow semantics.
- The selected canary source ref cannot be fixed consistently enough for the
  release-note helper and workflow to derive the same commit-time timestamp.
- SolidStart CSR build output cannot be served by the existing Go loopback
  Workbench without adding a Node runtime server.
- Exact latest Node/pnpm/SolidStart versions conflict with mise support,
  platform support, or repository tests.
- A required browser/visual verification command cannot run locally and no
  equivalent evidence can be gathered.

The blocked report must include attempted paths, files inspected or changed,
commands run, observed evidence, the specific unresolved decision, and the next
user input needed.

## Source Notes

- Current repository evidence: `.github/workflows/release.yml` reads
  `release-notes/${BASE_VERSION}.md` for both channels; `VERSIONING.md`
  documents canary releases using the base-version note as draft context;
  `cmd/slidex/workbench_assets/README.md` vendors React 18.3.1 UMD files;
  `cmd/slidex/workbench_assets/workbench-app.js` is the current hand-authored
  React Workbench; no root `package.json`, `pnpm-workspace.yaml`, or
  `pnpm-lock.yaml` exists.
- Existing `goals/release-notes-and-canary-versioning.md` contains the now
  superseded assumption that canary releases may use the base-version note as
  draft context. This Goal should replace that assumption for future work.
- Node.js official download page identified `v24.16.0` as Latest LTS on
  2026-06-12 KST: https://nodejs.org/en/download
- npm registry `pnpm/latest` identified pnpm `11.5.3` on 2026-06-12 KST:
  https://registry.npmjs.org/pnpm/latest
- SolidStart official site identified latest SolidStart `1.3.2` on
  2026-06-12 KST: https://start.solidjs.com/
- npm registry `@solidjs/start/latest` identified `@solidjs/start` `1.3.2` on
  2026-06-12 KST: https://registry.npmjs.org/@solidjs%2fstart/latest
- SolidStart docs describe `pnpm create solid`, project files, and SSR selection
  during project creation: https://docs.solidjs.com/solid-start/getting-started
- SolidStart `defineConfig` documents `ssr` as the toggle between client and
  server rendering: https://docs.solidjs.com/solid-start/reference/config/define-config

## Expected Final Report Format

- Summary of implemented channel release-note behavior and Workbench migration.
- Changed files grouped by release workflow/scripts, frontend monorepo, Go
  embedding/CLI, docs/plugin references, tests, and package assets.
- Production and canary release-note path examples.
- Exact pinned Go, Node, pnpm, SolidStart, and direct frontend dependency
  versions.
- Frontend build/test commands and Go verification commands with pass/fail
  results.
- Workbench HTTP/browser/visual evidence paths and status.
- Residual risks or manual release authoring steps.
- Commit hash if a commit was created.
