# 01 Create Deck Spec

Create a structured deck specification. Do not create a PPTX in this stage.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.

## Inputs

Read:

- `${ACTIVE_DECK_DIR}/brief.md`
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

## Requirements

- Create `${OUT_DIR}/deck_spec.json`.
- Follow `schemas/deck_spec.schema.json`.
- Include active deck id, active deck directory, and output directory in
  metadata when available.
- Give every slide one clear message.
- Use action headlines rather than generic titles.
- Keep slide messages concise.
- Avoid vague slides that only say "overview", "details", or "next steps"
  without a point of view.
- Define layout instructions, visual instructions, chart instructions, table
  instructions, speaker notes, and QA risks where useful.
- Prefer native PowerPoint charts, tables, shapes, and editable diagrams.
- Use generated visuals only when they improve the story.
- Include alt text guidance for meaningful images.

## Output Standards

The JSON must parse cleanly. After writing it, validate that it is valid JSON.
If a JSON Schema validator is available, also validate it against
`schemas/deck_spec.schema.json`.

Do not create `${OUT_DIR}/final_deck.pptx` in this stage.
