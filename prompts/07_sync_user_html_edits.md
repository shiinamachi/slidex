# 07 Sync User HTML Edits

Support direct user edits to `${OUT_DIR}/final_deck.html`.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/final_deck.generated_baseline.html` when present
- `${OUT_DIR}/deck_spec.json`

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Required Behavior

- Treat current `final_deck.html` as possibly user-edited.
- Compare current HTML to the latest generated baseline. If no baseline exists,
  parse current HTML and compare against `deck_spec.json`.
- Detect slide additions, removals, reordering, headline changes, body copy
  changes, data/chart/table changes, visual component changes, asset path
  changes, font or dependency changes, and removed QA-required elements.
- Preserve user edits unless they create QA, source-truth, or security risks.
- Update `deck_spec.json` so slide count, order, headlines, key messages,
  claims, and visual notes match the approved HTML.
- Update `notes.md` and create/update `${OUT_DIR}/html_edit_sync.md`.
- Determine whether `brief.md`, `strategy.md`, `source_inventory.md`, or
  `delivery_summary.md` also need updates. Update them when safe; otherwise
  mark them stale with a concrete reason.
- Never silently overwrite user-edited HTML. If regeneration is needed, write a
  timestamped backup such as `final_deck.pre_sync_YYYYMMDD_HHMMSS.html`.
- Re-render affected slides, rebuild PDF, rebuild montage, rerun QA, and update
  the accepted baseline after the edited HTML is accepted.

CLI path:

```bash
codex-business-deck-kit sync-html-edits --deck ${ACTIVE_DECK_DIR}
```
