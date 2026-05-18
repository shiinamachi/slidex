# Codex PPTX Production Workspace

## Communication

- Always ask questions and respond to the user in Korean.
- Use English for internal reasoning and intermediate reasoning.

## Project Purpose

This repository is a reusable Codex CLI prompt system for producing polished,
structured, editable PowerPoint decks. It is not an application, SaaS product,
or deck generator program. The workspace provides durable instructions,
templates, schemas, staged prompts, and QA checklists for future deck production
runs.

## Git Workflow

- Create a Git commit directly after each small, coherent change.
- Keep commits narrowly scoped to the change just completed.
- Do not include unrelated user changes or generated artifacts unless required
  for the task.
- Write commit messages using Conventional Commits, such as
  `docs: update agent instructions` or `fix: handle missing broker config`.

## Model And Sub-Agent Selection

- Use GPT-5.5 Xhigh for analysis, investigation, architecture, and design work.
- Use GPT-5.5 High for development, implementation, refactoring, and test fixes.
- Use sub-agents for work that can be performed in parallel.
- Give sub-agents concrete, independent ownership boundaries.
- Sub-agents must not revert or overwrite changes made by other agents or the
  user.

## Inputs To Inspect

For every future deck run, inspect available inputs before making design
decisions:

- `brief.md`
- `assets/template.pptx`
- `assets/reference_deck.pptx`
- `assets/logo.png` and other image assets
- `brand/guidelines.md`
- `brand/colors.json`
- `data/*.csv`
- `data/*.xlsx`
- screenshots, PDFs, notes, or source documents supplied by the user

If a template or reference deck exists, inspect it first and match its aspect
ratio, typography, color system, layout patterns, visual density, and brand
style. Default to 16:9 only when no source deck or template defines the size.

## Required PPTX Workflow

Every future deck production run must follow this sequence:

1. Intake the brief and source materials.
2. Extract deck strategy: audience, objective, story arc, tone, and slide plan.
3. Create a structured `out/deck_spec.json` that follows
   `schemas/deck_spec.schema.json`.
4. Build an editable `out/final_deck.pptx` using native PowerPoint objects.
5. Generate or source visuals only when they improve the story.
6. Render slides to images.
7. Create a QA montage or contact sheet.
8. Visually inspect rendered slides and run available validation checks.
9. Revise layout, overflow, overlap, fonts, contrast, charts, and consistency.
10. Deliver the final PPTX plus notes, QA report, and unresolved risks.

Do not skip visual rendering, montage creation, inspection, or revision before
final delivery.

## Storytelling Rules

- Every slide must have one clear message.
- Use action titles, not generic titles.
- Build a coherent story arc instead of a loose pile of slides.
- Keep slide text concise and avoid dense paragraphs.
- Prefer fewer, larger elements over many small elements.
- Use section dividers for long decks.
- Speaker notes should be added when they improve delivery.
- Avoid vague slides that do not advance the audience toward a decision,
  understanding, or action.

## Design And Layout Rules

- Maintain consistent margins, spacing, alignment, and hierarchy.
- Use brand colors sparingly and intentionally.
- Body text should generally be at least 18pt.
- For live presentation decks, important text should be larger.
- Do not use more than two font families unless the brand requires it.
- Prefer clean structure, strong contrast, and clear visual hierarchy over
  decorative density.
- Do not allow text overflow, object overlap, inconsistent margins, poor
  alignment, unreadable charts, broken images, or excessive text density.

## Native PowerPoint Object Rules

- Keep future PPTX outputs editable in PowerPoint.
- Preserve text as editable text.
- Use native PowerPoint charts where practical.
- Use editable tables, shapes, icons, and diagrams where practical.
- Avoid rasterizing whole slides.
- Raster images are acceptable for photos, generated illustrations, and visual
  assets, but not as a substitute for editable slide structure.

## Image Generation Rules

- Use image generation only for cover visuals, section visuals, diagrams,
  illustrations, or decorative assets that support the story.
- Do not invent a fake brand.
- Save generated visual prompts, image usage decisions, and design notes in
  `out/notes.md`.
- Add alt text for meaningful images.

## Data Visualization Rules

- Charts must be simple, labeled, and readable.
- Choose chart types that match the analytical point.
- Avoid chart clutter, tiny labels, ambiguous legends, and color-only meaning.
- Cite or identify data sources in speaker notes or slide notes when useful.
- Use native PowerPoint charts where practical so users can edit data later.

## Accessibility Rules

- Check text size, contrast, reading order, and alt text.
- Avoid relying on color alone to convey meaning.
- Label charts directly when possible.
- Add speaker notes for complex visuals or dense analytical slides.

## QA And Validation Rules

- Render all slides before final delivery.
- Create `out/qa_montage.png` or an equivalent contact sheet.
- Visually inspect the rendered slides before claiming QA passed.
- Check for text overflow, object overlap, inconsistent margins, poor alignment,
  font substitution, low contrast, unreadable charts, excessive text density,
  broken images, and missing alt text for meaningful images.
- Revise and re-render until meaningful visual and structural issues are fixed.
- List unresolved risks honestly; do not overclaim.

## Deliverables For Future Deck Runs

Expected final outputs are:

- `out/strategy.md`
- `out/deck_spec.json`
- `out/final_deck.pptx`
- `out/notes.md`
- rendered slide images
- `out/qa_montage.png`
- `out/qa_report.md`
- final delivery summary with any unresolved risks

## Do Not Do

- Do not build a web app, SaaS product, or unrelated tooling in this workspace.
- Do not create an actual final presentation deck during prompt-system setup.
- Do not skip strategy, deck spec, rendering, visual QA, revision, or final
  delivery notes in future deck workflows.
- Do not flatten entire slides into images when editable objects are practical.
- Do not claim visual QA passed unless rendered slides were inspected.
