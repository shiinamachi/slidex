# 03 Visual QA

Run visual QA on the generated deck. Do not claim QA passed unless rendered
slides were inspected.

## Inputs

Read:

- `out/final_deck.pptx`
- `out/deck_spec.json`
- `out/notes.md`
- `checklists/design_qa.md`
- `checklists/accessibility_qa.md`
- `checklists/delivery_qa.md`

## Tasks

1. Render every slide in `out/final_deck.pptx` to PNG images.
2. Create `out/qa_montage.png` as a contact sheet or montage of all slides.
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

## Output

Create `out/qa_report.md` with:

- Render method and files created
- Overall QA status
- Slide-by-slide findings
- Accessibility findings
- Design and consistency findings
- Required revisions
- Unresolved risks

If slides could not be rendered, state that clearly and do not mark QA as
passed.
