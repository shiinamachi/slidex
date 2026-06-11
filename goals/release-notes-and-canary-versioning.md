# Goal: Release notes and canary versioning workflow

Slug: `release-notes-and-canary-versioning`
Status: Draft
Activation: `/goal @goals/release-notes-and-canary-versioning.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only after the activation command is run.

## Goal Text

Implement a repository-managed release notes and version bump workflow for slidex, verified by tests, release package smoke checks, workflow syntax checks, and repository doctor, while preserving immutable release assets, production tag semantics, update verification, exact version pinning, and the CLI repository model. Add versioned release note markdown files under a dedicated repository folder, make the Release new version workflow use the current `VERSION` release note file for GitHub Release notes, add a mise-invoked version bump path that updates `VERSION`, syncs duplicated version metadata, and creates an empty release note file from a template, and change canary package versions and tags from `<VERSION>-<short-sha>` to `<VERSION>-canary.<timestamp>` with a deterministic UTC timestamp format. Between iterations, inspect the failing contract or test evidence, update the narrowest affected workflow, CLI, script, docs, or tests, and rerun the relevant verification. If no valid path remains or a required release/versioning decision is missing, stop with attempted paths, evidence, the blocker, and the exact decision needed.

## Interview Summary

- Desired outcome: Release notes are authored in repository markdown files, release publication uses the file for the current base version, version bumps are performed through a mise-accessible command instead of manual `VERSION` edits, and canary releases use `0.2.0-canary.<timestamp>` style versions.
- Non-goals: Do not publish a GitHub Release during implementation, do not change production release tags away from `v<VERSION>`, do not add floating or unpinned third-party release-note actions, do not commit generated deck outputs, and do not rebuild the project as a hosted service.
- Evidence: Local tests and workflow/package smoke checks pass, docs describe the new process, and repository contract tests validate the release notes and canary version policy.
- Constraints: Preserve immutable release asset behavior, `actions/attest` artifact verification, updater fail-closed behavior, exact runtime/dependency pins, conventional commit discipline, and deck outputs remaining scoped under `decks/<deck_id>/`.
- Scope: Allowed scope is this repository, especially `.github/workflows/release.yml`, `.mise.toml`, `VERSIONING.md`, `INSTALL.md` if needed, `scripts/`, `cmd/slidex/`, package/update tests, and a new release notes folder.
- Autonomy: Codex may implement low-risk, repo-pattern-aligned changes directly and keep iterating through test failures until verification passes.
- Budget: Continue until the evidence below passes or until no defensible path remains under the repository constraints.
- Blocked condition: Stop if a required policy decision is missing, especially timestamp format, release note template content, or whether version bumping should live in Go CLI code versus a shell script and mise task.
- Reporting: Final report should list changed files, canary version/tag format, release note folder/file convention, bump command, verification commands and results, and the commit hash if committed.

## Assumptions

- The release notes folder should be named `release-notes/`.
- The release note file for base version `0.2.0` should be `release-notes/0.2.0.md`, matching the exact `VERSION` file content without a leading `v`.
- A reusable template should live at `release-notes/_template.md`.
- Canary package versions should use the exact SemVer prerelease shape `<VERSION>-canary.<timestamp>`, and canary tags should be `v<VERSION>-canary.<timestamp>`.
- The timestamp should be UTC in compact numeric form `YYYYMMDDHHMMSS`, for example `0.2.0-canary.20260611032635`, because it sorts lexically and avoids punctuation that complicates asset names.
- The mise task may call a repo script or Go CLI helper; implementation should choose the smaller maintainable option that is testable and matches existing repo patterns.
- Production release notes must be present and non-placeholder. Canary releases may use the same base version note as draft context but should still include build metadata.

## Verification Checklist

- [ ] `release-notes/_template.md` exists and a current-version note such as `release-notes/<VERSION>.md` can be generated without overwriting an existing note.
- [ ] `.mise.toml` exposes a version bump task that updates `VERSION`, runs `go run ./cmd/slidex sync-version-metadata`, and creates `release-notes/<new-version>.md` from the template.
- [ ] `.github/workflows/release.yml` reads `release-notes/${base_version}.md` into the GitHub Release body and fails clearly when the required production note is missing or still placeholder-only.
- [ ] Canary release metadata in the workflow uses `<VERSION>-canary.<YYYYMMDDHHMMSS>` and no longer uses the short commit SHA as the package version suffix.
- [ ] `scripts/package-release.sh` accepts production versions, `dev-*`, `ci-*`, and the new canary pattern while rejecting malformed canary versions.
- [ ] Update/version parsing in `cmd/slidex` recognizes the new canary pattern as canary and preserves production behavior for `0.2.0` and `v0.2.0`.
- [ ] Same-base canary update ordering is deterministic with timestamp canaries and covered by tests.
- [ ] Documentation in `VERSIONING.md` explains release note authoring, version bumping, production tags, canary tags, and package asset naming.
- [ ] Existing release verification behavior remains intact: no `--clobber`, release assets remain immutable, checksums are verified, and artifact attestations are generated and verified.
- [ ] `python3` or another available parser successfully parses `.github/workflows/release.yml` as YAML.
- [ ] `gofmt` has been run on edited Go files.
- [ ] `go test ./...` passes.
- [ ] `go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json` passes if any CLI/spec contract surface is touched in a way that could affect specs.
- [ ] `go run ./cmd/slidex doctor --json` passes.
- [ ] Production package smoke passes with `SLIDEX_RELEASE_VERSION="v$(cat VERSION)" SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh`.
- [ ] Canary package smoke passes with a representative value such as `SLIDEX_RELEASE_VERSION="$(cat VERSION)-canary.20260611032635" SLIDEX_TARGETS=linux/amd64 SLIDEX_DIST_DIR="$(mktemp -d)" ./scripts/package-release.sh`.

## Constraints And Boundaries

- Keep changes scoped to release workflow, versioning, packaging, update selection, documentation, tests, and the new release notes assets.
- Do not introduce new remote actions or dependencies unless they are pinned to exact immutable versions and justified by the repository's dependency policy.
- Do not change production `VERSION` semantics: `VERSION` remains the base release version and production tags remain `v<VERSION>`.
- Do not use the commit hash as the canary package version suffix after this Goal is complete; commit SHA should remain available only as release metadata and provenance.
- Do not loosen checksum, release asset, or attestation verification to make tests pass.
- Do not modify unrelated deck workspaces or generated delivery artifacts.
- Preserve Korean-safe and repository-specific documentation style where existing docs use it.

## Iteration Policy

- Start by mapping all current assumptions and tests that mention canary versions, short SHAs, release workflow notes, package asset names, and update selection.
- Make the smallest coherent change that advances one contract at a time: release note files, bump command, release workflow, packaging validation, update parsing/ordering, docs, then tests.
- After each implementation slice, run the narrowest relevant test first, then broaden to the full verification checklist.
- If a test fails, treat the failure as contract evidence; update either the implementation or the test expectation only after deciding which contract should own the behavior.
- Keep commits small and Conventional Commit-formatted if committing during the Goal.

## Blocked Stop Condition

Stop and report instead of continuing if:

- The exact release note template fields need product or maintainer judgment.
- The desired version bump interface conflicts with mise task capabilities or existing CLI boundaries.
- GitHub Actions semantics make the chosen timestamp source non-deterministic or unsafe for reruns.
- The updater cannot safely order same-base canary releases with the timestamp policy without changing user-visible update semantics beyond the requested scope.
- Required verification cannot run locally and no equivalent evidence can be gathered.

The blocked report must include attempted paths, files inspected or changed, commands run, observed evidence, the specific unresolved decision, and the next user input needed.

## Expected Final Report Format

- Summary of implemented behavior.
- Changed files grouped by workflow, CLI/scripts, docs, tests, and release-note assets.
- Final canary version and tag examples.
- Version bump command.
- Verification commands and pass/fail results.
- Any residual risk or manual release step that remains.
- Commit hash if a commit was created.

## Notes

- This Goal records the June 11, 2026 session decision to use canary versions shaped like `0.2.0-canary.<timestamp>`.
- The current Release workflow name is `Release new version` and the workflow file is `.github/workflows/release.yml`.
- The existing release workflow already publishes immutable assets and verifies artifact attestations; this Goal should extend that workflow rather than replacing it.
