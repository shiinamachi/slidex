---
name: slidex-run
description: Run the full slidex CLI workflow for an existing deck through render, QA, delivery summary, and package gates.
user-invocable: true
---

# slidex-run

Use this for existing deck workspaces after startup input is complete.

## Workflow

1. Resolve the deck under `decks/<deck_id>/`.
2. Run `slidex inspect --deck decks/<deck_id> --write`.
3. Run `slidex run --deck decks/<deck_id>`.
4. If intake stops with exit code `3`, surface the Korean intake questions and wait for user answers.
5. Before claiming completion, verify current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Direct stage commands are for focused repair or verification only.
