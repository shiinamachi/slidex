# 04 Render HTML

Render the current HTML to images and a paginated PDF.

## Context

Read:

- `prompts/_active_deck_context.md`
- `prompts/_global_presentation_rules.md`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/final_deck.html`

Resolve `ACTIVE_DECK_DIR` and `OUT_DIR`.

## Required Outputs

- `${OUT_DIR}/rendered_slides/*.png`
- `${OUT_DIR}/final_deck.pdf`
- `${OUT_DIR}/render_manifest.json`
- `${OUT_DIR}/qa_montage.png`

## Required Command

Use the CLI for the production workflow:

```bash
codex-business-deck-kit render \
  --html ${OUT_DIR}/final_deck.html \
  --out ${OUT_DIR}/rendered_slides \
  --pdf ${OUT_DIR}/final_deck.pdf \
  --manifest ${OUT_DIR}/render_manifest.json \
  --pdf-mode paginated \
  --selector .slide \
  --width 1920 \
  --height 1080 \
  --font-preset pretendard
```

Adjust width, height, and font preset only when `deck_spec.json` or the user
requires a different preset.

## Rendering Requirements

- Render from the current `final_deck.html`.
- Wait for fonts as supported by the CLI rendering method.
- Capture each `.slide` element, not the full scrolling page.
- Enforce expected dimensions.
- Hide scrollbars in the captured slide surface.
- Fail on missing slide elements, blank screenshots, wrong dimensions, or
  visible clipping indicators.
- Build PDF from current rendered images, one slide image per PDF page.
- Record tool versions, dimensions, hashes, slide ids, created files, PDF page
  count and size, dependencies, and warnings in `render_manifest.json`.

Do not bypass the CLI for required production rendering.
