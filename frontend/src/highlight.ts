// Renders saved highlights back into the article body as <mark> elements.
//
// The store keeps a highlight as its *text* (not DOM offsets), so on load we
// find that passage in the rendered article and wrap it. Two properties make
// this robust enough to be the product's core "highlighter" verb:
//
//  1. It works on TEXT NODES via a DOM Range, never on the HTML string — so a
//     passage that spans an <a>/<strong> boundary is wrapped correctly and the
//     article's own markup (and the sanitizer's guarantees) are never disturbed.
//  2. Whitespace is normalized before matching, so a selection that crossed a
//     paragraph break (where `Selection.toString()` inserts newlines the DOM
//     text doesn't have) still matches.
//
// Each highlight wraps the first not-yet-marked occurrence, so two identical
// saved passages mark two occurrences rather than fighting over one.

export interface Mark {
  id: number;
  text: string;
}

const collapse = (s: string) => s.replace(/\s+/g, " ");

/**
 * Wrap each highlight's passage in `<mark class="hl" data-hl-id>` inside `root`.
 * Call it against freshly-rendered article HTML (marks are additive, so re-run
 * on a clean render rather than on already-marked DOM to avoid double-wrapping).
 */
export function applyHighlights(root: HTMLElement, marks: Mark[]): void {
  for (const m of marks) wrapFirst(root, m.text, m.id);
}

function wrapFirst(root: HTMLElement, rawNeedle: string, id: number): void {
  const needle = collapse(rawNeedle).trim();
  if (!needle) return;

  // Flatten the text nodes that aren't already inside a highlight.
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) =>
      (n as Text).parentElement?.closest("mark.hl")
        ? NodeFilter.FILTER_REJECT
        : NodeFilter.FILTER_ACCEPT,
  });
  const nodes: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) nodes.push(n as Text);

  // Build a whitespace-normalized haystack and, for every normalized character,
  // remember which text node + offset it came from. That lets us translate a
  // string match back into a precise DOM Range.
  let hay = "";
  const at: { node: Text; offset: number }[] = [];
  let prevSpace = false;
  for (const node of nodes) {
    const v = node.nodeValue ?? "";
    for (let i = 0; i < v.length; i++) {
      const space = /\s/.test(v[i]);
      if (space) {
        if (prevSpace || hay === "") continue; // collapse runs + drop leading ws
        hay += " ";
        at.push({ node, offset: i });
        prevSpace = true;
      } else {
        hay += v[i];
        at.push({ node, offset: i });
        prevSpace = false;
      }
    }
  }

  const idx = hay.indexOf(needle);
  if (idx < 0) return; // passage not present in this render — panel still shows it
  const start = at[idx];
  const end = at[idx + needle.length - 1];
  if (!start || !end) return;

  const range = document.createRange();
  range.setStart(start.node, start.offset);
  range.setEnd(end.node, end.offset + 1);

  const mark = document.createElement("mark");
  mark.className = "hl";
  mark.dataset.hlId = String(id);
  try {
    // surroundContents handles the common single-node case; extract+insert is the
    // fallback when the range crosses element boundaries (surroundContents throws).
    range.surroundContents(mark);
  } catch {
    try {
      mark.appendChild(range.extractContents());
      range.insertNode(mark);
    } catch {
      /* pathological range — leave the body untouched; the panel still lists it */
    }
  }
}
