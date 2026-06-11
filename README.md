# slidex

[한국어 문서 →](README.ko.md)

**Local CLI and Codex Plugin for producing HTML-first business documents and
page-style PDFs.**

`slidex` manages each document project as an isolated deck workspace, builds
static HTML slides with design tokens and Korean-safe typography, renders slide
PNGs and a final PDF, and verifies that every delivery artifact is fresh before
sharing.

---

## ⚡ Install with Codex App

Choose one release channel, then paste the matching prompt into a **Codex App**
chat. Codex will automatically install the CLI, register the plugin, and verify
the setup. The default install does not require GitHub CLI or GitHub login.

### Production build

Use this for the latest stable release:

```text
Install slidex production build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the production channel install instructions.
```

### Canary build (Pre-Release)

Canary builds are in-development pre-release builds and may be unstable. Use
this channel only when you need unreleased fixes or want to test upcoming
changes:

```text
Install slidex canary build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the canary channel install instructions; keep canary separate from any existing production install.
```

> See [INSTALL.md](INSTALL.md) for the full internal install reference.

---

## What is slidex?

Use `slidex` when you need a **local, reproducible workflow** for:

- 📊 Business presentations and investor decks
- 📝 Executive reports and proposals
- 🏛️ Government review documents
- 🤝 Customer decision materials

`slidex` is **not** a hosted web app. Everything runs on your machine —
the editable source is an HTML file, and the delivery artifact is a PDF.

### Key files

| File | Role |
|------|------|
| `decks/<deck_id>/out/final_deck.html` | Editable visual source of truth |
| `decks/<deck_id>/out/final_deck.pdf` | Primary delivery file |

---

## Features

| | Feature | Description |
|---|---------|-------------|
| 🗂️ | **Deck Workspaces** | Each project gets its own isolated directory under `decks/` |
| 🏗️ | **HTML-first Build** | Static HTML slides with CSS design tokens, 16:9 widescreen, Korean-capable fonts |
| 🖼️ | **Automated Rendering** | Slide PNGs and paginated PDF via headless Chrome/Chromium/Edge |
| ✅ | **Quality Assurance** | QA montage, QA report, and freshness checks before delivery |
| 📦 | **Package Verification** | Validates all required artifacts exist and match current HTML |
| 🔌 | **Codex Plugin** | Local React wizard workbench for deck creation via Codex App browser |
| 📋 | **Evidence-aware** | Every claim must be sourced, confirmed, or marked as an assumption |

---

## Quick Start

### 1. Create a deck

```bash
slidex init customer-retention
```

### 2. Write the brief

Edit the generated brief with your document goals, audience, and source
material:

```text
decks/customer-retention/brief.md
```

### 3. Add supporting material

Place logos, data files, reference documents, and brand guidelines into the
deck workspace directories (`assets/`, `brand/`, `data/`, `source/`).

### 4. Run the workflow

```bash
slidex run --deck decks/customer-retention
```

This runs the full pipeline: intake → strategy → spec → HTML build → render →
QA → delivery summary → package check.

### 5. Review the output

```text
decks/customer-retention/out/final_deck.html    ← editable HTML
decks/customer-retention/out/final_deck.pdf     ← delivery PDF
decks/customer-retention/out/qa_montage.png     ← visual QA overview
decks/customer-retention/out/qa_report.md       ← QA details
decks/customer-retention/out/delivery_summary.md ← delivery checklist
```

---

## Deck Workspace Structure

Every document project lives under `decks/<deck_id>/`:

```text
decks/<deck_id>/
  brief.md              ← document goal, audience, constraints
  DESIGN.md             ← deck-specific visual direction
  assets/               ← logos, product images, reference files
    reference_docs/
    images/
  brand/                ← brand guidelines, colors, fonts
  data/                 ← CSV, XLSX, chart/table source data
  source/               ← PDFs, DOCX, screenshots, meeting notes
  out/                  ← generated outputs (see below)
```

### Generated outputs

| File | Description |
|------|-------------|
| `out/strategy.md` | Content strategy |
| `out/deck_spec.json` | Structured slide spec |
| `out/final_deck.html` | Editable HTML slides |
| `out/final_deck.generated_baseline.html` | Baseline for diff |
| `out/rendered_slides/*.png` | Individual slide PNGs |
| `out/final_deck.pdf` | Paginated PDF |
| `out/render_manifest.json` | Render metadata and hashes |
| `out/qa_montage.png` | Visual QA montage |
| `out/qa_report.md` | QA findings |
| `out/notes.md` | Presenter notes |
| `out/delivery_summary.md` | Final delivery checklist |

---

## Workflow Pipeline

```text
  init ──→ intake ──→ strategy ──→ spec ──→ build ──→ render ──→ qa ──→ finalize ──→ package
  │                                                                                    │
  └──── slidex run --deck decks/<deck_id> ─────────────────────────────────────────────┘
```

Use `slidex run` for the standard end-to-end workflow. Use individual stage
commands only for inspection or repair:

```bash
slidex inspect --deck decks/<deck_id> --write
slidex intake --deck decks/<deck_id>
slidex strategy --deck decks/<deck_id>
slidex spec --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex finalize --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

If source material is insufficient, `slidex` stops with exit code `3` and
writes questions to `out/intake_questions.md`. Answer them in `brief.md` and
run again.

---

## Editing HTML Directly

If you edit `out/final_deck.html` by hand, sync and re-render before claiming
the PDF is current:

```bash
slidex sync-html-edits --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

---

## Codex Plugin Workbench

The repository includes a Codex Plugin at `plugins/slidex/` — an interactive
workbench for creating deck briefs through the Codex App in-app browser.

```bash
slidex workbench start --deck-id customer-retention
```

Open the returned `http://127.0.0.1:<port>/workbench/<session>` URL in the
Codex App browser. Plugin startup emits a Browser-first `browserOpen` intent:
Codex should use the Browser plugin / `@Browser` when available, with URL click
or manual navigation as fallback. New deck creation through the plugin must
display this React Wizard first; do not fall back to `slidex init`, manual
directory creation, or direct `out/final_deck.html` authoring. The CLI embeds
the default `decks/_template`, so installed binaries can bootstrap the
workbench even when the active workspace has no template folder. For production
or canary release installs, startup automatically checks and applies a newer
verified release before opening the wizard; if an update is applied, it returns
a restart or pending-activation instruction instead of opening the wizard.
Selecting `Complete & generate` writes `brief.md` and workbench artifacts to
the deck's `out/` directory, then starts `slidex run --deck decks/<deck_id>
--non-interactive` in the background.

---

## Troubleshooting

Check the CLI and render environment:

```bash
slidex doctor --render
```

If Chrome is not detected, set one of these environment variables:

```text
CHROME_BIN · GOOGLE_CHROME_BIN · CHROMIUM_BIN · MSEDGE_BIN
CHROME_FOR_TESTING_BIN · PLAYWRIGHT_CHROMIUM_BIN
PLAYWRIGHT_CHROME_BIN · PUPPETEER_EXECUTABLE_PATH
```

Deck-specific diagnostics:

```bash
slidex doctor --deck decks/<deck_id> --render
slidex inspect --deck decks/<deck_id>
```

Schema validation:

```bash
slidex validate-spec --spec decks/<deck_id>/out/deck_spec.json
```

---

## Command Reference

| Command | Description |
|---------|-------------|
| `slidex init <deck_id>` | Create a new deck workspace |
| `slidex run --deck decks/<deck_id>` | Run the full workflow |
| `slidex render --deck decks/<deck_id>` | Render PNGs and PDF |
| `slidex qa --deck decks/<deck_id>` | Run quality assurance |
| `slidex package --deck decks/<deck_id>` | Verify delivery artifacts |
| `slidex sync-html-edits --deck decks/<deck_id>` | Sync manual HTML edits |
| `slidex doctor --render` | Check CLI and render environment |
| `slidex workbench start --deck-id <deck_id>` | Start the Codex workbench |
| `slidex validate-spec --spec <path>` | Validate a deck spec JSON |

Run `slidex --help` for the full command list. See
[commands.md](commands.md) for advanced examples including render overrides and
Codex-specific commands.

---

## License

MIT License. Copyright (c) 2026 shiinamachi. See [LICENSE](LICENSE).
