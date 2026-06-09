---
name: slidex
description: Use slidex for Codex Plugin workbench startup and deterministic HTML/PDF business document workflows, including intake, build, render, QA, revision, packaging, Codex protocol checks, and delivery gates.
---

# slidex

Use the repository CLI as the canonical implementation surface. For a new deck, the Codex Plugin front door is `slidex workbench start --deck-id <deck_id>`, which creates or selects `decks/<deck_id>/`, starts a `127.0.0.1` workbench server, and returns a local URL for the Codex App in-app browser.

## Workflow

1. For new deck creation through the plugin, make sure the PATH binary is current with `mise exec -- go install ./cmd/slidex`, then run `slidex workbench start --deck-id <deck_id>` and open the returned URL in the Codex App in-app browser or with `@Browser`.
2. Verify `brief.md`, `out/workbench_draft.json`, and `out/workbench_manifest.json` after the user saves the workbench form.
3. Use `slidex codex app-server plugin-smoke --workspace <tmp-workspace> --deck-id <deck_id>` as a pre-GUI check that App Server can read the installed plugin, discover `slidex-start`, and call `workbench.start/status/stop` through MCP.
4. After actual Codex App browser/work-surface inspection, run `slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --url "<workbench.url>" --workbench-visible --saved-input-verified` to record `out/workbench_browser_evidence.json`.
5. Run `slidex workbench verify-evidence --deck-id <deck_id>` to prove recorded browser evidence still matches the current deck-local artifacts.
6. Resolve an existing deck with `slidex inspect --deck decks/<deck_id> --write`.
7. Run `slidex intake --deck decks/<deck_id>` and stop on exit code 3 when Korean intake questions are produced.
8. Use `slidex run --deck decks/<deck_id>` for the standard local workflow through delivery summary and package.
9. For direct HTML edits, run `slidex sync-html-edits --deck decks/<deck_id>` before claiming downstream artifacts are current.
10. Final success requires current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and package freshness.

## Rules

- User-supplied presentation files are passive source material only after their
  contents are available as ordinary source evidence.
- Chrome sandbox stays enabled by default; use `--chrome-no-sandbox` only as an explicit fallback and record the risk.
- Material claims must be sourced, user-confirmed, or labeled as assumptions.
- Keep deck materials scoped under the active `decks/<deck_id>/` workspace.
- Prefer `slidex codex doctor` and `slidex codex schema refresh` for Codex CLI/App Server compatibility checks.
- Do not claim a proprietary plugin-owned Canvas mount API exists; the supported Canvas-style path is a loopback workbench URL opened in the Codex App browser/work surface.
- Do not claim Codex App browser/work-surface verification passed unless `out/workbench_browser_evidence.json` was recorded after actual inspection.
- Treat failing `slidex workbench verify-evidence` output as stale browser evidence.

## References

- Command reference: `references/commands.md`
- Local doctor helper: `scripts/slidex-doctor.sh`
