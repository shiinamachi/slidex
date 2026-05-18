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
