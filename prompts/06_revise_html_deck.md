# 06 Revise HTML Deck

Fix QA findings while keeping HTML/spec/notes/rendered outputs synchronized.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Inputs

Read:

- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/notes.md`
- `${OUT_DIR}/render_manifest.json`
- rendered slide images and `${OUT_DIR}/qa_montage.png`
- `${ACTIVE_DECK_DIR}/DESIGN.md`

## Required Behavior

1. Fix meaningful issues in HTML and spec.
2. Correct unsupported claims, stale evidence, layout overflow, clipping,
   overlap, margins, alignment, text density, chart readability, contrast,
   broken images, missing fonts, and awkward Korean wrapping.
3. Update `notes.md` with revision decisions and unresolved risks.
4. Re-run `prompts/04_render_html.md` or the equivalent CLI render command.
5. Re-run business visual QA and update `qa_report.md`.
6. Repeat until no material QA issues remain or unresolved risks are explicitly
   accepted.

Do not hide layout problems by rasterizing whole slides into the HTML.
