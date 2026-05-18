# 04 Revise Deck

Revise the target deck based on visual QA findings while preserving editability.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.

## Inputs

Read:

- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/notes.md`
- rendered slide images in `${OUT_DIR}/rendered_slides/`
- `${OUT_DIR}/qa_montage.png`

## Tasks

1. Fix all meaningful visual and structural issues from
   `${OUT_DIR}/qa_report.md`.
2. Correct layout, overflow, overlap, margins, alignment, text density, chart
   readability, contrast, broken images, and missing alt text.
3. Preserve editable PowerPoint text, charts, tables, shapes, and diagrams.
4. Update `${OUT_DIR}/notes.md` with revision decisions.
5. Re-render all slides to PNG in `${OUT_DIR}/rendered_slides/`.
6. Recreate `${OUT_DIR}/qa_montage.png`.
7. Update `${OUT_DIR}/qa_report.md` with the new inspection result.
8. Repeat revision and re-rendering until the deck is visually acceptable or
   unresolved risks are explicitly documented.

Do not flatten slides into images to hide layout issues. Do not claim completion
until the revised rendered slides for the active deck have been inspected.
