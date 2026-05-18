# 05 Business Visual QA

Perform business, evidence, visual, HTML, PDF, and accessibility QA.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant
- `checklists/design_qa.md`
- `checklists/accessibility_qa.md`
- `checklists/delivery_qa.md`

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Inputs

Read:

- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/render_manifest.json`
- rendered images in `${OUT_DIR}/rendered_slides/`
- `${OUT_DIR}/final_deck.pdf`
- `${OUT_DIR}/qa_montage.png`
- `${OUT_DIR}/notes.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- source inventory, intake notes, and strategy

Run CLI QA when available:

```bash
mise exec -- slidex qa --deck ${ACTIVE_DECK_DIR}
```

## QA Coverage

Create or update `${OUT_DIR}/qa_report.md` covering:

- `Overall status: pass | pass_with_risks | fail`,
- render method and files checked,
- slide count/order parity across spec, HTML, rendered images, PDF, montage,
  and delivery summary when present,
- document-type fit,
- business logic and story arc,
- executive polish,
- claim provenance,
- unsupported metric and superlative detection,
- legal/compliance/security/privacy claim risk,
- source-reference fidelity,
- visual hierarchy,
- layout overflow and clipping,
- overlap/collision,
- chart/table readability,
- Korean wrapping,
- font loading,
- image sharpness,
- PDF integrity,
- accessibility,
- reference and DESIGN.md alignment,
- user-edit sync findings when relevant,
- exact runtime/library/font/CDN version pinning,
- required revisions,
- unresolved risks.

Do not mark QA as passed unless rendered slides and montage were visually
inspected.
