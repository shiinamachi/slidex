# Codex App one-shot install prompt

Paste this into a new Codex App thread:

```text
Install slidex from https://github.com/shiinamachi/slidex. Follow INSTALL.md in that repository: resolve the published GitHub Release tag first, use that exact tag's package for this OS/CPU, verify the SHA-256 checksum, add slidex to PATH, install the included Codex plugin with `codex plugin marketplace add <install-dir>` and `codex plugin add slidex@slidex-local` when Codex CLI is available, run `slidex --help` and `slidex doctor --render`, then report any manual follow-up.
```
