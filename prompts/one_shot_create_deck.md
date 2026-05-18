# One-Shot Create Deck

Use $slides and $imagegen.

Create a complete editable PowerPoint deck from the available brief and source
materials. Follow the full workflow; do not skip strategy, deck spec, rendering,
visual QA, revision, or final delivery.

## Full Workflow

1. Read `brief.md`, brand files, assets, data files, screenshots, PDFs, and
   source documents.
2. Inspect `assets/template.pptx` or `assets/reference_deck.pptx` first if
   present. Match aspect ratio, typography, colors, layout patterns, and brand
   style. Default to 16:9 only when no source deck or template defines size.
3. Create `out/strategy.md` with audience, objective, story arc, tone, slide
   sequence, visual direction, and QA risks.
4. Create `out/deck_spec.json` following `schemas/deck_spec.schema.json`.
5. Build `out/final_deck.pptx` with editable PowerPoint text, charts, tables,
   shapes, and diagrams where practical.
6. Use generated visuals only when they support the story. Save generated visual
   prompts and design notes in `out/notes.md`.
7. Render every slide to PNG images.
8. Create `out/qa_montage.png`.
9. Visually inspect the rendered slides and run available validation checks.
10. Write `out/qa_report.md` with slide-by-slide findings.
11. Fix meaningful issues: layout, overflow, overlap, margins, alignment,
    typography, contrast, text density, chart readability, broken images, and alt
    text.
12. Re-render slides, recreate the QA montage, and update the QA report.
13. Repeat revision until the deck is visually acceptable or unresolved risks are
    documented honestly.
14. Finalize delivery with a summary of files created, checks performed, and
    unresolved risks.

## Quality Rules

- Every slide must have one clear message.
- Use action titles, not generic titles.
- Prefer fewer, larger elements over many small elements.
- Keep slide text concise and avoid dense paragraphs.
- Use section dividers for long decks.
- Maintain consistent margins, spacing, alignment, and hierarchy.
- Use brand colors sparingly.
- Body text should generally be at least 18pt.
- Do not use more than two font families unless the brand requires it.
- Charts must be simple, labeled, and readable.
- Speaker notes should be added when they improve delivery.
- Preserve editability; avoid rasterizing whole slides.

Do not claim final delivery is complete unless rendering, QA montage creation,
visual inspection, revision, and final file verification have all happened.
