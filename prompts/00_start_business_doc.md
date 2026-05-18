# 00 Start Business Doc

Begin a `slidex` document run from sparse or rich materials.
Do not build final HTML, rendered images, or PDF until intake is complete.

## Context

Read, in order:

1. `prompts/_active_deck_context.md`
2. `prompts/_global_presentation_rules.md`
3. `prompts/_design_prompt_context.md`
4. `prompts/_candidate_output_context.md` only if prior candidate outputs are
   relevant

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR` before reading or writing files.

## Inspect Before Asking

Inspect available materials:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${ACTIVE_DECK_DIR}/brand/`
- `${ACTIVE_DECK_DIR}/assets/`
- `${ACTIVE_DECK_DIR}/assets/reference_docs/`
- `${ACTIVE_DECK_DIR}/source/`
- `${ACTIVE_DECK_DIR}/data/`
- prior `${OUT_DIR}` candidate outputs when relevant

User-supplied PPTX files are passive source documents only.

## Intake Gate

Determine whether the following are complete enough to produce responsibly:

- document type and use case,
- audience and decision context,
- core objective and desired outcome,
- company/product facts,
- material claims and evidence availability,
- tone,
- expected length,
- delivery constraints,
- design constraints,
- required inclusions and exclusions,
- confidentiality boundaries.

If anything material is missing, ask a short Korean Q&A round. Classify each
question as `blocking`, `assumption`, or `polish`. Repeat Q&A until blocking
questions are answered or assumptions are explicitly approved.

## Outputs

Write or update:

- `${OUT_DIR}/source_inventory.md`
- `${OUT_DIR}/intake_questions.md`
- `${ACTIVE_DECK_DIR}/brief.md` with confirmed facts, constraints, and approved
  assumptions

`intake_questions.md` must include questions, answers, assumptions, pending
issues, and approval status.

Stop after intake if the gate is incomplete. Do not fabricate missing facts.
