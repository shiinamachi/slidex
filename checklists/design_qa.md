# Design QA Checklist

## Story Clarity

- Each slide has one clear message.
- Headlines are action titles with a point of view.
- The sequence builds a coherent story arc.
- Section dividers clarify transitions in longer decks.

## Slide Hierarchy

- The most important element is visually dominant.
- Supporting content is clearly secondary.
- Dense paragraphs are replaced with concise bullets or visuals.
- Each slide has one dominant visual idea; secondary cards, chips, and footers
  do not compete with the headline.
- No dense card grid forces body copy below readable presentation size.
- Accent color is used for one clear emphasis system per slide, not scattered
  decoration.

## Layout Consistency

- Margins are consistent.
- Alignment is intentional.
- Spacing is even and repeatable.
- Similar slide types use similar structure.

## Design Prompt Alignment

- If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, its practical style directives are
  reflected in `deck_spec.json`.
- Rendered slides match the design prompt's intended tone, density, composition,
  imagery, chart style, and avoid list where practical.
- Conflicts between DESIGN.md, the brief, template, reference deck set, brand
  guidelines, accessibility, or editability are documented rather than hidden.
- Multiple reference decks are inventoried, and the applied or ignored influence
  of each material reference is documented in strategy, spec metadata, notes, or
  QA findings.

## Candidate Output Comparison

- Candidate outputs are compared for reusable hierarchy, composition, and copy
  patterns when the user asks for comparison or multiple outputs influence the
  revision.
- Visually attractive candidate patterns are rejected when they introduce
  unsupported claims, invented metrics, fake product names, or inaccessible
  density.
- Adopted candidate patterns are documented as adopted, adapted, or rejected in
  strategy, spec, notes, or QA findings.
- Final deck copy remains traceable to the brief, source documents, brand files,
  or explicit user evidence.

## Typography

- Body text is generally at least 18pt.
- Important live presentation text is larger.
- No more than two font families are used unless the brand requires it.
- Font substitution is checked in rendered slides.

## Color

- Brand colors are used sparingly.
- Contrast is strong enough for projection and screen sharing.
- Color is not the only carrier of meaning.

## Visual Quality

- Images are sharp and purposeful.
- Generated visuals support the story.
- Decorative elements do not compete with the message.

## Chart Quality

- Charts are simple, labeled, and readable.
- Axes, legends, and annotations are clear.
- Chart type matches the analytical message.

## Executive Polish

- The deck feels deliberate, concise, and decision-ready.
- No slide looks unfinished or generic.

## Editability

- Text remains editable.
- Charts, tables, shapes, and diagrams are native objects where practical.
- Whole slides are not rasterized.
