import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const workbenchDir = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const repoRoot = path.dirname(workbenchDir);
const publicDir = path.join(workbenchDir, ".output", "public");
const embedDir = path.join(repoRoot, "cmd", "slidex", "workbench_assets");
const packageJsonPath = path.join(workbenchDir, "package.json");

const scriptPattern = /<script\b[^>]*\btype=["']module["'][^>]*\bsrc=["']([^"']+)["'][^>]*><\/script>/g;
const stylePattern = /<link\b[^>]*\brel=["']stylesheet["'][^>]*\bhref=["']([^"']+)["'][^>]*>/g;
const modulePreloadPattern = /<link\b[^>]*\brel=["']modulepreload["'][^>]*\bhref=["']([^"']+)["'][^>]*>/g;
const inlineScriptPattern = /<script\b(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g;

async function exists(target) {
  try {
    await fs.access(target);
    return true;
  } catch {
    return false;
  }
}

async function removeGeneratedAssets() {
  await fs.mkdir(embedDir, { recursive: true });
  const entries = await fs.readdir(embedDir, { withFileTypes: true });
  await Promise.all(entries.map((entry) => {
    if (entry.name === "README.md") return Promise.resolve();
    return fs.rm(path.join(embedDir, entry.name), { recursive: true, force: true });
  }));
}

async function copyTree(source, dest) {
  const stats = await fs.stat(source);
  if (stats.isDirectory()) {
    await fs.mkdir(dest, { recursive: true });
    const entries = await fs.readdir(source, { withFileTypes: true });
    await Promise.all(entries.map((entry) => copyTree(path.join(source, entry.name), path.join(dest, entry.name))));
    return;
  }
  await fs.mkdir(path.dirname(dest), { recursive: true });
  await fs.copyFile(source, dest);
}

function normalizeAssetPath(raw) {
  const clean = raw.replace(/^\/+/, "");
  if (!clean || clean.startsWith(".") || clean.includes("..")) {
    throw new Error(`unsafe workbench asset path in generated HTML: ${raw}`);
  }
  return clean;
}

function collect(pattern, html) {
  return [...html.matchAll(pattern)].map((match) => normalizeAssetPath(match[1]));
}

function collectInlineScripts(html) {
  return [...html.matchAll(inlineScriptPattern)]
    .map((match) => match[1].trim())
    .filter(Boolean);
}

async function hashSources() {
  const files = [
    "app.config.ts",
    "package.json",
    "scripts/copy-assets.mjs",
    "tsconfig.json",
    "src/app.tsx",
    "src/entry-client.tsx",
    "src/entry-server.tsx",
    "src/routes/index.tsx",
    "src/styles.css",
    "src/workbench.tsx"
  ];
  const hash = createHash("sha256");
  for (const file of files) {
    const raw = await fs.readFile(path.join(workbenchDir, file));
    hash.update(file);
    hash.update("\0");
    hash.update(raw);
    hash.update("\0");
  }
  return hash.digest("hex");
}

if (!(await exists(publicDir))) {
  throw new Error(`missing SolidStart public output: ${publicDir}`);
}

await removeGeneratedAssets();
await copyTree(publicDir, embedDir);

const indexHTMLPath = path.join(embedDir, "index.html");
const indexHTML = await fs.readFile(indexHTMLPath, "utf8");
const scripts = collect(scriptPattern, indexHTML);
const styles = collect(stylePattern, indexHTML);
const modulePreloads = collect(modulePreloadPattern, indexHTML);
const inlineScripts = collectInlineScripts(indexHTML);
if (scripts.length === 0) {
  throw new Error("SolidStart output did not expose a module script in index.html");
}
if (!inlineScripts.some((script) => script.includes("window.manifest"))) {
  throw new Error("SolidStart output did not expose the window.manifest inline script");
}

const packageJson = JSON.parse(await fs.readFile(packageJsonPath, "utf8"));
const manifest = {
  schemaVersion: "slidex.workbench.assets.v1",
  sourcePackage: packageJson.name,
  sourceVersion: packageJson.version,
  framework: "solidstart",
  csr: true,
  entryHtml: "index.html",
  modulePreloads,
  inlineScripts,
  scripts,
  styles,
  sourceSha256: await hashSources()
};
await fs.writeFile(path.join(embedDir, "slidex-workbench-build.json"), `${JSON.stringify(manifest, null, 2)}\n`);

console.log(`copied SolidStart workbench assets to ${path.relative(repoRoot, embedDir)}`);
