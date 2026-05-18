# 01 Create Deck Spec

Create a structured deck specification. Do not create a PPTX in this stage.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.
Then read `prompts/_design_prompt_context.md` so any deck-specific design prompt
is captured in the structured spec.

## Inputs

Read:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${OUT_DIR}/strategy.md`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/assets/template.pptx`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx`
- `${ACTIVE_DECK_DIR}/data/*.csv`
- `${ACTIVE_DECK_DIR}/data/*.xlsx`
- source documents, screenshots, and PDFs in the active deck workspace
- `schemas/deck_spec.schema.json`

If a template or reference deck exists, preserve its aspect ratio and match its
brand style unless the brief explicitly says otherwise.

If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, apply it as deck-specific style
direction within the priority rules in `prompts/_design_prompt_context.md`.

## Requirements

- Create `${OUT_DIR}/deck_spec.json`.
- Follow `schemas/deck_spec.schema.json`.
- Include active deck id, active deck directory, and output directory in
  metadata when available.
- Include the design prompt source path in metadata when
  `${ACTIVE_DECK_DIR}/DESIGN.md` exists.
- Capture the distilled design prompt interpretation in `designSystem`, including
  style directives, avoid guidance, and any conflicts or overrides.
- Give every slide one clear message.
- Use action headlines rather than generic titles.
- Keep slide messages concise.
- Avoid vague slides that only say "overview", "details", or "next steps"
  without a point of view.
- Define layout instructions, visual instructions, chart instructions, table
  instructions, speaker notes, and QA risks where useful.
- Translate `DESIGN.md` into slide-level layout and visual instructions where it
  materially changes composition, density, imagery, chart style, or tone.
- Prefer native PowerPoint charts, tables, shapes, and editable diagrams.
- Use generated visuals only when they improve the story.
- Include alt text guidance for meaningful images.

## Output Standards

The JSON must parse cleanly. After writing it, validate that it is valid JSON.
If a JSON Schema validator is available, also validate it against
`schemas/deck_spec.schema.json`.

Do not create `${OUT_DIR}/final_deck.pptx` in this stage.
