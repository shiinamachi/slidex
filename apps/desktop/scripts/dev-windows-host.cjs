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
const windowsAppPath = toWindowsPath(process.cwd());
const devServerUrl = process.env.VITE_DEV_SERVER_URL || "http://localhost:5173";

const command = [
  "$ErrorActionPreference = 'Stop'",
  "if (-not (Get-Command pnpm.cmd -ErrorAction SilentlyContinue)) {",
  "  throw 'pnpm.cmd was not found on the Windows host. Install pnpm for Windows Node.js.'",
  "}",
  `$env:VITE_DEV_SERVER_URL = ${toPowerShellString(devServerUrl)}`,
  [
    "pnpm.cmd",
    "dlx",
    "--package",
    toPowerShellString(`electron@${electronVersion}`),
    "electron",
    toPowerShellString(windowsAppPath)
  ].join(" ")
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
