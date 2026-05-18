# 03 Visual QA

Run visual QA on the generated deck. Do not claim QA passed unless rendered
slides were inspected.

## Active Deck

First read `prompts/_active_deck_context.md`. Resolve `ACTIVE_DECK_DIR` and
`OUT_DIR` before reading inputs or writing files.

## Inputs

Read:

- `${OUT_DIR}/final_deck.pptx`
- `${OUT_DIR}/deck_spec.json`
- `${OUT_DIR}/notes.md`
- `checklists/design_qa.md`
- `checklists/accessibility_qa.md`
- `checklists/delivery_qa.md`

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
   - low contrast
   - unreadable charts
   - excessive text density
   - broken images
   - missing alt text for meaningful images

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
- Required revisions
- Unresolved risks

If slides could not be rendered, state that clearly and do not mark QA as
passed.
