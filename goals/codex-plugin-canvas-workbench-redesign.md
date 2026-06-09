# Goal: slidex Desktop App을 Codex Plugin Canvas Workbench로 재설계

Slug: `codex-plugin-canvas-workbench-redesign`
Status: Draft
Activation: `/goal @goals/codex-plugin-canvas-workbench-redesign.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only after the activation command is run.

## Goal Text

slidex의 별도 Electron Desktop App 방향을 중단하고 Codex Plugin 중심의 Codex App 통합 UX로 재설계 및 구현한다. 완료란 Codex CLI `0.138.0` 기준으로 plugin manifest, skills, MCP server, App Server protocol artifacts, doctor checks, docs, tests가 갱신되고, Codex App에서 slidex plugin을 설치한 뒤 `@slidex` 또는 동등한 plugin 호출을 실행하면 plugin이 별도 local frontend server를 background로 시작하고, Codex App browser/work surface가 해당 local URL을 열어 deck 생성 workbench를 즉시 표시하며, `decks/<deck_id>/` 생성과 초기 deck creation input 저장을 수행하는 상태를 의미한다. 이 Goal에서 "Canvas" 또는 "Canvas-style experience"는 undocumented plugin-owned arbitrary UI mount API가 아니라, OpenDesign App/MagicPath plugin 방식처럼 background frontend server와 Codex App browser/work surface를 결합한 local workbench UX를 뜻한다. 구현은 documented/supported Codex App URL-opening 또는 browser/work-surface mechanism을 사용해야 하며, proprietary Canvas mount API가 존재한다고 주장하지 않는다. 향후 공식 plugin-owned Canvas/frontend lifecycle API가 확인되면 optional enhancement로 검토하고 출처와 evidence를 남긴다. 기존 CLI-first 계약, deck-local workspace, HTML/PDF delivery contract, exact dependency pinning, user change preservation을 보존하고, later-stage authoring/render/QA 고도화는 이번 Goal 범위 밖으로 둔다.

## Interview Summary

- Desired outcome: slidex 사용자가 별도 Electron 앱을 열지 않고 Codex App 채팅에서 slidex plugin을 호출하여 deck 생성 workbench를 바로 시작한다.
- Primary product decision: `apps/desktop`은 canonical UX가 아니며, `plugins/slidex`가 새 front door가 된다.
- Codex integration target: Codex CLI `0.138.0` 기준의 plugin, skills, MCP, App Server protocol, doctor, docs, tests를 일관되게 갱신한다.
- Canvas/work-surface policy: 사용자가 말한 "Canvas"는 proprietary hidden mount API가 아니라, plugin이 local frontend server를 background로 띄우고 Codex App browser/work surface에서 그 local URL을 여는 Canvas-style workflow를 의미한다. 따라서 MVP의 primary path는 supported Codex App browser/work surface에서 local workbench를 여는 것이다. 공식 plugin-owned Canvas lifecycle API는 별도 future enhancement로만 다루며, 존재한다고 가정하지 않는다.
- MVP boundary: plugin invocation -> deck bootstrap -> workbench display -> initial deck creation data 저장까지만 완료한다. full strategy/spec/build/render/QA/finalize UI는 후속 Goal이다.
- Evidence required: official Codex docs, GPT Pro review, local repo inspection, Codex CLI `0.138.0` artifact proof, doctor JSON, Go tests, plugin install/invocation demo, workbench security tests.
- Non-goals: hosted SaaS, standalone Electron app shipping, full slide editor, full pipeline UI, legacy standalone instruction workflows, generated delivery artifacts committing, proprietary Canvas mount API claim.
- Autonomy: implementation 중 명확한 공개 계약과 local evidence로 판단 가능한 사항은 직접 진행한다. Codex App browser/work-surface URL opening, plugin-managed server lifecycle, dependency/license/security policy가 불명확하면 증거와 함께 멈춘다.
- Budget: explicit token/time budget 없음. 완료 증거가 모두 충족되거나 blocked stop condition에 도달할 때까지 진행한다.
- Blocked condition: plugin이 local frontend server를 시작 또는 조율할 수 없고 Codex App이 해당 local URL을 browser/work surface로 열거나 표시하는 지원 경로도 없거나, Codex CLI `0.138.0` schema/protocol generation을 재현할 수 없거나, Codex App plugin install/invocation demo를 3회 연속 검증하지 못하면 중단한다. proprietary Canvas mount API 부재만으로는 blocker가 아니다.

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
- Public Codex documentation reviewed for this Goal did not confirm a generic plugin-owned Canvas API that auto-mounts arbitrary plugin frontend code when a plugin is invoked. That is no longer the primary target; the primary target is plugin-managed background server plus Codex App browser/work-surface open.
- Apps SDK documentation confirms ChatGPT app widgets through MCP resources, `text/html;profile=mcp-app`, `_meta["openai/outputTemplate"]`, and the `window.openai` UI bridge. This is relevant as a design reference but is not, by itself, proof that Codex App plugins can mount arbitrary proprietary Canvas widgets.
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
- Treat proprietary Canvas mount APIs as optional/future-facing; the primary architecture is the Canvas-style local workbench pattern.

A follow-up GPT Pro refinement was run after the user clarified that "Canvas" means a background frontend server opened inside the Codex App browser/work surface, comparable to OpenDesign App or MagicPath plugin behavior. The follow-up review concluded that this server/browser flow should be the primary architecture, not a fallback, and that the document should reserve uncertainty only for proprietary hidden mount APIs.

## Canvas-Style Workbench Capability Gate

### Confirmed Or Assumed For This Goal

- User intent: "Canvas" means a Codex App browser/work-surface-hosted local workbench experience, not an undocumented arbitrary plugin-owned UI mount API.
- Primary UX target: plugin invocation starts a background local frontend server, waits for readiness, then opens the local workbench URL inside the Codex App browser/work surface.
- User expects this pattern to be comparable to the OpenDesign App / MagicPath plugin style described by the user.
- MVP success can be achieved without claiming a proprietary Canvas API, as long as the user can invoke the plugin and immediately begin deck creation in the Codex App-hosted local workbench.
- slidex can become a Codex Plugin-first integration that bundles skills and MCP configuration.
- A plugin invocation can guide Codex to run slidex CLI/MCP operations in a local/worktree thread.
- App Server/protocol artifacts can and should be generated for a specific Codex version.

### Still Unconfirmed Or Must Verify

- The exact Codex Plugin mechanism for opening a local URL in the Codex App browser/work surface.
- Whether the local URL should be opened by a tool response, skill-guided browser action, Codex App browser API, artifact/link action, or another supported mechanism.
- Process lifecycle expectations: how the plugin should start, monitor, reuse, and terminate the background frontend server.
- Codex App behavior for loopback URLs, allowed schemes, port handling, refresh behavior, session/deep-link URLs, file downloads, clipboard, drag-and-drop, and WebSocket/SSE if the frontend uses them.
- Whether any official plugin-owned Canvas/frontend lifecycle API exists. This is optional/future-facing and not required for the primary MVP architecture.

### Required Decision Gate

The first implementation phase must verify current Codex `0.138.0` documentation and generated plugin/app-server schema for the supported path to open a plugin-managed local frontend URL in Codex App.

If a documented proprietary plugin-owned Canvas/frontend lifecycle exists:

- cite the documentation or product contract in implementation notes,
- evaluate whether it improves the local workbench architecture,
- treat it as an optional enhancement unless it is clearly the supported URL-opening path.

If no proprietary Canvas/frontend lifecycle exists:

- continue with the primary Canvas-style local workbench architecture,
- do not claim undocumented mount APIs,
- verify the plugin invocation -> background server ready -> Codex App browser/work surface opens local URL -> deck creation persists flow end to end.

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
       -> background local frontend server for the workbench
       -> Codex App browser/work surface opened to the local URL
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

Primary strategy is the Canvas-style local workbench architecture:

1. User invokes the slidex Codex Plugin.
2. Plugin starts or reuses a local frontend server in the background.
3. Plugin waits for readiness through `/healthz`, `/readyz`, or equivalent.
4. Plugin opens the local workbench URL inside the Codex App browser/work surface.
5. The workbench persists initial deck creation state under `decks/<deck_id>/`.
6. The plugin records server/session metadata under `decks/<deck_id>/out/workbench_manifest.json` or equivalent.

Reference URL shape:

```text
http://127.0.0.1:<port>/workbench/<session-id>?token=<short-lived-token>
```

The exact URL shape can change, but it must be loopback-only, session-scoped, token-protected for state-changing actions, and redacted in logs/chat-visible output.

Fallback strategy only applies if browser URL opening is not available:

- generate a static HTML workbench artifact under `decks/<deck_id>/out/` and have Codex App preview/open it through a supported artifact/browser path,
- document why the background-server path failed,
- keep the fallback subordinate to the primary Canvas-style server architecture.

Optional future strategy:

- if official Codex documentation exposes a proprietary plugin-owned Canvas/frontend lifecycle API, evaluate it as an enhancement,
- do not make that hidden API the baseline requirement unless Codex explicitly documents it as the supported route.

The acceptance wording is:

> The slidex plugin starts a background local frontend server and opens its loopback workbench URL inside the Codex App browser/work surface, where the user begins deck creation.

The rejected wording is:

> The plugin depends on an undocumented arbitrary UI mount API.

### Frontend Server Lifecycle And Security

Because the background frontend server is the primary architecture, completion requires explicit lifecycle and security behavior:

- bind to `127.0.0.1` by default; support `localhost` or `::1` only when intentionally tested,
- never bind to `0.0.0.0` by default,
- handle port collisions, stale previous servers, concurrent slidex sessions, and crash restart,
- expose readiness checks before opening the browser surface,
- require unguessable session IDs or short-lived tokens for write-capable routes,
- protect mutating routes against CSRF with token checks and Origin/Referer validation where applicable,
- avoid permissive CORS,
- reject path traversal and symlink escapes where relevant,
- write only under the selected repository/deck workspace,
- redact full tokens, private deck contents, and sensitive paths from logs and chat-visible output,
- define shutdown/reuse semantics so the server is not an unmanaged background process,
- define draft persistence, autosave, reload, and crash recovery behavior for the deck-creation form.

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
6. slidex starts or reuses the background local frontend server.
7. The plugin waits for readiness and opens the local workbench URL in the Codex App browser/work surface.
8. User enters initial deck creation information.
9. slidex writes `brief.md` and `out/workbench_manifest.json` or equivalent deck-local state.

In scope:

- Codex CLI `0.138.0` compatibility,
- non-empty plugin MCP configuration,
- start skill,
- deck bootstrap,
- minimal workbench served through the background local frontend server,
- server lifecycle/readiness/session management,
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
- [ ] Verify the supported Codex Plugin mechanism for opening a local URL in the Codex App browser/work surface.
- [ ] Verify that the plugin can start or coordinate a background local frontend server for the slidex workbench.
- [ ] Verify loopback URL behavior inside Codex App: `127.0.0.1` versus `localhost`, allowed schemes, port restrictions, navigation behavior, refresh behavior, and session/deep-link URL behavior.
- [ ] Verify frontend server readiness flow: the plugin must not open the browser surface before `/healthz`, `/readyz`, or equivalent readiness check succeeds.
- [ ] Verify process lifecycle: start, reuse existing instance, detect stale instance, restart after crash, and clean shutdown where appropriate.
- [ ] Verify session isolation: each deck creation session has a session ID, workspace path, or equivalent state boundary.
- [ ] Verify repository persistence: deck metadata, generated slide files, assets, exports, and recovery state are written to predictable repo paths.
- [ ] Verify that no documentation or UI copy implies the existence of a proprietary plugin-owned Canvas mount API.
- [ ] If a documented official Canvas/frontend lifecycle API is found, record evidence and evaluate it as an optional enhancement, not as an MVP blocker.
- [ ] Verify `@openai/codex@0.138.0` exact package metadata if used for generation.
- [ ] Run `codex --version` in the environment used for generation and record `0.138.0`.
- [ ] Regenerate active protocol artifacts under `internal/codex/protocol/codex-cli-0.138.0/`.
- [ ] Update active code, tests, docs, plugin version lock, and doctor checks to `0.138.0`.
- [ ] Confirm no active runtime/plugin/docs references remain for `0.132.0` or `0.133.0` outside explicitly labeled historical notes.
- [ ] Implement or update `plugins/slidex/.mcp.json` so it defines a slidex MCP server.
- [ ] Add the `slidex-start` skill and default plugin prompt for deck creation workbench startup.
- [ ] Implement deck bootstrap and workbench start/status/stop through CLI/MCP code paths.
- [ ] Build the minimal non-Electron workbench served by the background local frontend server.
- [ ] Keep static artifact preview only as an explicit degraded fallback if Codex App cannot open local workbench URLs.
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
- [ ] Verify the workbench server does not bind to `0.0.0.0` by default and is not reachable from the LAN.
- [ ] Verify port collision handling and stale server recovery.
- [ ] Verify write endpoints require token authorization.
- [ ] Verify mutating routes reject unexpected origins and avoid permissive CORS.
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
- Do not claim visual or Canvas-style workbench QA passed unless the corresponding Codex App browser/work surface was actually inspected.
- Do not make `.app.json` or plugin app integration metadata pretend to be a proprietary custom UI mount unless current Codex documentation says so.
- Keep every runtime and dependency exactly pinned.
- When adding or updating dependencies, update lockfiles, manifests, docs, validation checks, and license/SHA evidence in the same coherent change.
- Do not revert unrelated user changes.
- Do not commit generated delivery artifacts unless explicitly requested.

## Iteration Policy

1. Start with a capability verification spike for Codex CLI/App `0.138.0` local URL opening, browser/work-surface behavior, plugin list/detail JSON, App Server, and MCP behavior.
2. Implement the Canvas-style local workbench path: plugin invocation -> background frontend server ready -> Codex App browser/work surface opens local URL -> deck creation state persists.
3. Upgrade `0.138.0` protocol/version gates before building UI behavior that depends on them.
4. Make `plugins/slidex` installable and useful before broadening workbench UI.
5. Implement the narrow deck bootstrap path before adding later workflow controls.
6. Add security tests when the workbench server or write-capable endpoints appear.
7. Run the narrowest relevant check after each coherent change, then the repository-required broader checks.
8. Commit each small coherent change separately with Conventional Commits.
9. Keep docs aligned with the implementation in the same change that alters behavior.

## Blocked Stop Conditions

Stop and report blocked status if any of these remain true after reasonable investigation:

- Codex Plugin execution cannot start or coordinate a local frontend server, and Codex App provides no supported way to open or display the resulting local URL in its browser/work surface.
- The end-to-end flow cannot be provided: invocation -> background frontend server ready -> Codex App browser/work surface opens local workbench URL -> deck creation state persists to the repo.
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
- how the Canvas-style local workbench flow was implemented and verified,
- whether any proprietary Canvas/frontend API was found and, if so, why it was or was not used,
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
- In this Goal, "Canvas" means the Codex App browser/work-surface-hosted local workbench opened from a plugin-managed background frontend server.
- A proprietary plugin-owned Canvas mount API is not required for MVP success.
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
