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

if (!isWsl()) {
  console.error("dev:electron:wsl must be run inside WSL.");
  process.exit(1);
}

const packageJson = require(path.join(process.cwd(), "package.json"));
const electronVersion = packageJson.devDependencies.electron;
const electronPackage = `electron@${electronVersion}`;
const windowsAppPath = toWindowsPath(process.cwd());
const devServerUrl = process.env.VITE_DEV_SERVER_URL || "http://localhost:5173";

const command = [
  "$ErrorActionPreference = 'Stop'",
  `$electronPackage = ${toPowerShellString(electronPackage)}`,
  `$appPath = ${toPowerShellString(windowsAppPath)}`,
  `$env:VITE_DEV_SERVER_URL = ${toPowerShellString(devServerUrl)}`,
  "$mise = Get-Command mise -ErrorAction SilentlyContinue",
  "if ($mise) {",
  "  Push-Location $appPath",
  "  try {",
  "    & $mise.Source exec -- pnpm dlx --package $electronPackage electron $appPath",
  "    exit $LASTEXITCODE",
  "  } finally {",
  "    Pop-Location",
  "  }",
  "}",
  "$pnpm = Get-Command pnpm.cmd -ErrorAction SilentlyContinue",
  "if ($pnpm) {",
  "  & $pnpm.Source dlx --package $electronPackage electron $appPath",
  "  exit $LASTEXITCODE",
  "}",
  "$npx = Get-Command npx.cmd -ErrorAction SilentlyContinue",
  "if ($npx) {",
  "  & $npx.Source --yes --package $electronPackage electron $appPath",
  "  exit $LASTEXITCODE",
  "}",
  "throw 'Neither mise, pnpm.cmd, nor npx.cmd was found on the Windows host. Install mise or Windows Node.js.'"
].join(os.EOL);

const child = spawn(
  "powershell.exe",
  ["-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command],
  {
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
