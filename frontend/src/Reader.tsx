import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { Item, Highlight } from "./types";
import { writeProgress } from "./useProgress";

interface Props {
  id: number;
  onClose: () => void;
  onProgress?: () => void;
}

// Reader — "The Index" editorial spread: centered measure, serif body, chartreuse
// drop-cap (CSS), and a first-class highlights panel (Tract's owned-Readwise leg).
// Tracks scroll-% so the library's "Continue reading" rail is real, not faked.
export function Reader({ id, onClose, onProgress }: Props) {
  const [item, setItem] = useState<Item | null>(null);
  const [highlights, setHighlights] = useState<Highlight[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [copied, setCopied] = useState(false);
  const draftRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    let alive = true;
    api
      .getItem(id)
      .then((it) => {
        if (!alive) return;
        setItem(it);
        setHighlights(it.highlights ?? []);
      })
      .catch((e) => alive && setError(e instanceof Error ? e.message : "load failed"));
    return () => { alive = false; };
  }, [id]);

  // Persist reading progress as scroll-%. Throttled via rAF; flushes on unmount so
  // the library reflects where you stopped.
  useEffect(() => {
    let ticking = false;
    let last = 0;
    function onScroll() {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(() => {
        const max = document.documentElement.scrollHeight - window.innerHeight;
        last = max > 0 ? (window.scrollY / max) * 100 : 0;
        writeProgress(id, last);
        ticking = false;
      });
    }
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      window.removeEventListener("scroll", onScroll);
      if (last > 0) writeProgress(id, last);
      onProgress?.();
    };
  }, [id, onProgress]);

  async function addHighlight(e: React.FormEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text) return;
    try {
      const h = await api.addHighlight(id, text);
      setHighlights((hs) => [...hs, h]);
      setDraft("");
      draftRef.current?.focus();
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not save highlight");
    }
  }

  async function copyLink() {
    if (!item) return;
    try {
      await navigator.clipboard.writeText(item.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch { /* clipboard unavailable; no-op */ }
  }

  const host = item ? hostOf(item) : "";

  return (
    <div className="reader">
      <header className="reader__bar">
        <button className="btn btn--ghost" onClick={onClose}>← Library</button>
        <div className="right">
          {item && <span className="kbd">{highlights.length} highlight{highlights.length === 1 ? "" : "s"}</span>}
          {item && (
            <button className="btn btn--ghost" onClick={copyLink}>
              {copied ? "Copied" : "Copy link"}
            </button>
          )}
        </div>
      </header>

      {error && <p className="feedback" role="alert" style={{ textAlign: "center", marginTop: 40 }}>{error}</p>}
      {!item && !error && <p className="muted" style={{ textAlign: "center", marginTop: 80 }}>Loading…</p>}

      {item && (
        <>
          <article className="article">
            <p className="article__eyebrow">{host}</p>
            <h1>{item.title || item.url}</h1>
            {item.excerpt && <p className="article__deck">{item.excerpt}</p>}
            <hr className="article__rule" />
            {item.html ? (
              <div className="prose" dangerouslySetInnerHTML={{ __html: item.html }} />
            ) : (
              <div className="prose">
                {(item.body || "").split("\n\n").filter(Boolean).map((p, i) => <p key={i}>{p}</p>)}
              </div>
            )}
          </article>

          <section className="highlights" aria-label="Highlights">
            <h3>Highlights</h3>
            {highlights.length === 0 ? (
              <p className="hl-empty">No highlights yet — keep the passages worth remembering.</p>
            ) : (
              highlights.map((h) => (
                <div className="hl-card" key={h.id}>
                  <p>{h.text}</p>
                  <div className="when">{new Date(h.createdAt).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" })}</div>
                </div>
              ))
            )}
            <form className="toolbar" onSubmit={addHighlight} style={{ marginTop: 16 }}>
              <div className="search">
                <input
                  ref={draftRef}
                  type="text"
                  placeholder="Add a passage worth keeping…"
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  aria-label="New highlight"
                />
              </div>
              <button className="btn" type="submit" disabled={!draft.trim()}>Highlight</button>
            </form>
          </section>
        </>
      )}
    </div>
  );
}

function hostOf(it: Item): string {
  if (it.siteName) return it.siteName;
  try { return new URL(it.url).hostname.replace(/^www\./, ""); } catch { return it.url; }
}
