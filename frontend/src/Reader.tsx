import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { api, failureMessage } from "./api";
import type { Item, Highlight } from "./types";
import { writeProgress } from "./useProgress";
import { applyHighlights } from "./highlight";
import { TokenAccess } from "./TokenAccess";
import { useI18n } from "./i18n";

interface Props {
  id: number;
  onClose: () => void;
  onProgress?: () => void;
}

interface PillState {
  text: string;
  x: number;
  y: number;
}

// Reader — "The Index" editorial spread: centered measure, serif body, chartreuse
// drop-cap, a live reading-progress hairline, and highlighting as the product's
// verb — select any passage to mark it, and marks are re-rendered from the store
// on every load. Scroll-% is tracked so the library's "Continue reading" is real.
export function Reader({ id, onClose, onProgress }: Props) {
  const { t, p, locale } = useI18n();
  const [item, setItem] = useState<Item | null>(null);
  const [highlights, setHighlights] = useState<Highlight[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [pill, setPill] = useState<PillState | null>(null);
  const [progress, setProgress] = useState(0);
  const [saving, setSaving] = useState(false);
  const proseRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let alive = true;
    api
      .getItem(id)
      .then((it) => {
        if (!alive) return;
        setItem(it);
        setHighlights(it.highlights ?? []);
      })
      .catch((e) => alive && setError(e instanceof Error ? e.message : t("err.load")));
    return () => {
      alive = false;
    };
  }, [id]);

  // Render the (server-sanitized) article HTML imperatively, then paint the saved
  // highlights over it. Re-running on a clean innerHTML each time avoids ever
  // double-wrapping a mark. item.html is already bluemonday-sanitized server-side,
  // so assigning it to innerHTML carries no new trust — same boundary as before.
  useLayoutEffect(() => {
    const el = proseRef.current;
    if (!el || !item) return;
    if (item.html) {
      el.innerHTML = item.html;
    } else {
      const paras = (item.body || "")
        .split(/\n\n+/)
        .map((t) => t.trim())
        .filter(Boolean)
        .map((t) => {
          const p = document.createElement("p");
          p.textContent = t;
          return p;
        });
      el.replaceChildren(...paras);
    }
    applyHighlights(el, highlights);
  }, [item, highlights]);

  // Reading progress: scroll-% drives both the hairline and the library's
  // "Continue reading" rail (persisted). Throttled via rAF; flushed on unmount.
  useEffect(() => {
    let ticking = false;
    let last = 0;
    function onScroll() {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(() => {
        const max = document.documentElement.scrollHeight - window.innerHeight;
        last = max > 0 ? (window.scrollY / max) * 100 : 0;
        setProgress(last);
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

  // Show the "Highlight" pill whenever there's a non-empty selection inside the
  // article. Works for both mouse drag (mouseup) and keyboard selection (keyup).
  const syncPill = useCallback(() => {
    const sel = window.getSelection();
    const prose = proseRef.current;
    if (!sel || sel.isCollapsed || !prose) return setPill(null);
    const text = sel.toString().trim();
    if (text.length < 2) return setPill(null);
    const anchor = sel.anchorNode;
    const focus = sel.focusNode;
    const inside = (n: Node | null) => !!n && prose.contains(n);
    if (!inside(anchor) || !inside(focus)) return setPill(null);
    const rect = sel.getRangeAt(0).getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) return setPill(null);
    setPill({ text, x: rect.left + rect.width / 2, y: rect.top });
  }, []);

  useEffect(() => {
    function onUp() {
      // let the selection settle before we read it
      window.setTimeout(syncPill, 0);
    }
    function onScrollHide() {
      setPill(null);
    }
    document.addEventListener("mouseup", onUp);
    document.addEventListener("keyup", onUp);
    window.addEventListener("scroll", onScrollHide, { passive: true });
    return () => {
      document.removeEventListener("mouseup", onUp);
      document.removeEventListener("keyup", onUp);
      window.removeEventListener("scroll", onScrollHide);
    };
  }, [syncPill]);

  async function saveHighlight() {
    if (!pill || saving) return;
    setSaving(true);
    try {
      const h = await api.addHighlight(id, pill.text);
      setHighlights((hs) => [...hs, h]);
      setPill(null);
      window.getSelection()?.removeAllRanges();
    } catch (err) {
      setError(failureMessage(err, t, "err.saveHl"));
    } finally {
      setSaving(false);
    }
  }

  async function removeHighlight(hid: number) {
    try {
      await api.deleteHighlight(id, hid);
      setHighlights((hs) => hs.filter((h) => h.id !== hid));
    } catch (err) {
      setError(failureMessage(err, t, "err.removeHl"));
    }
  }

  function scrollToMark(hid: number) {
    const el = proseRef.current?.querySelector<HTMLElement>(`mark.hl[data-hl-id="${hid}"]`);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    el.classList.add("is-flash");
    window.setTimeout(() => el.classList.remove("is-flash"), 1100);
  }

  async function copyLink() {
    if (!item) return;
    try {
      await navigator.clipboard.writeText(item.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      /* clipboard unavailable; no-op */
    }
  }

  const host = item ? hostOf(item) : "";
  const count = highlights.length;

  return (
    <div className="reader">
      <div className="reader__progress" aria-hidden>
        <i style={{ width: `${Math.max(0, Math.min(100, progress))}%` }} />
      </div>
      <header className="reader__bar">
        <button className="btn btn--ghost" onClick={onClose}>← {t("reader.back")}</button>
        <div className="right">
          {item && (
            <span className="kbd">{p(count, "count.highlight")}</span>
          )}
          {item && (
            <button className="btn btn--ghost" onClick={copyLink}>
              {copied ? t("reader.copied") : t("reader.copyLink")}
            </button>
          )}
          <TokenAccess />
        </div>
      </header>

      {error && (
        <p className="feedback" role="alert" style={{ textAlign: "center", marginTop: 40 }}>{error}</p>
      )}
      {!item && !error && (
        <p className="muted" style={{ textAlign: "center", marginTop: 80 }}>{t("common.loading")}</p>
      )}

      {item && (
        <>
          <article className="article">
            <p className="article__eyebrow">{host}</p>
            <h1>{item.title || item.url}</h1>
            {item.excerpt && <p className="article__deck">{item.excerpt}</p>}
            <hr className="article__rule" />
            <div className="prose" ref={proseRef} />
          </article>

          <section className="highlights" aria-label={t("reader.highlights")}>
            <h3>{t("reader.highlights")} <span className="highlights__n">{count}</span></h3>
            {count === 0 ? (
              <p className="hl-empty">{t("reader.hlEmpty")}</p>
            ) : (
              <ul className="hl-list">
                {highlights.map((h) => (
                  <li className="hl-card" key={h.id}>
                    <button className="hl-card__body" onClick={() => scrollToMark(h.id)} title={t("reader.jump")}>
                      <p>{h.text}</p>
                      <span className="when">
                        {new Date(h.createdAt).toLocaleDateString(locale, {
                          day: "numeric",
                          month: "short",
                          year: "numeric",
                        })}
                      </span>
                    </button>
                    <button
                      className="hl-card__del"
                      aria-label={t("reader.remove")}
                      title={t("reader.remove")}
                      onClick={() => removeHighlight(h.id)}
                    >
                      ✕
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}

      {pill && (
        <button
          className="hl-pill"
          style={{ left: pill.x, top: pill.y }}
          // preventDefault on mousedown so the click doesn't collapse the selection first
          onMouseDown={(e) => e.preventDefault()}
          onClick={saveHighlight}
          disabled={saving}
        >
          <svg viewBox="0 0 24 24" aria-hidden>
            <path d="M4 20h16M6 16l9-9 3 3-9 9H6z" />
          </svg>
          {saving ? t("add.saving") : t("reader.highlight")}
        </button>
      )}
    </div>
  );
}

function hostOf(it: Item): string {
  if (it.siteName) return it.siteName;
  try {
    return new URL(it.url).hostname.replace(/^www\./, "");
  } catch {
    return it.url;
  }
}
