# 04 Revise Deck

Revise the deck based on visual QA findings while preserving editability.

## Inputs

Read:

- `out/final_deck.pptx`
- `out/deck_spec.json`
- `out/qa_report.md`
- `out/notes.md`
- rendered slide images and `out/qa_montage.png`

## Tasks

1. Fix all meaningful visual and structural issues from `out/qa_report.md`.
2. Correct layout, overflow, overlap, margins, alignment, text density, chart
   readability, contrast, broken images, and missing alt text.
3. Preserve editable PowerPoint text, charts, tables, shapes, and diagrams.
4. Update `out/notes.md` with revision decisions.
5. Re-render all slides to PNG.
6. Recreate `out/qa_montage.png`.
7. Update `out/qa_report.md` with the new inspection result.
8. Repeat revision and re-rendering until the deck is visually acceptable or
   unresolved risks are explicitly documented.

Do not flatten slides into images to hide layout issues. Do not claim completion
until the revised rendered slides have been inspected.
