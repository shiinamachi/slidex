# codex-business-deck-kit Workspace

## Communication

- Always ask questions and respond to the user in Korean.
- Use English for internal reasoning and intermediate reasoning.

## Project Purpose

This repository is a reusable Codex CLI prompt system and local automation kit
for producing polished, structured, HTML-first business documents. It supports
business plans, IR or company introductions, government grant plans, proposals,
and executive review documents. It is not a hosted application or SaaS product.

The future production source of truth is `${OUT_DIR}/final_deck.html`, and the
primary delivery artifact is `${OUT_DIR}/final_deck.pdf`.

## Workspace Model

This repository supports multiple document workspaces under `decks/<deck_id>/`.
Before every run, resolve the active deck directory using
`prompts/_active_deck_context.md` and keep inputs and outputs scoped there.

- Preferred active path: `decks/<deck_id>/`
- Per-deck output path: `${ACTIVE_DECK_DIR}/out/`
- Legacy root-level `brief.md`, `assets/`, `brand/`, `data/`, and `out/` are
  supported only for single-document compatibility.
- Do not mix materials from multiple workspaces unless the user explicitly asks
  for a combined or comparative document.
- Use `shared/brand/`, `shared/assets/`, or `shared/data/` only when explicitly
  referenced or clearly needed as shared defaults.

## Git Workflow

- Create a Git commit directly after each small, coherent change.
- Keep commits narrowly scoped to the change just completed.
- Do not include unrelated user changes or generated artifacts unless required
  for the task.
- Write commit messages using Conventional Commits.

## Model And Sub-Agent Selection

- Use GPT-5.5 Xhigh for analysis, investigation, architecture, and design work.
- Use GPT-5.5 High for development, implementation, refactoring, and test fixes.
- Use sub-agents for work that can be performed in parallel.
- Give sub-agents concrete, independent ownership boundaries.
- Sub-agents must not revert or overwrite changes made by other agents or the
  user.

## Inputs To Inspect

For every future document run, inspect available inputs in the active workspace
before making design or content decisions:

- `${ACTIVE_DECK_DIR}/brief.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${ACTIVE_DECK_DIR}/assets/reference_docs/`
- `${ACTIVE_DECK_DIR}/assets/logo.png` and image assets
- `${ACTIVE_DECK_DIR}/brand/guidelines.md`
- `${ACTIVE_DECK_DIR}/brand/colors.json`
- `${ACTIVE_DECK_DIR}/data/*.csv`
- `${ACTIVE_DECK_DIR}/data/*.xlsx`
- `${ACTIVE_DECK_DIR}/source/`
- screenshots, PDFs, DOCX files, notes, user-supplied PPTX files, or other
  supplied source documents

User-supplied PPTX files may be inspected only as passive source/reference
documents. The system must not generate PPTX or list PPTX as a future required
or optional delivery artifact.

If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, treat it as the deck-specific style
prompt. Apply it after the brief, source evidence, approved brand constraints,
and accessibility requirements, but before general design defaults. Document its
source, applied directives, and conflicts in strategy, spec, notes, QA, or
delivery outputs.

## Required HTML/PDF Workflow

Every future production run must follow this sequence:

1. Resolve the active workspace.
2. Inspect all available inputs before asking questions.
3. Run Q&A intake when brief, audience, purpose, document type, claims,
   constraints, or desired outcome are incomplete.
4. Write or update `${ACTIVE_DECK_DIR}/brief.md`,
   `${OUT_DIR}/intake_questions.md`, `${OUT_DIR}/source_inventory.md`, and
   optional brand guidance from Q&A.
5. Create `${OUT_DIR}/strategy.md`.
6. Create `${OUT_DIR}/deck_spec.json` following
   `schemas/deck_spec.schema.json`.
7. Build `${OUT_DIR}/final_deck.html` as static, slide-based HTML/CSS.
8. Copy the latest generated HTML to
   `${OUT_DIR}/final_deck.generated_baseline.html`.
9. Render every `.slide` to `${OUT_DIR}/rendered_slides/*.png` using
   `codex-business-deck-kit render`.
10. Generate `${OUT_DIR}/final_deck.pdf`, one rendered slide image per page.
11. Create `${OUT_DIR}/render_manifest.json`.
12. Create `${OUT_DIR}/qa_montage.png`.
13. Run automated validation: HTML parse checks, slide count parity, required
    assets, font loading, overflow and clipping risk, blank slide detection,
    image dimensions, PDF page count and page size, manifest freshness, schema
    validation, and claim provenance.
14. Visually inspect rendered slides and montage.
15. Revise HTML/spec/notes until QA passes or unresolved risks are documented
    and accepted.
16. Finalize delivery with created files, checks, and unresolved risks.

No final delivery may claim success unless rendered slide images and the final
PDF were produced from the current HTML and inspected.

## Storytelling Rules

- Every slide must have one clear business purpose.
- Use action headlines, not generic titles.
- Build a coherent story arc toward a decision, understanding, or action.
- Keep slide text concise and avoid dense paragraphs.
- Prefer fewer, larger elements over many small elements.
- Separate facts, plans, projections, assumptions, evidence, and risks when
  relevant.
- Avoid vague slides that do not advance the document.

## Design And HTML Rules

- Maintain consistent margins, spacing, alignment, and hierarchy.
- Default to 16:9 Widescreen and `1920x1080` render size.
- Keep `1280x720`, `1600x900`, `2560x1440`, and custom dimensions configurable.
- Use CSS variables for design tokens.
- Use a documented Korean-capable font preset: `pretendard` by default, with
  `noto-sans-kr`, `noto-sans-cjk-kr`, `ibm-plex-sans-kr`, `suit`, and `custom`
  available.
- Store font choice in `deck_spec.json` and reflect it in CSS variables.
- Use `word-break: keep-all`, `overflow-wrap: normal`, `hyphens: none`, and
  `line-break: strict` for Korean-heavy output.
- Do not embed whole-slide PNGs as HTML content.
- Do not invent screenshots, logos, customers, product UI, metrics, or
  unsupported technical scope.

## Business QA Rules

- The document type must be explicit and the structure must fit that type.
- Every material claim must be sourced, user-confirmed, or written as an
  assumption.
- Unsupported metrics, ROI, market leadership, customer counts, patents,
  certifications, guarantees, compliance claims, security claims, and outcome
  claims must be removed or rewritten.
- Reference materials are not facts for the target company unless the user
  confirms they apply.
- Tone must be concrete, restrained, evidence-aware, and free of empty hype.
- Charts and tables need clear source labels or notes.
- Korean copy must use natural business phrasing and avoid awkward mid-word
  breaks.
- Visual polish must be suitable for board, investor, government review, or
  customer decision contexts.

## HTML Edit Sync

Users may edit `${OUT_DIR}/final_deck.html` directly. After any direct edit,
run the sync prompt or CLI command. The sync flow must:

- compare the current HTML to `final_deck.generated_baseline.html` or the spec,
- detect slide additions, removals, reordering, copy changes, visual changes,
  asset changes, and dependency changes,
- preserve user edits by default,
- update `deck_spec.json`, `notes.md`, `html_edit_sync.md`, and any derivative
  files that safely follow from the edit,
- mark stale derivative files with concrete reasons when they cannot be updated,
- re-render images, rebuild PDF and montage, rerun QA, and update the accepted
  baseline.

## Deliverables For Future Runs

Expected final outputs are:

- `${OUT_DIR}/strategy.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/final_deck.generated_baseline.html`
- rendered slide images in `${OUT_DIR}/rendered_slides/`
- `${OUT_DIR}/final_deck.pdf`
- `${OUT_DIR}/render_manifest.json`
- `${OUT_DIR}/qa_montage.png`
- `${OUT_DIR}/qa_report.md`
- `${OUT_DIR}/notes.md`
- `${OUT_DIR}/delivery_summary.md`

## Do Not Do

- Do not build a web app, SaaS product, or hosted service in this workspace.
- Do not create an actual final business document during prompt-system setup.
- Do not skip intake, strategy, spec, rendering, visual QA, revision, or final
  delivery notes in future workflows.
- Do not generate PPTX as a required, optional, or legacy export path.
- Do not flatten entire slides into images when editable HTML structure is
  practical.
- Do not claim visual QA passed unless rendered slides and the PDF were
  inspected.
