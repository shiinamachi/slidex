# slidex install guide

This file is the canonical install guide for Codex and for human operators.
Prefer the GitHub Release package. Build from source only when a release package
is not available for the target platform.

## Codex App one-shot

Paste the short prompt from `CODEX_INSTALL_PROMPT.md` into a new Codex App
thread. Codex should open this repository, read this file, resolve the published
release tag, download the matching package from that exact tag, verify the
SHA-256 checksum, and run the verification commands below.

## What the release contains

GitHub Actions builds release packages for these targets:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

Each package includes the `slidex` binary plus the runtime files the CLI expects
at the workspace root: `decks/_template`, `schemas`, Codex plugin files under
`plugins/slidex`, the repo marketplace under `.agents/plugins`, and the Codex
protocol bundle under `internal/codex/protocol`.

Code signing is deferred. Always verify the SHA-256 checksum before trusting a
downloaded package.

## Install from a release package

1. Resolve the release tag to install. When using GitHub CLI:

   ```bash
   gh release view --repo shiinamachi/slidex --json tagName -q .tagName
   ```

2. Select the package for the local OS and CPU:

   ```text
   slidex_<tag>_linux_amd64.tar.gz
   slidex_<tag>_linux_arm64.tar.gz
   slidex_<tag>_darwin_amd64.tar.gz
   slidex_<tag>_darwin_arm64.tar.gz
   slidex_<tag>_windows_amd64.zip
   slidex_<tag>_windows_arm64.zip
   slidex_<tag>_checksums.txt
   ```

3. Download the package and checksum file from the same release tag.

4. Verify the checksum.

   Linux:

   ```bash
   sha256sum -c slidex_<tag>_checksums.txt --ignore-missing
   ```

   macOS:

   ```bash
   grep ' slidex_<tag>_darwin_<arch>.tar.gz$' slidex_<tag>_checksums.txt | shasum -a 256 -c -
   ```

   Windows PowerShell:

   ```powershell
   $expected = (Select-String "slidex_<tag>_windows_<arch>.zip$" slidex_<tag>_checksums.txt).Line.Split(" ")[0].ToLowerInvariant()
   $actual = (Get-FileHash slidex_<tag>_windows_<arch>.zip -Algorithm SHA256).Hash.ToLowerInvariant()
   if ($actual -ne $expected) { throw "SHA-256 mismatch" }
   ```

5. Extract the package into a stable install directory.

   macOS or Linux:

   ```bash
   mkdir -p "$HOME/.local/share/slidex"
   tar -xzf slidex_<tag>_<os>_<arch>.tar.gz -C "$HOME/.local/share/slidex" --strip-components=1
   mkdir -p "$HOME/.local/bin"
   ln -sf "$HOME/.local/share/slidex/slidex" "$HOME/.local/bin/slidex"
   ```

   Windows PowerShell:

   ```powershell
   $installRoot = Join-Path $env:LOCALAPPDATA "slidex"
   New-Item -ItemType Directory -Force $installRoot | Out-Null
   Expand-Archive -Force slidex_<tag>_windows_<arch>.zip $env:TEMP\slidex-extract
   Copy-Item -Recurse -Force (Join-Path $env:TEMP "slidex-extract\slidex_<tag>_windows_<arch>\*") $installRoot
   ```

6. Add the binary location to `PATH`.

   macOS or Linux:

   ```bash
   export PATH="$HOME/.local/bin:$PATH"
   ```

   Windows PowerShell for the current session:

   ```powershell
   $env:Path = "$installRoot;$env:Path"
   ```

7. Run verification from the install directory:

   ```bash
   cd "$HOME/.local/share/slidex"
   slidex --help
   slidex doctor --render
   ```

   Windows PowerShell:

   ```powershell
   Set-Location $installRoot
   slidex --help
   slidex doctor --render
   ```

`slidex init` and `slidex workbench` expect the packaged runtime files to be
available from the workspace root, so create decks while your current directory
is the extracted package directory unless you are developing from the source
repository.

## Optional Codex Plugin setup

The release package includes a repo marketplace at:

```text
.agents/plugins/marketplace.json
```

When Codex plugin support is available, add or open that marketplace from the
extracted package directory, then install and enable the `slidex` plugin. After
installation, start a new Codex thread before invoking `@slidex` or the bundled
skills.

Codex CLI path from the extracted package directory:

```bash
codex plugin marketplace add "$(pwd)"
codex plugin add slidex@slidex-local --json
codex plugin list
```

If the Codex App plugin directory is the available surface, open the local
marketplace at `.agents/plugins/marketplace.json`, install `slidex`, and start a
new thread.

## Source build fallback

Use this only for development or when a release package is unavailable for the
target platform.

```bash
git clone https://github.com/shiinamachi/slidex.git
cd slidex
mise install
mise exec -- go install ./cmd/slidex
slidex --help
slidex doctor --render
```
