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
// when TRACT_TOKEN is set, and TRACT_PRIVATE=1 extends that to the read routes
// too (see internal/api/auth.go, internal/api/private_test.go). The token lives
// in localStorage — single-user tool, same trust level as the saved articles —
// and rides on every call built against serverBase() (same-origin on the web,
// the user's configured server in the native shell), so it is only ever sent to
// the user's own server.
//
// IT USED TO RIDE ONLY ON MUTATING CALLS, and that made TRACT_PRIVATE=1
// unusable from this very UI: private mode answers 401 to GET /api/items,
// GET /api/items/{id} and GET /api/search, and listItems/getItem/search sent no
// Authorization header at all. The app shell loaded, then every read failed with
// "this server requires an access token" — including for the owner, who had
// already pasted the token. The server grew a mode its own client could not
// speak, and nothing failed until someone turned the mode on.
//
// Sending it on reads costs nothing on an unprotected server: requireToken is a
// no-op when Token == "", and an open read route ignores the header.

const TOKEN_KEY = "tract-token";

// A build-time default, the exact sibling of VITE_DEFAULT_SERVER above and used the same
// way: for a PERSONAL build of your own app, against your own server, on your own device.
//
// Why it exists. TRACT_PRIVATE=1 gates the read routes, and the token lives in
// localStorage PER INSTALL — so a freshly installed app on a phone shows the shell and
// then 401s on every read until someone types the token in. That is a real first-run
// cliff for a single-user tool whose whole point is that the library is just there.
//
// Why it is not a leak. The value never appears in this repo: it is injected at build
// time by whoever runs the build, exactly like the server address, and a build without
// it behaves as it always did. It does not weaken the server — TRACT_PRIVATE still
// demands the token; this only decides whether the client already knows it. A build made
// for someone else, or a hosted build, simply omits it.
//
// localStorage still WINS when it holds anything, so pasting a different token, or
// pressing Clear, keeps working and is not silently overridden by the build.
const DEFAULT_TOKEN = (import.meta.env.VITE_DEFAULT_TOKEN ?? "").trim();

export function getToken(): string {
  try {
    const stored = localStorage.getItem(TOKEN_KEY);
    if (stored !== null) return stored;
    return DEFAULT_TOKEN;
  } catch {
    return DEFAULT_TOKEN; // localStorage unavailable — the build-time default still applies
  }
}

export function setToken(token: string): void {
  try {
    const t = token.trim();
    // Clear WRITES AN EMPTY STRING rather than removing the key. With a build-time
    // default in play, removeItem would hand the next read straight back to that default
    // — so "Clear" would appear to do nothing and the app would stay unlocked, which is
    // the opposite of what the button says. An empty stored value is an explicit choice
    // and outranks the build.
    localStorage.setItem(TOKEN_KEY, t);
  } catch {
    /* persistence best-effort */
  }
}

/** Headers for any API request: JSON content type when there is a body, plus
 * the bearer token when one is stored — on reads as well as writes, so
 * TRACT_PRIVATE=1 servers are reachable from this client. */
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
  listItems: () =>
    fetch(apiUrl("/items"), { headers: authHeaders(false) }).then((r) => json<Item[]>(r)),

  getItem: (id: number) =>
    fetch(apiUrl(`/items/${id}`), { headers: authHeaders(false) }).then((r) => json<Item>(r)),

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
    fetch(apiUrl(`/search?q=${encodeURIComponent(q)}`), {
      headers: authHeaders(false),
    }).then((r) => json<Item[]>(r)),

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
