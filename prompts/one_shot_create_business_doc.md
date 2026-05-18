# One-Shot Create Business Doc

Run the full `slidex` workflow only after the intake gate is
complete. If material intake questions remain, stop and ask them in Korean.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Workflow

1. Inspect all available inputs in the active workspace.
2. If brief, audience, purpose, document type, claims, constraints, or desired
   outcome are incomplete, run the intake Q&A flow and stop.
3. Write or update `intake_questions.md`, `source_inventory.md`, and `brief.md`.
4. Create `strategy.md`.
5. Create schema-valid `deck_spec.json`.
6. Build `final_deck.html`.
7. Save `final_deck.generated_baseline.html`.
8. Render `.slide` elements to `rendered_slides/*.png` with
   `mise exec -- slidex render`.
9. Generate `final_deck.pdf`, `render_manifest.json`, and `qa_montage.png`.
10. Run business visual QA and create `qa_report.md`.
11. Revise HTML/spec/notes, re-render, and rerun QA until material issues are
    fixed or accepted as risks.
12. Finalize delivery and write `delivery_summary.md`.

## Quality Rules

- Use action headlines and a coherent business story arc.
- Keep copy concise, evidence-aware, and appropriate for decision stakeholders.
- Source or explicitly mark every material claim.
- Remove unsupported metrics, superlatives, fake customers, fake screenshots,
  fake logos, and invented technical scope.
- Use static HTML/CSS with fixed slide dimensions and Korean-capable font
  presets.
- Keep runtime, renderer, library, CDN, and font dependencies exact-pinned.
  Do not use `latest`, `main`, range operators, `^`, `~`, `x`, or `*`.
- Render current HTML to PNG and PDF before QA.
- Do not generate or deliver legacy presentation-file output.
