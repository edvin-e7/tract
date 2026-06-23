import { useCallback, useState } from "react";

// Reading progress is stored client-side (localStorage) keyed by item id: the
// reader writes scroll-% as you read, the library reads it back to build a real
// "Continue reading" rail. No fake bars — an item only appears as in-progress
// once you've actually scrolled it.
const KEY = "tract-progress";

export interface Progress { pct: number; at: number }
type Store = Record<string, Progress>;

function read(): Store {
  try { return JSON.parse(localStorage.getItem(KEY) || "{}") as Store; } catch { return {}; }
}

export function useProgress() {
  const [store, setStore] = useState<Store>(read);

  const set = useCallback((id: number, pct: number) => {
    setStore((prev) => {
      const next = { ...prev, [id]: { pct: Math.max(0, Math.min(100, Math.round(pct))), at: Date.now() } };
      localStorage.setItem(KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const get = useCallback((id: number) => store[String(id)]?.pct ?? 0, [store]);

  return { store, get, set };
}

// Standalone writer for the reader, which doesn't need the reactive store.
export function writeProgress(id: number, pct: number) {
  try {
    const s = read();
    s[String(id)] = { pct: Math.max(0, Math.min(100, Math.round(pct))), at: Date.now() };
    localStorage.setItem(KEY, JSON.stringify(s));
  } catch { /* storage unavailable; progress is best-effort */ }
}
