# 2026-05-18 Update Design: codex-business-deck-kit

Status: Draft with 2026-05-18 user Q&A round 1 incorporated; pending
reviewer sign-off.

This document is the implementation design for changing the current
`codex-pptx-system` prompt workspace into `codex-business-deck-kit`.
It is intentionally detailed so a future Codex CLI session can use this file as
its `/goal` objective and implement the update without leaving known gaps.

The implementation target includes both prompt-system updates and a required Go
CLI named `codex-business-deck-kit`.

## Non-Negotiable Goal

Rework the system from a PPTX-first deck prompt workspace into an HTML-first
business document kit.

The future system must create business-grade documents such as business plans,
IR/company introductions, government grant plans, proposals, and executive
review decks by:

1. understanding sparse or rich user materials through repeated Q&A,
2. creating a structured business document specification,
3. building slide-like HTML as the visual source,
4. rendering each HTML slide to an image,
5. combining rendered images into a PDF,
6. running strict business, visual, evidence, and delivery QA,
7. supporting user edits made directly to the HTML and syncing them back into
   the deck workspace.

## Current-State Findings

The repository is currently a PPTX production prompt system:

- `README.md`, `AGENTS.md`, `commands.md`, and staged prompts describe a
  PowerPoint-oriented workflow.
- `prompts/02_build_pptx.md` creates `${OUT_DIR}/final_deck.pptx`.
- `prompts/03_visual_qa.md` renders PPTX slides to PNG and creates a QA montage.
- `schemas/deck_spec.schema.json` describes an editable PowerPoint deck and
  includes PowerPoint-native chart/table flags.
- HTML support exists only as secondary context in global presentation rules,
  candidate output comparison, and delivery QA.
- `decks/spikalabs/out/final_deck.html` is the strongest available example of
  the desired future direction: a static HTML/CSS deck rendered slide-by-slide
  through headless Chrome/CDP into PNG images, then QA'd through montage and
  browser checks.
- `decks/spikalabs/out/spikalabs_rendered_slides_linear.pdf` is an example of
  images being joined into a PDF, but it is not currently documented as a
  formal delivery artifact in `notes.md`, `qa_report.md`, or
  `delivery_summary.md`.

## Confirmed User Decisions

These decisions were confirmed in the 2026-05-18 Q&A round.

1. PDF shape:
   - The final PDF must behave like a normal presentation export: one slide per
     PDF page.
   - The spikalabs long vertical PDF is historical reference only and must not
     become the default output.

2. Source of truth:
   - `deck_spec.json` is the structured planning and QA contract.
   - `final_deck.html` is the final visual source of truth for rendering and
     PDF output.
   - User-edited HTML must be parsed or diffed and synchronized back to
     `deck_spec.json`, `notes.md`, `qa_report.md`, rendered images, and PDF.

3. PPTX status:
   - PPTX must be completely removed from the future production workflow.
   - Do not keep PPTX as a required, optional, or legacy export path.
   - User-supplied PPTX files may be treated only as passive source/reference
     documents if present. The system must not generate PPTX.

4. Program scope:
   - A separate local CLI program is allowed and preferred if it improves
     reliability.
   - Rust or Go are acceptable candidates; this design recommends Go for the
     first production CLI.

5. HTML edit UX:
   - Use the proposed flow: users may edit `${OUT_DIR}/final_deck.html`
     directly, then run a dedicated sync prompt or CLI command that reconciles
     those edits into the active deck workspace and regenerates dependent
     artifacts.

6. Intake gate:
   - When materials are sparse, Codex must inspect references and then repeat
     Q&A until the desired document is fully understood.
   - Codex must write the understood requirements into deck workspace documents
     before production.
   - Production must not begin until the brief is complete or assumptions are
     explicitly approved.

7. Business document categories:
   - No special document category needs stronger emphasis now.
   - Keep document type metadata useful for context, but use one strong
     baseline business-document QA standard across all document types.

8. Slide standard and fonts:
   - Do not hard-code the spikalabs `1600x900` size as the default merely
     because that deck used it.
   - Set the default slide standard only after web reference research.
   - Provide several standard Korean font presets and allow users to override
     fonts per deck.

## Web Research Basis For Defaults

The default slide and rendering recommendations below are based on current web
reference checks performed on 2026-05-18.

- Microsoft Support documents `Widescreen (16:9)` as the default value for new
  PowerPoint presentations and lists Widescreen as `13.333 in x 7.5 in`.
  Source: https://support.microsoft.com/en-gb/office/change-the-size-of-your-powerpoint-slides-040a811c-be43-40b9-8d04-0de5ed79987e
- Google Slides supports Widescreen `16:9` and custom sizes in pixels, inches,
  centimeters, or points.
  Source: https://support.google.com/docs/answer/3447672/change-the-size-of-your-slides-computer
- reveal.js documents that HTML presentations have a normal authored size and
  preserve aspect ratio while scaling to the display.
  Source: https://revealjs.com/presentation-size/
- Chrome Headless officially supports screenshots and print-to-PDF, and Chrome
  DevTools Protocol exposes screenshot/PDF page APIs suitable for a local CLI.
  Sources: https://developer.chrome.com/docs/chromium/headless and
  https://chromedevtools.github.io/devtools-protocol/tot/Page/
- Pretendard documents webfont usage, variable fonts, and Korean-capable font
  stacks suitable for cross-platform web output.
  Source: https://github.com/orioncactus/pretendard/blob/main/packages/pretendard/docs/en/README.md
- Google's Noto Sans CJK write-up describes Korean coverage and broad digital
  content/UI suitability.
  Source: https://developers.googleblog.com/noto-a-cjk-font-that-is-complete-beautiful-and-right-for-your-language-and-region/

Based on these references, use `16:9` Widescreen as the default aspect ratio and
`1920x1080` as the default raster render size for presentation-quality PDF
output. Keep `1280x720`, `1600x900`, `2560x1440`, and custom dimensions as
configurable presets.

## Target System Name

Use `codex-business-deck-kit` as the system name everywhere user-facing.

Implementation must update references in:

- `README.md`
- `AGENTS.md`
- `commands.md`
- prompt file names and headings where practical
- schema titles and descriptions
- examples and templates
- delivery language in checklists

The repository directory does not need to be renamed unless the user explicitly
requests a filesystem-level rename.

## Target Workspace Model

Retain the existing multi-deck workspace model, but change the semantics from
PowerPoint deck output to HTML/PDF business document output.

Recommended layout:

```text
decks/<deck_id>/
  brief.md
  DESIGN.md
  assets/
    reference_docs/
    logo.png
    images/
  brand/
    guidelines.md
    colors.json
  data/
    *.csv
    *.xlsx
  source/
    *.pdf
    *.pptx
    *.docx
    notes.md
    screenshots/
  out/
    intake_questions.md
    source_inventory.md
    strategy.md
    deck_spec.json
    final_deck.html
    final_deck.generated_baseline.html
    rendered_slides/
      slide_01.png
      slide_02.png
    final_deck.pdf
    render_manifest.json
    qa_montage.png
    qa_report.md
    notes.md
    delivery_summary.md
    html_edit_sync.md
```

`final_deck.pdf` is always the primary delivery PDF and must contain one slide
per page. Do not create long vertically stacked PDFs unless the user explicitly
requests an auxiliary experimental artifact for a specific deck.

## Required End-To-End Workflow

The required workflow becomes:

1. Resolve active deck workspace.
2. Inspect all available inputs before asking questions.
3. Run Q&A intake when brief, audience, purpose, document type, claims,
   constraints, or desired outcome are incomplete.
4. Write or update workspace source docs from Q&A:
   - `${ACTIVE_DECK_DIR}/brief.md`
   - `${OUT_DIR}/intake_questions.md`
   - `${OUT_DIR}/source_inventory.md`
   - optional `${ACTIVE_DECK_DIR}/brand/guidelines.md`
5. Create `${OUT_DIR}/strategy.md`.
6. Create `${OUT_DIR}/deck_spec.json` using the new schema.
7. Build `${OUT_DIR}/final_deck.html` as static, slide-based HTML/CSS.
8. Render every `.slide` to `${OUT_DIR}/rendered_slides/*.png`.
9. Generate `${OUT_DIR}/final_deck.pdf` from rendered images, one slide image
   per PDF page.
10. Create `${OUT_DIR}/render_manifest.json`.
11. Create `${OUT_DIR}/qa_montage.png`.
12. Run automated validation:
    - HTML parse validity.
    - slide count parity.
    - required asset existence.
    - font loading.
    - overflow and clipping checks.
    - blank slide detection.
    - image dimension checks.
    - PDF page-count and page-size checks.
    - render manifest freshness checks.
    - schema validation.
    - claim provenance checks.
13. Visually inspect rendered slides and montage.
14. Revise HTML/spec/notes until QA passes or unresolved risks are documented.
15. If the user directly edits `final_deck.html`, run the HTML edit sync flow.
16. Finalize delivery with created files, checks, and unresolved risks.

No final delivery may claim success unless rendered slide images and the final
PDF were produced from the current HTML and inspected.

## Prompt Set Design

Replace or add prompts as follows. Exact filenames may be adjusted, but the
capabilities are required.

### `prompts/00_start_business_doc.md`

Purpose: one command to begin from sparse materials.

Required behavior:

- Resolve active deck.
- Inspect `brief.md`, `DESIGN.md`, brand files, assets, references, data, and
  source files.
- Build a source inventory.
- Determine document type, audience, decision context, required evidence, and
  output constraints.
- If required information is missing, ask focused questions in Korean.
- Repeat Q&A until all material questions are answered or assumptions are
  explicitly approved.
- Write `${OUT_DIR}/intake_questions.md` with questions, answers, assumptions,
  pending issues, and approval status.
- Update `${ACTIVE_DECK_DIR}/brief.md` with confirmed facts and constraints.
- Do not build final HTML/PDF until intake is complete.

### `prompts/01_create_business_strategy.md`

Purpose: transform intake and source materials into document strategy.

Required output:

- `${OUT_DIR}/strategy.md`

Must include:

- document type and use case,
- audience and decision journey,
- core objective,
- story arc,
- section plan,
- claim/evidence plan,
- design direction,
- reference influence,
- risks and missing evidence,
- baseline business QA focus areas.

### `prompts/02_create_business_doc_spec.md`

Purpose: create a schema-valid specification for an HTML/PDF business document.

Required output:

- `${OUT_DIR}/deck_spec.json`

Must include:

- metadata,
- document type,
- output format contract,
- render config,
- PDF config,
- design system,
- source inventory references,
- claim provenance requirements,
- slide list,
- HTML implementation notes,
- business QA risks,
- accessibility notes,
- user-edit sync policy.

### `prompts/03_build_html_deck.md`

Purpose: create the static HTML slide source.

Required output:

- `${OUT_DIR}/final_deck.html`
- update `${OUT_DIR}/notes.md`

Build rules:

- HTML/CSS is the visual source.
- Use semantic slide sections with stable ids:
  `<section class="slide" data-slide-id="slide_01">`.
- Do not embed rendered slide PNGs as the slide content.
- Load webfonts or local fonts predictably.
- Use CSS suitable for screenshot capture.
- Ensure all slide content is visible within the fixed slide viewport.
- Keep copy concise and business-grade.
- Avoid unsupported claims, fake metrics, fake customers, fake product names,
  fake screenshots, and invented technical scope.
- Document external dependencies in `notes.md`.

### `prompts/04_render_html.md`

Purpose: render the current HTML to images and PDF.

Required outputs:

- `${OUT_DIR}/rendered_slides/*.png`
- `${OUT_DIR}/final_deck.pdf`
- `${OUT_DIR}/render_manifest.json`
- `${OUT_DIR}/qa_montage.png`

Rendering requirements:

- Use the `codex-business-deck-kit` CLI, headless Chrome, or CDP.
- Wait for `document.fonts.ready`.
- Capture each `.slide` element, not the full scrolling page.
- Enforce expected dimensions, default `1920x1080`.
- Hide scrollbars in the captured slide surface.
- Fail on missing slide elements, blank screenshots, wrong dimensions, or
  visible clipping.
- Build PDF from current rendered images, one slide image per PDF page.
- Record exact render method, tool versions, dimensions, and created files.

### `prompts/05_business_visual_qa.md`

Purpose: perform business and visual QA.

Required output:

- `${OUT_DIR}/qa_report.md`

QA must cover:

- slide count/order parity across spec, HTML, rendered images, PDF, montage,
  and delivery summary,
- document-type fit,
- business logic and story arc,
- executive polish,
- claim provenance,
- unsupported metric/superlative detection,
- legal/compliance/security/privacy claim risk,
- source-reference fidelity,
- visual hierarchy,
- layout overflow,
- overlap/collision,
- chart/table readability,
- Korean wrapping,
- font loading,
- image sharpness,
- PDF integrity,
- accessibility,
- reference and DESIGN.md alignment.

### `prompts/06_revise_html_deck.md`

Purpose: fix QA findings.

Required behavior:

- Read QA report, rendered slides, montage, spec, and HTML.
- Fix meaningful issues in HTML and spec.
- Re-render images and PDF.
- Rebuild montage.
- Update notes and QA report.
- Repeat until no material QA issues remain or unresolved risks are explicitly
  accepted.

### `prompts/07_sync_user_html_edits.md`

Purpose: support direct user edits to HTML.

Required behavior:

- Treat current `${OUT_DIR}/final_deck.html` as possibly user-edited.
- Compare current HTML to the last known generated baseline, if available.
- If no baseline exists, parse current HTML and compare against
  `deck_spec.json`.
- Extract slide additions, removals, reordering, copy changes, visual changes,
  asset changes, and dependency changes.
- Preserve user edits unless they create QA, source-truth, or security risks.
- Update `deck_spec.json` to match approved HTML structure and copy.
- Update `notes.md` and create/update `${OUT_DIR}/html_edit_sync.md`.
- Re-render all affected slides, rebuild PDF, rebuild montage, and rerun QA.
- Never silently overwrite user-edited HTML. If regeneration is needed, write a
  backup such as `final_deck.pre_sync_YYYYMMDD_HHMMSS.html`.

### `prompts/08_finalize_business_delivery.md`

Purpose: final delivery verification.

Required behavior:

- Verify all required files exist.
- Verify current HTML hash and dependency hashes match rendered images/PDF
  through `${OUT_DIR}/render_manifest.json`.
- Verify QA report is current.
- Verify no unresolved material issues remain unless documented.
- Write `${OUT_DIR}/delivery_summary.md`.

### `prompts/one_shot_create_business_doc.md`

Purpose: full autonomous flow.

Important rule:

- It may run autonomously only after the intake gate is complete. If material
  intake questions remain, it must stop and ask those questions rather than
  fabricating the brief.

## New Schema Requirements

Replace or version `schemas/deck_spec.schema.json`. A compatible path is
preferred, but the schema title and description must reflect
`codex-business-deck-kit`.

Required top-level fields:

- `metadata`
- `documentType`
- `audience`
- `objective`
- `desiredOutcome`
- `tone`
- `sourceInventory`
- `intakeStatus`
- `outputContract`
- `renderConfig`
- `pdfConfig`
- `designSystem`
- `storyArc`
- `slides`
- `claimProvenance`
- `businessQa`
- `userEditPolicy`

Recommended field details:

```json
{
  "documentType": "business_plan | ir_deck | company_profile | proposal | government_grant_plan | executive_report | custom",
  "intakeStatus": {
    "status": "complete | incomplete | blocked | assumptions_approved",
    "questionsAsked": [],
    "openQuestions": [],
    "approvedAssumptions": []
  },
  "outputContract": {
    "sourceHtml": "out/final_deck.html",
    "renderedSlidesDir": "out/rendered_slides",
    "primaryPdf": "out/final_deck.pdf",
    "renderManifest": "out/render_manifest.json",
    "pdfMode": "paginated",
    "qaMontage": "out/qa_montage.png"
  },
  "renderConfig": {
    "engine": "codex-business-deck-kit-cli | chrome-cdp | other",
    "slideSelector": ".slide",
    "widthPx": 1920,
    "heightPx": 1080,
    "deviceScaleFactor": 1,
    "waitForFonts": true,
    "captureElementOnly": true
  },
  "pdfConfig": {
    "source": "rendered_images",
    "mode": "paginated",
    "pageAspectRatio": "16:9",
    "pageSizeInches": {
      "width": 13.333,
      "height": 7.5
    },
    "imageFit": "exact",
    "background": "#ffffff"
  },
  "claimProvenance": {
    "required": true,
    "unsupportedClaimsPolicy": "remove_or_rewrite",
    "claims": []
  },
  "businessQa": {
    "documentTypeChecklist": [],
    "copyRisks": [],
    "evidenceRisks": [],
    "legalRisks": [],
    "visualRisks": []
  },
  "userEditPolicy": {
    "allowDirectHtmlEdits": true,
    "syncRequiredAfterHtmlEdits": true,
    "preserveUserEditsByDefault": true
  }
}
```

Slide fields must add HTML-focused details:

- `htmlId`
- `sectionRole`
- `headline`
- `keyMessage`
- `bodyContent`
- `layoutIntent`
- `visualIntent`
- `evidenceRefs`
- `claims`
- `renderRisks`
- `qaChecks`

Remove PowerPoint-native fields from required and optional schema paths. The
new production schema must not include PPTX export flags, native PowerPoint
chart/table requirements, or PPTX delivery fields.

## HTML Production Standards

The HTML must be static and renderable in a local browser without a server
unless asset paths require otherwise.

Baseline rules:

- `<!doctype html>` with `lang` set appropriately.
- One `.deck` root containing one or more `.slide` elements.
- Every slide has stable `data-slide-id`.
- Default aspect ratio is `16:9` Widescreen.
- Default render size is `1920px x 1080px`.
- Use configurable size presets:
  - `wide-1080p`: `1920x1080`, default.
  - `wide-720p`: `1280x720`, faster draft rendering.
  - `wide-900p`: `1600x900`, compatibility with spikalabs historical output.
  - `wide-1440p`: `2560x1440`, high-resolution review output.
  - `custom`: user-specified width, height, and aspect ratio.
- No page-level scrollbars visible inside screenshots.
- No whole-slide PNGs embedded as the HTML content.
- Use CSS variables for design tokens.
- Use deterministic local or documented remote font loading.
- Korean-heavy outputs must support font presets and per-deck override:
  - `pretendard`, recommended default for modern Korean business documents.
  - `noto-sans-kr`, stable broad-coverage fallback.
  - `noto-sans-cjk-kr`, local/system fallback for CJK-heavy environments.
  - `ibm-plex-sans-kr`, optional more corporate/geometric style.
  - `suit`, optional clean Korean UI/business style.
  - `custom`, supplied through `brand/guidelines.md`, `brand/fonts/`, or
    `deck_spec.json`.
- Font choice must be stored in `deck_spec.json` and reflected in CSS
  variables. Users must be able to change it without rewriting prompts.
- Apply `word-break: keep-all`, `overflow-wrap: normal`, `hyphens: none`, and
  `line-break: strict`.
- Do not rely on arbitrary responsive reflow for final render; fixed slide
  dimensions must be the production target.
- HTML can include real source images, logos, or generated images only when they
  support the story and are documented.
- Do not invent screenshots, logos, customers, product UI, or metrics.

## Rendering And PDF Standards

Required rendering behavior:

- Render from the current `final_deck.html`.
- Capture each `.slide` element independently.
- Write sequential PNGs using zero-padded names: `slide_01.png`.
- Fail if any slide image is missing, zero-byte, blank, wrong size, or visibly
  clipped.
- Generate PDF from rendered images, not from stale HTML or older images.
- Record render freshness and artifact hashes in
  `${OUT_DIR}/render_manifest.json`.

Recommended implementation:

- Add a local CLI program named `codex-business-deck-kit`.
- The shorter executable name `business-deck-kit` may exist only as an alias,
  but all documentation and acceptance criteria must use
  `codex-business-deck-kit` as the canonical CLI name.
- Recommended implementation language: Go.
- Use headless Chrome through Chrome DevTools Protocol for HTML inspection,
  element screenshots, and PDF-related browser behavior.
- Inputs:
  - `--html decks/<deck_id>/out/final_deck.html`
  - `--out decks/<deck_id>/out/rendered_slides`
  - `--pdf decks/<deck_id>/out/final_deck.pdf`
  - `--pdf-mode paginated`
  - `--selector .slide`
  - `--width 1920`
  - `--height 1080`
  - `--font-preset pretendard|noto-sans-kr|noto-sans-cjk-kr|ibm-plex-sans-kr|suit|custom`
- The CLI must also create `qa_montage.png` or expose a separate `montage`
  subcommand used by the required workflow.
- The CLI must emit `${OUT_DIR}/render_manifest.json` after rendering.

`render_manifest.json` must include:

- source HTML path and SHA-256 hash,
- relevant inline CSS or stylesheet dependency hashes,
- referenced local asset hashes,
- font preset and font dependency identifiers,
- slide selector,
- ordered slide IDs,
- expected and actual slide image dimensions,
- PNG file paths and hashes,
- PDF file path and hash,
- PDF mode, page count, page size, and image fit,
- render timestamp,
- tool name and version,
- Chrome/Chromium version,
- operating system,
- unresolved render warnings.

If `sharp` remains in the toolchain, it should be used only for image
composition, montage generation, or image-to-PDF assembly. It must not be the
primary slide layout renderer.

## Business Document QA Standards

Business QA must be stricter than the current generic design/accessibility QA.

Required checks:

- The document type is explicit and the structure fits that type.
- Every slide has a clear business purpose.
- The document leads the audience toward a decision, understanding, or action.
- All material claims are sourced, user-confirmed, or written as assumptions.
- Unsupported metrics, ROI, market leadership, customer counts, patents,
  certifications, technical guarantees, compliance claims, security claims, and
  outcome claims are removed or rewritten.
- Reference materials are not copied as facts for the target company unless the
  user explicitly confirms they apply.
- If the user supplies a specific document type, apply type-aware checks only
  where they materially reduce risk; do not overcomplicate the default QA with
  separate mandatory category systems.
- When relevant, separate facts, plans, projections, assumptions, scope,
  dependencies, exclusions, evidence, and risks.
- Charts and tables have clear source labels or notes.
- No fake logos, fake screenshots, fake customers, fake case studies, or fake
  product names.
- Tone is appropriate for business stakeholders: concrete, restrained,
  evidence-aware, and free of empty AI/innovation hype.
- Korean copy uses natural business phrasing and avoids awkward mid-word breaks.
- Visual polish is suitable for board, investor, government review, or customer
  decision contexts.

QA report must include:

- `Overall status: pass | pass_with_risks | fail`
- render method,
- files checked,
- slide-by-slide findings,
- business logic findings,
- claim provenance findings,
- visual/accessibility findings,
- PDF findings,
- user-edit sync findings when relevant,
- required revisions,
- unresolved risks.

## User HTML Edit Sync Design

The edit sync flow is required because users may want to manually edit the HTML
after generation.

Implementation requirements:

1. Store a generated baseline when HTML is built:
   - `final_deck.generated_baseline.html`, or
   - a hash/manifest that records the generated HTML state.
2. When sync starts, compare current `final_deck.html` against baseline.
3. Detect:
   - slide count changes,
   - slide reorder,
   - headline changes,
   - body copy changes,
   - data/chart/table changes,
   - visual component changes,
   - asset path changes,
   - font or external dependency changes,
   - removed QA-required elements.
4. Update `deck_spec.json` so slide count, order, headlines, key messages,
   claims, and visual notes match the approved HTML.
5. Determine whether user edits also require updates to:
   - `${ACTIVE_DECK_DIR}/brief.md`,
   - `${OUT_DIR}/strategy.md`,
   - `${OUT_DIR}/source_inventory.md`,
   - `${OUT_DIR}/notes.md`,
   - `${OUT_DIR}/delivery_summary.md`.
6. Update those files when the edit changes document type, audience, objective,
   desired outcome, core claims, evidence, assets, source dependencies, or
   delivery contract.
7. If a derivative file should not be changed automatically, mark it as
   potentially stale in `html_edit_sync.md` and `qa_report.md` with a concrete
   reason.
8. Preserve user changes by default.
9. If a user edit introduces unsupported claims or render failures, report the
   issue and propose a corrected version instead of silently discarding the edit.
10. Re-render images and PDF from the edited HTML.
11. Rebuild `render_manifest.json`.
12. Re-run QA and update the report.
13. Write `html_edit_sync.md` with:
   - sync date,
   - detected changes,
   - accepted changes,
   - corrected/rejected changes,
   - derivative files updated,
   - derivative files marked stale,
   - files regenerated,
   - remaining risks.

## Sparse-Material Intake And Q&A Design

The system must support starting with:

- only reference materials,
- only `DESIGN.md`,
- reference materials plus `DESIGN.md`,
- almost no materials.

Intake must inspect before asking:

- `brief.md`
- `DESIGN.md`
- `brand/`
- `assets/`
- `assets/reference_docs/`
- `source/`
- `data/`
- prior `out/` candidate outputs if relevant.

If a user supplies PPTX files as source materials, inspect them as generic
reference documents only. Do not preserve PPTX-specific input directory names
as part of the new primary workflow.

Intake must ask questions in rounds. Each round should be short and targeted.
Questions are material when the answer changes any of:

- document type,
- target audience,
- objective,
- desired outcome,
- company/product facts,
- claim permission,
- evidence availability,
- tone,
- document length,
- delivery format,
- design constraints,
- required inclusions/exclusions,
- confidentiality boundaries.

The system should distinguish:

- blocking questions: cannot produce responsibly without the answer,
- assumption questions: can proceed only if the assumption is recorded,
- polish questions: useful but not blocking.

The output of intake is not just chat history. It must be written into files:

- `brief.md` for confirmed requirements,
- `out/intake_questions.md` for Q&A history and assumptions,
- `out/source_inventory.md` for materials inspected,
- `out/strategy.md` for synthesized direction.

## Command Guide Changes

Update `commands.md` to include:

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/00_start_business_doc.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/01_create_business_strategy.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/02_create_business_doc_spec.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/03_build_html_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/04_render_html.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/05_business_visual_qa.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/06_revise_html_deck.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/07_sync_user_html_edits.md
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/08_finalize_business_delivery.md
```

One-shot:

```bash
DECK_ID=customer-retention codex exec --sandbox workspace-write - < prompts/one_shot_create_business_doc.md
```

The one-shot prompt must stop for Q&A if the intake gate is incomplete.

Document CLI commands such as:

```bash
codex-business-deck-kit render \
  --html decks/customer-retention/out/final_deck.html \
  --out decks/customer-retention/out/rendered_slides \
  --pdf decks/customer-retention/out/final_deck.pdf \
  --manifest decks/customer-retention/out/render_manifest.json \
  --pdf-mode paginated \
  --selector .slide \
  --width 1920 \
  --height 1080 \
  --font-preset pretendard
```

## Required File Updates During Implementation

Implementation should update or replace these files:

- `README.md`
- `AGENTS.md`
- `commands.md`
- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md`
- `prompts/00_intake_and_strategy.md`
- `prompts/01_create_deck_spec.md`
- `prompts/02_build_pptx.md`
- `prompts/03_visual_qa.md`
- `prompts/04_revise_deck.md`
- `prompts/05_finalize_delivery.md`
- `prompts/one_shot_create_deck.md`
- `schemas/deck_spec.schema.json`
- `checklists/design_qa.md`
- `checklists/accessibility_qa.md`
- `checklists/delivery_qa.md`
- `decks/README.md`
- `decks/_template/README.md`
- `decks/_template/brief.md`
- `decks/_template/DESIGN.md`
- `examples/sample_brief.md`
- `examples/sample_deck_spec.json`

Renaming or deleting old prompts is allowed if all README and command references
are updated. If old prompt filenames remain temporarily to reduce migration
friction, their contents must be replaced with deprecation notices that point to
the new HTML/PDF prompts and must not instruct Codex to create PPTX.

## CLI Program Implementation

Implementing a separate local CLI is required for this transition. The CLI must
make deterministic rendering, validation, sync, and packaging reliable while
Codex remains responsible for reasoning-heavy writing, design, and Q&A.

### Language Recommendation

Use Go for the first production CLI.

Reasons:

- The main workload is orchestration: file I/O, JSON validation, DOM inspection,
  Chrome/CDP calls, image/PDF packaging, and clear command-line UX. Go is a
  pragmatic fit for this.
- Go cross-compilation is straightforward for pure Go binaries through
  `GOOS`/`GOARCH`, which helps distribute a local CLI.
- Rust is also viable, but Rust cross-compilation may require target-specific
  linkers or platform tooling. Its extra rigor is useful for complex engines,
  but the bottleneck here is Chrome rendering and document QA rather than
  memory-sensitive computation.
- Headless Chrome and Chrome DevTools Protocol are the durable integration
  boundary. The CLI should drive installed Chrome/Chromium rather than trying
  to implement an HTML/CSS renderer itself.

### Suggested Go Layout

```text
cmd/codex-business-deck-kit/
  main.go
internal/config/
internal/deck/
internal/render/
internal/pdf/
internal/montage/
internal/qa/
internal/sync/
internal/schema/
```

Suggested commands:

```bash
codex-business-deck-kit inspect --deck decks/<deck_id>
codex-business-deck-kit validate-spec --spec decks/<deck_id>/out/deck_spec.json
codex-business-deck-kit render --html decks/<deck_id>/out/final_deck.html --pdf decks/<deck_id>/out/final_deck.pdf
codex-business-deck-kit qa --deck decks/<deck_id>
codex-business-deck-kit sync-html-edits --deck decks/<deck_id>
codex-business-deck-kit package --deck decks/<deck_id>
```

Suggested responsibilities:

- `inspect`: inventory inputs and output manifest data.
- `validate-spec`: validate `deck_spec.json` against the schema.
- `render`: capture `.slide` elements to PNG, generate paginated PDF, and
  write `render_manifest.json`.
- `qa`: run automated DOM/render/PDF checks and emit machine-readable findings.
- `sync-html-edits`: compare user-edited HTML against the generated baseline.
- `package`: verify final deliverables and write delivery manifest data.

Do not build a SaaS product or hosted web app unless explicitly requested later.
The allowed scope is local automation that makes the Codex CLI workflow
reliable.

## Backward Compatibility

PPTX generation is removed from the future system. Existing PPTX files in
historical deck folders may remain as archived artifacts or source references,
but they are not part of the new workflow and must not be listed as deliverables.

For future runs:

- PPTX output is forbidden.
- User-supplied PPTX files may be inspected only as generic reference/source
  documents.
- Existing `brief.md`, `DESIGN.md`, `brand/`, `assets/`, `data/`, and `source/`
  remain valid inputs.
- Existing HTML candidate outputs may be used as design hypotheses but not as
  factual sources.

## Acceptance Criteria

The implementation is complete only when:

- The system name is changed to `codex-business-deck-kit` in user-facing docs.
- The canonical CLI binary is `codex-business-deck-kit`; any shorter executable
  name is documented only as an alias.
- A Go CLI skeleton exists at `cmd/codex-business-deck-kit/` or an equivalent
  clearly documented Go command path.
- The CLI provides the minimum required commands: `inspect`, `validate-spec`,
  `render`, `qa`, `sync-html-edits`, and `package`.
- The `render` command can render HTML `.slide` elements to PNG, create a
  one-slide-per-page PDF, create or support `qa_montage.png`, and write
  `render_manifest.json`.
- The `sync-html-edits` command or prompt flow can detect direct HTML edits,
  update required derived documents, and mark any unupdated derived documents as
  stale risks.
- The primary workflow is HTML-first and PDF-delivery-first.
- Required deliverables are documented as:
  - `strategy.md`
  - `deck_spec.json`
  - `final_deck.html`
  - `final_deck.generated_baseline.html`
  - `rendered_slides/*.png`
  - `final_deck.pdf`
  - `render_manifest.json`
  - `qa_montage.png`
  - `qa_report.md`
  - `notes.md`
  - `delivery_summary.md`
- PPTX is removed from required and optional future deliverables.
- No primary prompt asks Codex to create, validate, render, or deliver PPTX.
- The schema supports HTML source, render config, PDF config, business QA,
  claim provenance, intake state, and user HTML edit policy.
- Prompt workflow includes sparse-material Q&A intake.
- Prompt workflow includes direct user HTML edit sync.
- QA checklists include business document quality, claim provenance, visual
  rendering, HTML, PDF, and accessibility gates.
- Commands are updated for the new prompt names and workflow.
- Templates and examples reflect the new business document HTML/PDF model.
- The default render contract is 16:9 Widescreen at `1920x1080`, with
  configurable presets and per-deck Korean font selection.
- A reviewer agent finds no missing requirement relative to this design and the
  original user request.

## Implementation Order

Recommended order:

1. Rename system language in README, AGENTS, and commands.
2. Update workspace/output model docs.
3. Update schema.
4. Replace or add staged prompts.
5. Update QA checklists.
6. Update templates and examples.
7. Add the required Go CLI skeleton and minimum commands.
8. Run repository-wide search for stale PPTX-first required-delivery language.
9. Review against acceptance criteria.

## Finalization Gate For This Design Document

This design document itself is not final until:

- a separate review agent inspects this document,
- all reviewer-identified gaps are fixed,
- the final document no longer labels required decisions as unresolved.
