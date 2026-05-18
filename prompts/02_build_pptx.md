# 02 Build PPTX

Use $slides and $imagegen.

Build the editable PowerPoint deck from `${OUT_DIR}/deck_spec.json`.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.
Then read `prompts/_design_prompt_context.md` so any deck-specific style prompt
is applied consistently during the build.
If candidate outputs are being used as comparison references, also read
`prompts/_candidate_output_context.md`.

## Inputs

Read:

- `${OUT_DIR}/deck_spec.json`
- `schemas/deck_spec.schema.json`
- `${OUT_DIR}/strategy.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/assets/template.pptx`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx` for legacy single-reference
  compatibility
- `${ACTIVE_DECK_DIR}/assets/reference_decks/` for any number of reference decks
- `${ACTIVE_DECK_DIR}/assets/logo.png`
- other available assets and data files in the active deck workspace
- prior model-generated candidate outputs in `${OUT_DIR}` when the user asks to
  compare or improve from them

Inspect `${ACTIVE_DECK_DIR}/assets/template.pptx`, legacy
`${ACTIVE_DECK_DIR}/assets/reference_deck.pptx`, and all files under
`${ACTIVE_DECK_DIR}/assets/reference_decks/` first if present. Match source
aspect ratio, typography, color system, layout rhythm, page density, and brand
style according to the approved template and the documented reference influence.
Default to 16:9 only when no template or reference deck in the set defines the
size. When references conflict, follow the brief, approved template, brand,
accessibility, and editability constraints first; document any ignored reference
patterns.
If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, apply its distilled directives from
`deck_spec.json` and re-check the original file for any important nuance that
was missed.

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
- Apply deck-specific `DESIGN.md` style directives to typography, spacing,
  composition, imagery, iconography, charts, and visual density unless they
  conflict with higher-priority inputs or QA requirements.
- Run an enterprise copy edit before building slides. Remove or rewrite
  unsupported superlatives, invented metrics, unverifiable outcomes, and salesy
  transformation language. Forbidden unless explicitly sourced: `No.1`, `best`,
  `only`, `proven`, `deployed`, `commercialized`, `ROI`, percentage reduction,
  revenue/profit improvement, time saved, customer scale claims, and completed
  result language.
- Prefer precise B2B wording such as `supports`, `enables`,
  `is designed to`, `helps standardize`, `provides a path for`, and
  `applies to`. Keep action titles specific, but do not overclaim.
- When a candidate output has stronger visual hierarchy, adopt its reusable
  layout patterns only after stripping unsupported copy and checking the pattern
  against the brief, brand, accessibility, and editability requirements.

## Notes

Create or update `${OUT_DIR}/notes.md` with:

- Active deck id, active deck directory, and output directory
- Design decisions
- Design prompt source, interpretation, applied directives, and any ignored or
  conflicting directives
- Template and reference deck observations, including per-reference influence,
  conflicts, and ignored patterns
- Candidate-output comparison observations, adopted/adapted/rejected patterns,
  and claim-control decisions when applicable
- Generated visual prompts
- Image source notes
- Any assumptions or unresolved implementation risks

Do not claim completion until the PPTX opens successfully or the best available
local validation has been run.
