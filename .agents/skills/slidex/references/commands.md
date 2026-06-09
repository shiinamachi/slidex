# slidex Command Reference

Primary workflow:

```bash
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

The repository vendors Codex protocol schema `0.132.0`; installed Codex CLI
versions at or above that minimum satisfy the runtime gate.

```bash
slidex codex doctor --json
slidex codex schema refresh --codex-version 0.132.0
slidex codex app-server probe
```

Goal mirror:

```bash
slidex goal set --deck decks/<deck_id> --objective "..."
slidex goal status --deck decks/<deck_id>
slidex goal complete --deck decks/<deck_id>
```
