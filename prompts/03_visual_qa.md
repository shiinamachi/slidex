# 03 Visual QA

Run visual QA on the generated deck. Do not claim QA passed unless rendered
slides were inspected.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.
Then read `prompts/_global_presentation_rules.md` for text wrapping, typography,
and HTML webfont rules.
Then read `prompts/_design_prompt_context.md` so deck-specific style intent is
included in QA.
If candidate outputs were compared or remain in `${OUT_DIR}`, also read
`prompts/_candidate_output_context.md`.

## Inputs

Read:

- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/notes.md`
- `${ACTIVE_DECK_DIR}/DESIGN.md`
- `${ACTIVE_DECK_DIR}/assets/reference_deck.pptx` and
  `${ACTIVE_DECK_DIR}/assets/reference_decks/` when present, for the documented
  reference influence only
- `checklists/design_qa.md`
- `checklists/accessibility_qa.md`
- `checklists/delivery_qa.md`
- candidate HTML/PPTX outputs, rendered images, screenshots, or QA montages in
  `${OUT_DIR}` when they influenced the final deck

## Tasks

1. Render every slide in `${OUT_DIR}/final_deck.pptx` to PNG images in
   `${OUT_DIR}/rendered_slides/`.
2. Create `${OUT_DIR}/qa_montage.png` as a contact sheet or montage of all
   slides.
3. Inspect the rendered slides visually.
4. Run any available validation checks for the PPTX and generated images.
5. Check for:
   - text overflow
   - object overlap
   - inconsistent margins
   - poor alignment
   - font substitution
   - missing or blocked webfont loading in HTML outputs
   - low contrast
   - unreadable charts
   - excessive text density
   - broken images
   - missing alt text for meaningful images
   - mismatch with `${ACTIVE_DECK_DIR}/DESIGN.md` style directives when present
   - meaningful mismatch with the documented reference deck influence in
     `deck_spec.json` or `${OUT_DIR}/notes.md`
   - HTML/PPTX/spec parity when HTML deck outputs exist: slide count, order,
     headlines, key messages, required constraints, and removed/added slides
   - unsupported claims introduced by HTML or candidate outputs
   - external dependencies, hidden raster slide images, broken font loading,
     mid-word Korean wrapping, overflow, or element collisions in HTML outputs
   - Korean line breaks that split words or syllables rather than eojeol or
     natural phrases

If multiple candidate outputs exist, include a comparison finding: which output
has stronger visual hierarchy, which has safer copy, which violates source
truth, and which reusable patterns should or should not carry into the final
deck.

Use the best available rendering method in the environment, such as a $slides
render/export capability, LibreOffice headless export, PowerPoint/Keynote export,
or another configured PPTX-to-image renderer. Document the exact method used. If
no renderer is available, stop, report the blocker, and do not mark QA as passed.

## Output

Create `${OUT_DIR}/qa_report.md` with:

- Active deck id, active deck directory, and output directory
- Render method and files created
- Overall QA status
- Slide-by-slide findings
- Accessibility findings
- Design and consistency findings
- Reference deck alignment findings, including any unresolved conflict between
  multiple reference decks
- DESIGN.md alignment findings when a deck-specific design prompt exists
- Candidate-output comparison findings when applicable, including adopted,
  adapted, and rejected patterns
- HTML/PPTX/spec parity findings when HTML outputs exist
- Required revisions
- Unresolved risks

If slides could not be rendered, state that clearly and do not mark QA as
passed.
