# Global Presentation Rules

These rules apply to every deck run unless a higher-priority user instruction,
approved template, or brand guide explicitly overrides them.

## Korean Text Wrapping

Korean text must wrap at eojeol or natural phrase boundaries. Do not allow
mid-syllable, mid-word, or visually awkward Korean breaks in titles, cards,
chips, footers, labels, or speaker-facing text.

For HTML or static web deck outputs:

- Use `word-break: keep-all`.
- Use `overflow-wrap: normal` for slide text by default.
- Use `hyphens: none`.
- Use `line-break: strict`.
- Add manual `<br>` breaks only after complete eojeol or natural phrases.
- Do not solve overflow by enabling arbitrary Korean character breaks. Resize
  the text box, reduce copy, change layout, or adjust type scale first.

For PPTX or native slide outputs:

- Use wider text boxes, shorter copy, or explicit line breaks at eojeol/phrase
  boundaries.
- Re-check rendered slides after font changes because Korean line length can
  shift materially between fonts.
- Treat mid-word Korean wrapping as a QA failure unless it is impossible to
  avoid and documented as an unresolved risk.

## Web Font Usage For HTML Decks

HTML deck outputs must load and apply a webfont rather than relying only on
local system fonts. If no approved brand webfont is supplied, use Pretendard for
Korean-heavy B2B decks.

- Load the webfont with a documented external stylesheet or local `@font-face`.
- Apply the webfont to every text role, including headings, body, labels, chips,
  counters, footers, and mono-styled technical labels, unless the brand guide
  explicitly requires a separate mono font.
- Keep sensible fallback fonts for offline or blocked-network environments.
- Document any external font dependency in `${OUT_DIR}/notes.md` or
  `${OUT_DIR}/qa_report.md`.
- During QA, inspect rendered slides after the webfont loads and verify that the
  font change did not introduce overflow, overlap, or awkward Korean wrapping.
