# slidex Codex Plugin

`plugins/slidex` is the primary UX front door for new slidex deck creation.
The Go CLI remains the implementation source of truth.

## Startup Flow

1. Install or enable the local `slidex` plugin.
2. Invoke `@slidex` or `slidex-start` in a Codex App local/worktree thread.
3. Run `slidex workbench start --deck-id <deck_id>` from the workspace root.
4. Open the returned `http://127.0.0.1:<port>/workbench/<session>` URL in the
   Codex App in-app browser, either by clicking the URL, navigating manually, or
   asking `@Browser` to navigate there.
5. Save initial deck creation input from the workbench.
6. Verify `decks/<deck_id>/brief.md` and
   `decks/<deck_id>/out/workbench_draft.json` plus
   `decks/<deck_id>/out/workbench_manifest.json`.
7. After actually inspecting the Codex App browser surface, record deck-local
   evidence:

```bash
slidex workbench evidence --deck-id <deck_id> \
  --inspector "<name-or-role>" \
  --surface codex_app_in_app_browser \
  --invocation "@slidex create a deck called <deck_id>" \
  --url "http://127.0.0.1:<port>/workbench/<session>" \
  --workbench-visible \
  --saved-input-verified
```

This writes `decks/<deck_id>/out/workbench_browser_evidence.json`. Do not claim
the Codex App browser/work-surface path has passed until this evidence reflects
an actual inspection.

The workbench binds to `127.0.0.1`, uses session-scoped URLs, requires
`X-Slidex-Workbench-Token` for writes, and records only token hashes in
manifests.

Because the plugin MCP configuration runs `slidex` from `PATH`, install the
current repository binary before local plugin invocation tests:

```bash
mise exec -- go install ./cmd/slidex
```

Before a full Codex App GUI smoke, verify the App Server/plugin/MCP layer:

```bash
slidex codex app-server plugin-smoke --workspace /tmp/slidex-plugin-smoke --deck-id plugin-smoke
```

This proves App Server can read the installed `slidex` plugin, discover
`slidex-start`, and call `workbench.start/status/stop` through the plugin MCP
server. It does not prove that the Codex App browser displayed the workbench;
that still requires `workbench_browser_evidence.json` from actual inspection.

## Codex 0.138.0 Evidence

- Local CLI: `codex --version` reported `codex-cli 0.138.0`.
- Protocol generation: `slidex codex schema refresh --codex-version 0.138.0`
  writes `internal/codex/protocol/codex-cli-0.138.0/`.
- npm package evidence for `@openai/codex@0.138.0`:
  - tarball: `https://registry.npmjs.org/@openai/codex/-/codex-0.138.0.tgz`
  - integrity: `sha512-m5vUQeN+oFkCt594xbVujSzzT3CiihFuRXlbQfqJ7sEXH4yNeY99e6ryqFZgQSz/deQcaVbjhhO/TR0YJ1Vsjg==`
  - shasum: `f5365efc6b1723eca8723c8dc779fe7ebc797ab4`

## Surface Decision

Official Codex docs confirm plugins, bundled skills, bundled MCP servers, and
the Codex App in-app browser for local development URLs. The generated Codex
App Server `0.138.0` schema does not expose a documented plugin-owned arbitrary
Canvas mount API or a client request method that directly opens the Codex App
browser from a plugin.

Therefore slidex uses a Canvas-style local workbench: the plugin starts a
loopback frontend server and returns a local URL for the supported Codex App
browser/work-surface path. It does not claim or depend on a proprietary Canvas
mount lifecycle.

Official references:

- https://developers.openai.com/codex/plugins
- https://developers.openai.com/codex/plugins/build
- https://developers.openai.com/codex/mcp
- https://developers.openai.com/codex/app/browser
- https://developers.openai.com/codex/app-server
