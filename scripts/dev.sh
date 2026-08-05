#!/usr/bin/env bash
# Wrapper macOS/Linux → launcher lintas platform (scripts/dev.mjs)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js belum terpasang. Install dari https://nodejs.org/ lalu coba lagi."
  exit 1
fi

exec node "$ROOT/scripts/dev.mjs" "$@"
