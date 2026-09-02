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

# go build has already READ the embed dir, so the binary is complete. Put the tracked
# placeholder back now: cmd/tract/dist/index.html is the one file in dist/ that git
# tracks, and the `rm -rf` above replaces it with the build's own index.html. Leaving
# it means every build dirties the tree with a bundle-hash diff, and — measured
# 2026-09-02 — one of those diffs eventually gets committed. A committed build artifact
# there is silent, not loud: dist/assets/ is gitignored, so a clone gets an index.html
# asking for bundles that are not in the binary, and spaHandler answers every missing
# path with index.html at 200 OK. Blank page, no error, nothing red.
# cmd/tract/embed_test.go is the gate; this line is what keeps it from ever firing.
if git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1 &&
   git -C "$ROOT" ls-files --error-unmatch cmd/tract/dist/index.html >/dev/null 2>&1; then
  git -C "$ROOT" checkout -- cmd/tract/dist/index.html
  echo "› restored the tracked dist/index.html placeholder (binary already embedded the real one)"
fi

echo "✓ built $ROOT/bin/tract"
