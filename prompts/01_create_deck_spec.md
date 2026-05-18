# 01 Create Deck Spec

Create a structured deck specification. Do not create a PPTX in this stage.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.
Then read `prompts/_global_presentation_rules.md` for text wrapping, typography,
and HTML webfont rules.
Then read `prompts/_design_prompt_context.md` so any deck-specific design prompt
is captured in the structured spec.
If candidate outputs are being compared, also read
`prompts/_candidate_output_context.md`.

## Inputs

Read:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${OUT_DIR}/strategy.md`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/assets/template.pptx`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx` for legacy single-reference
  compatibility
- `${ACTIVE_DECK_DIR}/assets/reference_decks/` for any number of reference decks
- `${ACTIVE_DECK_DIR}/data/*.csv`
- `${ACTIVE_DECK_DIR}/data/*.xlsx`
- source documents, screenshots, and PDFs in the active deck workspace
- prior model-generated candidate outputs in `${OUT_DIR}` when the user asks to
  compare or improve from them
- `schemas/deck_spec.schema.json`

If a template, legacy reference deck, or files under `assets/reference_decks/`
exist, preserve the approved aspect ratio and match the relevant brand and
style patterns unless the brief explicitly says otherwise. When references
conflict, document which inputs controlled the spec and why.

If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, apply it as deck-specific style
direction within the priority rules in `prompts/_design_prompt_context.md`.

## Requirements

- Create `${OUT_DIR}/deck_spec.json`.
- Follow `schemas/deck_spec.schema.json`.
- Include active deck id, active deck directory, and output directory in
  metadata when available.
- Include every template, legacy reference deck, and `assets/reference_decks/`
  file that materially affects the deck in `metadata.referenceFiles`, with its
  path, kind, role, priority, and notes when useful.
- Include the design prompt source path in metadata when
  `${ACTIVE_DECK_DIR}/DESIGN.md` exists.
- Capture the distilled design prompt interpretation in `designSystem`, including
  style directives, avoid guidance, and any conflicts or overrides.
- Capture any global typography requirements that materially affect production,
  including Korean eojeol/phrase-based wrapping and required webfont usage for
  HTML deck outputs.
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
- Include a copy provenance pass: every material claim, metric, customer
  statement, technology capability, and outcome must be traceable to the brief,
  brand file, source document, or explicit user-provided evidence. Unsupported
  claims must be removed or rewritten as conservative capability language.
- When candidate outputs are compared, include
  `candidateOutputComparison` entries that summarize which visual/copy patterns
  are adopted, adapted, or rejected. Do not let a visually stronger candidate
  override source truth, brand constraints, accessibility, editability, or
  enterprise copy standards.

## Output Standards

The JSON must parse cleanly. After writing it, validate that it is valid JSON.
If a JSON Schema validator is available, also validate it against
`schemas/deck_spec.schema.json`.

Do not create `${OUT_DIR}/final_deck.pptx` in this stage.
