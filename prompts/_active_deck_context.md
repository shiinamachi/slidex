# Active Deck Context

This prompt system supports multiple business document workspaces in one
repository. Before reading or writing deck-specific files, resolve the active
workspace and keep all inputs and outputs scoped to that directory.

## Preferred Layout

- `decks/<deck_id>/brief.md`
- `decks/<deck_id>/DESIGN.md`
- `decks/<deck_id>/assets/reference_docs/`
- `decks/<deck_id>/assets/images/`
- `decks/<deck_id>/brand/`
- `decks/<deck_id>/data/`
- `decks/<deck_id>/source/`
- `decks/<deck_id>/out/`

Repository-level `prompts/`, `schemas/`, `checklists/`, and templates remain
shared prompt-system resources.

## Resolve The Active Deck

Set `ACTIVE_DECK_DIR` before doing document work.

1. If the user names a deck path or `ACTIVE_DECK_DIR`, use that path.
2. If a `DECK_ID` environment variable is available, map it to
   `decks/<DECK_ID>`.
3. If the current working directory is inside `decks/<deck_id>`, use that deck.
4. If exactly one non-template directory under `decks/` contains `brief.md`, use
   it.
5. If no deck directory is selected and a root-level `brief.md` exists, use the
   repository root for legacy single-document compatibility.
6. If multiple candidate decks exist and no target is specified, ask the user in
   Korean to choose before writing files.

Ignore `decks/_template/` when selecting an active deck.

## Output Rules

- Set `OUT_DIR` to `${ACTIVE_DECK_DIR}/out`, or `out` for legacy root mode.
- Write strategy, spec, HTML, baseline HTML, rendered images, PDF, manifest,
  montage, notes, QA report, sync report, and delivery summary only under
  `OUT_DIR`.
- Store rendered slide images in `${OUT_DIR}/rendered_slides/` unless the user
  explicitly requests a different folder.
- Never write generated outputs to root `out/` when the active workspace is
  under `decks/`.

## Isolation Rules

- Read deck-specific inputs from `ACTIVE_DECK_DIR` first.
- Treat user-supplied PPTX files only as passive source/reference documents.
- If `${ACTIVE_DECK_DIR}/DESIGN.md` exists, treat it as the deck-specific style
  prompt and apply it according to `prompts/_design_prompt_context.md`.
- Use `shared/brand/`, `shared/assets/`, or `shared/data/` only when the user
  explicitly references them or when the active workspace lacks equivalent
  files and the shared defaults are clearly relevant.
- Do not mix source files from multiple workspaces unless the user asks for a
  combined or comparative document.
- Record the active deck id, directory, design prompt source, and output
  contract in strategy, spec metadata, notes, QA, and delivery outputs when
  relevant.
