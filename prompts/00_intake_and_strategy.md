# 00 Intake And Strategy

You are preparing the target PowerPoint deck in the active deck workspace, but
you must not create a PPTX in this stage.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before inspecting deck materials or writing files.

## Inputs

Read available inputs from `ACTIVE_DECK_DIR` before making recommendations:

- `${ACTIVE_DECK_DIR}/brief.md`
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

If `${ACTIVE_DECK_DIR}/brief.md` is missing, use `brief.template.md` or
`decks/_template/brief.md` to identify the missing fields and ask only the
questions that materially affect strategy. Do not block on minor gaps; state
reasonable assumptions.

## Tasks

1. Summarize the user brief in plain language.
2. Identify missing or ambiguous information without stopping the work.
3. Inspect any template or reference deck first if present, and note aspect
   ratio, typography, color style, layout patterns, and brand cues.
4. Define the target audience, objective, desired outcome, tone, and decision
   context.
5. Propose a story arc with one clear role for each section.
6. Recommend a slide sequence with slide type, action title, key message, and
   likely visual treatment.
7. Identify source data or evidence needed for charts, tables, and claims.
8. List design risks and QA risks to watch later.

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
- Data and evidence plan
- Visual direction
- QA risks

Do not create `${OUT_DIR}/deck_spec.json` or any PPTX in this stage.
