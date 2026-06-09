# Business Visual QA Checklist

## Story And Business Logic

- Document type is explicit and the structure fits that type.
- Each slide has one clear business purpose.
- Headlines are action titles with a point of view.
- The sequence leads the audience toward a decision, understanding, or action.
- Facts, plans, projections, assumptions, dependencies, exclusions, evidence,
  and risks are separated when relevant.

## Claim Provenance

- Every material claim is sourced, user-confirmed, or marked as an assumption.
- Unsupported metrics, ROI, market leadership, customer counts, patents,
  certifications, compliance, security, privacy, and outcome claims are removed
  or rewritten.
- Reference materials are not copied as facts for the target company unless the
  user confirmed they apply.
- No fake logos, screenshots, customers, case studies, product names, or
  invented technical scope appear.

## HTML Layout Quality

- `.deck` contains stable `.slide` elements with `data-slide-id`.
- Slide dimensions match the render contract.
- No full-slide PNG is embedded as the slide body.
- Margins, alignment, spacing, and hierarchy are consistent.
- The most important element is visually dominant.
- No text overflow, clipping, overlap, or unintended scrollbars are visible.
- Charts and tables are readable and have source notes where useful.

## Automated Editorial Gates

`slidex qa` and `slidex package` emit `ED-*` rule IDs for the automated subset
of the editorial policy:

- `ED-STRUCT-001`: HTML slide count, rendered PNG count, PDF page count, and
  render manifest slide count must reconcile.
- `ED-STRUCT-002`: each non-appendix slide needs one primary headline or
  explicit headline metadata.
- `ED-STRUCT-003`: each non-appendix slide needs a takeaway or reader question.
- `ED-GRID-001`: `.slide` safe margin must meet
  `editorialDesignPolicy.safeMarginPx`.
- `ED-GRID-002`: major grid gaps are reported when below
  `editorialDesignPolicy.gridGutterPx` or off the spacing scale.
- `ED-HIER-001`: competing primary headlines on one non-appendix slide fail.
- `ED-TYPE-001`: Korean documents must use a Korean-capable font stack.
- `ED-TYPE-002`: explicit body, table, chart, footer, source, and caption font
  sizes must meet the policy minimums.
- `ED-TYPE-003`: full justification fails without an explicit exception.
- `ED-TYPE-004`: excessive CJK text runs are reported against
  `editorialDesignPolicy.copyLimits.cjkLineChars`, with appendix relaxation
  allowed only when the policy says so.
- `ED-COPY-001` and `ED-COPY-002`: copy length and bullet density are checked
  against `editorialDesignPolicy.copyLimits`; appendix relaxation is allowed
  only when the policy says so.
- `ED-CLAIM-001`, `ED-CLAIM-002`, and `ED-CLAIM-003`: material claims require
  source, user confirmation, or assumption labeling; metric claims require unit
  and period metadata; unsupported superlative or guarantee language fails.
- `ED-DATAVIZ-001` and `ED-DATAVIZ-002`: charts and tables require a title or
  caption and a source line.
- `ED-A11Y-001`: explicit text/background color pairs must meet 4.5:1 normal
  text or 3:1 large text contrast thresholds unless policy overrides them.
- `ED-A11Y-002`: meaningful images require alt text, `aria-label`, or
  decorative marking.
- `ED-RENDER-001`, `ED-RENDER-002`, and `ED-RENDER-003`: rendered PNG/PDF
  hashes must trace to the current HTML, and direct HTML edits must be synced
  against `final_deck.generated_baseline.html`.
- `ED-PACKAGE-001`: required delivery files and rendered slide PNGs must exist
  before package handoff.

Manual visual inspection is still required for alignment drift, clipping,
non-explicit inherited contrast nuance, chart semantics, and final PDF review
until those checks have dedicated automated evidence.

## Typography And Korean Copy

- Font preset is documented in spec and reflected in CSS.
- Webfont or local font dependencies are documented.
- Korean text wraps at eojeol or natural phrase boundaries.
- Copy is restrained, concrete, and suitable for business stakeholders.

## Design Prompt Alignment

- DESIGN.md directives are reflected in strategy, spec, notes, and HTML where
  practical.
- Conflicts with source truth, brand, accessibility, or delivery constraints are
  documented.
- Candidate output influence is documented as adopt, adapt, or reject when used.

## Executive Polish

- The document feels deliberate, concise, and decision-ready.
- Visual density is appropriate for board, investor, government review, or
  customer decision contexts.
- Images are sharp, purposeful, and documented.
