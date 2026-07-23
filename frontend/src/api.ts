import type { Item, Highlight } from "./types";

// ---- server address --------------------------------------------------------
// On the web the SPA is served by the Go binary itself, so "" (same-origin) is
// right and nothing changes. Inside a native shell (Capacitor iOS/Android) the
// bundle is served from the app container, so the API lives on another origin
// entirely — the user's own tract server. That address is user-configured in
// the key popover (persisted next to the token), or baked in at build time via
// VITE_DEFAULT_SERVER for personal builds pre-pointed at e.g.
// http://<mac>.local:8080.

const SERVER_KEY = "tract-server";

const DEFAULT_SERVER = normalizeServer(import.meta.env.VITE_DEFAULT_SERVER ?? "");

function normalizeServer(u: string): string {
  return u.trim().replace(/\/+$/, "");
}

export function getServer(): string {
  try {
    return normalizeServer(localStorage.getItem(SERVER_KEY) ?? "");
  } catch {
    return ""; // localStorage unavailable — behave as same-origin
  }
}

export function setServer(url: string): void {
  try {
    const u = normalizeServer(url);
    if (u) localStorage.setItem(SERVER_KEY, u);
    else localStorage.removeItem(SERVER_KEY);
  } catch {
    /* persistence best-effort */
  }
}

/** Origin every API call targets: stored server, else build-time default, else
 * same-origin (""). The bearer token below rides only on calls built from this
 * one base, so it is only ever sent to the user's own server. */
export function serverBase(): string {
  return getServer() || DEFAULT_SERVER;
}

/** True when running inside the Capacitor native shell, where same-origin
 * points at the bundled files rather than a tract server. iOS serves from
 * capacitor://localhost; Android from https://localhost with the injected
 * bridge global as the tell. */
export function isNativeShell(): boolean {
  if (window.location.protocol === "capacitor:") return true;
  return window.location.hostname === "localhost" && "Capacitor" in window;
}

function apiUrl(path: string): string {
  return `${serverBase()}/api${path}`;
}

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
  // In a native shell with no server configured, every call fails against the
  // bundled files — whatever the raw error says, the actionable fix is one
  // thing: set the server address.
  if (isNativeShell() && !serverBase()) return t("err.noServer");
  if (e instanceof ApiError && e.status === 401) return t("err.unauthorized");
  return e instanceof Error ? e.message : t(fallbackKey);
}

// ---- access token (TRACT_TOKEN) ----------------------------------------
// The server gates every mutating route behind `Authorization: Bearer <token>`
// when TRACT_TOKEN is set (see internal/api/auth.go). The token lives in
// localStorage — single-user tool, same trust level as the saved articles —
// and is attached ONLY to mutating calls against serverBase() (same-origin on
// the web, the user's configured server in the native shell); read-only GETs
// stay bare so they keep working without a token, matching the server's
// contract.

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
  listItems: () => fetch(apiUrl("/items")).then((r) => json<Item[]>(r)),

  getItem: (id: number) => fetch(apiUrl(`/items/${id}`)).then((r) => json<Item>(r)),

  addItem: (url: string) =>
    fetch(apiUrl("/items"), {
      method: "POST",
      headers: authHeaders(true),
      body: JSON.stringify({ url }),
    }).then((r) => json<Item>(r)),

  deleteItem: (id: number) =>
    fetch(apiUrl(`/items/${id}`), { method: "DELETE", headers: authHeaders(false) }).then(
      expectNoContent,
    ),

  search: (q: string) =>
    fetch(apiUrl(`/search?q=${encodeURIComponent(q)}`)).then((r) => json<Item[]>(r)),

  addHighlight: (id: number, text: string) =>
    fetch(apiUrl(`/items/${id}/highlights`), {
      method: "POST",
      headers: authHeaders(true),
      body: JSON.stringify({ text }),
    }).then((r) => json<Highlight>(r)),

  deleteHighlight: (id: number, hid: number) =>
    fetch(apiUrl(`/items/${id}/highlights/${hid}`), {
      method: "DELETE",
      headers: authHeaders(false),
    }).then(expectNoContent),
};
