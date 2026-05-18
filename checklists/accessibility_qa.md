# Accessibility QA Checklist

## Text And Reading

- Important text is readable at final PDF size.
- Dense paragraphs are reduced or split.
- Reading order follows headline, key message, body, visual, source/risk notes.
- Korean line breaks are natural and do not split words or syllables awkwardly.

## Contrast And Color

- Text has sufficient contrast against backgrounds.
- Chart labels and annotations remain legible.
- Color is not the only carrier of meaning.
- Red/green or status colors include text, symbols, labels, or patterns.

## Images And Alternatives

- Meaningful images have alt text or documented alt text requirements in spec
  or notes.
- Decorative images are marked or documented as decorative.
- Generated or sourced visuals have usage notes in `${OUT_DIR}/notes.md`.

## HTML And PDF

- HTML uses semantic slide sections and meaningful heading structure.
- Font loading does not create overflow, clipping, or unreadable fallbacks.
- Rendered PNGs are sharp and correct size.
- Final PDF has one slide per page and preserves readable content.

## Charts And Tables

- Charts and tables include source labels or notes where useful.
- Labels are direct and readable.
- Legends are avoided when direct labeling is clearer.
