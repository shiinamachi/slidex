# Design Prompt Context

Business document workspaces may include a deck-specific `DESIGN.md` file that
acts as a style prompt for the active document.

## Optional Design Prompt

After resolving `ACTIVE_DECK_DIR`, check for:

- `${ACTIVE_DECK_DIR}/DESIGN.md`

For legacy root-mode documents, this resolves to `DESIGN.md` at the repository
root. If the file is absent, continue without blocking and do not invent a
style prompt.

## How To Apply DESIGN.md

Use `DESIGN.md` to extract concrete visual guidance:

- visual tone and stylistic references
- typography and font preferences
- color mood and accent guidance
- layout rhythm, density, and composition patterns
- imagery, icon, illustration, chart, and table style
- explicit avoid lists and constraints

Reconcile the design prompt with other inputs using this priority:

1. The user's explicit request and confirmed brief.
2. Source truth, approved brand guidelines, brand colors, and legal/compliance
   constraints.
3. Deck-specific `DESIGN.md` style guidance.
4. General repository HTML, business QA, accessibility, and delivery rules.

Do not let `DESIGN.md` override factual content, claim provenance, approved
brand constraints, accessibility, PDF deliverability, or visual QA requirements.
If it conflicts with a higher-priority input, follow the higher-priority input
and document the conflict.

## Required Documentation

When `DESIGN.md` exists:

- summarize its practical interpretation in `${OUT_DIR}/strategy.md`,
- include the source path and distilled style directives in
  `${OUT_DIR}/deck_spec.json`,
- reflect the directives in HTML/CSS design tokens and layout notes,
- record applied, ignored, and conflicting directives in `${OUT_DIR}/notes.md`,
- check rendered HTML/PDF output against those directives during QA,
- list any partially applied directives as unresolved risks when relevant.

When `DESIGN.md` does not exist, note that no deck-specific design prompt was
provided only where that context is useful. Do not treat absence as a blocker.
