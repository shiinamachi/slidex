#!/usr/bin/env node

const command = process.argv[2] || "command";

console.error(
  [
    `apps/desktop ${command} is disabled.`,
    "The Electron prototype is tombstoned and is not a shipping path.",
    "Use plugins/slidex and slidex workbench for new slidex UX work."
  ].join("\n")
);

process.exit(2);
