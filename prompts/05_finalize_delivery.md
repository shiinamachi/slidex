# 05 Finalize Delivery

Finalize the deck delivery without overclaiming.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before verifying files or writing final notes.
Then read `prompts/_design_prompt_context.md` so final delivery can report any
deck-specific design prompt that was used.
If candidate outputs were compared or influenced the final deck, also read
`prompts/_candidate_output_context.md`.

## Required Files

Verify that these files exist:

- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/notes.md`
- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/qa_montage.png`
- rendered slide images in `${OUT_DIR}/rendered_slides/`

## Final Checks

- Confirm the PPTX exists and can be opened or validated with available tools.
- Confirm rendered slides were inspected.
- Confirm all meaningful QA findings were fixed or documented.
- Confirm no text overflow, object overlap, broken images, missing fonts, or
  unreadable charts remain, unless listed as unresolved risks.
- Confirm meaningful images have alt text or documented alt text requirements.
- Confirm generated visual prompts and design notes are documented in
  `${OUT_DIR}/notes.md`.
- Confirm every material template, legacy reference deck, and
  `assets/reference_decks/` file is listed in `deck_spec.json`
  `metadata.referenceFiles` or documented as intentionally unused.
- Confirm reference deck influence, conflicts, ignored patterns, and unresolved
  risks are documented in `${OUT_DIR}/notes.md` or `${OUT_DIR}/qa_report.md`.
- If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, confirm its source path, applied
  directives, and any ignored or conflicting directives are documented in
  `${OUT_DIR}/deck_spec.json`, `${OUT_DIR}/notes.md`, or
  `${OUT_DIR}/qa_report.md`.
- If candidate outputs influenced the deck, confirm adopted, adapted, and
  rejected patterns are documented, and that unsupported candidate claims did not
  enter the final deck.
- If HTML outputs exist, confirm HTML slide count, slide order, headlines, and
  key messages match `deck_spec.json`, or document why HTML and PPTX/spec differ.

## Output

Write a final delivery summary for the user with:

- Active deck id, active deck directory, and output directory
- Files created
- Checks performed
- Reference deck usage summary when reference decks were provided
- DESIGN.md usage summary when a deck-specific design prompt exists
- Candidate-output comparison summary when candidate outputs influenced the deck
- How to use the delivered files for this deck
- Known limitations
- Unresolved risks

Do not claim QA passed if rendering or visual inspection did not happen. Do not
hide unresolved risks.
