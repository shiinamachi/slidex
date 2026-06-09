# Goal: slidex 편집 디자인 품질 게이트 구축

Slug: `editorial-design-quality-gates`
Status: Draft
Activation: `/goal @goals/editorial-design-quality-gates.md`

This file is not an active Goal. It becomes active thread-scoped Goal state only after the activation command is run.

## Goal Text

slidex가 생성하는 HTML-first 비즈니스 문서가 편집 디자인 업계의 핵심 원칙을 구조적으로 준수하도록 schema, template, CLI QA, render verification, PDF/package verification, docs, fixtures, tests를 구현한다. 완료는 `go test ./...`, spec validation, `slidex doctor --json`, template smoke test, sample deck workflow, 현재 `out/final_deck.html`에서 파생된 PNG/PDF 산출물, `qa_report.md`, `render_manifest.json`, `delivery_summary.md`, 그리고 수동 시각 검사 기록으로 검증한다. 작업 중에도 `cmd/slidex` 중심 CLI, deck-local `decks/<deck_id>/`, HTML-first source of truth, PDF primary delivery artifact, claim provenance, Korean-capable typography, direct HTML edit sync 계약을 보존한다. 각 반복에서는 가장 높은 severity의 evidence-backed 실패를 먼저 고치고, 검증 결과를 기록한 뒤 다음 최소 변경을 선택한다. 유효한 경로가 없거나 필수 renderer, artifact, source, claim decision, Korean font fallback, visual inspection evidence가 확보되지 않으면 시도 경로와 증거, blocker, 필요한 입력을 보고하고 중단한다.

## Interview Summary

- Desired outcome: slidex 결과물(`out/final_deck.html`, rendered slide PNGs, `out/final_deck.pdf`, QA/package artifacts)이 목적과 독자, 시각 계층, grid/spacing, typography, whitespace, concise/plain language, evidence transparency, data visualization integrity, accessibility, consistency, render reproducibility 원칙을 통과하도록 자동 품질 게이트를 갖춘다.
- Non-goals: hosted app/SaaS/웹 에디터 전환, PPTX/Keynote/Figma/PDF-first workflow 전환, legacy standalone workflow 복원, deck-specific 임시 스크립트로 품질을 맞추는 접근, 모든 deck에 단일 미학 강제, 모든 슬라이드의 pixel-perfect golden image 고정, 외부 font 무단 vendoring, OCR 중심 QA, 이번 범위에서 완전한 PDF/UA 인증 달성, chart grammar 전체 재구축.
- Evidence: Go tests, spec validation, doctor output, `slidex init` smoke test, full deck workflow, render manifest hash freshness, slide count/page count/PNG count reconciliation, QA report rule results, package freshness, `qa_montage.png`와 `final_deck.pdf` inspected evidence.
- Constraints: `cmd/slidex` canonical CLI, `decks/<deck_id>/` scoped inputs/outputs, `out/final_deck.html` source of truth, `out/final_deck.pdf` primary artifact, material claim provenance, unsupported claim failure/removal, Korean font/wrapping defaults, direct HTML edits requiring `slidex sync-html-edits --deck <deck_dir>`.
- Scope: allowed to touch Go CLI code, schemas, fixtures, `decks/_template/`, docs, plugin/skill packaging checks, tests, and examples needed for the quality gate. Do not commit ignored deck outputs or generated delivery artifacts unless explicitly requested.
- Autonomy: iterate aggressively on low-risk repo changes that make the quality gate testable; ask only when a product decision, unsupported claim source, dependency addition, or renderer/font policy cannot be inferred safely.
- Budget: stop when the named verification passes, or when no defensible implementation path remains under the current repository/runtime constraints. Do not mark complete merely because a subset of checks passes.
- Blocked condition: stop if renderer/PDF generation, claim sourcing, current-artifact freshness, Korean glyph rendering, sync-html-edits, or visual inspection evidence cannot be obtained.
- Reporting: final report should list changed files, implemented rule IDs, commands run, artifact paths inspected, remaining warnings, known assumptions, and any blocked or deferred items.

## Research And Work Instruction Summary

GPT Pro research was run through ChatGPT UI model `Latest • 5.5` with `Pro • Extended` checked. The prompt used repository contract context only; no codebase archive was uploaded.

The resulting editorial design principles to enforce are:

- Purpose and reader first: every deck must state the audience, decision question, requested decision or discussion, and key conclusion early enough for board, investor, government, customer, or business readers.
- Visual hierarchy: headline, takeaway, evidence, metric, chart, table, source, and footnote must not compete at the same visual level.
- Grid, alignment, and balance: repeated slide structure, safe margins, consistent gutters, and predictable footer/source placement reduce reader effort.
- Typography: readability, Korean glyph stability, numeric legibility, role-specific type scale, line height, and controlled type tokens are more important than decorative font choices.
- Whitespace and scanning: margin, grouping, headings, lists, and tables should reduce decision cost rather than merely add visual polish.
- Concise/plain language: text must support fast verification with short sections, active voice where useful, explicit actors, and limited bullets.
- Evidence transparency: material claims must be sourced, user-confirmed, or labeled assumption; unsupported metrics, ROI, market leadership, certifications, guarantees, security, compliance, and outcome claims must fail or be rewritten.
- Data visualization integrity: charts/tables need a title or caption, source, unit, date range, axis/direct labels where applicable, and text equivalents for generated or image-based visuals.
- Accessibility: color contrast, color-independent meaning, text equivalents, non-clipped content, selectable text where renderer permits, and Korean-safe wrapping are part of render quality.
- Consistency and reproducibility: HTML, PNGs, PDF, manifest, QA report, montage, and delivery summary must all describe the same current artifact state.

## Implementation Directions

- Extend existing spec contracts rather than inventing a separate workflow. Prefer mapping to current `schemas/deck_spec.schema.json` concepts such as `designSystem`, `claimProvenance`, `businessQa`, `renderConfig`, `slides`, `layoutIntent`, `visualIntent`, and `qaChecks`.
- Add or refine a document/editorial profile with audience, locale, document type, decision requirement, decision question, primary reader, and evidence mode.
- Add an editorial design policy covering 16:9 aspect ratio, static HTML/CSS slide structure, Korean business font preset, grid margins/gutters, typography limits, copy limits, evidence policy, and accessibility thresholds.
- Add slide metadata for slide type, section, intent, reader question, takeaway, required sources, and appendix status.
- Strengthen claim/source modeling so material claims can be traced in both spec and HTML. Numeric values, percentages, currencies, market sizes, growth rates, customer names, policy/regulatory statements, competitor comparisons, performance claims, ROI, risk levels, and superlative language are material-claim candidates.
- Keep `.slide` as the static HTML/CSS unit. Non-appendix slides should have one primary headline, a takeaway or reader question, body evidence, source note when claims or charts are present, and a stable footer/page marker.
- Put reusable CSS design tokens in templates: Korean-capable font stack, 16:9 slide size, 8px spacing scale, safe margins, body/caption minimum sizes, contrast-safe text colors, `word-break: keep-all`, `line-break: strict`, left alignment, and breakable classes for URLs/code/English identifiers.
- Implement editorial QA either as a dedicated `qa-design` command or as part of the existing `qa` command. If a separate command is too disruptive, integrate behind an `--editorial-design` flag while still writing a distinct section in `qa_report.md`.
- Extend render/package verification so `render_manifest.json` proves current `final_deck.html` hash lineage for PNGs and PDF, and package gates fail stale derivative artifacts.
- Add docs for editorial policy, rule IDs, severity, appendix exceptions, direct HTML edit sync, and known renderer/PDF accessibility limits.

## Rule Set Target

- `ED-STRUCT-001` Blocker: `.slide` count, rendered PNG count, PDF page count, and manifest slide count must reconcile.
- `ED-STRUCT-002` Fail: non-appendix slide requires one primary headline or explicit headline metadata.
- `ED-STRUCT-003` Fail: non-appendix slide requires a takeaway or reader question.
- `ED-GRID-001` Fail: major content blocks must stay inside the safe area.
- `ED-GRID-002` Warn/Fail: grid alignment drift above threshold is reported.
- `ED-GRID-003` Blocker: text, chart, table, footer, or source note clipping fails QA.
- `ED-HIER-001` Fail: competing primary headlines or calls to action on one slide fail.
- `ED-HIER-002` Warn: explicit type sizes above the configured scale are reported.
- `ED-TYPE-001` Fail: Korean profile without Korean-capable font stack fails.
- `ED-TYPE-002` Fail: body/table/chart label text below minimum size fails.
- `ED-TYPE-003` Fail: full justification fails unless an explicit, documented exception exists.
- `ED-TYPE-004` Warn/Fail: excessive CJK line length is reported, with appendix relaxation allowed.
- `ED-COPY-001` Warn/Fail: headline, takeaway, or bullet length above policy limits is reported.
- `ED-COPY-002` Fail: excessive bullet count or deeply nested bullets fail unless appendix policy permits.
- `ED-COPY-003` Warn: decision ask without actor/action/decision target is reported.
- `ED-CLAIM-001` Blocker: material claim without source, user confirmation, or assumption label fails.
- `ED-CLAIM-002` Fail: metrics missing required period, unit, formula, or source metadata fail.
- `ED-CLAIM-003` Fail: unsupported superlative or guarantee language fails.
- `ED-DATAVIZ-001` Fail: chart/table without title or caption fails.
- `ED-DATAVIZ-002` Fail: chart/table without source line fails.
- `ED-DATAVIZ-003` Warn/Fail: missing axis, unit, date range, legend, or direct label is reported where applicable.
- `ED-DATAVIZ-004` Fail: color-only meaning fails.
- `ED-DATAVIZ-005` Warn: unfamiliar chart type without explanatory text is reported.
- `ED-A11Y-001` Blocker: normal text contrast below 4.5:1 or large text below 3:1 fails.
- `ED-A11Y-002` Fail: meaningful image/chart/diagram without alt or text description fails.
- `ED-A11Y-003` Fail: image-embedded primary text without HTML text equivalent fails.
- `ED-RENDER-001` Blocker: PNGs not generated from current `final_deck.html` hash fail.
- `ED-RENDER-002` Blocker: PDF not generated from current `final_deck.html` hash fails.
- `ED-RENDER-003` Blocker: direct HTML edits without successful `sync-html-edits` fail.
- `ED-RENDER-004` Fail: PDF page render and `rendered_slides/*.png` visual diff above threshold fails.
- `ED-PACKAGE-001` Blocker: expected delivery output missing fails.
- `ED-PACKAGE-002` Fail: `delivery_summary.md` missing artifact hashes, QA status, assumptions, or blockers fails.

## Verification Checklist

- [ ] Existing repository structure is mapped before implementation, especially `cmd/slidex`, `schemas/`, `decks/_template/`, `fixtures/minimal_deck/`, and any current render/QA/package code.
- [ ] Schema changes validate `document/editorial profile`, editorial design policy, slide intent metadata, source registry, and claim provenance using existing naming conventions where possible.
- [ ] Templates emit static `.slide` HTML/CSS with Korean-safe type defaults, safe margins, source notes, footer/page marker, and tokenized layout.
- [ ] QA emits editorial rule IDs, severities, paths/slide IDs, messages, and remediation hints in `qa_report.md` and any structured output.
- [ ] Render/package checks prove PNG and PDF lineage from the current `out/final_deck.html` hash.
- [ ] `gofmt` is run after Go edits.
- [ ] `go test ./...` passes.
- [ ] `go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json` passes when spec contracts change.
- [ ] `go run ./cmd/slidex doctor --json` passes for packaging or repository-health changes.
- [ ] A `slidex init` smoke test is run when template changes are made, and the temporary deck is removed.
- [ ] A sample deck workflow produces current `strategy.md`, `deck_spec.json`, `out/final_deck.html`, `out/final_deck.generated_baseline.html`, `out/rendered_slides/*.png`, `out/final_deck.pdf`, `out/render_manifest.json`, `out/qa_montage.png`, `out/qa_report.md`, `notes.md`, and `delivery_summary.md`.
- [ ] `qa_montage.png` and `out/final_deck.pdf` are visually inspected before claiming visual QA success.
- [ ] Fixtures cover pass/fail cases for unsupported claims, stale artifacts, clipping, contrast failure, chart source omission, Korean font fallback, and appendix relaxation.
- [ ] Documentation explains policy, rule IDs, severity, exceptions, source/claim handling, and direct HTML edit sync.

## Constraints And Boundaries

- Keep the canonical CLI entry point under `cmd/slidex`.
- Keep deck-specific inputs and outputs under `decks/<deck_id>/`.
- Do not create root-level `brief.md`, `assets/`, `brand/`, `data/`, or `out/` for new work.
- Do not commit ignored local deck outputs, generated delivery artifacts, `apps/desktop/node_modules/`, or `apps/desktop/dist/`.
- Do not add floating dependency versions. Pin any added runtime/library/version exactly and update lockfiles, manifests, docs, and validation checks in the same coherent change.
- Do not restore legacy standalone instruction-file workflows or direct execution fallback paths.
- Preserve `slidex run --deck decks/<deck_id>` as the standard workflow.
- Preserve direct-edit freshness semantics: `slidex sync-html-edits --deck <deck_dir>` is required before derivative artifacts are current.
- Treat every material claim as sourced, user-confirmed, assumption-labeled, removed, or rewritten.
- Do not claim visual QA passed unless rendered slides and PDF are inspected.

## Iteration Policy

1. Explore existing schema, template, render, QA, package, and docs structure before editing.
2. Implement schema/template defaults before QA enforcement.
3. Implement static DOM/spec QA before render/PDF visual diff.
4. Implement render manifest freshness before package gates.
5. Add focused fixtures for each new or changed rule as soon as the rule exists.
6. After each meaningful implementation step, run the narrowest relevant check, then the broader check required by repository rules.
7. Fix blocker-level QA failures before broadening scope or tuning warning thresholds.
8. Preserve unrelated user changes and keep commits narrowly scoped.
9. Leave uncertain thresholds as documented policy defaults with fixtures and TODOs only when a stricter value cannot be justified from current evidence.

## Blocked Stop Conditions

Stop and report blocked status if any of these remain true after reasonable local investigation:

- `cmd/slidex` cannot be built or invoked.
- Required renderer/PDF tooling is unavailable and `slidex doctor --json` cannot identify a supported remediation path.
- `final_deck.html` exists but PNGs or PDF cannot be generated from the same current HTML hash.
- PDF page count, PNG count, `.slide` count, and manifest slide count cannot be reconciled.
- Direct HTML edits are detected but `sync-html-edits` cannot run successfully.
- A material claim cannot be sourced, user-confirmed, labeled as assumption, removed, or rewritten.
- Korean glyph rendering fails and no acceptable system font fallback is available.
- `qa_montage.png` or `out/final_deck.pdf` cannot be inspected.
- Required artifacts would need to be written outside `decks/<deck_id>/`.
- A dependency addition or external source vendoring decision is required but exact version/SHA-256/license policy cannot be determined.

## Expected Final Report Format

The final Goal run should report:

- implemented editorial principles and rule IDs,
- changed files by area,
- commands run and pass/fail results,
- generated deck/artifact paths used for verification,
- visual inspection evidence for PNG montage and PDF,
- remaining warnings or accepted limitations,
- blocked/uncertain claims separated from confirmed results,
- follow-up Goals recommended only for genuinely separate work such as PDF/UA certification or advanced chart grammar.

## Assumptions

- The initial Goal artifact is documentation only and does not activate a thread Goal.
- GPT Pro research was used for design direction, but actual implementation must inspect the local repo before editing.
- No project codebase archive was uploaded to ChatGPT; GPT Pro saw only the repository/product context included in the prompt.
- Existing schema/package names should be preserved where they already cover the intended concept.
- PDF/UA conformance is out of scope for this Goal; selectable text, text equivalents, contrast, source notes, page count, freshness, and visual fidelity are in scope.
- Appendix slides may have relaxed copy density rules, but contrast, clipping, source, and artifact freshness still apply.
- CJK line length, visual diff, and grid drift thresholds may need fixture-driven tuning.
- OCR may be used as a diagnostic fallback, not as the primary QA mechanism.

## Research Notes

GPT Pro cited or referenced the following sources as support for the principles. Treat these as research context, not as repository instructions:

- Bain Capital Ventures, "Creating an Effective Board Deck for Great Meetings": https://baincapitalventures.com/insight/guide-to-creating-an-effective-board-meeting-deck/
- Nielsen Norman Group, "5 Principles of Visual Design in UX": https://www.nngroup.com/articles/principles-visual-design/
- Adobe Learn, "Understanding the Basic Principles of Graphic Design": https://www.adobe.com/learn/express/web/graphic-design-basics
- U.S. Office of Personnel Management, "Plain Language": https://www.opm.gov/information-management/plain-language/
- Digital.gov, "Writing for understanding": https://digital.gov/guides/plain-language/writing
- Welsh Government, "Guide to developing the Project Business Case": https://www.gov.wales/sites/default/files/publications/2018-08/guide-to-developing-the-project-business-case.pdf
- U.S. Web Design System, "Data visualizations": https://designsystem.digital.gov/components/data-visualizations/
- W3C, "Web Content Accessibility Guidelines (WCAG) 2.2": https://www.w3.org/TR/WCAG22/
- Digital.gov, "Test for understanding": https://digital.gov/guides/plain-language/test

## Activation

Review this file, then run:

`/goal @goals/editorial-design-quality-gates.md`
