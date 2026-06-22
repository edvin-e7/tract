import { useCallback, useEffect, useState } from "react";
import { api } from "./api";
import type { Item } from "./types";
import { Reader } from "./Reader";

// Minimal functional shell: add-URL, list, search, reader view. Deliberately
// plain — final visual design comes through a later design pass, not here.
export default function App() {
  const [items, setItems] = useState<Item[]>([]);
  const [query, setQuery] = useState("");
  const [url, setUrl] = useState("");
  const [openId, setOpenId] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (q: string) => {
    try {
      const data = q.trim() ? await api.search(q) : await api.listItems();
      setItems(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  // Debounced search / initial load.
  useEffect(() => {
    const t = setTimeout(() => void refresh(query), query ? 250 : 0);
    return () => clearTimeout(t);
  }, [query, refresh]);

  async function onAdd(e: React.FormEvent) {
    e.preventDefault();
    const value = url.trim();
    if (!value || busy) return;
    setBusy(true);
    setError(null);
    try {
      await api.addItem(value);
      setUrl("");
      setQuery("");
      await refresh("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not add");
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
      setError(err instanceof Error ? err.message : "could not delete");
    }
  }

  if (openId !== null) {
    return <Reader id={openId} onClose={() => setOpenId(null)} />;
  }

  return (
    <div className="app">
      <header className="masthead">
        <h1>Tract</h1>
        <p className="tagline">Save anything. Read it clean. Search it later.</p>
      </header>

      <form className="add" onSubmit={onAdd}>
        <input
          type="url"
          placeholder="Paste a URL to save…"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          aria-label="URL to save"
        />
        <button type="submit" disabled={busy}>
          {busy ? "Saving…" : "Save"}
        </button>
      </form>

      <input
        className="search"
        type="search"
        placeholder="Search your library…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        aria-label="Search library"
      />

      {error && <p className="error" role="alert">{error}</p>}

      <ul className="list">
        {items.length === 0 && (
          <li className="empty">
            {query.trim() ? "No matches." : "Nothing saved yet."}
          </li>
        )}
        {items.map((it) => (
          <li key={it.id} className="row">
            <button className="row-main" onClick={() => setOpenId(it.id)}>
              <span className="row-title">{it.title || it.url}</span>
              <span className="row-meta">
                {it.siteName || new URL(it.url).hostname}
              </span>
              {it.excerpt && <span className="row-excerpt">{it.excerpt}</span>}
            </button>
            <button
              className="row-del"
              aria-label={`Delete ${it.title || it.url}`}
              onClick={() => onDelete(it.id)}
            >
              ✕
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
