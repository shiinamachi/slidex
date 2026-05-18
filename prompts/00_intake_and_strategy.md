# 00 Intake And Strategy

You are preparing a PowerPoint deck, but you must not create a PPTX in this
stage.

## Inputs

Read available inputs before making recommendations:

- `brief.md`
- `brand/guidelines.md`
- `brand/colors.json`
- `assets/template.pptx`
- `assets/reference_deck.pptx`
- `assets/logo.png`
- `data/*.csv`
- `data/*.xlsx`
- screenshots, PDFs, notes, and other supplied source documents

If `brief.md` is missing, use `brief.template.md` to identify the missing
fields and ask only the questions that materially affect strategy. Do not block
on minor gaps; state reasonable assumptions.

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

Create `out/strategy.md` with these sections:

- Brief summary
- Working assumptions
- Missing information
- Audience and objective
- Story arc
- Recommended slide sequence
- Brand and design direction
- Data and evidence plan
- Visual direction
- QA risks

Do not create `out/deck_spec.json` or any PPTX in this stage.
