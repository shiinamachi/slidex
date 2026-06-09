---
name: slidex-finalize
description: Finalize a slidex deck by rendering, QAing, inspecting visual evidence, and checking package freshness.
user-invocable: true
---

# slidex-finalize

Use this when the user asks for final delivery readiness.

## Gates

- `decks/<deck_id>/out/final_deck.html` is the current visual source.
- Rendered slide PNGs and `final_deck.pdf` were produced from that HTML.
- `render_manifest.json`, `qa_montage.png`, `qa_report.md`, and `delivery_summary.md` are fresh.
- Visual inspection was actually performed before claiming visual QA passed.
- `slidex package --deck decks/<deck_id>` passes.

Run `slidex sync-html-edits --deck decks/<deck_id>` before final delivery if `out/final_deck.html` was edited directly.
