# 04 Revise Deck

Revise the target deck based on visual QA findings while preserving editability.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.
Then read `prompts/_global_presentation_rules.md` for text wrapping, typography,
and HTML webfont rules.
Then read `prompts/_design_prompt_context.md` so revisions preserve any
deck-specific style prompt that was applied.
If candidate outputs are being compared or used for revision guidance, also read
`prompts/_candidate_output_context.md`.

## Inputs

Read:

- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/notes.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- rendered slide images in `${OUT_DIR}/rendered_slides/`
- `${OUT_DIR}/qa_montage.png`
- candidate HTML/PPTX outputs, rendered images, screenshots, or QA montages in
  `${OUT_DIR}` when they influenced the revision

## Tasks

1. Fix all meaningful visual and structural issues from
   `${OUT_DIR}/qa_report.md`.
2. Correct layout, overflow, overlap, margins, alignment, text density, chart
   readability, contrast, broken images, and missing alt text.
3. Correct Korean line breaks that split words or syllables; prefer copy
   reduction, wider boxes, or manual breaks at eojeol/phrase boundaries.
4. For HTML deck outputs, correct missing webfont loading or inconsistent font
   application across text roles.
5. Correct meaningful mismatches with `${ACTIVE_DECK_DIR}/DESIGN.md` when a
   deck-specific design prompt exists, unless doing so would violate brand,
   accessibility, editability, or readability requirements.
6. Correct meaningful mismatches with the documented reference deck influence
   from `deck_spec.json` or `${OUT_DIR}/notes.md`, unless doing so would violate
   higher-priority inputs or create visual QA issues.
7. If candidate outputs are used, adopt only verified improvements to hierarchy,
   spacing, structure, or enterprise phrasing. Reject unsupported claims,
   invented metrics, fake product/package names, unverifiable outcomes, and
   unsourced technical scope.
8. After any slide addition, deletion, renumbering, or material copy change,
   update `deck_spec.json`, notes, rendered slide files, QA montage, and QA
   report so all deliverables agree on slide count, slide order, headlines, and
   key messages. Do not leave an obsolete spec behind after HTML-only revisions.
9. Preserve editable PowerPoint text, charts, tables, shapes, and diagrams.
10. Update `${OUT_DIR}/notes.md` with revision decisions and any design prompt or
   reference deck directives that remain partially applied or intentionally
   ignored.
11. Re-render all slides to PNG in `${OUT_DIR}/rendered_slides/`.
12. Recreate `${OUT_DIR}/qa_montage.png`.
13. Update `${OUT_DIR}/qa_report.md` with the new inspection result.
14. Repeat revision and re-rendering until the deck is visually acceptable or
   unresolved risks are explicitly documented.

Do not flatten slides into images to hide layout issues. Do not claim completion
until the revised rendered slides for the active deck have been inspected.
