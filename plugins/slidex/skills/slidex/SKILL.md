---
name: slidex
description: Use slidex for local HTML-first deck intake, render, QA, package, sync, migration, and Codex protocol diagnostics.
---

# slidex Plugin Skill

Use `slidex run --deck decks/<deck_id>` as the primary workflow. Use direct stage commands only for focused repair or verification.

Required final gates are current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Do not create non-HTML/PDF deliverables. Treat user-supplied presentation files
only as passive source material after their contents are available as ordinary
source evidence.
