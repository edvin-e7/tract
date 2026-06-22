#!/usr/bin/env bash
# Build the frontend and stage its output into the Go embed dir, then build the
# single binary. macOS/Linux portable: no GNU-only flags, explicit paths.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EMBED_DIR="$ROOT/cmd/tract/dist"

echo "› building frontend"
( cd "$ROOT/frontend" && npm install && npm run build )

echo "› staging dist into embed dir"
rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR"
cp -R "$ROOT/frontend/dist/." "$EMBED_DIR/"

echo "› building binary"
( cd "$ROOT" && go build -o "$ROOT/bin/tract" ./cmd/tract )

echo "✓ built $ROOT/bin/tract"
