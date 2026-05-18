# Candidate Output Context

Deck workspaces may contain prior model-generated candidate outputs in
`${OUT_DIR}`. Examples include `final_deck.html`,
`final_deck_gemini.html`, `final_deck_*.html`, alternate PPTX files,
rendered slide images, screenshots, or QA montages.

## When To Use Candidate Outputs

Use this context when the user explicitly asks to compare outputs, reuse another
model's result, improve one output from another, or when multiple candidate
outputs are present and materially affect a revision. Do not treat candidate
outputs as a substitute for the brief, source documents, approved template,
brand files, or `DESIGN.md`.

## How To Compare

Treat candidate outputs as design and copy hypotheses, not factual sources.
Compare them slide by slide for:

- story clarity and action-title strength
- enterprise copy quality and source faithfulness
- visual hierarchy, spacing, grid discipline, and information density
- component consistency, typography, and accent use
- case-study, architecture, process, and data-flow structure
- HTML/PPTX/spec parity: slide count, order, headlines, key messages, and
  required constraints
- QA risks such as overflow, tiny text, broken assets, external dependencies,
  rasterized slides, or mid-word Korean wrapping

Classify findings as:

- `adopt`: patterns that can be used directly without weakening facts,
  accessibility, editability, or brand fit
- `adapt`: patterns that are useful but need simplification, source grounding,
  or layout adjustment
- `reject`: unsupported claims, invented metrics, fake product names,
  unsourced technical scope, proprietary claims, clutter, or inaccessible
  density

## Copy Provenance Rules

Before adopting candidate-output copy, trace every material claim, metric,
customer statement, technology capability, and outcome back to the brief, brand
file, source document, or explicit user evidence.

Unless explicitly sourced, remove or rewrite:

- superlatives such as `No.1`, `best`, `only`, or `proven`
- percentages, ROI, cost/time reduction, revenue/profit improvement, customer
  counts, or scale claims
- completed-result language such as `deployed`, `commercialized`, or
  `achieved` when the source says the project is ongoing
- invented technical scope such as RAG, fine-tuning, private VPC, compliance,
  security, data privacy, or orchestration details not present in the inputs
- fake screenshots, fake logos, fake UI, or invented product/package names

Prefer conservative enterprise wording such as `supports`, `enables`,
`is designed to`, `helps standardize`, `provides a path for`, and
`applies to`, while keeping headlines specific and decision-oriented.

## Documentation Requirements

When candidate outputs materially influence the final deck:

- document the inventory and comparison in `${OUT_DIR}/strategy.md`
- record adopted, adapted, and rejected patterns in
  `${OUT_DIR}/deck_spec.json` under `candidateOutputComparison` when the schema
  supports it
- describe the comparison and copy-provenance decisions in
  `${OUT_DIR}/notes.md`
- include candidate-output alignment and source-truth findings in
  `${OUT_DIR}/qa_report.md`
- update all affected outputs so slide count, slide order, headlines, key
  messages, rendered images, QA montage, notes, and spec remain synchronized
