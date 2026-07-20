import type { Item, Highlight } from "./types";

// Relative base: served from the same origin as the Go binary in prod, proxied
// to :8080 in dev. Every request below is same-origin by construction — the
// access token must never ride along to any other host.
const BASE = "/api";

/** Error carrying the HTTP status so the UI can react to 401 specifically. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Human message for a failed call: a 401 becomes the actionable "set your
 * access token" hint; anything else keeps the server/network message. The
 * translator is passed in so this layer stays i18n-free. */
export function failureMessage(
  e: unknown,
  t: (key: string) => string,
  fallbackKey: string,
): string {
  if (e instanceof ApiError && e.status === 401) return t("err.unauthorized");
  return e instanceof Error ? e.message : t(fallbackKey);
}

// ---- access token (TRACT_TOKEN) ----------------------------------------
// The server gates every mutating route behind `Authorization: Bearer <token>`
// when TRACT_TOKEN is set (see internal/api/auth.go). The token lives in
// localStorage — single-user tool, same trust level as the saved articles —
// and is attached ONLY to mutating same-origin calls; read-only GETs stay
// bare so they keep working without a token, matching the server's contract.

const TOKEN_KEY = "tract-token";

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return ""; // localStorage unavailable — behave as tokenless
  }
}

export function setToken(token: string): void {
  try {
    const t = token.trim();
    if (t) localStorage.setItem(TOKEN_KEY, t);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* persistence best-effort */
  }
}

/** Headers for mutating requests: JSON content type when there is a body,
 * plus the bearer token when one is stored. */
function authHeaders(json: boolean): HeadersInit {
  const h: Record<string, string> = {};
  if (json) h["Content-Type"] = "application/json";
  const token = getToken();
  if (token) h.Authorization = `Bearer ${token}`;
  return h;
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // non-JSON error body; keep the status message
    }
    throw new ApiError(msg, res.status);
  }
  return res.json() as Promise<T>;
}

/** For DELETEs (204 on success): surface the server's error body + status. */
async function expectNoContent(res: Response): Promise<void> {
  if (res.ok || res.status === 204) return;
  let msg = `delete failed (${res.status})`;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) msg = body.error;
  } catch {
    // non-JSON error body; keep the status message
  }
  throw new ApiError(msg, res.status);
}

export const api = {
  listItems: () => fetch(`${BASE}/items`).then((r) => json<Item[]>(r)),

  getItem: (id: number) => fetch(`${BASE}/items/${id}`).then((r) => json<Item>(r)),

  addItem: (url: string) =>
    fetch(`${BASE}/items`, {
      method: "POST",
      headers: authHeaders(true),
      body: JSON.stringify({ url }),
    }).then((r) => json<Item>(r)),

  deleteItem: (id: number) =>
    fetch(`${BASE}/items/${id}`, { method: "DELETE", headers: authHeaders(false) }).then(
      expectNoContent,
    ),

  search: (q: string) =>
    fetch(`${BASE}/search?q=${encodeURIComponent(q)}`).then((r) => json<Item[]>(r)),

  addHighlight: (id: number, text: string) =>
    fetch(`${BASE}/items/${id}/highlights`, {
      method: "POST",
      headers: authHeaders(true),
      body: JSON.stringify({ text }),
    }).then((r) => json<Highlight>(r)),

  deleteHighlight: (id: number, hid: number) =>
    fetch(`${BASE}/items/${id}/highlights/${hid}`, {
      method: "DELETE",
      headers: authHeaders(false),
    }).then(expectNoContent),
};
