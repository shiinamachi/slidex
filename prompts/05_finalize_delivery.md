# 05 Finalize Delivery

Finalize the deck delivery without overclaiming.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before verifying files or writing final notes.
Then read `prompts/_design_prompt_context.md` so final delivery can report any
deck-specific design prompt that was used.

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
- If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, confirm its source path, applied
  directives, and any ignored or conflicting directives are documented in
  `${OUT_DIR}/deck_spec.json`, `${OUT_DIR}/notes.md`, or
  `${OUT_DIR}/qa_report.md`.

## Output

Write a final delivery summary for the user with:

- Active deck id, active deck directory, and output directory
- Files created
- Checks performed
- DESIGN.md usage summary when a deck-specific design prompt exists
- How to use the delivered files for this deck
- Known limitations
- Unresolved risks

Do not claim QA passed if rendering or visual inspection did not happen. Do not
hide unresolved risks.
