# slidex Command Reference

Primary workflow:

```bash
mise exec -- go install ./cmd/slidex
slidex workbench start --deck-id <deck_id>
slidex workbench evidence --deck-id <deck_id> --inspector "<name-or-role>" --surface codex_app_in_app_browser --invocation "@slidex create a deck called <deck_id>" --url "<workbench.url>" --workbench-visible --saved-input-verified
slidex workbench verify-evidence --deck-id <deck_id>
slidex run --deck decks/<deck_id>
```

Stage commands:

```bash
slidex inspect --deck decks/<deck_id> --write
slidex intake --deck decks/<deck_id>
slidex strategy --deck decks/<deck_id>
slidex spec --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex revise --deck decks/<deck_id>
slidex finalize --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

Codex checks:

The repository vendors Codex protocol schema `0.138.0`; installed Codex CLI
versions at or above that minimum satisfy the runtime gate.

```bash
slidex codex doctor --json
slidex codex schema refresh --codex-version 0.138.0
slidex codex app-server probe
slidex codex app-server plugin-smoke --workspace /tmp/slidex-plugin-smoke --deck-id plugin-smoke
```

Goal mirror:

```bash
slidex goal set --deck decks/<deck_id> --objective "..."
slidex goal status --deck decks/<deck_id>
slidex goal complete --deck decks/<deck_id>
```
