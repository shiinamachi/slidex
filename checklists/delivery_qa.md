# Delivery QA Checklist

## Final Files

- Active deck id and `ACTIVE_DECK_DIR` are documented.
- `${OUT_DIR}/strategy.md` exists.
- `${OUT_DIR}/deck_spec.json` exists.
- `${OUT_DIR}/final_deck.html` exists.
- `${OUT_DIR}/final_deck.generated_baseline.html` exists.
- `${OUT_DIR}/rendered_slides/*.png` exists.
- `${OUT_DIR}/final_deck.pdf` exists.
- `${OUT_DIR}/render_manifest.json` exists.
- `${OUT_DIR}/qa_montage.png` exists.
- `${OUT_DIR}/qa_report.md` exists.
- `${OUT_DIR}/notes.md` exists.
- `${OUT_DIR}/delivery_summary.md` exists before final handoff.

## Render And PDF Integrity

- Render manifest records current HTML hash and generated artifact hashes.
- Rendered slide count matches spec, HTML, PDF page count, and montage.
- PNG dimensions match the expected render size.
- No rendered slide is blank, clipped, or stale.
- PDF uses paginated mode with one slide image per page.
- Manifest freshness has been checked after direct HTML edits.

## Business QA

- Document type fit is checked.
- Story arc and decision journey are coherent.
- Claim provenance findings are resolved or documented.
- Legal, compliance, security, privacy, and unsupported outcome risks are
  documented.
- Source references are used faithfully.

## Visual And Accessibility

- No material overflow, overlap, broken image, missing font, unreadable chart,
  or low-contrast issue remains unless documented as accepted risk.
- Korean wrapping is acceptable.
- Font preset and external dependencies are documented.
- Meaningful images have alt text or documented alt text requirements.

## User HTML Edit Sync

- If the user edited `final_deck.html`, `html_edit_sync.md` exists.
- Sync findings list detected changes, accepted changes, rejected/corrected
  changes, stale derivative files, regenerated files, and baseline hashes.
- `final_deck.generated_baseline.html` matches the accepted current HTML after
  sync.
