#!/usr/bin/env bash
set -euo pipefail

exec slidex doctor --codex --render --json "$@"

