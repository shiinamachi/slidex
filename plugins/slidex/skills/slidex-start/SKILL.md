---
name: slidex-start
description: Start a slidex deck creation session from Codex by bootstrapping decks/<deck_id>, starting the loopback Solid Workbench, and explicitly opening the returned local workbench URL with @Browser by default.
user-invocable: true
---

# slidex-start

Use this as the required slidex plugin entry point for every new deck. A new
deck creation request must show the local Workbench before generation proceeds.

## Workflow

1. Resolve the active workspace root from the current Codex local/worktree thread.
2. Pick or ask for a `deck_id`; it must be a simple deck id, not a path.
3. Summarize the user's invocation into `initialRequest` and extract any explicit title, audience, decision goal, source notes, key messages, constraints, claims, and output expectations.
4. Run `slidex workbench start --deck-id <deck_id>` from the workspace root, passing any extracted fields with the matching `--initial-request`, `--title`, `--audience`, `--decision-goal`, `--source-notes`, `--key-messages`, `--required-claims`, `--constraints`, and `--output-expectations` flags when available. The CLI has an embedded fallback copy of the default `decks/_template`, so missing workspace template files are not a reason to create directories manually.
5. If using MCP, call `workbench.start` with the same seed fields as structured arguments.
6. Treat `autoUpdate` in the response as authoritative. For release-package installs, `workbench.start` automatically checks the configured production/canary release channel and applies a newer verified release before opening the Workbench. If `autoUpdate.blocksWorkbench` is `true`, do not continue deck creation in this thread.
7. If `autoUpdate.status` is `applied_restart_required`, tell the user Codex must be restarted and a new thread started before using slidex again. If `autoUpdate.status` is `pending_activation`, surface `autoUpdate.pendingActivationCommand` when present and tell the user the staged update must be activated after the old slidex MCP process exits, then Codex must be restarted and a new thread started.
8. When the response is not blocked by `autoUpdate`, confirm the JSON response reports a loopback `workbench.url`, `serverBind: 127.0.0.1`, and `tokenRedacted: true`.
9. Follow the returned `agentBrowserInstruction`: explicitly use `@Browser` to open `workbenchURL` or `workbench.url` in the Codex App in-app browser. Do not use Chrome or an external browser for this startup. If Browser use is unavailable in the current thread, present the URL and say that the Browser plugin must be enabled or the user must open the URL manually. Public Codex 0.138.0 docs do not expose a plugin-owned arbitrary Canvas mount API or direct browser-open App Server request.
10. Have the user complete the local Workbench. The Workbench asks for enough deck intake detail for the agent to proceed: title, audience, decision goal, source notes, key messages, output expectations, and optional claim/constraint details.
11. When the user selects `Complete & generate`, the workbench saves `brief.md`, `out/workbench_draft.json`, and `out/workbench_manifest.json`, records `wizardCompletedAt`, and starts `slidex run --deck decks/<deck_id> --non-interactive` in the background.
12. Verify `decks/<deck_id>/out/workbench_manifest.json` reports `generationStatus` as `running`, `completed`, or `failed`, and inspect `decks/<deck_id>/out/workbench_generation.log` if generation fails.
13. Use `slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a local HTTP pre-GUI save check when needed; it verifies workbench HTML bootstrap, draft/save persistence, token redaction, and `out/workbench_save_smoke.json`, but it is not Codex App GUI/browser evidence.
14. After the Codex App browser surface has actually been inspected, run `slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --thread-id "<codex-app-thread-id-if-visible>" --url "<workbench.url>" --screenshot "<path-to-codex-browser-screenshot.png>" --workbench-visible --saved-input-verified` to write `decks/<deck_id>/out/workbench_browser_evidence.json` and, when provided, a decoded nonblank PNG/JPEG `decks/<deck_id>/out/workbench_browser_screenshot.<ext>`.
15. Run `slidex workbench verify-evidence --deck-id <deck_id> --require-screenshot` before treating the browser evidence as current.

Do not start `slidex run`, `slidex init`, `deck.bootstrap`, or direct `out/final_deck.html` authoring for a new deck before the local Workbench has been displayed. `deck.bootstrap` is a deprecated MCP alias that must return the same Workbench startup response as `workbench.start`, including `agentBrowserInstruction` when configured.

Do not start a second duplicate `slidex run` outside the wizard while `generationStatus` is `running`.

## Rules

- Keep writes under `decks/<deck_id>/`.
- Do not expose full workbench write tokens in chat-visible output.
- Never recover from default template lookup issues by manually creating deck folders or writing `out/final_deck.html`; rerun `slidex workbench start` with the current CLI because the default template is embedded in the binary.
- Do not bypass `autoUpdate.blocksWorkbench`; after an automatic update is applied, stop and require Codex restart/new thread before opening the Workbench.
- Treat packaged MCP `SLIDEX_BROWSER_OPEN=agent` as the default startup policy: explicitly use `@Browser` via the returned `agentBrowserInstruction`; use `browserOpenMode=manual` only when the user does not want any browser opened.
- If local plugin invocation does not expose `workbench.start`, install the current repository binary with `mise exec -- go install ./cmd/slidex` because the plugin MCP config resolves `slidex` through PATH.
- Use `slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a headless pre-GUI App Server check that sends installed `slidex:slidex-start` skill input, verifies the loopback workbench starts, and writes smoke evidence JSON; it does not replace actual Codex App GUI/browser inspection.
- Use `slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a local HTTP pre-GUI save check; it does not replace actual Codex App GUI/browser inspection.
- Treat the local Solid Workbench as the Canvas-style surface for this plugin; do not claim a proprietary Canvas lifecycle API exists.
- Do not claim the Codex App browser/work-surface path passed unless `out/workbench_browser_evidence.json` was recorded after actual inspection.
- Prefer `--screenshot` when recording browser evidence so the inspected Codex App surface is decoded as a nonblank PNG/JPEG and hashed under deck `out/`.
- Treat a failing `slidex workbench verify-evidence` result as stale browser evidence that must be re-inspected or re-recorded.
