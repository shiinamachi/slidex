# One-Shot Create Deck

Use $slides and $imagegen.

Create a complete editable PowerPoint deck from the available brief and source
materials. Follow the full workflow; do not skip strategy, deck spec, rendering,
visual QA, revision, or final delivery.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading deck materials or writing files.
Then read `prompts/_design_prompt_context.md` so any deck-specific style prompt
is applied through the full workflow.

## Full Workflow

1. Read `${ACTIVE_DECK_DIR}/brief.md`, `${ACTIVE_DECK_DIR}/DESIGN.md` when
   present, brand files, assets, data files,
   screenshots, PDFs, and source documents from the active deck workspace.
2. Inspect `${ACTIVE_DECK_DIR}/assets/template.pptx` or
   `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx` first if present. Match
   aspect ratio, typography, colors, layout patterns, and brand style. Default
   to 16:9 only when no source deck or template defines size.
3. Create `${OUT_DIR}/strategy.md` with active deck id and directory, audience,
   objective, story arc, tone, slide sequence, design prompt interpretation,
   visual direction, and QA risks.
4. Create `${OUT_DIR}/deck_spec.json` following
   `schemas/deck_spec.schema.json`, including the design prompt source and
   distilled style directives when `${ACTIVE_DECK_DIR}/DESIGN.md` exists.
5. Build `${OUT_DIR}/final_deck.pptx` with editable PowerPoint text, charts,
   tables, shapes, and diagrams where practical, applying DESIGN.md style
   guidance within brand, accessibility, and editability constraints.
6. Use generated visuals only when they support the story. Save generated visual
   prompts and design notes in `${OUT_DIR}/notes.md`.
7. Render every slide to PNG images in `${OUT_DIR}/rendered_slides/`.
8. Create `${OUT_DIR}/qa_montage.png`.
9. Visually inspect the rendered slides and run available validation checks.
10. Write `${OUT_DIR}/qa_report.md` with slide-by-slide findings.
11. Fix meaningful issues: layout, overflow, overlap, margins, alignment,
    typography, contrast, text density, chart readability, broken images, and alt
    text, plus any meaningful mismatch with DESIGN.md when a deck-specific
    design prompt exists.
12. Re-render slides in `${OUT_DIR}/rendered_slides/`, recreate
    `${OUT_DIR}/qa_montage.png`, and update `${OUT_DIR}/qa_report.md`.
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
- Apply `${ACTIVE_DECK_DIR}/DESIGN.md` as a deck-specific style prompt when
  present, but do not let it override approved brand, template, accessibility,
  or editability requirements.

Do not claim final delivery is complete unless rendering, QA montage creation,
visual inspection, revision, and final file verification have all happened.
