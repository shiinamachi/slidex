# Design Prompt Context

Deck workspaces may include a deck-specific `DESIGN.md` file that acts as a
style prompt for the active deck.

## Optional Design Prompt

After resolving `ACTIVE_DECK_DIR`, check for:

- `${ACTIVE_DECK_DIR}/DESIGN.md`

For legacy root-mode decks, this resolves to `DESIGN.md` at the repository root.
If the file is absent, continue without blocking and do not invent a style
prompt.

## How To Apply DESIGN.md

Use `DESIGN.md` to extract concrete presentation style guidance, such as:

- visual tone and stylistic references
- typography preferences
- color mood and accent guidance
- layout rhythm, density, and composition patterns
- imagery, icon, illustration, and chart style
- explicit style constraints and avoid lists

Reconcile the design prompt with other inputs using this priority:

1. The user's explicit request and the deck brief.
2. Approved template, reference deck set, brand guidelines, and brand colors.
3. Deck-specific `DESIGN.md` style guidance.
4. General repository design, accessibility, QA, and editability rules.

Do not let `DESIGN.md` override factual content, approved brand constraints,
PowerPoint editability, accessibility, or visual QA requirements. If it
conflicts with a higher-priority input, follow the higher-priority input and
document the conflict.

When multiple reference decks are available, treat them as a reference set
rather than a single source of truth. Follow explicit user priority notes first,
then the approved template and brand constraints, and document any reference
patterns that are applied, blended, or ignored.

## Required Documentation

When `DESIGN.md` exists:

- summarize its practical interpretation in `${OUT_DIR}/strategy.md`
- include the source path and distilled style directives in
  `${OUT_DIR}/deck_spec.json`
- record how the prompt affected the build in `${OUT_DIR}/notes.md`
- check the rendered deck against those directives during visual QA
- list any ignored, conflicting, or only partially applied directives as risks

When `DESIGN.md` does not exist, note that no deck-specific design prompt was
provided only where that context is useful. Do not treat absence as a blocker.
