# 01 Create Deck Spec

Create a structured deck specification. Do not create a PPTX in this stage.

## Inputs

Read:

- `brief.md`
- `out/strategy.md`
- `brand/guidelines.md`
- `brand/colors.json`
- `assets/template.pptx`
- `assets/reference_deck.pptx`
- `data/*.csv`
- `data/*.xlsx`
- source documents, screenshots, and PDFs
- `schemas/deck_spec.schema.json`

If a template or reference deck exists, preserve its aspect ratio and match its
brand style unless the brief explicitly says otherwise.

## Requirements

- Create `out/deck_spec.json`.
- Follow `schemas/deck_spec.schema.json`.
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

Do not create `out/final_deck.pptx` in this stage.
