# slidex Repository Guide

## Communication

- Always ask questions and respond to the user in Korean.
- Use English for internal reasoning and intermediate reasoning.

## Repository Purpose

This repository is the `slidex` local CLI and automation kit for producing,
rendering, validating, and packaging HTML-first business documents. It is not a
hosted application, SaaS product, or general web app.

The canonical CLI entry point is `cmd/slidex`. The production document source
of truth is a deck-local `out/final_deck.html`, and the primary delivery
artifact is a deck-local `out/final_deck.pdf`.

## Repository Work Rules

- Treat this as a CLI tool repository. Prefer changes to Go CLI code, schemas,
  fixtures, templates, plugin packaging, docs, and tests over ad hoc document
  generation.
- Do not recreate legacy standalone instruction-file workflows or direct
  execution fallback paths.
- Use `slidex init <deck_id>` to create deck workspaces; do not instruct users
  to copy templates manually unless debugging the template itself.
- Keep deck-specific inputs and outputs scoped under `decks/<deck_id>/`.
- Root-level `brief.md`, `assets/`, `brand/`, `data/`, and `out/` are legacy
  migration inputs only. Do not create them for new work.
- Do not commit ignored local deck outputs or generated delivery artifacts
  unless the user explicitly asks for that artifact to be versioned.

## Important Paths

- `cmd/slidex/`: Go CLI commands, workflow orchestration, render, QA, package,
  Codex protocol helpers, and tests.
- `schemas/`: JSON contracts used by CLI validation and structured Codex
  integration.
- `decks/_template/`: default template used by `slidex init`.
- `fixtures/minimal_deck/`: regression fixture used by Go tests.
- `plugins/slidex/` and `.agents/skills/slidex/`: companion plugin, skill, hook,
  and local agent guidance package checked by `slidex doctor`.
- `internal/codex/protocol/codex-cli-0.138.0/`: vendored Codex protocol bundle.

## Git Workflow

- Create a Git commit directly after each small, coherent change.
- Keep commits narrowly scoped to the change just completed.
- Do not include unrelated user changes or generated artifacts unless required
  for the task.
- Write commit messages using Conventional Commits.

## Runtime And Dependency Pinning

- Manage runtimes with mise.
- Pin Go exactly in `.mise.toml` and in the `go.mod` `go` directive. Keep those
  values synchronized.
- Pin every runtime and library version to an exact version. Do not use version
  ranges or floating labels such as `latest`, `main`, `master`, `HEAD`, `>=`,
  `<=`, `>`, `<`, `^`, `~`, `x`, or `*`.
- Remote CSS, fonts, browser/render dependencies, and image-processing tools
  must either be vendored locally with SHA-256 recorded or referenced with an
  exact immutable version in manifest and notes outputs.
- When adding or updating dependencies, update the relevant lockfile, manifest,
  documentation, and validation checks in the same coherent change.

## Development Workflow

- Use `rg` / `rg --files` first when searching.
- Use `gofmt` after editing Go files.
- For CLI or schema changes, run `go test ./...`.
- For spec contract changes, also run
  `go run ./cmd/slidex validate-spec --spec examples/sample_deck_spec.json`.
- For packaging or repository-health changes, run
  `go run ./cmd/slidex doctor --json`.
- For template changes, smoke-test `slidex init` with a temporary deck and
  remove that deck afterward.
- For render or PDF changes, verify current HTML renders to PNG/PDF and that
  `slidex qa` / `slidex package` still reflect freshness accurately.

## CLI Product Contract

When implementing CLI behavior, writing docs, or running an actual deck, preserve
these product rules:

- A deck lives under `decks/<deck_id>/` and writes outputs under
  `decks/<deck_id>/out/`.
- `brief.md`, `DESIGN.md`, `assets/`, `brand/`, `data/`, and `source/` are
  inputs; `out/` is generated state and delivery output.
- `DESIGN.md` is deck-specific design guidance, not an execution workflow file.
- `slidex run --deck decks/<deck_id>` is the standard workflow. Stage commands
  are for inspection, repair, or focused verification.
- Expected delivery outputs are `strategy.md`, `deck_spec.json`,
  `final_deck.html`, `final_deck.generated_baseline.html`,
  `rendered_slides/*.png`, `final_deck.pdf`, `render_manifest.json`,
  `qa_montage.png`, `qa_report.md`, `notes.md`, and `delivery_summary.md`.
- Final success requires rendered slide PNGs and the final PDF to be produced
  from the current HTML and inspected.
- Direct edits to `out/final_deck.html` require
  `slidex sync-html-edits --deck <deck_dir>` before claiming derivative
  artifacts are current.

## Document Quality Contract

When the CLI generates or validates document content, preserve these rules:

- Every material claim must be sourced, user-confirmed, or labeled as an
  assumption.
- Unsupported metrics, ROI, market leadership, customer counts, patents,
  certifications, guarantees, compliance claims, security claims, and outcome
  claims must be removed or rewritten.
- User-supplied presentation files are passive source material only after their
  contents are available as ordinary source evidence.
- Do not generate non-HTML/PDF files as required, optional, or compatibility
  delivery formats.
- Do not flatten entire slides into whole-slide PNGs when editable HTML
  structure is practical.
- Use static HTML/CSS slide structure with `.slide` elements, default 16:9
  widescreen, CSS design tokens, Korean-capable font presets, and Korean-safe
  wrapping defaults.
- Keep copy concise, evidence-aware, and suitable for board, investor,
  government review, or customer decision contexts.

## Do Not Do

- Do not add or restore legacy standalone instruction-set workflow files.
- Do not build a hosted service or SaaS product in this workspace.
- Do not treat root-level compatibility folders as the normal workspace model.
- Do not claim visual QA passed unless rendered slides and the PDF were
  inspected.
- Do not revert unrelated user changes.
