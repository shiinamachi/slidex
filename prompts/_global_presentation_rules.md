# Global Business Document Rules

These rules apply to every `slidex` run unless a higher
priority user instruction, approved source, or brand guide explicitly overrides
them.

## HTML Source And Render Contract

- `${OUT_DIR}/final_deck.html` is the final visual source of truth.
- `${OUT_DIR}/final_deck.pdf` is the primary delivery artifact.
- Default aspect ratio is 16:9 Widescreen.
- Default render size is `1920px x 1080px`.
- Supported presets are `wide-1080p`, `wide-720p`, `wide-900p`, `wide-1440p`,
  and `custom`.
- Use a `.deck` root containing semantic
  `<section class="slide" data-slide-id="slide_01">` elements.
- Capture each `.slide` element independently; do not capture a full scrolling
  page.
- Do not embed rendered full-slide PNGs as the HTML slide content.
- Hide page-level scrollbars in the captured surface.

## Runtime And Dependency Pinning

Use mise-managed runtimes for local automation. Runtime and library versions
must be exact pins, not ranges or floating labels.

- Keep Go pinned exactly in `.mise.toml` and the `go.mod` `go` directive.
- Do not use `latest`, `main`, `master`, `HEAD`, `>=`, `<=`, `>`, `<`, `^`,
  `~`, `x`, or `*` for runtime, library, font, CDN, or renderer versions.
- Remote CSS, fonts, scripts, image assets, browser/render tools, and other
  dependencies must be vendored locally with SHA-256 or referenced by an exact
  immutable version.
- Record exact versions, SHA-256 values when available, and unresolved
  dependency risks in `deck_spec.json`, `render_manifest.json`, `qa_report.md`,
  or `notes.md` as appropriate.

## Korean Text Wrapping

Korean text must wrap at eojeol or natural phrase boundaries. Do not allow
mid-syllable, mid-word, or visually awkward Korean breaks in titles, cards,
chips, footers, labels, tables, or speaker-facing text.

For HTML output:

- Use `word-break: keep-all`.
- Use `overflow-wrap: normal` for slide text by default.
- Use `hyphens: none`.
- Use `line-break: strict`.
- Add manual `<br>` breaks only after complete eojeol or natural phrases.
- Do not solve overflow by enabling arbitrary Korean character breaks. Reduce
  copy, adjust layout, resize the text box, or revise the type scale first.

## Font Presets

Korean-heavy business documents must use a deterministic Korean-capable font
preset and record the choice in `deck_spec.json` and CSS variables.

- `pretendard`: recommended default for modern Korean business documents.
- `noto-sans-kr`: stable broad-coverage fallback.
- `noto-sans-cjk-kr`: local/system fallback for CJK-heavy environments.
- `ibm-plex-sans-kr`: optional corporate/geometric style.
- `suit`: optional clean Korean UI/business style.
- `custom`: supplied through `brand/guidelines.md`, `brand/fonts/`, or
  `deck_spec.json`.

Document external or local font dependencies in `${OUT_DIR}/notes.md`,
`${OUT_DIR}/render_manifest.json`, and QA findings when relevant.

## Business Copy Integrity

- Every material claim, metric, customer statement, technical capability,
  certification, security statement, compliance claim, and outcome claim must
  trace to source material, explicit user confirmation, or a recorded
  assumption.
- Remove or rewrite unsupported superlatives and metrics.
- Do not invent customer names, logos, screenshots, product names, market
  positions, patents, certifications, or implementation scope.
- Prefer concrete, restrained wording such as `supports`, `enables`,
  `is designed to`, `helps standardize`, `provides a path for`, and
  `applies to`.
