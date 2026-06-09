# Goal: slidex Desktop App을 Codex Plugin Canvas Workbench로 재설계

Slug: `codex-plugin-canvas-workbench-redesign`
Status: Draft
Activation: `/goal @goals/codex-plugin-canvas-workbench-redesign.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only after the activation command is run.

## Goal Text

slidex의 별도 Electron Desktop App 방향을 중단하고 Codex Plugin 중심의 Codex App 통합 UX로 재설계 및 구현한다. 완료란 Codex CLI `0.138.0` 기준으로 plugin manifest, skills, MCP server, App Server protocol artifacts, doctor checks, docs, tests가 갱신되고, Codex App에서 slidex plugin을 설치한 뒤 `@slidex` 또는 동등한 plugin 호출을 실행하면 문서화된 Codex App 작업 표면 안에서 deck 생성 workbench가 즉시 열려 `decks/<deck_id>/` 생성과 초기 deck creation input 저장을 수행하는 상태를 의미한다. 공개 또는 제품 계약으로 plugin-owned Canvas frontend API가 확인되면 그 API를 사용하고 증거를 남긴다. 확인되지 않으면 임의 Canvas mount를 주장하지 않고 Codex App이 지원하는 in-app browser, artifact preview, local workbench URL, 또는 동등한 지원 표면으로 MVP를 검증하며 Canvas 미지원은 명시적 product gap으로 기록한다. 기존 CLI-first 계약, deck-local workspace, HTML/PDF delivery contract, exact dependency pinning, user change preservation을 보존하고, later-stage authoring/render/QA 고도화는 이번 Goal 범위 밖으로 둔다.

## Interview Summary

- Desired outcome: slidex 사용자가 별도 Electron 앱을 열지 않고 Codex App 채팅에서 slidex plugin을 호출하여 deck 생성 workbench를 바로 시작한다.
- Primary product decision: `apps/desktop`은 canonical UX가 아니며, `plugins/slidex`가 새 front door가 된다.
- Codex integration target: Codex CLI `0.138.0` 기준의 plugin, skills, MCP, App Server protocol, doctor, docs, tests를 일관되게 갱신한다.
- Canvas/work-surface policy: true Canvas API가 확인되면 구현하고 출처를 남긴다. 확인되지 않으면 지원되는 Codex App 작업 표면으로 MVP를 만들되 Canvas라고 과장하지 않는다.
- MVP boundary: plugin invocation -> deck bootstrap -> workbench display -> initial deck creation data 저장까지만 완료한다. full strategy/spec/build/render/QA/finalize UI는 후속 Goal이다.
- Evidence required: official Codex docs, GPT Pro review, local repo inspection, Codex CLI `0.138.0` artifact proof, doctor JSON, Go tests, plugin install/invocation demo, workbench security tests.
- Non-goals: hosted SaaS, standalone Electron app shipping, full slide editor, full pipeline UI, legacy standalone instruction workflows, generated delivery artifacts committing, unsupported Canvas claim.
- Autonomy: implementation 중 명확한 공개 계약과 local evidence로 판단 가능한 사항은 직접 진행한다. Canvas API 여부, plugin UI product contract, dependency/license/security policy가 불명확하면 증거와 함께 멈춘다.
- Budget: explicit token/time budget 없음. 완료 증거가 모두 충족되거나 blocked stop condition에 도달할 때까지 진행한다.
- Blocked condition: plugin-owned Canvas가 필수인데 지원 계약을 찾지 못하고 fallback 승인이 없거나, Codex CLI `0.138.0` schema/protocol generation을 재현할 수 없거나, Codex App plugin install/invocation demo를 3회 연속 검증하지 못하면 중단한다.

## Research And Review Summary

### Repository Evidence

- `cmd/slidex` is the canonical CLI entry point.
- Deck workspaces must stay under `decks/<deck_id>/`; generated outputs stay under `decks/<deck_id>/out/`.
- `apps/desktop` is currently Electron boilerplate. Its IPC bridge reports `cliBridge: "not-implemented"`, so it is not a completed slidex product surface.
- `plugins/slidex` already exists but its `.mcp.json` is empty and its version lock still targets Codex CLI `0.132.0`.
- The repository snapshot inspected for this Goal is hard-coded around `0.132.0`, even though the user described the current design baseline as `0.133.0`. The implementation must remove active assumptions for both `0.132.0` and `0.133.0`.
- Active protocol artifacts are under `internal/codex/protocol/codex-cli-0.132.0/`; the new active bundle must become `internal/codex/protocol/codex-cli-0.138.0/`.

### Official Codex And Apps SDK Evidence

- Codex plugins are confirmed as bundles of skills, app integrations, and MCP servers, installable through Codex surfaces and invocable from prompts.
- Codex plugin packaging is confirmed around `.codex-plugin/plugin.json`, local/repo marketplace flows, skills, MCP config, app integrations, hooks, interface metadata, assets, and default prompts.
- Codex App features confirmed by official docs include local/worktree/cloud threads, skills, plugins, MCP, automations, integrated terminal, Git/diff tools, in-app browser, web search, image generation, and task/sidebar artifact previews.
- Codex App Server is confirmed as a JSON-RPC interface with version-specific generated TypeScript and JSON schema outputs.
- Public Codex documentation reviewed for this Goal did not confirm a generic plugin-owned Canvas API that auto-mounts arbitrary plugin frontend code when a plugin is invoked.
- Apps SDK documentation confirms ChatGPT app widgets through MCP resources, `text/html;profile=mcp-app`, `_meta["openai/outputTemplate"]`, and the `window.openai` UI bridge. This is relevant as a design reference but is not, by itself, proof that Codex App plugins can mount arbitrary Canvas widgets.
- npm registry evidence confirms `@openai/codex@0.138.0` exists with exact tarball and integrity metadata.

### GPT Pro Review Evidence

GPT Pro review was run in ChatGPT with model picker showing `Latest 5.5` and `Pro Extended`. The reviewed archive was:

- Archive: `slidex-gpt-pro-root-2516d608a8e642f08190b15f38218532-20260609-171222-e2cbcf5f.zip`
- SHA-256: `651a312944b93ae392c89d242862cf97872d31394c01536da1cf33a03831b5c1`
- Completion marker verified: `<GPT_PRO_COMPLETE id="20260609-8b84e66b-e03d-4f45-a881-62d35dfa97ea"/>`

GPT Pro's critical review agreed with the main direction:

- Retire Electron as the product path.
- Make `plugins/slidex` the front door.
- Keep `cmd/slidex` as the engine.
- Add a minimal local workbench rendered inside Codex App through documented/supported surfaces.
- Upgrade every active Codex protocol, doctor, plugin, and test path to `0.138.0`.
- Treat unsupported Canvas behavior as a product gap unless proven otherwise.

## Confirmed Versus Unconfirmed Capability Gate

### Confirmed Target

- slidex can become a Codex Plugin-first integration.
- The plugin can bundle skills and MCP configuration.
- A plugin invocation can guide Codex to run slidex CLI/MCP operations in a local/worktree thread.
- Codex App has supported surfaces that can show artifacts/previews and browser-based local targets.
- App Server/protocol artifacts can and should be generated for a specific Codex version.

### Unconfirmed Target

- A general plugin-owned Canvas lifecycle that lets slidex register arbitrary frontend code and auto-mount it inside Codex App Canvas immediately on plugin invocation.
- A documented `plugin invoked -> open Canvas widget` manifest or MCP contract in the public Codex docs reviewed for this Goal.

### Required Decision Gate

The first implementation phase must verify current Codex `0.138.0` documentation and generated plugin/app-server schema for any supported plugin UI or Canvas field.

If a supported Canvas/frontend contract exists:

- cite the documentation or product contract in implementation notes,
- encode it in plugin manifest/MCP resources as documented,
- demonstrate the frontend opening inside Codex App Canvas.

If no supported Canvas/frontend contract exists:

- do not build an imaginary Canvas mount,
- implement the MVP through the most direct supported Codex App surface,
- record this as a product gap,
- phrase the result as Codex App workbench integration, not Canvas support.

## Target Architecture

### Architectural Principle

The old direction was:

```text
apps/desktop Electron shell
  -> future child-process bridge
  -> slidex CLI
```

The new direction is:

```text
Codex App thread
  -> slidex Codex Plugin
       -> skills for user-facing workflow semantics
       -> MCP server for deterministic deck/workbench operations
       -> supported Codex App work surface for the workbench
       -> cmd/slidex as the local CLI source of truth
```

The CLI remains canonical. The plugin becomes the UX distribution and activation mechanism.

### Plugin Package

`plugins/slidex` must become the primary integration package.

Required changes:

- Update `.codex-plugin/plugin.json` with meaningful interface metadata, capabilities, default prompt, assets if needed, and correct references to skills/MCP/hooks only when they are actually supported.
- Update `.codex-plugin/version-lock.json` to require Codex CLI `0.138.0`.
- Make `.mcp.json` non-empty and define a `slidex` MCP server.
- Add or split skills so the default invocation starts deck creation, not the entire render/package pipeline.
- Preserve existing final-delivery guidance in a separate workflow skill if useful.

The plugin must not assume its plugin cache directory is the repository root. It must locate the active workspace from Codex thread context, command args, or explicit configuration, and locate the slidex binary through PATH or an explicit configured path.

### Skills

Minimum skill set:

- `slidex-start`: default entry point. Creates/selects a deck, starts or emits the workbench surface, records next steps.
- `slidex-run`: existing full workflow guidance for `slidex run --deck decks/<deck_id>`.
- `slidex-finalize`: render, QA, package, visual inspection, and delivery gate guidance.

The default plugin prompt should say, in substance: start a new slidex deck in this workspace and open the slidex workbench.

### MCP And CLI Surface

The MVP MCP/CLI API should expose a small deck/workbench surface:

- `deck.bootstrap`: validate deck id and create `decks/<deck_id>/` through the existing `slidex init <deck_id>` path or equivalent shared Go code.
- `deck.inspect`: return current deck state and expected files.
- `workbench.start`: start a loopback workbench server or generate a static workbench artifact.
- `workbench.status`: return URL/path, process state, artifact path, and redacted token status.
- `workbench.stop`: stop only the local workbench process created by slidex.

Security requirements must be enforced in code:

- validate `deck_id`,
- resolve paths and reject traversal,
- write only under `decks/<deck_id>/`,
- avoid arbitrary user-supplied commands,
- bind local servers only to `127.0.0.1`,
- require random capability tokens for write-capable HTTP endpoints,
- redact tokens from logs, manifests, and chat-visible output.

### Workbench Frontend

The workbench is a local web surface, not Electron.

Preferred layout:

```text
ui/workbench/                 # frontend source if a separate package is needed
internal/workbench/           # Go server, static embedding, or generated artifact support
plugins/slidex/               # plugin package and skill/MCP activation
```

MVP workbench capabilities:

- show deck id and workspace path,
- let the user enter deck title, audience, decision goal, source-material notes, and output expectations,
- save initial brief/state under `decks/<deck_id>/`,
- write `decks/<deck_id>/out/workbench_manifest.json`,
- show links or paths for generated workspace files,
- indicate that later pipeline stages are separate follow-up work.

Out of MVP:

- full slide authoring,
- strategy/spec/build/render/QA/finalize UI,
- editable visual canvas for every slide,
- hosted multi-user collaboration.

### Codex App Surface Strategy

Primary strategy:

- use a supported Codex App surface to open a local loopback workbench or render a workbench artifact.

Fallback strategy:

- generate a static HTML workbench artifact under `decks/<deck_id>/out/` and have Codex App preview/open it through a supported artifact/browser path.

Canvas strategy:

- implement only if current Codex documentation or product contract confirms the required plugin-owned frontend lifecycle.

The acceptance wording is:

> The slidex workbench appears inside Codex App using a documented and supported Codex App work surface.

The rejected wording is:

> The plugin mounts arbitrary UI into Canvas.

### App Server And Protocol Strategy

Codex App Server remains important for versioned protocol alignment and future structured integrations. It should not become the default browser UI backend unless a documented contract requires it.

Required `0.138.0` protocol layout:

```text
internal/codex/protocol/codex-cli-0.138.0/
  protocol_manifest.json
  method_constants.go
  generated_types.go
  schema/
```

Optional if consumed by UI tooling:

```text
internal/codex/protocol/codex-cli-0.138.0/ts/
```

Doctor must fail if generated artifacts, constants, manifest, plugin version lock, or docs disagree on the active Codex version.

### Desktop Retirement

`apps/desktop` must not remain the canonical future product path.

Acceptable outcomes:

- remove it if no reusable code is needed,
- or convert it into a deprecation/tombstone note,
- or extract useful renderer ideas into `ui/workbench/` and stop shipping Electron packaging as part of the redesigned product path.

The root README and command docs must no longer describe Electron as the future app direction.

## Minimum Viable Redesign Milestone

The tight MVP is:

1. User installs or enables the local slidex plugin.
2. User opens Codex App in a local/worktree thread for the slidex repository.
3. User invokes `@slidex create a new deck called <deck_id>` or equivalent.
4. Codex loads the slidex start skill.
5. The plugin MCP server or CLI creates `decks/<deck_id>/`.
6. slidex starts or emits the workbench.
7. Codex App shows the workbench through the chosen supported work surface.
8. User enters initial deck creation information.
9. slidex writes `brief.md` and `out/workbench_manifest.json` or equivalent deck-local state.

In scope:

- Codex CLI `0.138.0` compatibility,
- non-empty plugin MCP configuration,
- start skill,
- deck bootstrap,
- minimal workbench,
- security boundaries,
- doctor/test/docs proof,
- desktop de-scope.

Out of scope:

- full deck generation UX,
- complete slide editor,
- visual slide canvas,
- render/QA/package frontend,
- hosted service,
- cross-workspace cloud sync.

## Codex CLI 0.138.0 Upgrade Plan

### Hard Version Changes

Replace active assumptions:

- `cmd/slidex/workflow.go`: required Codex version becomes `0.138.0`.
- `plugins/slidex/.codex-plugin/version-lock.json`: required Codex CLI version becomes `0.138.0`.
- README, `commands.md`, plugin docs, skill references, doctor docs: active examples become `0.138.0`.
- `internal/codex/protocol/codex-cli-0.132.0/`: no longer the active protocol bundle.
- Any `0.133.0` design assumption: remove or mark explicitly historical.

### Generated Artifacts

Regenerate from exact Codex CLI `0.138.0`:

```bash
slidex codex schema refresh --codex-version 0.138.0
```

or the repository's current equivalent command after its usage text is updated.

The active protocol manifest must contain:

```json
{
  "codexVersion": "0.138.0"
}
```

The active Go constants must include:

```go
RequiredCodexCLIVersion = "0.138.0"
```

Record the exact npm package evidence for `@openai/codex@0.138.0`, including tarball URL, integrity, and shasum, in a manifest or implementation note if that package is used for generation.

### Tests

Update or add tests proving:

- `0.137.0` fails the minimum version gate,
- `0.138.0` passes,
- `0.139.0` passes if semver-compatible with the project's policy,
- doctor JSON reports `0.138.0`,
- generated protocol manifest and constants are `0.138.0`,
- schema refresh refuses stale active versions unless explicitly in historical fixture mode,
- `.mcp.json` defines a slidex MCP server,
- plugin version lock and runtime code agree,
- no active runtime/plugin/docs reference `0.132.0` or `0.133.0` except explicitly allowed historical notes,
- deck bootstrap writes only under `decks/<deck_id>/`,
- workbench loopback and token constraints hold.

### Doctor

`slidex doctor --codex --json` should be the primary proof mechanism.

Expected fields include:

```json
{
  "requiredCodex": "0.138.0",
  "minimumRequiredCodex": "0.138.0",
  "codexVersion": "0.138.0",
  "protocolSchema": {
    "version": "0.138.0",
    "path": "internal/codex/protocol/codex-cli-0.138.0",
    "status": "ok"
  },
  "plugin": {
    "name": "slidex",
    "installed": true,
    "enabled": true,
    "mcpConfigured": true,
    "defaultPromptPresent": true
  },
  "workbench": {
    "mode": "loopback-or-static",
    "status": "available"
  },
  "status": "ok"
}
```

Doctor should fail if:

- installed Codex is below `0.138.0`,
- protocol manifest is not `0.138.0`,
- generated constants are not `0.138.0`,
- plugin version lock is not `0.138.0`,
- plugin MCP config is empty,
- Electron remains the active product path,
- workbench violates loopback, token, or path constraints.

## Verification Checklist

- [ ] Re-read current official Codex plugin, Codex App, Codex App Server, and Apps SDK docs before implementation.
- [ ] Verify whether Codex CLI/App `0.138.0` exposes a documented plugin-owned Canvas or frontend mount contract.
- [ ] If such a contract exists, cite it and implement it.
- [ ] If not, document the gap and implement the supported work surface fallback without claiming Canvas support.
- [ ] Verify `@openai/codex@0.138.0` exact package metadata if used for generation.
- [ ] Run `codex --version` in the environment used for generation and record `0.138.0`.
- [ ] Regenerate active protocol artifacts under `internal/codex/protocol/codex-cli-0.138.0/`.
- [ ] Update active code, tests, docs, plugin version lock, and doctor checks to `0.138.0`.
- [ ] Confirm no active runtime/plugin/docs references remain for `0.132.0` or `0.133.0` outside explicitly labeled historical notes.
- [ ] Implement or update `plugins/slidex/.mcp.json` so it defines a slidex MCP server.
- [ ] Add the `slidex-start` skill and default plugin prompt for deck creation workbench startup.
- [ ] Implement deck bootstrap and workbench start/status/stop through CLI/MCP code paths.
- [ ] Build the minimal non-Electron workbench or static artifact fallback.
- [ ] Ensure deck bootstrap uses `slidex init <deck_id>` semantics or shared code that preserves template behavior.
- [ ] Ensure workbench writes only deck-local source/state files.
- [ ] Ensure any generated workbench state lives under `decks/<deck_id>/out/`.
- [ ] Run `gofmt` after Go edits.
- [ ] Run `go test ./...`.
- [ ] If schema contracts changed, run `go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json`.
- [ ] Run `go run ./cmd/slidex doctor --json`.
- [ ] Run `go run ./cmd/slidex doctor --codex --json` or the updated equivalent.
- [ ] If a UI package is introduced, run its exact pinned package-manager build/typecheck command.
- [ ] Smoke-test plugin invocation in a fresh Codex App thread.
- [ ] Capture evidence that `@slidex` invocation creates `decks/<deck_id>/` and displays the workbench in Codex App.
- [ ] Verify `brief.md`, `DESIGN.md` if applicable, `out/slidex_state.json` if applicable, and `out/workbench_manifest.json` are current and deck-local.
- [ ] Verify path traversal and writes outside `decks/<deck_id>/` fail.
- [ ] Verify loopback server binds only to `127.0.0.1`.
- [ ] Verify write endpoints require token authorization.
- [ ] Verify tokens are redacted from logs, manifests, and chat-visible output.
- [ ] Remove, de-scope, or tombstone `apps/desktop` as the active product path.
- [ ] Update README, `commands.md`, plugin docs, and skill docs so Codex Plugin workbench is the documented direction.
- [ ] Do not commit ignored deck outputs, generated delivery artifacts, `apps/desktop/node_modules/`, or `apps/desktop/dist/`.

## Constraints And Boundaries

- Preserve `cmd/slidex` as the canonical CLI implementation surface.
- Preserve deck-local workspace rules under `decks/<deck_id>/`.
- Preserve generated output rules under `decks/<deck_id>/out/`.
- Do not create root-level `brief.md`, `assets/`, `brand/`, `data/`, or `out/` for new work.
- Do not restore legacy standalone instruction-file workflows or direct execution fallback paths.
- Do not build a hosted service or SaaS product.
- Do not claim visual or Canvas QA passed unless the corresponding UI surface was actually inspected.
- Do not make `.app.json` or plugin app integration metadata pretend to be a custom UI mount unless current Codex documentation says so.
- Keep every runtime and dependency exactly pinned.
- When adding or updating dependencies, update lockfiles, manifests, docs, validation checks, and license/SHA evidence in the same coherent change.
- Do not revert unrelated user changes.
- Do not commit generated delivery artifacts unless explicitly requested.

## Iteration Policy

1. Start with a capability verification spike for Codex CLI/App `0.138.0` plugin UI, Canvas, App Server, plugin list/detail JSON, and MCP behavior.
2. If Canvas is confirmed, implement against that contract; otherwise choose the supported work surface fallback and record the gap.
3. Upgrade `0.138.0` protocol/version gates before building UI behavior that depends on them.
4. Make `plugins/slidex` installable and useful before broadening workbench UI.
5. Implement the narrow deck bootstrap path before adding later workflow controls.
6. Add security tests when the workbench server or write-capable endpoints appear.
7. Run the narrowest relevant check after each coherent change, then the repository-required broader checks.
8. Commit each small coherent change separately with Conventional Commits.
9. Keep docs aligned with the implementation in the same change that alters behavior.

## Blocked Stop Conditions

Stop and report blocked status if any of these remain true after reasonable investigation:

- No supported Codex plugin-owned Canvas/frontend contract exists and the user does not accept a supported Codex App work-surface fallback.
- Codex CLI `0.138.0` cannot be installed, invoked, or used to regenerate protocol artifacts in the current environment.
- `codex --version` cannot be made to report `0.138.0` in the generation environment.
- `slidex doctor --codex --json` cannot prove `0.138.0` compatibility after three focused attempts.
- Codex App plugin installation or invocation cannot be demonstrated after three attempts because of product/auth/environment issues.
- The plugin cannot determine the active workspace root safely.
- Required workbench behavior would need writes outside `decks/<deck_id>/`.
- Secure loopback/token behavior cannot be implemented or tested.
- Dependency pinning, license, integrity, or vendoring policy cannot be satisfied for a new required dependency.

## Expected Final Report Format

The final Goal run should report in Korean:

- changed files grouped by area,
- Codex `0.138.0` research sources and capability decision,
- GPT Pro review points incorporated,
- whether true Canvas was confirmed or a supported work-surface fallback was used,
- plugin install/invocation demo evidence,
- workbench URL/artifact path and deck path used for verification,
- protocol artifact paths and version proof,
- commands run and pass/fail results,
- doctor JSON summary,
- security tests and their results,
- desktop retirement/de-scope outcome,
- known residual risks and follow-up Goals for later deck workflow stages.

## Assumptions

- The immediate business objective is deck creation startup inside Codex App, not a full replacement for every later slidex workflow stage.
- The future slidex user is already working in a Codex local/worktree thread with repository access.
- A supported Codex App surface is acceptable if a true plugin-owned Canvas API is not publicly or contractually available.
- The Go CLI remains the source of truth for workspace creation and validation.
- Exact package evidence for `@openai/codex@0.138.0` is sufficient as a starting point, but the implementation must still prove the local generation command used a real `0.138.0` binary.

## Source References

- Codex plugins: <https://developers.openai.com/codex/plugins.md>
- Build Codex plugins: <https://developers.openai.com/codex/plugins/build.md>
- Codex App features: <https://developers.openai.com/codex/app/features.md>
- Codex App Server: <https://developers.openai.com/codex/app-server.md>
- Apps SDK MCP server and widgets: <https://developers.openai.com/apps-sdk/build/mcp-server.md>
- Apps SDK reference: <https://developers.openai.com/apps-sdk/reference.md>
- npm package metadata for `@openai/codex@0.138.0`: <https://registry.npmjs.org/@openai%2Fcodex/0.138.0>
