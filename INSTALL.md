# slidex — Internal Install Instructions for Codex

> **This file is read by Codex, not by end users.** When a user pastes the
> one-shot prompt from `CODEX_INSTALL_PROMPT.md` into Codex App, Codex opens
> this repository and follows these steps. Execute every step in order and
> report the result of each.

---

## Step 1 — Detect the local environment

Determine the operating system and CPU architecture of the machine where the
install will run.

| Variable | How to detect | Expected values |
|----------|---------------|-----------------|
| `OS` | `uname -s` on Unix, or `$env:OS` / `[System.Runtime.InteropServices.RuntimeInformation]::OSDescription` on Windows | `linux`, `darwin`, `windows` |
| `ARCH` | `uname -m` on Unix (`x86_64` → `amd64`, `aarch64` / `arm64` → `arm64`), or `$env:PROCESSOR_ARCHITECTURE` on Windows (`AMD64` → `amd64`, `ARM64` → `arm64`) | `amd64`, `arm64` |

Choose the package format:

- `linux` or `darwin` → `.tar.gz`
- `windows` → `.zip`

---

## Step 2 — Resolve the latest release tag

Use GitHub CLI if available:

```bash
gh release view --repo shiinamachi/slidex --json tagName -q .tagName
```

If `gh` is not installed, query the GitHub API:

```bash
curl -sL https://api.github.com/repos/shiinamachi/slidex/releases/latest | grep -Po '"tag_name":\s*"\K[^"]+'
```

Store the result as `TAG`.

---

## Step 3 — Download the release package and checksums

The release asset naming convention is:

```text
slidex_<TAG>_<OS>_<ARCH>.<EXT>
slidex_<TAG>_checksums.txt
```

Download both files from:

```text
https://github.com/shiinamachi/slidex/releases/download/<TAG>/slidex_<TAG>_<OS>_<ARCH>.<EXT>
https://github.com/shiinamachi/slidex/releases/download/<TAG>/slidex_<TAG>_checksums.txt
```

### Available targets

| OS | Arch | Package file |
|----|------|-------------|
| `linux` | `amd64` | `slidex_<TAG>_linux_amd64.tar.gz` |
| `linux` | `arm64` | `slidex_<TAG>_linux_arm64.tar.gz` |
| `darwin` | `amd64` | `slidex_<TAG>_darwin_amd64.tar.gz` |
| `darwin` | `arm64` | `slidex_<TAG>_darwin_arm64.tar.gz` |
| `windows` | `amd64` | `slidex_<TAG>_windows_amd64.zip` |
| `windows` | `arm64` | `slidex_<TAG>_windows_arm64.zip` |

---

## Step 4 — Verify the SHA-256 checksum

**Linux:**

```bash
sha256sum -c slidex_${TAG}_checksums.txt --ignore-missing
```

**macOS:**

```bash
grep " slidex_${TAG}_darwin_${ARCH}.tar.gz$" slidex_${TAG}_checksums.txt | shasum -a 256 -c -
```

**Windows PowerShell:**

```powershell
$expected = (Select-String "slidex_${TAG}_windows_${ARCH}.zip$" "slidex_${TAG}_checksums.txt").Line.Split(" ")[0].ToLowerInvariant()
$actual   = (Get-FileHash "slidex_${TAG}_windows_${ARCH}.zip" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch: expected $expected, got $actual" }
Write-Host "SHA-256 verified."
```

> **If the checksum does not match, stop immediately and report the failure.**

---

## Step 5 — Extract and install

### macOS / Linux

```bash
INSTALL_DIR="$HOME/.local/share/slidex"
mkdir -p "$INSTALL_DIR"
tar -xzf "slidex_${TAG}_${OS}_${ARCH}.tar.gz" -C "$INSTALL_DIR" --strip-components=1

mkdir -p "$HOME/.local/bin"
ln -sf "$INSTALL_DIR/slidex" "$HOME/.local/bin/slidex"
```

### Windows PowerShell

```powershell
$installRoot = Join-Path $env:LOCALAPPDATA "slidex"
New-Item -ItemType Directory -Force $installRoot | Out-Null

$extractDir = Join-Path $env:TEMP "slidex-extract"
Expand-Archive -Force "slidex_${TAG}_windows_${ARCH}.zip" $extractDir
Copy-Item -Recurse -Force (Join-Path $extractDir "slidex_${TAG}_windows_${ARCH}\*") $installRoot
Remove-Item -Recurse -Force $extractDir -ErrorAction SilentlyContinue
```

---

## Step 6 — Add to PATH

### macOS / Linux

```bash
export PATH="$HOME/.local/bin:$PATH"
```

To persist, append the export to `~/.bashrc`, `~/.zshrc`, or the appropriate
shell profile.

### Windows PowerShell (current session)

```powershell
$env:Path = "$installRoot;$env:Path"
```

To persist, add the directory via System → Environment Variables, or run:

```powershell
[Environment]::SetEnvironmentVariable("Path", "$installRoot;$([Environment]::GetEnvironmentVariable('Path','User'))", "User")
```

---

## Step 7 — Register the Codex plugin

The release package includes a local plugin marketplace at:

```text
.agents/plugins/marketplace.json
```

### Option A — Codex CLI

From the install directory:

```bash
codex plugin marketplace add "$(pwd)"
codex plugin add slidex@shiinamachi --json
codex plugin list
```

### Option B — Codex App UI

1. Open the Codex App.
2. Navigate to Settings → Plugins → Add Marketplace.
3. Point to the `marketplace.json` file inside the install directory.
4. Install and enable the `slidex` plugin.
5. Start a **new Codex thread** before invoking `@slidex` or bundled skills.

---

## Step 8 — Verify the installation

Run the following commands from the install directory:

```bash
slidex --help
slidex doctor --render
```

Expected results:

- `slidex --help` prints the CLI usage and exits with code `0`.
- `slidex doctor --render` checks the workspace structure and browser
  availability, and exits with code `0`.

> If `slidex doctor --render` reports that Chrome is not detected, set one of
> these environment variables to the browser binary path:
>
> `CHROME_BIN`, `GOOGLE_CHROME_BIN`, `CHROMIUM_BIN`, `MSEDGE_BIN`,
> `CHROME_FOR_TESTING_BIN`, `PLAYWRIGHT_CHROMIUM_BIN`,
> `PLAYWRIGHT_CHROME_BIN`, `PUPPETEER_EXECUTABLE_PATH`

---

## What the release package contains

Each release package includes:

| Contents | Path |
|----------|------|
| `slidex` binary | root |
| Deck template | `decks/_template/` |
| JSON schemas | `schemas/` |
| Codex plugin | `plugins/slidex/` |
| Plugin marketplace | `.agents/plugins/marketplace.json` |
| Codex protocol bundle | `internal/codex/protocol/codex-cli-0.138.0/` |

Code signing is deferred. Always verify the SHA-256 checksum before trusting a
downloaded package.

---

## Source build (development only)

Use this only when a release package is unavailable for the target platform or
when developing slidex itself.

```bash
git clone https://github.com/shiinamachi/slidex.git
cd slidex
mise install
mise exec -- go install ./cmd/slidex
slidex --help
slidex doctor --render
```

Requires [mise](https://mise.jdx.dev/) and Go `1.26.3` (pinned in
`.mise.toml`).
