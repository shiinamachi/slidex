# 02 Create Business Doc Spec

Create a schema-valid specification for an HTML/PDF business document.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant
- `schemas/deck_spec.schema.json`

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Inputs

Read:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${OUT_DIR}/intake_questions.md`
- `${OUT_DIR}/source_inventory.md`
- `${OUT_DIR}/strategy.md`
- brand, assets, data, and source files
- prior candidate outputs when relevant

If intake is incomplete or assumptions are not approved, stop and ask Korean
questions.

## Required Output

Create `${OUT_DIR}/deck_spec.json` following `schemas/deck_spec.schema.json`.

The spec must include:

- metadata,
- document type,
- audience, objective, desired outcome, and tone,
- source inventory references,
- intake status,
- output contract,
- render config,
- PDF config,
- design system and font preset,
- story arc,
- slide list,
- HTML implementation notes,
- claim provenance requirements,
- business QA risks,
- accessibility notes,
- user-edit sync policy.

Every slide must include `htmlId`, `sectionRole`, `headline`, `keyMessage`,
`bodyContent`, `layoutIntent`, `visualIntent`, `evidenceRefs`, `claims`,
`renderRisks`, and `qaChecks` as appropriate.

Validate the JSON. If the local CLI is available, run:

```bash
codex-business-deck-kit validate-spec --spec ${OUT_DIR}/deck_spec.json
```

Do not create HTML, rendered images, or PDF in this stage.
