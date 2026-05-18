# 02 Build PPTX

Use $slides and $imagegen.

Build the editable PowerPoint deck from `out/deck_spec.json`.

## Inputs

Read:

- `out/deck_spec.json`
- `schemas/deck_spec.schema.json`
- `out/strategy.md`
- `brand/guidelines.md`
- `brand/colors.json`
- `assets/template.pptx`
- `assets/reference_deck.pptx`
- `assets/logo.png`
- other available assets and data files

Inspect `assets/template.pptx` or `assets/reference_deck.pptx` first if present.
Match source aspect ratio, typography, color system, layout rhythm, page density,
and brand style. Default to 16:9 only when no template or reference deck defines
the size.

## Build Rules

- Create `out/final_deck.pptx`.
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

Create or update `out/notes.md` with:

- Design decisions
- Template or reference deck observations
- Generated visual prompts
- Image source notes
- Any assumptions or unresolved implementation risks

Do not claim completion until the PPTX opens successfully or the best available
local validation has been run.
