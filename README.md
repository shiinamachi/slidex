# slidex

[Korean documentation](README.ko.md)

`slidex` is a local CLI and Codex Plugin workflow for producing HTML-first
business documents and page-style PDFs. It keeps each document project in its
own deck workspace, builds static HTML slides, renders slide PNGs and a PDF,
and checks that the delivery artifacts are fresh enough to share.

The editable visual source of truth is:

```text
decks/<deck_id>/out/final_deck.html
```

The primary delivery file is:

```text
decks/<deck_id>/out/final_deck.pdf
```

## What slidex is for

Use `slidex` when you need a local, reproducible workflow for business
presentations, proposals, executive reports, investor materials, government
review documents, or customer decision decks.

`slidex` is not a hosted web app. It writes files on your machine under
`decks/<deck_id>/`, and the final package is made from local HTML, PNG, PDF,
JSON, and Markdown artifacts.

## Install

### Requirements

- Windows, macOS, or Linux
- Chrome, Chromium, or Microsoft Edge for rendering HTML to PNG/PDF
- Optional: Codex App or Codex CLI if you want to use the bundled Codex Plugin
- Source build only: Git and `mise`

### Recommended: install from GitHub Releases

Use the packaged release for normal installation. It includes the `slidex`
binary, deck template, schemas, Codex plugin files, and repo marketplace needed
for local use. Release packages are built by GitHub Actions; code signing is
deferred.

Follow:

```text
INSTALL.md
```

Codex App one-shot prompt:

```text
Install slidex from https://github.com/shiinamachi/slidex. Follow INSTALL.md in that repository: resolve the published GitHub Release tag first, use that exact tag's package for this OS/CPU, verify the SHA-256 checksum, add slidex to PATH, install the included Codex plugin with `codex plugin marketplace add <install-dir>` and `codex plugin add slidex@slidex-local` when Codex CLI is available, run `slidex --help` and `slidex doctor --render`, then report any manual follow-up.
```

### Source build fallback

Use this path for development or when a release package is unavailable for the
target platform. The repository pins Go exactly in `.mise.toml` and `go.mod`.
Run all source build commands from the repository root.

```bash
git clone https://github.com/shiinamachi/slidex.git
cd slidex
mise install
mise exec -- go install ./cmd/slidex
```

Add Go's install directory to your `PATH`.

macOS or Linux:

```bash
export PATH="$(mise exec -- go env GOPATH)/bin:$PATH"
```

Windows PowerShell:

```powershell
$env:Path = "$(mise exec -- go env GOPATH)\bin;$env:Path"
```

Windows `cmd.exe`:

```bat
for /f "delims=" %G in ('mise exec -- go env GOPATH') do set "PATH=%G\bin;%PATH%"
```

Persist the same path update in your shell profile or system environment if you
want `slidex` available in future terminal sessions.

Verify the install:

```bash
slidex --help
slidex doctor --render
```

If `slidex` is not found, confirm that the `bin` directory under the path
printed by this command is on your `PATH`:

```bash
mise exec -- go env GOPATH
```

### Update an existing install

For release package installs, repeat the `INSTALL.md` release package steps for
the new release tag.

For source builds:

```bash
git pull
mise install
mise exec -- go install ./cmd/slidex
slidex doctor --render
```

The Codex Plugin resolves the `slidex` binary from `PATH`, so reinstall the CLI
after pulling repository changes if you use the plugin.

## Quick Start

Create a new deck workspace:

```bash
slidex init customer-retention
```

Edit the generated brief:

```text
decks/customer-retention/brief.md
```

Add any source material, brand files, images, spreadsheets, and notes under the
same deck directory. Then run the standard workflow:

```bash
slidex run --deck decks/customer-retention
```

When the run completes, open the delivery summary:

```text
decks/customer-retention/out/delivery_summary.md
```

The files most users review are:

```text
decks/customer-retention/out/final_deck.html
decks/customer-retention/out/final_deck.pdf
decks/customer-retention/out/qa_montage.png
decks/customer-retention/out/qa_report.md
decks/customer-retention/out/delivery_summary.md
```

## Deck Workspaces

Every document project lives under `decks/<deck_id>/`.

```text
decks/<deck_id>/
  brief.md
  DESIGN.md
  assets/
    reference_docs/
    images/
  brand/
    guidelines.md
    colors.json
  data/
  source/
  out/
```

Input files:

- `brief.md`: document goal, audience, decision context, constraints, and source
  notes
- `DESIGN.md`: deck-specific visual direction
- `assets/`: logos, product images, reference images, and supporting files
- `brand/`: brand guidelines, colors, fonts, and related rules
- `data/`: CSV, XLSX, and other chart or table source data
- `source/`: PDFs, DOCX files, screenshots, meeting notes, and other evidence

Generated files:

- `out/strategy.md`
- `out/deck_spec.json`
- `out/final_deck.html`
- `out/final_deck.generated_baseline.html`
- `out/rendered_slides/*.png`
- `out/final_deck.pdf`
- `out/render_manifest.json`
- `out/qa_montage.png`
- `out/qa_report.md`
- `out/notes.md`
- `out/delivery_summary.md`

Use a portable deck ID. It must start with a letter or number and may contain
letters, numbers, `_`, `-`, and `.`. Avoid names that are reserved device names
on Windows, such as `CON`, `NUL`, `COM1`, and `LPT1`.

## Prepare Inputs

Start with a clear `brief.md`. At minimum, include:

- the business question the document must answer
- the audience and decision owner
- the desired decision or next step
- the required language, tone, length, and format
- facts that are approved to use
- claims that need source evidence or should be treated as assumptions

Place supporting material in the deck workspace before running `slidex run`.
Every material claim should be sourced, confirmed by the user, or labeled as an
assumption. Unsupported claims about ROI, market leadership, customer counts,
certifications, security, patents, guarantees, or outcomes should be removed or
rewritten before delivery.

## Run The Workflow

The standard command is:

```bash
slidex run --deck decks/customer-retention
```

This runs intake, strategy, spec, HTML build, baseline creation, rendering, QA,
delivery summary, and package checks.

If the available material is not enough, `slidex` stops with exit code `3` and
writes questions to:

```text
decks/customer-retention/out/intake_questions.md
```

Answer the questions by adding detail to `brief.md`, then run the same command
again.

Use stage commands only when you need to inspect or repair one part of the
workflow:

```bash
slidex inspect --deck decks/customer-retention --write
slidex intake --deck decks/customer-retention
slidex strategy --deck decks/customer-retention
slidex spec --deck decks/customer-retention
slidex build --deck decks/customer-retention
slidex render --deck decks/customer-retention
slidex qa --deck decks/customer-retention
slidex finalize --deck decks/customer-retention
slidex package --deck decks/customer-retention
```

## Review The Output

Before sharing a deck, inspect:

- `out/final_deck.pdf`
- `out/qa_montage.png`
- `out/rendered_slides/*.png`
- `out/qa_report.md`
- `out/delivery_summary.md`

`slidex package --deck decks/<deck_id>` verifies that required delivery files
exist and that rendered artifacts still match the current HTML.

If you edit `out/final_deck.html` by hand, sync the edit before claiming the PDF
or PNGs are current:

```bash
slidex sync-html-edits --deck decks/customer-retention
slidex render --deck decks/customer-retention
slidex qa --deck decks/customer-retention
slidex package --deck decks/customer-retention
```

## Optional: Codex Plugin Workbench

The repository includes a local Codex Plugin in `plugins/slidex`. The plugin is
a front door for creating a deck brief through a local loopback workbench. The
CLI remains the source of truth for build, render, QA, and package stages.

Before using the plugin, make sure the installed `slidex` binary is current:

```bash
mise exec -- go install ./cmd/slidex
```

Start a workbench:

```bash
slidex workbench start --deck-id customer-retention
```

Open the returned `http://127.0.0.1:<port>/workbench/<session>` URL in the Codex
App browser. Saving the workbench writes:

```text
decks/customer-retention/brief.md
decks/customer-retention/out/workbench_draft.json
decks/customer-retention/out/workbench_manifest.json
```

Then run the normal workflow:

```bash
slidex run --deck decks/customer-retention
```

## Troubleshooting

Check the CLI and render environment:

```bash
slidex doctor --render
```

If Chrome is not detected, set one of these environment variables to the browser
binary path:

```text
CHROME_BIN
GOOGLE_CHROME_BIN
CHROMIUM_BIN
MSEDGE_BIN
CHROME_FOR_TESTING_BIN
PLAYWRIGHT_CHROMIUM_BIN
PLAYWRIGHT_CHROME_BIN
PUPPETEER_EXECUTABLE_PATH
```

For a deck-specific check:

```bash
slidex doctor --deck decks/customer-retention --render
slidex inspect --deck decks/customer-retention
```

For schema validation:

```bash
slidex validate-spec --spec decks/customer-retention/out/deck_spec.json
```

## Command Reference

Common commands:

```text
slidex init <deck_id>
slidex run --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
slidex sync-html-edits --deck decks/<deck_id>
slidex doctor --deck decks/<deck_id> --render
slidex workbench start --deck-id <deck_id>
```

Run `slidex --help` for the full command list. See `commands.md` for additional
examples, including advanced render overrides and Codex-specific commands.
