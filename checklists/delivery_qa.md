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

## Documentation

- Generated visual prompts are documented.
- Design decisions are documented.
- DESIGN.md usage, applied directives, and unresolved style prompt conflicts are
  documented when a deck-specific design prompt exists.
- Data sources and assumptions are noted.
- Unresolved risks are listed honestly.
