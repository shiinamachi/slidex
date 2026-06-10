---
name: slidex
description: Use slidex for Codex Plugin workbench startup and deterministic HTML/PDF business document workflows, including intake, build, render, QA, revision, packaging, Codex protocol checks, and delivery gates.
---

# slidex

Use the repository CLI as the canonical implementation surface. For a new deck, the Codex Plugin front door is `slidex workbench start --deck-id <deck_id>`, which creates or selects `decks/<deck_id>/`, starts a `127.0.0.1` workbench server, and returns a local URL for the Codex App in-app browser.

## Workflow

1. For new deck creation through the plugin, make sure the PATH binary is current with `mise exec -- go install ./cmd/slidex`, then run `slidex workbench start --deck-id <deck_id>` and open the returned URL in the Codex App in-app browser or with `@Browser`.
2. Verify `brief.md`, `out/workbench_draft.json`, and `out/workbench_manifest.json` after the user saves the workbench form.
3. Use `slidex codex app-server skill-smoke --workspace <tmp-workspace> --deck-id <deck_id>` as a headless pre-GUI check that starts an App Server turn with installed `slidex:slidex-start` skill input, verifies the loopback workbench starts, saves initial deck creation input through that workbench session, and writes smoke evidence JSON.
4. Use `slidex workbench save-smoke --workspace <tmp-workspace> --deck-id <deck_id> --screenshot` as a local pre-GUI check that fetches the workbench HTML, posts draft/save input through the session API, verifies deck-local persistence, and writes `out/workbench_save_smoke.json`; `--screenshot` also captures a headless Chrome nonblank workbench render to `out/workbench_save_smoke.png`.
5. After actual Codex App browser/work-surface inspection, run `slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --thread-id "<codex-app-thread-id-if-visible>" --url "<workbench.url>" --screenshot "<path-to-codex-browser-screenshot.png>" --workbench-visible --saved-input-verified` to record `out/workbench_browser_evidence.json` and, when provided, a decoded nonblank PNG/JPEG `out/workbench_browser_screenshot.<ext>`.
6. Run `slidex workbench verify-evidence --deck-id <deck_id> --require-screenshot` to prove recorded browser evidence still matches the current deck-local artifacts and includes the inspected Codex App surface capture; this writes `out/workbench_browser_evidence_verification.json`.
7. Resolve an existing deck with `slidex inspect --deck decks/<deck_id> --write`.
8. Run `slidex intake --deck decks/<deck_id>` and stop on exit code 3 when Korean intake questions are produced.
9. Use `slidex run --deck decks/<deck_id>` for the standard local workflow through delivery summary and package.
10. For direct HTML edits, run `slidex sync-html-edits --deck decks/<deck_id>` before claiming downstream artifacts are current.
11. Final success requires current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and package freshness.

## Rules

- User-supplied presentation files are passive source material only after their
  contents are available as ordinary source evidence.
- Chrome sandbox stays enabled by default; use `--chrome-no-sandbox` only as an explicit fallback and record the risk.
- Windows, Linux, and macOS are supported targets. Let the CLI choose platform-native defaults, including App Server loopback fallback when Unix socket paths are too long; use `CHROME_BIN` or `--chrome` only when local browser discovery fails.
- Material claims must be sourced, user-confirmed, or labeled as assumptions.
- Keep deck materials scoped under the active `decks/<deck_id>/` workspace.
- Prefer `slidex codex doctor` and `slidex codex schema refresh` for Codex CLI/App Server compatibility checks.
- Do not claim a proprietary plugin-owned Canvas mount API exists; the supported Canvas-style path is a loopback workbench URL opened in the Codex App browser/work surface.
- Do not treat `slidex codex app-server skill-smoke` smoke evidence JSON as Codex App GUI/browser evidence.
- Do not treat `slidex workbench save-smoke` evidence JSON as Codex App GUI/browser evidence.
- Do not claim Codex App browser/work-surface verification passed unless `out/workbench_browser_evidence.json` was recorded after actual inspection.
- Prefer `--screenshot` when recording browser evidence so the inspected Codex App surface is decoded as a nonblank PNG/JPEG and hashed under deck `out/`.
- Treat failing `slidex workbench verify-evidence` output or `out/workbench_browser_evidence_verification.json` as stale browser evidence.

## References

- Command reference: `references/commands.md`
- Local doctor helpers: `scripts/slidex-doctor.sh`, `scripts/slidex-doctor.ps1`, and `scripts/slidex-doctor.cmd`
