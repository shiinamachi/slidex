# 08 Finalize Business Delivery

Verify final delivery without overclaiming.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/render_manifest.json`
- `${OUT_DIR}/qa_report.md`

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Required Files

Verify that these files exist:

- `${OUT_DIR}/strategy.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/final_deck.generated_baseline.html`
- rendered slide images in `${OUT_DIR}/rendered_slides/`
- `${OUT_DIR}/final_deck.pdf`
- `${OUT_DIR}/render_manifest.json`
- `${OUT_DIR}/qa_montage.png`
- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/notes.md`

If direct HTML edits occurred, also verify `${OUT_DIR}/html_edit_sync.md`.

## Final Checks

- Verify current HTML hash and dependency hashes match rendered images/PDF
  through `render_manifest.json`.
- Verify QA report is current.
- Verify slide count/order parity across spec, HTML, rendered images, PDF,
  montage, and delivery summary.
- Verify no unresolved material issues remain unless documented and accepted.
- Verify rendered slides and montage were visually inspected.
- Verify PDF has one slide per page.
- Verify claim provenance, DESIGN.md alignment, font loading, Korean wrapping,
  accessibility, and delivery risks are documented.

CLI package check:

```bash
codex-business-deck-kit package --deck ${ACTIVE_DECK_DIR}
```

## Output

Write `${OUT_DIR}/delivery_summary.md` with:

- active deck id, active deck directory, and output directory,
- files created,
- checks performed,
- render manifest hash summary,
- source and DESIGN.md usage,
- direct HTML edit sync status when relevant,
- how to use delivered files,
- known limitations and unresolved risks.

Do not claim success if rendering, PDF creation, montage creation, visual
inspection, or final file verification did not happen.
