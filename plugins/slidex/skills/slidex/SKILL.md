---
name: slidex
description: Compatibility skill for slidex. Prefer slidex-start for new deck workbench startup, slidex-run for full workflows, and slidex-finalize for delivery gates.
---

# slidex Plugin Skill

For a new deck, use `slidex-start` first. The React Wizard saves initial deck creation input and starts generation with `slidex run --deck decks/<deck_id> --non-interactive` after the user selects `Complete & generate`. Use `slidex run --deck decks/<deck_id>` manually only when repairing or resuming an existing deck.

Required final gates are current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Do not create non-HTML/PDF deliverables. Treat user-supplied presentation files
only as passive source material after their contents are available as ordinary
source evidence.
