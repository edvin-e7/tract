#!/usr/bin/env node
// Deterministic anti-slop CSS linter. Codifies the design discipline Tract's
// stylesheet already follows, so future edits can't silently drift into the
// generic "AI default" look. It is intentionally small and dependency-free;
// `node scripts/lint-css.mjs` exits non-zero on any violation.
//
// It guards exactly the tells the README claims are kept out:
//   1. no hover-lift        — a :hover rule must not translateY upward
//   2. no AI-default gradients — gradients must use color tokens, never raw
//      hex/rgb literals or the give-away 135° purple-slop angle
//   3. tokenized shadows    — box-shadow colors must come from var()/color-mix,
//      never a raw rgb()/hex baked into the declaration
//
// Raw colors ARE allowed where they belong: inside :root / [data-theme] custom
// property definitions (the token layer). The rules below only inspect real
// property declarations, so those definitions are never flagged.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");
const CSS_DIR = join(ROOT, "frontend", "src");

function cssFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...cssFiles(p));
    else if (name.endsWith(".css")) out.push(p);
  }
  return out;
}

// Map a character index in the source to a 1-based line number.
function lineAt(src, index) {
  let line = 1;
  for (let i = 0; i < index && i < src.length; i++) if (src[i] === "\n") line++;
  return line;
}

const violations = [];
function flag(file, index, src, rule, detail) {
  violations.push({ file, line: lineAt(src, index), rule, detail });
}

const HEX_OR_RGB = /#[0-9a-fA-F]{3,8}\b|rgba?\(/;

for (const file of cssFiles(CSS_DIR)) {
  const src = readFileSync(file, "utf8");

  // Rule 1 — no hover-lift. Find each `<selector>:hover { ... }` block and reject
  // an upward translateY inside it (the canonical "card floats on hover" tell).
  const hoverRe = /([^{}]*:hover[^{}]*)\{([^{}]*)\}/g;
  for (let m; (m = hoverRe.exec(src)); ) {
    if (/translateY\(\s*-/.test(m[2])) {
      flag(file, m.index, src, "hover-lift", `${m[1].trim()} uses an upward translateY on hover`);
    }
  }

  // Rule 2 — no AI-default gradients. Any gradient() value must be built from
  // tokens; raw hex/rgb or the 135deg slop angle is a violation.
  const gradRe = /(linear-gradient|radial-gradient|conic-gradient)\(([^;{}]*)\)/g;
  for (let m; (m = gradRe.exec(src)); ) {
    const inner = m[2];
    if (HEX_OR_RGB.test(inner)) {
      flag(file, m.index, src, "raw-gradient-color", `${m[1]} uses a raw color; use a var() token`);
    }
    if (/\b135deg\b/.test(inner)) {
      flag(file, m.index, src, "ai-default-gradient", `${m[1]} uses the give-away 135deg angle`);
    }
  }

  // Rule 3 — tokenized shadows. `box-shadow` declarations (NOT `--shadow:` token
  // definitions, which this regex ignores) must not bake in a raw color.
  const shadowRe = /(?<![\w-])box-shadow:\s*([^;]+);/g;
  for (let m; (m = shadowRe.exec(src)); ) {
    if (HEX_OR_RGB.test(m[1])) {
      flag(file, m.index, src, "raw-shadow-color", "box-shadow bakes in a raw color; use var()/color-mix");
    }
  }
}

if (violations.length === 0) {
  console.log("anti-slop css lint: clean");
  process.exit(0);
}

console.error(`anti-slop css lint: ${violations.length} violation(s)\n`);
for (const v of violations) {
  const rel = v.file.replace(ROOT + "/", "");
  console.error(`  ${rel}:${v.line}  [${v.rule}] ${v.detail}`);
}
process.exit(1);
