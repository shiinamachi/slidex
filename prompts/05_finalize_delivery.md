# 05 Finalize Delivery

Finalize the deck delivery without overclaiming.

## Required Files

Verify that these files exist:

- `out/final_deck.pptx`
- `out/deck_spec.json`
- `out/notes.md`
- `out/qa_report.md`
- `out/qa_montage.png`
- rendered slide images

## Final Checks

- Confirm the PPTX exists and can be opened or validated with available tools.
- Confirm rendered slides were inspected.
- Confirm all meaningful QA findings were fixed or documented.
- Confirm no text overflow, object overlap, broken images, missing fonts, or
  unreadable charts remain, unless listed as unresolved risks.
- Confirm meaningful images have alt text or documented alt text requirements.
- Confirm generated visual prompts and design notes are documented in
  `out/notes.md`.

## Output

Write a final delivery summary for the user with:

- Files created
- Checks performed
- How to use the deck
- Known limitations
- Unresolved risks

Do not claim QA passed if rendering or visual inspection did not happen. Do not
hide unresolved risks.
