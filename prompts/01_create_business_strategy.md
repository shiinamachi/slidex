# 01 Create Business Strategy

Transform intake and source materials into a business document strategy.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Inputs

Read:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${OUT_DIR}/intake_questions.md`
- `${OUT_DIR}/source_inventory.md`
- brand, assets, data, and source files in the active workspace
- prior candidate outputs when relevant

If intake is incomplete, stop and ask the missing Korean questions instead of
creating a strategy.

## Required Strategy Content

Create `${OUT_DIR}/strategy.md` with:

- document type and use case,
- audience and decision journey,
- core objective,
- desired outcome,
- story arc,
- section plan,
- proposed slide list with action headlines and key messages,
- claim/evidence plan,
- design direction and font preset recommendation,
- reference influence and candidate-output decisions when relevant,
- risks and missing evidence,
- baseline business QA focus areas.

Do not create `deck_spec.json`, HTML, rendered images, or PDF in this stage.
