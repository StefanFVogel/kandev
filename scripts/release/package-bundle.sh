#!/usr/bin/env bash
# Finalize the dist/kandev/ release layout from already-built pieces.
# Caller must have run, in this order:
#   - scripts/release/package-web.sh  (produces dist/web/)
#   - go build ./cmd/{kandev,agentctl} -o dist/kandev/bin/...
# After this: dist/kandev/{bin,web} is ready to install or tar.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WEB_SRC="$ROOT_DIR/dist/web"
BUNDLE="$ROOT_DIR/dist/kandev"

if [ ! -d "$WEB_SRC" ]; then
  echo "Missing $WEB_SRC; run scripts/release/package-web.sh first" >&2
  exit 1
fi

mkdir -p "$BUNDLE/web"
cp -R "$WEB_SRC/." "$BUNDLE/web/"

if [ ! -f "$BUNDLE/bin/kandev" ] && [ ! -f "$BUNDLE/bin/kandev.exe" ]; then
  echo "Missing native launcher in $BUNDLE/bin; build cmd/kandev first" >&2
  exit 1
fi

if [ ! -f "$BUNDLE/bin/agentctl" ] && [ ! -f "$BUNDLE/bin/agentctl.exe" ]; then
  echo "Missing agentctl in $BUNDLE/bin; build cmd/agentctl first" >&2
  exit 1
fi

echo "Bundle assembled at $BUNDLE"
