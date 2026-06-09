---
name: slidex
description: Use slidex for Codex Plugin workbench startup and deterministic HTML/PDF business document workflows, including intake, build, render, QA, revision, packaging, Codex protocol checks, and delivery gates.
---

# slidex

Use the repository CLI as the canonical implementation surface. For a new deck, the Codex Plugin front door is `slidex workbench start --deck-id <deck_id>`, which creates or selects `decks/<deck_id>/`, starts a `127.0.0.1` workbench server, and returns a local URL for the Codex App in-app browser.

## Workflow

1. For new deck creation through the plugin, make sure the PATH binary is current with `mise exec -- go install ./cmd/slidex`, then run `slidex workbench start --deck-id <deck_id>` and open the returned URL in the Codex App in-app browser or with `@Browser`.
2. Verify `brief.md`, `out/workbench_draft.json`, and `out/workbench_manifest.json` after the user saves the workbench form.
3. Resolve an existing deck with `slidex inspect --deck decks/<deck_id> --write`.
4. Run `slidex intake --deck decks/<deck_id>` and stop on exit code 3 when Korean intake questions are produced.
5. Use `slidex run --deck decks/<deck_id>` for the standard local workflow through delivery summary and package.
6. For direct HTML edits, run `slidex sync-html-edits --deck decks/<deck_id>` before claiming downstream artifacts are current.
7. Final success requires current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and package freshness.

## Rules

- User-supplied presentation files are passive source material only after their
  contents are available as ordinary source evidence.
- Chrome sandbox stays enabled by default; use `--chrome-no-sandbox` only as an explicit fallback and record the risk.
- Material claims must be sourced, user-confirmed, or labeled as assumptions.
- Keep deck materials scoped under the active `decks/<deck_id>/` workspace.
- Prefer `slidex codex doctor` and `slidex codex schema refresh` for Codex CLI/App Server compatibility checks.
- Do not claim a proprietary plugin-owned Canvas mount API exists; the supported Canvas-style path is a loopback workbench URL opened in the Codex App browser/work surface.

## References

- Command reference: `references/commands.md`
- Local doctor helper: `scripts/slidex-doctor.sh`
