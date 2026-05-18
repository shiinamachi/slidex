# Candidate Output Context

Workspaces may contain prior model-generated candidate outputs in `${OUT_DIR}`.
Examples include `final_deck.html`, `final_deck_*.html`, screenshots, rendered
slide images, QA montages, prior PDFs, or draft notes.

## When To Use Candidate Outputs

Use this context when the user explicitly asks to compare outputs, reuse another
model's result, improve one output from another, or when multiple candidate
outputs are present and materially affect a revision. Do not treat candidate
outputs as a substitute for the brief, source documents, approved brand files,
or `DESIGN.md`.

## How To Compare

Treat candidate outputs as design and copy hypotheses, not factual sources.
Compare them slide by slide for:

- story clarity and action-headline strength,
- business copy quality and source faithfulness,
- visual hierarchy, spacing, grid discipline, and information density,
- component consistency, typography, and accent use,
- HTML/spec/render/PDF parity: slide count, order, headlines, key messages, and
  required constraints,
- QA risks such as overflow, tiny text, broken assets, external dependencies,
  hidden full-slide raster content, or mid-word Korean wrapping.

Classify findings as:

- `adopt`: patterns that can be used directly without weakening facts,
  accessibility, HTML maintainability, or brand fit,
- `adapt`: useful patterns that need simplification, source grounding, or layout
  adjustment,
- `reject`: unsupported claims, invented metrics, fake product names, unsourced
  technical scope, clutter, or inaccessible density.

## Copy Provenance Rules

Before adopting candidate-output copy, trace every material claim, metric,
customer statement, technology capability, and outcome back to the brief, brand
file, source document, or explicit user evidence.

Unless explicitly sourced, remove or rewrite superlatives, percentages, ROI,
cost/time reduction, revenue/profit improvement, customer counts, scale claims,
completed-result language, compliance/security details, and invented product or
implementation scope.

## Documentation Requirements

When candidate outputs materially influence the final document:

- document the inventory and comparison in `${OUT_DIR}/strategy.md`,
- record adopted, adapted, and rejected patterns in `${OUT_DIR}/deck_spec.json`
  when the schema supports it,
- describe comparison and claim-control decisions in `${OUT_DIR}/notes.md`,
- include candidate-output alignment and source-truth findings in
  `${OUT_DIR}/qa_report.md`,
- update affected outputs so slide count, order, headlines, key messages,
  rendered images, PDF, montage, notes, and spec remain synchronized.
