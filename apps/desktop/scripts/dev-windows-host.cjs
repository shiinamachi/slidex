const { execFileSync, spawn } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

function isWsl() {
  if (process.env.WSL_DISTRO_NAME) {
    return true;
  }

  try {
    return fs.readFileSync("/proc/version", "utf8").toLowerCase().includes("microsoft");
  } catch {
    return false;
  }
}

function toPowerShellString(value) {
  return `'${value.replaceAll("'", "''")}'`;
}

function toWindowsPath(value) {
  return execFileSync("wslpath", ["-w", value], { encoding: "utf8" }).trim();
}

function getElectronArgs(defaultAppPath) {
  const rawArgs = process.env.SLIDEX_DESKTOP_WINDOWS_ELECTRON_ARGS_JSON;
  if (!rawArgs) {
    return [defaultAppPath];
  }

  const args = JSON.parse(rawArgs);
  if (!Array.isArray(args) || args.some((arg) => typeof arg !== "string")) {
    throw new Error("SLIDEX_DESKTOP_WINDOWS_ELECTRON_ARGS_JSON must be a JSON string array.");
  }
  return args;
}

function getPnpmVersion(packageManager) {
  const match = /^pnpm@(.+)$/.exec(packageManager);
  if (!match) {
    throw new Error(`Expected packageManager to be pinned as pnpm@<version>, got ${packageManager}.`);
  }
  return match[1];
}

if (!isWsl()) {
  console.error("dev:electron:wsl must be run inside WSL.");
  process.exit(1);
}

const packageJson = require(path.join(process.cwd(), "package.json"));
const nodeTool = `node@${packageJson.engines.node}`;
const pnpmTool = `npm:pnpm@${getPnpmVersion(packageJson.packageManager)}`;
const electronVersion = packageJson.devDependencies.electron;
const electronPackage = `electron@${electronVersion}`;
const windowsAppPath = toWindowsPath(process.cwd());
const electronArgs = getElectronArgs(windowsAppPath);
const devServerUrl = process.env.VITE_DEV_SERVER_URL || "http://localhost:5173";

const command = [
  "$ErrorActionPreference = 'Stop'",
  "function Find-HostCommand($name, $candidates) {",
  "  $command = Get-Command $name -ErrorAction SilentlyContinue",
  "  if ($command) { return $command.Source }",
  "  foreach ($candidate in $candidates) {",
  "    if ($candidate -and (Test-Path $candidate)) { return $candidate }",
  "  }",
  "  return $null",
  "}",
  "$miseCandidates = @(",
  "  (Join-Path $env:LOCALAPPDATA 'mise\\bin\\mise.exe'),",
  "  (Join-Path $env:LOCALAPPDATA 'mise\\shims\\mise.exe'),",
  "  (Join-Path $env:LOCALAPPDATA 'Microsoft\\WinGet\\Links\\mise.exe'),",
  "  (Join-Path $env:USERPROFILE 'scoop\\shims\\mise.exe'),",
  "  (Join-Path $env:USERPROFILE '.local\\bin\\mise.exe')",
  ")",
  "$wingetMise = Get-ChildItem -Path (Join-Path $env:LOCALAPPDATA 'Microsoft\\WinGet\\Packages') -Filter mise.exe -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1",
  "if ($wingetMise) { $miseCandidates += $wingetMise.FullName }",
  "$pnpmCandidates = @(",
  "  (Join-Path $env:LOCALAPPDATA 'mise\\shims\\pnpm.cmd'),",
  "  (Join-Path $env:APPDATA 'npm\\pnpm.cmd')",
  ")",
  "$npmCandidates = @(",
  "  (Join-Path $env:LOCALAPPDATA 'mise\\shims\\npm.cmd'),",
  "  (Join-Path $env:APPDATA 'npm\\npm.cmd'),",
  "  (Join-Path $env:ProgramFiles 'nodejs\\npm.cmd')",
  ")",
  "$npxCandidates = @(",
  "  (Join-Path $env:LOCALAPPDATA 'mise\\shims\\npx.cmd'),",
  "  (Join-Path $env:APPDATA 'npm\\npx.cmd'),",
  "  (Join-Path $env:ProgramFiles 'nodejs\\npx.cmd')",
  ")",
  `$electronPackage = ${toPowerShellString(electronPackage)}`,
  `$electronArgs = @(${electronArgs.map(toPowerShellString).join(", ")})`,
  `$nodeTool = ${toPowerShellString(nodeTool)}`,
  `$pnpmTool = ${toPowerShellString(pnpmTool)}`,
  `$env:VITE_DEV_SERVER_URL = ${toPowerShellString(devServerUrl)}`,
  "$electronCache = Join-Path $env:LOCALAPPDATA (Join-Path 'slidex-desktop\\electron-host' $electronPackage)",
  "$electronDownloadCache = Join-Path $env:LOCALAPPDATA (Join-Path 'slidex-desktop\\electron-download-cache' $electronPackage)",
  "$electronBin = Join-Path $electronCache 'node_modules\\.bin\\electron.cmd'",
  "$electronInstallerScript = Join-Path $electronCache 'install-electron-host.cjs'",
  "$env:electron_config_cache = $electronDownloadCache",
  "function Resolve-ElectronPackageDir {",
  "  $candidates = @(",
  "    (Join-Path $electronCache (Join-Path 'node_modules\\.pnpm' (Join-Path $electronPackage 'node_modules\\electron'))),",
  "    (Join-Path $electronCache 'node_modules\\electron')",
  "  )",
  "  foreach ($candidate in $candidates) {",
  "    if ($candidate -and (Test-Path (Join-Path $candidate 'package.json'))) { return $candidate }",
  "  }",
  "  return $null",
  "}",
  "function Test-ElectronHost {",
  "  $electronPackageDir = Resolve-ElectronPackageDir",
  "  if (-not $electronPackageDir) { throw 'Electron package is not installed in the host cache.' }",
  "  $electronExe = Join-Path $electronPackageDir 'dist\\electron.exe'",
  "  $electronPathTxt = Join-Path $electronPackageDir 'path.txt'",
  "  if ((Test-Path $electronBin) -and (Test-Path $electronExe) -and (Test-Path $electronPathTxt)) { return }",
  "  throw 'Electron host cache is incomplete.'",
  "}",
  "function Write-ElectronInstaller {",
  "@'",
  "const fs = require(\"node:fs\");",
  "const { execFileSync } = require(\"node:child_process\");",
  "const path = require(\"node:path\");",
  "const { pathToFileURL } = require(\"node:url\");",
  "",
  "function toPowerShellString(value) {",
  "  return `'${value.replaceAll(\"'\", \"''\")}'`;",
  "}",
  "",
  "async function main() {",
  "  const packageDir = process.env.ELECTRON_PACKAGE_DIR;",
  "  const cacheRoot = process.env.electron_config_cache;",
  "  const version = require(path.join(packageDir, \"package.json\")).version;",
  "  const packageNodeModules = path.join(packageDir, \"..\");",
  "  const { downloadArtifact } = await import(",
  "    pathToFileURL(path.join(packageNodeModules, \"@electron\", \"get\", \"dist\", \"index.js\")).href",
  "  );",
  "  const zipPath = await downloadArtifact({",
  "    version,",
  "    artifactName: \"electron\",",
  "    platform: \"win32\",",
  "    arch: process.arch,",
  "    cacheRoot,",
  "    checksums: require(path.join(packageDir, \"checksums.json\"))",
  "  });",
  "  const dist = path.join(packageDir, \"dist\");",
  "  fs.rmSync(dist, { recursive: true, force: true });",
  "  fs.mkdirSync(dist, { recursive: true });",
  "  execFileSync(",
  "    \"powershell.exe\",",
  "    [",
  "      \"-NoProfile\",",
  "      \"-Command\",",
  "      `$ErrorActionPreference = 'Stop'; Expand-Archive -LiteralPath ${toPowerShellString(zipPath)} -DestinationPath ${toPowerShellString(dist)} -Force`",
  "    ],",
  "    { stdio: \"inherit\" }",
  "  );",
  "  await fs.promises.writeFile(path.join(packageDir, \"path.txt\"), \"electron.exe\");",
  "}",
  "",
  "main().catch((error) => {",
  "  console.error(error);",
  "  process.exit(1);",
  "});",
  "'@ | Set-Content -Path $electronInstallerScript -Encoding utf8",
  "}",
  "function Ensure-ElectronHost($installCommand, $nodeCommand) {",
  "  try { Test-ElectronHost; return } catch {}",
  "  Remove-Item -Recurse -Force $electronCache -ErrorAction SilentlyContinue",
  "  New-Item -ItemType Directory -Force -Path $electronCache | Out-Null",
  "  New-Item -ItemType Directory -Force -Path $electronDownloadCache | Out-Null",
  "  Set-Content -Path (Join-Path $electronCache 'package.json') -Encoding utf8 -Value '{\"private\":true}'",
  "  Push-Location $electronCache",
  "  try {",
  "    & $installCommand",
  "    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }",
  "  } finally {",
  "    Pop-Location",
  "  }",
  "  Write-ElectronInstaller",
  "  $env:ELECTRON_PACKAGE_DIR = Resolve-ElectronPackageDir",
  "  if (-not $env:ELECTRON_PACKAGE_DIR) { throw 'Electron package install did not produce a package directory.' }",
  "  & $nodeCommand $electronInstallerScript",
  "  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }",
  "  Test-ElectronHost",
  "}",
  "$mise = Find-HostCommand 'mise' $miseCandidates",
  "if ($mise) {",
  "  Ensure-ElectronHost { & $mise exec --yes $nodeTool $pnpmTool -- pnpm add --allow-build=electron $electronPackage } { param($scriptPath) & $mise exec --yes $nodeTool -- node $scriptPath }",
  "  & $mise exec --yes $nodeTool -- $electronBin @electronArgs",
  "  exit $LASTEXITCODE",
  "}",
  "$pnpm = Find-HostCommand 'pnpm.cmd' $pnpmCandidates",
  "if ($pnpm) {",
  "  Ensure-ElectronHost { & $pnpm add --allow-build=electron $electronPackage } { param($scriptPath) node $scriptPath }",
  "  & $electronBin @electronArgs",
  "  exit $LASTEXITCODE",
  "}",
  "$npm = Find-HostCommand 'npm.cmd' $npmCandidates",
  "if ($npm) {",
  "  Ensure-ElectronHost { & $npm install --no-save $electronPackage } { param($scriptPath) node $scriptPath }",
  "  & $electronBin @electronArgs",
  "  exit $LASTEXITCODE",
  "}",
  "$npx = Find-HostCommand 'npx.cmd' $npxCandidates",
  "if ($npx) {",
  "  & $npx --yes $electronPackage @electronArgs",
  "  exit $LASTEXITCODE",
  "}",
  "throw 'Neither mise, pnpm.cmd, npm.cmd, nor npx.cmd was found on the Windows host. Install mise or Windows Node.js.'"
].join(os.EOL);

const child = spawn(
  "powershell.exe",
  ["-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command],
  {
    cwd: "/mnt/c/Windows/System32",
    stdio: "inherit"
  }
);

child.on("exit", (code) => {
  process.exit(code ?? 1);
});

child.on("error", (error) => {
  console.error(`Failed to launch powershell.exe: ${error.message}`);
  process.exit(1);
});
