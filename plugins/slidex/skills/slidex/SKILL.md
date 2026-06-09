---
name: slidex
description: Compatibility skill for slidex. Prefer slidex-start for new deck workbench startup, slidex-run for full workflows, and slidex-finalize for delivery gates.
---

# slidex Plugin Skill

For a new deck, use `slidex-start` first. Use `slidex run --deck decks/<deck_id>` only after the initial deck creation input has been saved.

Required final gates are current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Do not create non-HTML/PDF deliverables. Treat user-supplied presentation files
only as passive source material after their contents are available as ordinary
source evidence.
