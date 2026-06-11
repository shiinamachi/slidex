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

Store the chosen extension as `EXT` (`tar.gz` or `zip`).

---

## Step 2 — Resolve the latest release tag

Verified release installs require GitHub CLI (`gh`) because release integrity and
artifact attestations are verified with GitHub CLI commands before the package is
trusted. If `gh` is unavailable or unauthenticated, stop and report that a
verified release install cannot be completed.

Use GitHub CLI:

```bash
gh release view --repo shiinamachi/slidex --json tagName -q .tagName
```

For diagnostic lookup only, the public GitHub API can show whether a release tag
exists, but this does not replace GitHub CLI release and attestation
verification:

```bash
curl -sL https://api.github.com/repos/shiinamachi/slidex/releases/latest | grep -Po '"tag_name":\s*"\K[^"]+'
```

Store the result as `TAG`.

Release tags include the leading `v`, while release asset names do not. Store
the asset version separately:

```bash
ASSET_VERSION="${TAG#v}"
```

```powershell
$ASSET_VERSION = $TAG.TrimStart("v")
```

Default installs use the latest stable release. If the user explicitly asks for
a canary install, choose the newest prerelease tag that matches
`v<VERSION>-<short-commit-sha>` and use its matching canary assets instead. Do
not switch an existing install between production and canary in place; the
package's `.slidex/install.json` records the immutable channel for that install.

With GitHub CLI, resolve a canary tag deterministically:

```bash
gh release list --repo shiinamachi/slidex --json tagName,isPrerelease,isDraft,publishedAt --limit 100 \
  -q '.[] | select(.isDraft == false and .isPrerelease == true and (.tagName | test("^v[0-9]+\\.[0-9]+\\.[0-9]+-[0-9a-f]{7,40}$"))) | .tagName' \
  | head -n 1
```

---

## Step 3 — Download the release package and checksums

The release asset naming convention is:

```text
slidex_<ASSET_VERSION>_<OS>_<ARCH>.<EXT>
slidex_<ASSET_VERSION>_checksums.txt
```

Download both files from:

```text
https://github.com/shiinamachi/slidex/releases/download/<TAG>/slidex_<ASSET_VERSION>_<OS>_<ARCH>.<EXT>
https://github.com/shiinamachi/slidex/releases/download/<TAG>/slidex_<ASSET_VERSION>_checksums.txt
```

### Available targets

| OS | Arch | Package file |
|----|------|-------------|
| `linux` | `amd64` | `slidex_<ASSET_VERSION>_linux_amd64.tar.gz` |
| `linux` | `arm64` | `slidex_<ASSET_VERSION>_linux_arm64.tar.gz` |
| `darwin` | `amd64` | `slidex_<ASSET_VERSION>_darwin_amd64.tar.gz` |
| `darwin` | `arm64` | `slidex_<ASSET_VERSION>_darwin_arm64.tar.gz` |
| `windows` | `amd64` | `slidex_<ASSET_VERSION>_windows_amd64.zip` |
| `windows` | `arm64` | `slidex_<ASSET_VERSION>_windows_arm64.zip` |

---

## Step 4 — Verify the SHA-256 checksum and GitHub artifact attestation

Store the package filename:

```bash
PACKAGE_FILE="slidex_${ASSET_VERSION}_${OS}_${ARCH}.${EXT}"
```

```powershell
$PACKAGE_FILE = "slidex_${ASSET_VERSION}_${OS}_${ARCH}.${EXT}"
```

**Linux:**

```bash
sha256sum -c slidex_${ASSET_VERSION}_checksums.txt --ignore-missing
```

**macOS:**

```bash
grep " slidex_${ASSET_VERSION}_darwin_${ARCH}.tar.gz$" slidex_${ASSET_VERSION}_checksums.txt | shasum -a 256 -c -
```

**Windows PowerShell:**

```powershell
$expected = (Select-String "slidex_${ASSET_VERSION}_windows_${ARCH}.zip$" "slidex_${ASSET_VERSION}_checksums.txt").Line.Split(" ")[0].ToLowerInvariant()
$actual   = (Get-FileHash "slidex_${ASSET_VERSION}_windows_${ARCH}.zip" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch: expected $expected, got $actual" }
Write-Host "SHA-256 verified."
```

> **If the checksum does not match, stop immediately and report the failure.**

Then verify GitHub release integrity and artifact attestation:

```bash
gh release verify "$TAG" --repo shiinamachi/slidex
gh release verify-asset "$TAG" "$PACKAGE_FILE" --repo shiinamachi/slidex
gh attestation verify "$PACKAGE_FILE" \
  --repo shiinamachi/slidex \
  --cert-oidc-issuer https://token.actions.githubusercontent.com \
  --cert-identity-regex '^https://github.com/shiinamachi/slidex/.github/workflows/release.yml@refs/(heads/(main|develop)|tags/v[0-9].*)$'
```

> **If `gh` is not installed, authentication is unavailable, or any attestation
> command fails, stop and report the package as unverified. Do not continue the
> install as a verified release package.**

---

## Step 5 — Extract and install

### macOS / Linux

```bash
INSTALL_DIR="$HOME/.local/share/slidex"
mkdir -p "$INSTALL_DIR"
tar -xzf "slidex_${ASSET_VERSION}_${OS}_${ARCH}.tar.gz" -C "$INSTALL_DIR" --strip-components=1

mkdir -p "$HOME/.local/bin"
ln -sf "$INSTALL_DIR/slidex" "$HOME/.local/bin/slidex"
```

### Windows PowerShell

```powershell
$installRoot = Join-Path $env:LOCALAPPDATA "slidex"
New-Item -ItemType Directory -Force $installRoot | Out-Null

$extractDir = Join-Path $env:TEMP "slidex-extract"
Expand-Archive -Force "slidex_${ASSET_VERSION}_windows_${ARCH}.zip" $extractDir
Copy-Item -Recurse -Force (Join-Path $extractDir "slidex_${ASSET_VERSION}_windows_${ARCH}\*") $installRoot
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

After adding or updating the local marketplace/plugin, restart Codex and start a
new thread before invoking `@slidex` or bundled skills.

### Option B — Codex App UI

1. Open the Codex App.
2. Navigate to Settings → Plugins → Add Marketplace.
3. Point to the `marketplace.json` file inside the install directory.
4. Install and enable the `slidex` plugin.
5. Restart Codex and start a **new Codex thread** before invoking `@slidex` or
   bundled skills.

---

## Step 8 — Verify the installation

Run the following commands from the install directory:

```bash
slidex --help
slidex update status --json
slidex doctor --render
```

Expected results:

- `slidex --help` prints the CLI usage and exits with code `0`.
- `slidex update status --json` reports `production` or `canary` for release
  package installs. Source checkouts and `go install` development binaries
  report `local-development` with automatic updates disabled.
- `slidex doctor --render` checks the workspace structure and browser
  availability, and exits with code `0`.
- After registering the plugin, restart Codex, start a new thread, and run:

  ```bash
  slidex codex app-server plugin-smoke --json
  slidex update verify --json
  ```

- If update status reports `pendingActivation: true`, run the reported
  `pendingActivationCommand` before plugin smoke.
- If update status reports `restartRequired: true`, restart Codex, start a new
  thread, rerun `slidex codex app-server plugin-smoke --json`, and rerun
  `slidex update verify --json`.
- Treat bundled skills as active only after plugin smoke reports
  `pluginVerificationStatus: "verified"`, `pluginInstalled: true`,
  `pluginEnabled: true`, `startSkillFound: true`, `pluginPath` under
  `<installRoot>/plugins/slidex`, and `startSkillPath` ending in
  `skills/slidex-start/SKILL.md`, and after `slidex update verify --json`
  reports `restartRequired: false`, `pluginVerificationStatus: "verified"`,
  and matching `verifiedPluginPath` / `verifiedStartSkillPath` values.

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
| Install/update metadata | `.slidex/install.json` |

Code signing is deferred. Always verify the SHA-256 checksum and GitHub artifact
attestation before trusting a downloaded release package.

---

## Updating an existing release install

Run:

```bash
slidex update status --json
slidex update check --json
```

`production` installs follow stable GitHub Releases only. `canary` installs
follow canary prereleases only. `local-development` disables automatic release
updates.

When a matching release archive and checksum file have been downloaded, apply
the verified bundle as one unit:

```bash
slidex update apply --yes --json
```

By default, `update apply` requires GitHub CLI release integrity and artifact
attestation verification. It runs documented GitHub surfaces equivalent to
`gh release verify`, `gh release verify-asset`, and `gh attestation verify` for
the selected release asset. If this verification cannot be completed, the update
fails before activation.

For a manually downloaded archive, pass the local files explicitly:

```bash
slidex update apply \
  --archive slidex_<ASSET_VERSION>_<OS>_<ARCH>.<EXT> \
  --checksums slidex_<ASSET_VERSION>_checksums.txt \
  --target-version <ASSET_VERSION> \
  --target-tag <TAG> \
  --yes \
  --json
```

Do not pass `--candidate` for unattended release updates. A direct extracted
candidate has no release archive attestation evidence, so `update apply
--candidate` requires `--attestation-policy allow-unverified` and is treated as
an explicit manual security decision.

`update apply` validates the candidate bundle before activation. On Unix-like
systems it stages the candidate, keeps a backup of the previous install root,
and marks Codex plugin restart verification as required. On Windows it writes a
pending update handoff because the running executable may be locked. If
`update status --json` reports `pendingActivation: true`, complete the handoff
before plugin smoke by running the reported `pendingActivationCommand`. On
Windows this command uses an activator binary outside both the old install root
and the staged candidate so those directories can be renamed safely:

```bash
<pendingActivationCommand from update status>
```

`--attestation-policy allow-unverified` exists only for an explicit manual
product/security decision. Runs that use it are not considered unattended
verified updates.

After any update that may change bundled plugin content:

1. Restart Codex.
2. Start a new thread.
3. Run `slidex codex app-server plugin-smoke --json`.
4. Confirm plugin smoke reports `pluginVerificationStatus: "verified"`,
   `pluginInstalled: true`, `pluginEnabled: true`, `startSkillFound: true`,
   `pluginPath` under `<installRoot>/plugins/slidex`, and `startSkillPath`
   ending in `skills/slidex-start/SKILL.md`.
5. Run `slidex update verify --json` and confirm `restartRequired` is false,
   `pluginVerificationStatus` is `verified`, and `verifiedPluginPath` /
   `verifiedStartSkillPath` match this install root.

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
