# Delivery QA Checklist

## Final Files

- Active deck id and `ACTIVE_DECK_DIR` are documented.
- `${OUT_DIR}/final_deck.pptx` exists.
- `${OUT_DIR}/deck_spec.json` exists.
- `${OUT_DIR}/notes.md` exists.
- `${OUT_DIR}/qa_report.md` exists.
- `${OUT_DIR}/qa_montage.png` exists.
- Rendered slide images exist in `${OUT_DIR}/rendered_slides/`.
- No generated outputs were written to another deck workspace.

## PPTX Integrity

- PPTX opens correctly.
- No broken images are visible.
- No missing fonts or substitution issues are unresolved.
- No whole-slide rasterization is used unless explicitly justified.

## Visual Issues

- No text overflow remains.
- No object overlap remains.
- Margins and alignment are consistent.
- Charts are readable.
- Tables fit and remain editable.

## HTML Deck Parity

- If HTML output exists, slide count, order, headlines, and key messages match
  `deck_spec.json`, or the spec is updated to match the approved final
  structure.
- HTML render screenshots exist for every slide.
- HTML has no unintended external dependencies unless documented.
- HTML does not introduce unsupported claims that are absent from the approved
  spec.
- Korean text wraps by word or phrase, not mid-word.
- HTML loads a documented webfont and applies it consistently across every text
  role, including mono-styled labels, unless a brand guide explicitly requires a
  separate mono face.

## Documentation

- Generated visual prompts are documented.
- Design decisions are documented.
- DESIGN.md usage, applied directives, and unresolved style prompt conflicts are
  documented when a deck-specific design prompt exists.
- Candidate-output comparison decisions are documented when candidate outputs
  influenced the deck.
- Data sources and assumptions are noted.
- Unresolved risks are listed honestly.
