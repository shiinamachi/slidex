---
name: slidex
description: Compatibility skill for slidex. Prefer slidex-start for new deck workbench startup, slidex-run for full workflows, and slidex-finalize for delivery gates.
---

# slidex Plugin Skill

For a new deck, use `slidex-start` first. This is mandatory: the React Wizard
must be displayed before generation proceeds. The React Wizard saves initial
deck creation input and starts generation with
`slidex run --deck decks/<deck_id> --non-interactive` after the user selects
`Complete & generate`. Use `slidex run --deck decks/<deck_id>` manually only
when repairing or resuming an existing deck.

`slidex-start` / `workbench.start` also owns plugin-only automatic updates.
For release-package installs it checks the active production/canary channel and
applies a verified update before opening the wizard. If the response reports
`autoUpdate.blocksWorkbench: true`, stop deck creation and follow the returned
restart or pending-activation instruction.

Do not use `slidex-run`, `slidex init`, manual directory creation, or direct
`out/final_deck.html` authoring for new deck creation. If an MCP caller selects
`deck.bootstrap`, treat it as a deprecated alias for `workbench.start`; it must
return the same React Wizard startup response, including browser-open
suppression when configured.

Required final gates are current rendered PNGs, `final_deck.pdf`, `render_manifest.json`, `qa_montage.png`, `qa_report.md`, `delivery_summary.md`, and `slidex package --deck decks/<deck_id>`.

Do not create non-HTML/PDF deliverables. Treat user-supplied presentation files
only as passive source material after their contents are available as ordinary
source evidence.
