# slidex Codex Plugin

`plugins/slidex` is the primary UX front door for new slidex deck creation.
The Go CLI remains the implementation source of truth.

## Startup Flow

1. Install or enable the local `slidex` plugin.
2. Invoke `@slidex` or `slidex-start` in a Codex App local/worktree thread.
3. Run `slidex workbench start --deck-id <deck_id>` from the workspace root.
   New deck creation must go through this local Workbench path; do not fall back to
   `slidex init`, manual directory creation, or direct `out/final_deck.html`
   authoring.
   When the user invocation contains usable details, pass them as seed fields
   such as `--initial-request`, `--title`, `--audience`, `--decision-goal`,
   `--source-notes`, `--key-messages`, `--required-claims`, `--constraints`,
   and `--output-expectations`.
4. For release-package installs, startup automatically checks the configured
   production/canary channel and applies a newer verified release before the
   Workbench opens. If the response reports `autoUpdate.blocksWorkbench: true`,
   stop this thread and follow the returned restart or pending-activation
   instruction.
5. Follow the returned `agentBrowserInstruction`. Packaged Codex MCP startup
   sets `SLIDEX_BROWSER_OPEN=agent`, so the response tells the agent to use
   `@Browser` to open the `http://127.0.0.1:<port>/workbench/<session>` URL in
   the Codex App in-app browser without emitting the legacy structured
   `browserOpen` intent. Pass `browserOpenMode=manual` only when no browser
   should be opened.
6. Complete the local Solid Workbench. It asks for title, audience, decision goal,
   source notes, key messages, output expectations, and optional
   claim/constraint details.
7. Select `Complete & generate` in the wizard. This writes `brief.md`,
   `out/workbench_draft.json`, and `out/workbench_manifest.json`, records
   `wizardCompletedAt`, and starts `slidex run --deck decks/<deck_id>
   --non-interactive` in the background.
8. Verify `decks/<deck_id>/out/workbench_manifest.json` reports
   `generationStatus` and inspect `decks/<deck_id>/out/workbench_generation.log`
   if generation fails.
9. Optionally run local HTTP save smoke before GUI evidence:

```bash
slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id>
```

This fetches the workbench HTML, posts draft/save input through the session API,
verifies `brief.md`, `out/workbench_draft.json`,
`out/workbench_manifest.json`, token redaction, and writes
`out/workbench_save_smoke.json`. It is not Codex App GUI/browser evidence.

10. After actually inspecting the Codex App browser surface, record deck-local
   evidence:

```bash
slidex workbench evidence --deck-id <deck_id> \
  --inspector "<name-or-role>" \
  --surface codex_app_in_app_browser \
  --invocation "@slidex create a deck called <deck_id>" \
  --thread-id "<codex-app-thread-id-if-visible>" \
  --url "http://127.0.0.1:<port>/workbench/<session>" \
  --screenshot "<path-to-codex-browser-screenshot.png>" \
  --workbench-visible \
  --saved-input-verified
```

Windows PowerShell 또는 `cmd.exe`에서는 같은 명령을 단일 라인으로 실행할 수 있습니다.

```powershell
slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --thread-id "<codex-app-thread-id-if-visible>" --url "http://127.0.0.1:<port>/workbench/<session>" --screenshot "<path-to-codex-browser-screenshot.png>" --workbench-visible --saved-input-verified
```

`--invocation` is required and must describe the actual `@slidex` or
`slidex-start` plugin call. `--thread-id` should be recorded when the Codex App
thread id is visible. This writes
`decks/<deck_id>/out/workbench_browser_evidence.json`. Do not claim the Codex
App browser/work-surface path has passed until this evidence reflects an actual
inspection. `--screenshot` is optional but recommended; it verifies the
inspected Codex App browser capture is a decodable nonblank PNG/JPEG, copies it
under `out/workbench_browser_screenshot.<ext>`, and records its hash in the
evidence.

After recording the evidence, verify it still matches the current deck-local
artifacts:

```bash
slidex workbench verify-evidence --deck-id <deck_id> --require-screenshot
```

The workbench binds to `127.0.0.1`, uses session-scoped URLs, requires
`X-Slidex-Workbench-Token` for writes, and records only token hashes in
manifests.

The CLI embeds the default `decks/_template`, so installed binaries can start a
new workbench even when the active user workspace does not contain a template
folder. The MCP `deck.bootstrap` tool is kept only as a deprecated alias and
returns the same Workbench startup response as `workbench.start`.
Set `SLIDEX_AUTO_UPDATE=0` only when deliberately disabling release update
preflight for diagnostics.

Because the plugin MCP configuration runs `slidex` from `PATH`, local source
checkout plugin invocation tests should install the current repository binary:

```bash
mise exec -- go install ./cmd/slidex
```

The CLI and plugin workflow target Windows, Linux, and macOS. Browser discovery
checks common Chrome/Chromium and Microsoft Edge locations, Chrome-for-Testing,
Playwright, and Puppeteer cache paths on each OS, and the managed App Server
chooses a platform-native default transport: Unix sockets on Linux/macOS, with
`127.0.0.1` loopback WebSocket fallback when the OS socket path limit would be
exceeded, and `127.0.0.1` loopback WebSocket on Windows. Local doctor helpers
are available as `scripts/slidex-doctor.sh` for Unix shells,
`scripts/slidex-doctor.ps1` for Windows PowerShell, and
`scripts/slidex-doctor.cmd` for Windows `cmd.exe`.

Before a full Codex App GUI smoke, run the headless pre-GUI App Server skill
smoke as a separate plugin/App Server path check:

```bash
slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id skill-smoke
```

This starts an App Server turn with the installed `slidex:slidex-start` skill
input, verifies that the loopback workbench starts, saves initial deck creation
input through that same workbench session, and writes smoke evidence JSON. It
does not prove that the Codex App GUI/browser displayed the workbench; that
still requires `slidex workbench evidence` followed by
`slidex workbench verify-evidence` after actual inspection.

## Codex 0.138.0 Evidence

- Local CLI: `codex --version` reported `codex-cli 0.138.0`.
- Protocol generation: `slidex codex schema refresh --codex-version 0.138.0`
  writes `internal/codex/protocol/codex-cli-0.138.0/`.
- npm package evidence for `@openai/codex@0.138.0`:
  - tarball: `https://registry.npmjs.org/@openai/codex/-/codex-0.138.0.tgz`
  - integrity: `sha512-m5vUQeN+oFkCt594xbVujSzzT3CiihFuRXlbQfqJ7sEXH4yNeY99e6ryqFZgQSz/deQcaVbjhhO/TR0YJ1Vsjg==`
  - shasum: `f5365efc6b1723eca8723c8dc779fe7ebc797ab4`

## Surface Decision

Official Codex docs confirm plugins, bundled skills, bundled MCP servers,
Browser Use for operating the Codex App in-app browser, and the Codex App
in-app browser for local development URLs. The generated Codex App Server
`0.138.0` schema does not expose a documented plugin-owned arbitrary Canvas
mount API or a client request method that directly opens the Codex App browser
from a plugin. The schema's `openPage` / `open_page` entries are Web Search
actions, not a plugin workbench browser-open request contract.

Therefore slidex uses a Canvas-style local workbench: the plugin starts a
loopback frontend server and returns a local URL. Packaged Codex MCP config
sets `SLIDEX_BROWSER_OPEN=agent`, which returns an `agentBrowserInstruction`
telling Codex to use `@Browser` for the Codex App in-app browser while
suppressing the legacy structured `browserOpen` intent. Callers can pass
`browserOpenMode=manual` to return only the URL or `browserOpen=true` when they
want the structured `browserOpen` intent. It does not claim or depend on a
proprietary Canvas mount lifecycle.

Official references:

- https://developers.openai.com/codex/plugins
- https://developers.openai.com/codex/plugins/build
- https://developers.openai.com/codex/mcp
- https://developers.openai.com/codex/app/browser
- https://developers.openai.com/codex/app-server
