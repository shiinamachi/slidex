# 02 Build PPTX

Use $slides and $imagegen.

Build the editable PowerPoint deck from `${OUT_DIR}/deck_spec.json`.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.

## Inputs

Read:

- `${OUT_DIR}/deck_spec.json`
- `schemas/deck_spec.schema.json`
- `${OUT_DIR}/strategy.md`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/assets/template.pptx`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx`
- `${ACTIVE_DECK_DIR}/assets/logo.png`
- other available assets and data files in the active deck workspace

Inspect `${ACTIVE_DECK_DIR}/assets/template.pptx` or
`${ACTIVE_DECK_DIR}/assets/reference_deck.pptx` first if present. Match source
aspect ratio, typography, color system, layout rhythm, page density, and brand
style. Default to 16:9 only when no template or reference deck defines the size.

## Build Rules

- Create `${OUT_DIR}/final_deck.pptx`.
- Keep all text editable as PowerPoint text.
- Use native PowerPoint charts where practical.
- Use native PowerPoint tables, shapes, connectors, icons, and diagrams where
  practical.
- Do not rasterize whole slides.
- Use generated visuals only when they improve the slide.
- Use image generation for cover visuals, section visuals, diagrams,
  illustrations, or decorative assets that support the story.
- Add speaker notes where they improve delivery.
- Maintain consistent margins, spacing, alignment, and hierarchy.
- Use no more than two font families unless the brand requires it.
- Keep body text generally at least 18pt; use larger important text for live
  presentation decks.
- Keep charts simple, labeled, and readable.
- Add alt text for meaningful images.

## Notes

Create or update `${OUT_DIR}/notes.md` with:

- Active deck id, active deck directory, and output directory
- Design decisions
- Template or reference deck observations
- Generated visual prompts
- Image source notes
- Any assumptions or unresolved implementation risks

Do not claim completion until the PPTX opens successfully or the best available
local validation has been run.
