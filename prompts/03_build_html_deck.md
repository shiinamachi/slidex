# 03 Build HTML Deck

Create the static HTML slide source for the business document.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `prompts/_design_prompt_context.md`
- `prompts/_candidate_output_context.md` when relevant

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Inputs

Read:

- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/strategy.md`
- `${OUT_DIR}/source_inventory.md`
- `${OUT_DIR}/intake_questions.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- brand, assets, data, and source files

## Required Outputs

- `${OUT_DIR}/final_deck.html`
- `${OUT_DIR}/final_deck.generated_baseline.html`
- update `${OUT_DIR}/notes.md`

## Build Rules

- HTML/CSS is the visual source.
- Use `<!doctype html>` and an appropriate `lang`.
- Use a `.deck` root containing semantic slide sections:
  `<section class="slide" data-slide-id="slide_01">`.
- Do not embed rendered full-slide PNGs as slide content.
- Use CSS variables for design tokens and font preset.
- Load webfonts or local fonts predictably. Use exact immutable versions or
  local vendored files with SHA-256; never use floating CDN/font references.
- Use fixed production slide dimensions, default `1920x1080`.
- Ensure all slide content is visible inside the fixed slide viewport.
- Apply Korean wrapping rules from `_global_presentation_rules.md`.
- Keep copy concise, concrete, and business-grade.
- Avoid unsupported claims, fake metrics, fake customers, fake product names,
  fake screenshots, and invented technical scope.
- Document exact-pinned external dependencies, generated image decisions, and
  risks in `${OUT_DIR}/notes.md`.

After writing `final_deck.html`, copy the generated HTML to
`final_deck.generated_baseline.html`. This baseline represents the latest
approved generated state before direct user HTML edits.
