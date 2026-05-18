# 00 Intake And Strategy

You are preparing the target PowerPoint deck in the active deck workspace, but
you must not create a PPTX in this stage.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before inspecting deck materials or writing files.
Then read `prompts/_design_prompt_context.md` so any deck-specific style prompt
is handled consistently.

## Inputs

Read available inputs from `ACTIVE_DECK_DIR` before making recommendations:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/assets/template.pptx`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx`
- `${ACTIVE_DECK_DIR}/assets/logo.png`
- `${ACTIVE_DECK_DIR}/assets/`
- `${ACTIVE_DECK_DIR}/data/*.csv`
- `${ACTIVE_DECK_DIR}/data/*.xlsx`
- `${ACTIVE_DECK_DIR}/source/`
- screenshots, PDFs, notes, and other supplied source documents in the active
  deck workspace

Use `shared/brand/`, `shared/assets/`, or `shared/data/` only according to the
isolation rules in `prompts/_active_deck_context.md`.

If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, read it as a deck-specific style
prompt. Extract practical style directives, avoid lists, visual references,
layout density guidance, and conflicts with template or brand inputs.

If `${ACTIVE_DECK_DIR}/brief.md` is missing, use `brief.template.md` or
`decks/_template/brief.md` to identify the missing fields and ask only the
questions that materially affect strategy. Do not block on minor gaps; state
reasonable assumptions.

## Tasks

1. Summarize the user brief in plain language.
2. Identify missing or ambiguous information without stopping the work.
3. Inspect any template or reference deck first if present, and note aspect
   ratio, typography, color style, layout patterns, and brand cues.
4. Interpret `${ACTIVE_DECK_DIR}/DESIGN.md` when present and reconcile it with
   the brief, template, reference deck, brand guidelines, accessibility, and
   editability requirements.
5. Define the target audience, objective, desired outcome, tone, and decision
   context.
6. Propose a story arc with one clear role for each section.
7. Recommend a slide sequence with slide type, action title, key message, and
   likely visual treatment.
8. Identify source data or evidence needed for charts, tables, and claims.
9. List design risks and QA risks to watch later, including any risk that the
   design prompt may conflict with brand, accessibility, or readability.

## Output

Create `${OUT_DIR}/strategy.md` with these sections:

- Brief summary
- Working assumptions
- Missing information
- Active deck id and directory
- Audience and objective
- Story arc
- Recommended slide sequence
- Brand and design direction
- Design prompt interpretation
- Data and evidence plan
- Visual direction
- QA risks

Do not create `${OUT_DIR}/deck_spec.json` or any PPTX in this stage.
