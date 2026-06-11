---
name: slidex-run
description: Run the full slidex CLI workflow for an existing deck through render, QA, delivery summary, and package gates.
user-invocable: true
---

# slidex-run

Use this only for existing deck workspaces after startup input is complete. If
the user is creating a new deck, stop this skill path and use `slidex-start`
instead so the local Workbench is displayed first. If the Workbench has already
started generation and `out/workbench_manifest.json` reports
`generationStatus: running`, monitor that run instead of starting a duplicate
one.

## Workflow

1. Confirm this is not a new deck creation request. If it is new, switch to `slidex-start` and do not run the rest of this workflow.
2. Resolve the deck under `decks/<deck_id>/`.
3. Run `slidex inspect --deck decks/<deck_id> --write`.
4. Run `slidex run --deck decks/<deck_id>`.
5. If intake stops with exit code `3`, surface the Korean intake questions and wait for user answers.
6. Before claiming completion, verify current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Direct stage commands are for focused repair or verification only.

Do not use `slidex init`, manual directory creation, or direct `out/final_deck.html` authoring as a fallback for new deck startup. The default template is embedded in the CLI; new deck startup should still go through `slidex-start` / `workbench.start`.
