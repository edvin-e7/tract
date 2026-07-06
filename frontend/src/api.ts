import type { Item, Highlight } from "./types";

// Relative base: served from the same origin as the Go binary in prod, proxied
// to :8080 in dev.
const BASE = "/api";

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // non-JSON error body; keep the status message
    }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

export const api = {
  listItems: () => fetch(`${BASE}/items`).then((r) => json<Item[]>(r)),

  getItem: (id: number) => fetch(`${BASE}/items/${id}`).then((r) => json<Item>(r)),

  addItem: (url: string) =>
    fetch(`${BASE}/items`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    }).then((r) => json<Item>(r)),

  deleteItem: async (id: number) => {
    const res = await fetch(`${BASE}/items/${id}`, { method: "DELETE" });
    if (!res.ok && res.status !== 204) throw new Error(`delete failed (${res.status})`);
  },

  search: (q: string) =>
    fetch(`${BASE}/search?q=${encodeURIComponent(q)}`).then((r) => json<Item[]>(r)),

  addHighlight: (id: number, text: string) =>
    fetch(`${BASE}/items/${id}/highlights`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    }).then((r) => json<Highlight>(r)),

  deleteHighlight: async (id: number, hid: number) => {
    const res = await fetch(`${BASE}/items/${id}/highlights/${hid}`, { method: "DELETE" });
    if (!res.ok && res.status !== 204) throw new Error(`delete failed (${res.status})`);
  },
};
