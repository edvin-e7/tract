import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import type { Item } from "./types";
import { Reader } from "./Reader";
import { useTheme } from "./useTheme";
import { useProgress } from "./useProgress";
import { LANGS, useI18n, type Plural, type Translator } from "./i18n";

type Filter = "all" | "highlighted";

// Library = "The Queue" archetype (spatial cards), deliberately distinct from the
// master-detail ledger used elsewhere: resume-first (Continue reading) over a card
// grid. The reader is the editorial spread.
export default function App() {
  const [items, setItems] = useState<Item[]>([]);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [openId, setOpenId] = useState<number | null>(null);
  const [adding, setAdding] = useState(false);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const addRef = useRef<HTMLInputElement>(null);
  const [theme, toggleTheme] = useTheme();
  const progress = useProgress();
  const { t, p, locale, lang, setLang } = useI18n();

  const refresh = useCallback(async (q: string) => {
    try {
      const data = q.trim() ? await api.search(q) : await api.listItems();
      setItems(data);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("err.load"));
    }
  }, [t]);

  useEffect(() => {
    const t = setTimeout(() => void refresh(query), query ? 220 : 0);
    return () => clearTimeout(t);
  }, [query, refresh]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchRef.current?.focus();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => { if (adding) addRef.current?.focus(); }, [adding]);

  async function onAdd(e: React.FormEvent) {
    e.preventDefault();
    const value = url.trim();
    if (!value || busy) return;
    setBusy(true);
    setError(null);
    try {
      await api.addItem(value);
      setUrl("");
      setAdding(false);
      setQuery("");
      await refresh("");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("err.saveLink"));
    } finally {
      setBusy(false);
    }
  }

  async function onDelete(id: number) {
    try {
      await api.deleteItem(id);
      if (openId === id) setOpenId(null);
      await refresh(query);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("err.delete"));
    }
  }

  const shown = useMemo(
    () => (filter === "highlighted" ? items.filter((i) => i.highlightCount > 0) : items),
    [items, filter],
  );

  // "Continue reading": items with real, unfinished progress, most-recent first.
  const continuing = useMemo(() => {
    if (query.trim()) return [];
    return items
      .map((it) => ({ it, p: progress.store[String(it.id)] }))
      .filter((x) => x.p && x.p.pct >= 3 && x.p.pct < 95)
      .sort((a, b) => b.p!.at - a.p!.at)
      .slice(0, 3)
      .map((x) => x.it);
  }, [items, progress.store, query]);

  const continuingIds = useMemo(() => new Set(continuing.map((i) => i.id)), [continuing]);
  const grid = useMemo(() => shown.filter((i) => !continuingIds.has(i.id)), [shown, continuingIds]);
  const totalMarks = useMemo(() => items.reduce((n, i) => n + i.highlightCount, 0), [items]);

  if (openId !== null) {
    return <Reader id={openId} onClose={() => setOpenId(null)} onProgress={() => refresh(query)} />;
  }

  return (
    <div className="q">
      <header className="qbar">
        <div className="qbrand"><span className="qlogo" aria-hidden>t</span> Tract</div>
        <label className="qsearch">
          {icon.search}
          <input
            ref={searchRef}
            type="search"
            placeholder={t("search.placeholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label={t("search.aria")}
          />
          <span className="kbd">⌘K</span>
        </label>
        <div className="qspacer" />
        <div className="chips">
          {(["all", "highlighted"] as Filter[]).map((f) => (
            <button key={f} className={`chip${filter === f ? " is-active" : ""}`} onClick={() => setFilter(f)}>
              {f === "all" ? t("filter.all") : t("filter.highlighted")}
            </button>
          ))}
        </div>
        <label className="qlang" title={t("lang.aria")}>
          <span className="sr-only">{t("lang.aria")}</span>
          <select
            value={lang}
            onChange={(e) => setLang(e.target.value as typeof lang)}
            aria-label={t("lang.aria")}
          >
            {LANGS.map((l) => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>
        </label>
        <button className="qicon" onClick={toggleTheme} title={t("theme.toggle")} aria-label={t("theme.toggle")}>
          {theme === "ink" ? icon.sun : icon.moon}
        </button>
        <button className="btn btn--accent" onClick={() => setAdding((v) => !v)}>+ {t("save.link")}</button>
      </header>

      <main className="qmain">
        {adding && (
          <form className="qadd" onSubmit={onAdd}>
            {icon.link}
            <input
              ref={addRef}
              type="url"
              placeholder={t("add.placeholder")}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && setAdding(false)}
              aria-label={t("add.aria")}
            />
            <button className="btn btn--accent" type="submit" disabled={busy}>{busy ? t("add.saving") : t("add.save")}</button>
          </form>
        )}

        {error && <p className="feedback" role="alert">{error}</p>}

        {continuing.length > 0 && (
          <section className="qsec">
            <h2 className="qsec__h">{t("sec.continue")} <span>— {t("sec.continueSub")}</span></h2>
            <div className="qrail">
              {continuing.map((it) => (
                <button className="qcont" key={it.id} onClick={() => setOpenId(it.id)}>
                  <span className={`qmono ${tint(it)}`}>{monogram(it)}</span>
                  <span className="qcont__t">{it.title || it.url}</span>
                  <span className="qcont__meta">{hostOf(it)} · {t("progress.in", { n: progress.get(it.id) })}</span>
                  <span className="qpr"><i style={{ width: `${progress.get(it.id)}%` }} /></span>
                </button>
              ))}
            </div>
          </section>
        )}

        <section className="qsec">
          <h2 className="qsec__h">
            {query.trim() ? t("sec.results") : filter === "highlighted" ? t("filter.highlighted") : t("sec.upnext")}
            <span>— {p(grid.length, "count.article")}</span>
          </h2>
          {grid.length === 0 ? (
            <p className="empty">
              {query.trim()
                ? t("empty.search")
                : filter === "highlighted"
                  ? t("empty.highlighted")
                  : t("empty.library")}
            </p>
          ) : (
            <div className="qgrid">
              {grid.map((it) => (
                <button className="qcard" key={it.id} onClick={() => setOpenId(it.id)}>
                  <span className={`qmono ${tint(it)}`}>{monogram(it)}</span>
                  <span className="qcard__t">{it.title || it.url}</span>
                  {it.excerpt && <span className="qcard__x">{it.excerpt}</span>}
                  <span className="qcard__foot">
                    <span>{hostOf(it)} · {relativeTime(it.createdAt, t, p, locale)}</span>
                    {it.highlightCount > 0 && <span className="qpill">{it.highlightCount} ✦</span>}
                    <span
                      className="qdel"
                      role="button"
                      tabIndex={0}
                      aria-label={t("delete.aria", { title: it.title || it.url })}
                      onClick={(e) => { e.stopPropagation(); void onDelete(it.id); }}
                      onKeyDown={(e) => { if (e.key === "Enter") { e.stopPropagation(); void onDelete(it.id); } }}
                    >✕</span>
                  </span>
                </button>
              ))}
            </div>
          )}
        </section>
      </main>

      <footer className="statusbar">
        <div className="hints"><span><b>⌘K</b> {t("foot.search")}</span><span><b>↵</b> {t("foot.open")}</span></div>
        <div>
          <b>{items.length}</b> {p(items.length, "noun.article")} · <b>{totalMarks}</b> {p(totalMarks, "noun.highlight")}
        </div>
      </footer>
    </div>
  );
}

/* ---- helpers ---- */

function monogram(it: Item): string {
  const s = (it.siteName || hostOf(it) || it.title || "?").trim();
  return (s[0] || "?").toUpperCase();
}

// Deterministic accent tint per item so the grid has rhythm without randomness.
function tint(it: Item): string {
  return ["t-a", "t-b", "t-c", "t-d"][it.id % 4];
}

function relativeTime(iso: string, t: Translator, p: Plural, locale: string): string {
  const then = new Date(iso).getTime();
  const mins = Math.round((Date.now() - then) / 60000);
  if (mins < 1) return t("time.justnow");
  if (mins < 60) return t("time.min", { n: mins });
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return p(hrs, "time.hr");
  const days = Math.round(hrs / 24);
  if (days < 7) return p(days, "time.day");
  // Older than a week: fall back to a date, formatted in the active language's
  // locale (English default is en-US, so this matches the prior behavior).
  return new Date(iso).toLocaleDateString(locale, { day: "numeric", month: "short" });
}

function hostOf(it: Item): string {
  if (it.siteName) return it.siteName;
  try { return new URL(it.url).hostname.replace(/^www\./, ""); } catch { return it.url; }
}

const icon = {
  search: <svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.2-3.2" /></svg>,
  link: <svg viewBox="0 0 24 24"><path d="M10 14a3.5 3.5 0 0 0 5 0l3-3a3.5 3.5 0 0 0-5-5l-1.5 1.5M14 10a3.5 3.5 0 0 0-5 0l-3 3a3.5 3.5 0 0 0 5 5l1.5-1.5" /></svg>,
  sun: <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="4.2" /><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4" /></svg>,
  moon: <svg viewBox="0 0 24 24"><path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z" /></svg>,
};
