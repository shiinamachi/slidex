---
name: slidex-start
description: Start a slidex deck creation session from Codex by bootstrapping decks/<deck_id>, starting the loopback workbench, and opening or instructing the Codex App browser to open the returned local URL.
user-invocable: true
---

# slidex-start

Use this as the default slidex plugin entry point.

## Workflow

1. Resolve the active workspace root from the current Codex local/worktree thread.
2. Pick or ask for a `deck_id`; it must be a simple deck id, not a path.
3. Run `slidex workbench start --deck-id <deck_id>` from the workspace root.
4. Confirm the JSON response reports a loopback `workbench.url`, `serverBind: 127.0.0.1`, and `tokenRedacted: true`.
5. Open the returned URL in the Codex App in-app browser by clicking it or asking `@Browser` to navigate to it. Public Codex 0.138.0 docs do not expose a plugin-owned arbitrary Canvas mount API.
6. Have the user enter deck title, audience, decision goal, source notes, and output expectations in the workbench.
7. Verify `decks/<deck_id>/brief.md`, `decks/<deck_id>/out/workbench_draft.json`, and `decks/<deck_id>/out/workbench_manifest.json` are written.
8. Use `slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a local HTTP pre-GUI save check when needed; it verifies workbench HTML bootstrap, draft/save persistence, token redaction, and `out/workbench_save_smoke.json`, but it is not Codex App GUI/browser evidence.
9. After the Codex App browser surface has actually been inspected, run `slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --thread-id "<codex-app-thread-id-if-visible>" --url "<workbench.url>" --screenshot "<path-to-codex-browser-screenshot.png>" --workbench-visible --saved-input-verified` to write `decks/<deck_id>/out/workbench_browser_evidence.json` and, when provided, `decks/<deck_id>/out/workbench_browser_screenshot.<ext>`.
10. Run `slidex workbench verify-evidence --deck-id <deck_id> --require-screenshot` before treating the browser evidence as current.

Do not run the full render, QA, or package workflow during startup unless the user asks for it.

## Rules

- Keep writes under `decks/<deck_id>/`.
- Do not expose full workbench write tokens in chat-visible output.
- If local plugin invocation does not expose `workbench.start`, install the current repository binary with `mise exec -- go install ./cmd/slidex` because the plugin MCP config resolves `slidex` through PATH.
- Use `slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a headless pre-GUI App Server check that sends installed `slidex:slidex-start` skill input, verifies the loopback workbench starts, and writes smoke evidence JSON; it does not replace actual Codex App GUI/browser inspection.
- Use `slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id>` only as a local HTTP pre-GUI save check; it does not replace actual Codex App GUI/browser inspection.
- Treat the local workbench as the Canvas-style surface for this plugin; do not claim a proprietary Canvas lifecycle API exists.
- Do not claim the Codex App browser/work-surface path passed unless `out/workbench_browser_evidence.json` was recorded after actual inspection.
- Prefer `--screenshot` when recording browser evidence so the inspected Codex App surface is hashed under deck `out/`.
- Treat a failing `slidex workbench verify-evidence` result as stale browser evidence that must be re-inspected or re-recorded.
