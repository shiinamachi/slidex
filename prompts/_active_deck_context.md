# Active Deck Context

This prompt system supports multiple deck workspaces in one repository. Before
reading or writing deck-specific files, resolve the active deck directory and
keep all inputs and outputs scoped to that deck.

## Preferred Layout

- `decks/<deck_id>/brief.md`
- `decks/<deck_id>/assets/`
- `decks/<deck_id>/brand/`
- `decks/<deck_id>/data/`
- `decks/<deck_id>/source/`
- `decks/<deck_id>/out/`

Repository-level files such as `prompts/`, `schemas/`, `checklists/`, and
templates remain shared prompt-system resources.

## Resolve The Active Deck

Set `ACTIVE_DECK_DIR` before doing deck work.

1. If the user names a deck path or `ACTIVE_DECK_DIR`, use that path.
2. If a `DECK_ID` environment variable is available, map it to
   `decks/<DECK_ID>`.
3. If the current working directory is inside `decks/<deck_id>`, use that deck.
4. If exactly one non-template directory under `decks/` contains `brief.md`, use
   it.
5. If no deck directory is selected and a root-level `brief.md` exists, use the
   repository root for legacy single-deck compatibility.
6. If multiple candidate decks exist and no target is specified, ask the user to
   choose before writing files.

Ignore `decks/_template/` when selecting an active deck.

## Output Rules

- Set `OUT_DIR` to `${ACTIVE_DECK_DIR}/out`, or `out` for legacy root mode.
- Write strategy, deck spec, PPTX, notes, rendered images, QA montage, QA
  report, and final delivery notes only under `OUT_DIR`.
- Store rendered slide images in `${OUT_DIR}/rendered_slides/` unless the user
  explicitly requests a different folder.
- Never write generated deck outputs to root `out/` when the active deck is
  under `decks/`.

## Isolation Rules

- Read deck-specific inputs from `ACTIVE_DECK_DIR` first.
- Use `shared/brand/`, `shared/assets/`, or `shared/data/` only when the user
  explicitly references them or when the active deck lacks equivalent files and
  the shared defaults are clearly relevant.
- Do not mix source files from multiple deck workspaces unless the user asks for
  a combined or comparative deck.
- Record the active deck id and directory in strategy notes, deck spec metadata
  when possible, and delivery notes.
